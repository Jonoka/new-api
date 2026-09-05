package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
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
	info.EnsureTaskSubmissionIdentity()
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
		if err != nil {
			return err
		}
		return model.TransferTaskSubmissionTx(tx, info.TaskSubmissionID, info.TaskSubmissionLeaseToken, task.ID, chargedQuota)
	}
	if info.Billing == nil {
		if chargedQuota != 0 {
			return errors.New("paid async task has no billing reservation")
		}
		reservation := model.GroupReservationRequest{
			Source:                 model.GroupReservationWallet,
			UserId:                 info.UserId,
			ModelName:              info.OriginModelName,
			TokenId:                task.PrivateData.TokenId,
			SkipTokenQuota:         task.PrivateData.TokenId == 0,
			ExpectedReserved:       0,
			TargetReserved:         0,
			SubmissionID:           info.TaskSubmissionID,
			SubmissionLeaseToken:   info.TaskSubmissionLeaseToken,
			SubmissionOperationID:  common.GetUUID(),
			SubmissionTaskRowID:    info.TaskSubmissionTaskRowID,
			SubmissionLeaseSeconds: int64(model.TaskSubmissionLeaseDuration / time.Second),
		}
		if err := model.DB.Transaction(func(tx *gorm.DB) error {
			if err := model.EnsureZeroTaskSubmissionTx(tx, reservation); err != nil {
				return err
			}
			return persist(tx)
		}); err != nil {
			if resolveErr := resolveTaskSubmissionHandoffError(c, info, task, chargedQuota, err); resolveErr != nil {
				return resolveErr
			}
			*originalTask = *task
			return nil
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
		Source:                 session.funding.Source(),
		RequestId:              info.RequestId,
		UserId:                 info.UserId,
		ModelName:              info.OriginModelName,
		SubscriptionId:         info.SubscriptionId,
		TokenId:                info.TokenId,
		TokenKey:               info.TokenKey,
		TokenUnlimited:         info.TokenUnlimited,
		SkipTokenQuota:         info.IsPlayground || info.SkipTokenQuota,
		ExpectedReserved:       session.preConsumedQuota,
		TargetReserved:         chargedQuota,
		PostConsume:            true,
		SubmissionID:           info.TaskSubmissionID,
		SubmissionLeaseToken:   info.TaskSubmissionLeaseToken,
		SubmissionTaskRowID:    info.TaskSubmissionTaskRowID,
		SubmissionLeaseSeconds: int64(model.TaskSubmissionLeaseDuration / time.Second),
	}
	_, err := model.WithReconciledGroupReservation(reservation, func(tx *gorm.DB, result *model.GroupReservationResult) error {
		task.PrivateData.SubscriptionId = result.SubscriptionId
		return persist(tx)
	})
	if err != nil {
		if resolveErr := resolveTaskSubmissionHandoffError(c, info, task, chargedQuota, err); resolveErr != nil {
			return resolveErr
		}
		closeTransferredBillingSession(session, info, chargedQuota)
		*originalTask = *task
		return nil
	}
	closeTransferredBillingSession(session, info, chargedQuota)
	*originalTask = *task
	return nil
}

func closeTransferredBillingSession(session *BillingSession, info *relaycommon.RelayInfo, chargedQuota int) {
	session.preConsumedQuota = chargedQuota
	switch funding := session.funding.(type) {
	case *WalletFunding:
		funding.consumed = chargedQuota
	case *SubscriptionFunding:
		funding.preConsumed = int64(chargedQuota)
		info.SubscriptionPreConsumed = int64(chargedQuota)
	}
	if info.IsPlayground || info.SkipTokenQuota {
		session.tokenConsumed = 0
	} else {
		session.tokenConsumed = chargedQuota
	}
	session.settled = true
	session.fundingSettled = true
	session.stopTaskSubmissionHeartbeatLocked()
	info.FinalPreConsumedQuota = chargedQuota
	info.PriceData.Quota = chargedQuota
}

func resolveTaskSubmissionHandoffError(c *gin.Context, info *relaycommon.RelayInfo, task *model.Task, chargedQuota int, observed error) error {
	fundingSource := task.PrivateData.BillingSource
	if fundingSource == "" {
		fundingSource = model.TaskAccountingFundingWallet
	}
	tokenID := task.PrivateData.TokenId
	if info.IsPlayground || info.SkipTokenQuota {
		tokenID = 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resolution, err := model.ResolveTaskSubmissionHandoff(ctx, model.TaskSubmissionHandoffExpectation{
		SubmissionID:   info.TaskSubmissionID,
		LeaseToken:     info.TaskSubmissionLeaseToken,
		Task:           task,
		FundingSource:  fundingSource,
		ModelName:      info.OriginModelName,
		UserID:         info.UserId,
		SubscriptionID: task.PrivateData.SubscriptionId,
		TokenID:        tokenID,
		ChargedQuota:   chargedQuota,
	})
	if err != nil {
		return fmt.Errorf("task billing handoff failed (%v); durable owner unresolved: %w", observed, err)
	}
	*task = resolution.Task
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
