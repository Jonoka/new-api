package service

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HandoffTaskBilling transfers a live reservation only after the durable task,
// original counters and consume-log event commit together.
func HandoffTaskBilling(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task, expectedStatus model.TaskStatus, chargedQuota int) error {
	if info == nil || task == nil || chargedQuota < 0 {
		return errors.New("invalid async billing handoff")
	}
	if task.UserId != info.UserId {
		return errors.New("async billing handoff user mismatch")
	}
	originalTask := task
	copyTask := *task
	task = &copyTask
	task.Group = info.UsingGroup
	task.ChannelId = info.ChannelId
	task.PrivateData.BillingSource = info.BillingSource
	task.PrivateData.SubscriptionId = info.SubscriptionId
	task.PrivateData.TokenId = info.TokenId
	if info.IsPlayground || info.SkipTokenQuota {
		task.PrivateData.TokenId = 0
	}
	if task.PrivateData.BillingSource == "" {
		task.PrivateData.BillingSource = BillingSourceWallet
	}

	request := model.AsyncTaskHandoffRequest{
		Task:           task,
		ExpectedStatus: expectedStatus,
		ChargedQuota:   chargedQuota,
		InitialLog:     initialTaskAccountingLog(c, info, task, chargedQuota),
	}
	persist := func(tx *gorm.DB) error {
		_, err := model.PersistAsyncTaskHandoffTx(tx, request)
		return err
	}
	if info.Billing == nil {
		if chargedQuota != 0 {
			return errors.New("paid async task has no billing reservation")
		}
		if err := model.DB.Transaction(persist); err != nil {
			return err
		}
		*originalTask = *task
		return nil
	}
	session, ok := info.Billing.(*BillingSession)
	if !ok {
		return fmt.Errorf("unsupported async billing session %T", info.Billing)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.settled || session.refunded || session.fundingSettled {
		return errors.New("async billing reservation is already closed")
	}
	reservation := model.GroupReservationRequest{
		Source:           session.funding.Source(),
		RequestId:        info.RequestId,
		UserId:           info.UserId,
		ModelName:        info.OriginModelName,
		SubscriptionId:   info.SubscriptionId,
		TokenId:          info.TokenId,
		TokenKey:         info.TokenKey,
		TokenUnlimited:   info.TokenUnlimited,
		SkipTokenQuota:   info.IsPlayground || info.SkipTokenQuota,
		ExpectedReserved: session.preConsumedQuota,
		TargetReserved:   chargedQuota,
		PostConsume:      true,
	}
	_, err := model.WithReconciledGroupReservation(reservation, func(tx *gorm.DB, result *model.GroupReservationResult) error {
		task.PrivateData.SubscriptionId = result.SubscriptionId
		return persist(tx)
	})
	if err != nil {
		return err
	}
	session.settled = true
	session.fundingSettled = true
	info.FinalPreConsumedQuota = chargedQuota
	info.PriceData.Quota = chargedQuota
	*originalTask = *task
	return nil
}

func initialTaskAccountingLog(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task, quota int) model.TaskAccountingLogFacts {
	facts := BuildTaskAccountingLogFacts(c, info, quota)
	facts.TokenID = task.PrivateData.TokenId
	if facts.RequestID == "" {
		facts.RequestID = info.RequestId
	}
	return facts
}
