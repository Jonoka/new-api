package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareTaskHandoff(t *testing.T) (*model.User, *model.Token, *relaycommon.RelayInfo, *model.Task) {
	t.Helper()
	user, token := seedGroupBillingWallet(t, 1000, 1000)
	channel := &model.Channel{Name: fmt.Sprintf("task-handoff-%d", time.Now().UnixNano())}
	require.NoError(t, model.DB.Create(channel).Error)
	info := groupBillingInfo(user, token)
	info.ChannelMeta = &relaycommon.ChannelMeta{ChannelId: channel.Id}
	info.ForcePreConsume = true
	info.UsingGroup = "paid"
	info.PriceData = types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}, Quota: 300}
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	task := model.InitTask(constant.TaskPlatformImage, info)
	task.Status = model.TaskStatusQueued
	task.PrivateData.BillingContext = &model.TaskBillingContext{OriginModelName: info.OriginModelName, GroupRatio: 1}
	return user, token, info, task
}

func TestAsyncTaskHandoffDurableChargeAndTerminalRefundWithBatchEnabled(t *testing.T) {
	oldBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })
	user, token, info, task := prepareTaskHandoff(t)
	c := groupBillingContext(t)
	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	require.NoError(t, HandoffTaskBilling(c, info, task, "", 200))
	require.Positive(t, task.ID)
	quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 800, quota)
	require.Equal(t, 800, remain)
	require.Equal(t, 200, used)
	var chargedUser model.User
	require.NoError(t, model.DB.First(&chargedUser, user.Id).Error)
	require.Equal(t, 200, chargedUser.UsedQuota)
	require.Equal(t, 1, chargedUser.RequestCount)
	require.False(t, info.Billing.(*BillingSession).NeedsRefund())

	decision := model.TaskTerminalDecision{TaskRowID: task.ID, ExpectedStatus: model.TaskStatusQueued, Status: model.TaskStatusFailure, FinalQuota: 0, Reason: "test timeout"}
	_, err := model.AcceptTaskTerminalDecision(context.Background(), decision)
	require.NoError(t, err)
	_, err = model.ApplyTaskAccountingDecision(context.Background(), task.ID)
	require.NoError(t, err)
	_, err = model.AcceptTaskTerminalDecision(context.Background(), decision)
	require.NoError(t, err)
	_, err = model.ApplyTaskAccountingDecision(context.Background(), task.ID)
	require.NoError(t, err)
	quota, remain, used = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 1000, quota)
	require.Equal(t, 1000, remain)
	require.Zero(t, used)
	require.NoError(t, model.DB.First(&chargedUser, user.Id).Error)
	require.Zero(t, chargedUser.UsedQuota)
	require.Equal(t, 1, chargedUser.RequestCount)
	var settledTask model.Task
	require.NoError(t, model.DB.First(&settledTask, task.ID).Error)
	require.EqualValues(t, model.TaskStatusFailure, settledTask.Status)
	require.Zero(t, settledTask.Quota)
}

func TestAsyncTaskHandoffFailureKeepsReservationAndTaskIdentity(t *testing.T) {
	user, token, info, task := prepareTaskHandoff(t)
	c := groupBillingContext(t)
	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	// A missing channel fails after the task insert, forcing the entire handoff
	// including its reservation resize and generated identity to roll back.
	info.ChannelId += 1000000
	require.Error(t, HandoffTaskBilling(c, info, task, "", 200))
	require.Zero(t, task.ID)
	require.Equal(t, 300, info.Billing.GetPreConsumedQuota())
	require.True(t, info.Billing.(*BillingSession).NeedsRefund())
	quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 700, quota)
	require.Equal(t, 700, remain)
	require.Equal(t, 300, used)
	var count int64
	require.NoError(t, model.DB.Model(&model.Task{}).Where("task_id = ?", task.TaskID).Count(&count).Error)
	require.Zero(t, count)
}

func TestBillingSessionFinalAdjustmentsRollbackTogether(t *testing.T) {
	for _, operation := range []string{"settle", "refund"} {
		t.Run(operation, func(t *testing.T) {
			oldBatch := common.BatchUpdateEnabled
			common.BatchUpdateEnabled = true
			t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })
			user, token, info, _ := prepareTaskHandoff(t)
			c := groupBillingContext(t)
			require.Nil(t, ReconcileBillingReservation(c, 300, info))
			session := info.Billing.(*BillingSession)
			callbackName := "test:task-final-token-failure"
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == "tokens" {
					tx.AddError(errors.New("injected token write failure"))
				}
			}))
			t.Cleanup(func() { _ = model.DB.Callback().Update().Remove(callbackName) })
			if operation == "settle" {
				require.Error(t, session.Settle(500))
			} else {
				session.Refund(c)
			}
			require.False(t, session.settled)
			require.False(t, session.refunded)
			quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
			require.Equal(t, 700, quota)
			require.Equal(t, 700, remain)
			require.Equal(t, 300, used)
			require.NoError(t, model.DB.Callback().Update().Remove(callbackName))
			wantRemaining, wantUsed := 1000, 0
			if operation == "settle" {
				require.NoError(t, session.Settle(500))
				require.NoError(t, session.Settle(500))
				wantRemaining, wantUsed = 500, 500
			} else {
				session.Refund(c)
				session.Refund(c)
			}
			quota, remain, used = readGroupBillingWallet(t, user.Id, token.Id)
			require.Equal(t, wantRemaining, quota)
			require.Equal(t, wantRemaining, remain)
			require.Equal(t, wantUsed, used)
		})
	}
}
