package service

import (
	"context"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resetTaskSubmissionDurabilityFixture(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.TaskSubmission{}))
	require.NoError(t, model.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.TaskSubmission{}).Error)
	resetTaskAccountingFixture(t, model.DB, model.LOG_DB)
}

func TestTaskSubmissionPostAcceptIncreaseSurvivesTokenDeletion(t *testing.T) {
	for _, physical := range []bool{false, true} {
		t.Run(map[bool]string{false: "soft_deleted", true: "removed"}[physical], func(t *testing.T) {
			useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
			resetTaskSubmissionDurabilityFixture(t)
			user, token, info, task := prepareTaskHandoff(t)
			c := groupBillingContext(t)
			require.Nil(t, ReconcileBillingReservation(c, 300, info))
			deletion := model.DB
			if physical {
				deletion = deletion.Unscoped()
			}
			require.NoError(t, deletion.Delete(&model.Token{}, token.Id).Error)
			require.NoError(t, HandoffTaskBilling(c, info, task, "", 500))
			require.Positive(t, task.ID)
			require.NoError(t, model.DB.First(user, user.Id).Error)
			require.Equal(t, 500, user.Quota)
			require.Equal(t, 500, user.UsedQuota)
			require.Equal(t, 1, user.RequestCount)
			if !physical {
				var deleted model.Token
				require.NoError(t, model.DB.Unscoped().First(&deleted, token.Id).Error)
				require.True(t, deleted.DeletedAt.Valid)
				require.Equal(t, 500, deleted.RemainQuota)
				require.Equal(t, 500, deleted.UsedQuota)
			}
		})
	}
}

func TestTaskSubmissionExactHandoffReplayAndTransferredRefundNoOp(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskSubmissionDurabilityFixture(t)
	user, token, info, task := prepareTaskHandoff(t)
	c := groupBillingContext(t)
	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	preHandoffTask := *task

	require.NoError(t, HandoffTaskBilling(c, info, task, "", 200))
	canonicalTaskID := task.ID
	session := info.Billing.(*BillingSession)
	quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 800, quota)
	require.Equal(t, 800, remain)
	require.Equal(t, 200, used)

	// Simulate a caller that did not observe the committed handoff and retained
	// its stale live-session view. The replay must resolve the exact stored owner.
	session.settled = false
	session.fundingSettled = false
	session.refunded = false
	session.preConsumedQuota = 300
	session.tokenConsumed = 300
	session.funding.(*WalletFunding).consumed = 300
	replayedTask := preHandoffTask
	require.NoError(t, HandoffTaskBilling(c, info, &replayedTask, "", 200))
	require.Equal(t, canonicalTaskID, replayedTask.ID)

	var taskCount, accountingCount, eventCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Where("task_id = ?", task.TaskID).Count(&taskCount).Error)
	require.NoError(t, model.DB.Model(&model.TaskAccounting{}).Where("task_row_id = ?", canonicalTaskID).Count(&accountingCount).Error)
	require.NoError(t, model.DB.Model(&model.TaskAccountingEvent{}).Where("task_row_id = ? AND kind = ?", canonicalTaskID, "initial").Count(&eventCount).Error)
	require.EqualValues(t, 1, taskCount)
	require.EqualValues(t, 1, accountingCount)
	require.EqualValues(t, 1, eventCount)
	var storedUser model.User
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	require.Equal(t, 200, storedUser.UsedQuota)
	require.Equal(t, 1, storedUser.RequestCount)

	// A stale refund after transfer is a DB-enforced no-op.
	session.settled = false
	session.fundingSettled = false
	session.refunded = false
	session.preConsumedQuota = 300
	session.tokenConsumed = 300
	session.funding.(*WalletFunding).consumed = 300
	session.Refund(c)
	quota, remain, used = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 800, quota)
	require.Equal(t, 800, remain)
	require.Equal(t, 200, used)
}

func TestTaskSubmissionDefiniteHandoffRollbackRefundsOnce(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskSubmissionDurabilityFixture(t)
	user, token, info, task := prepareTaskHandoff(t)
	c := groupBillingContext(t)
	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	info.ChannelId += 1000000
	require.Error(t, HandoffTaskBilling(c, info, task, "", 200))
	submission, err := model.GetTaskSubmission(info.TaskSubmissionID)
	require.NoError(t, err)
	require.Equal(t, model.TaskSubmissionStateActive, submission.State)
	require.Equal(t, 300, submission.ReservedQuota)

	session := info.Billing.(*BillingSession)
	session.Refund(c)
	session.Refund(c)
	quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 1000, quota)
	require.Equal(t, 1000, remain)
	require.Zero(t, used)
	submission, err = model.GetTaskSubmission(info.TaskSubmissionID)
	require.NoError(t, err)
	require.Equal(t, model.TaskSubmissionStateReleased, submission.State)
	var accountingCount int64
	require.NoError(t, model.DB.Model(&model.TaskAccounting{}).Count(&accountingCount).Error)
	require.Zero(t, accountingCount)
}

func TestTaskSubmissionExpiryVersusLateHandoff(t *testing.T) {
	useTaskAccountingDB(t, serviceTestSQLite, serviceTestSQLite, "sqlite")
	resetTaskSubmissionDurabilityFixture(t)
	user, token, info, task := prepareTaskHandoff(t)
	submissionID, leaseToken, err := CreateQueuedTaskSubmission(task)
	require.NoError(t, err)
	info.TaskSubmissionID = submissionID
	info.TaskSubmissionLeaseToken = leaseToken
	info.TaskSubmissionTaskRowID = task.ID
	c := groupBillingContext(t)
	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	session := info.Billing.(*BillingSession)
	session.mu.Lock()
	session.stopTaskSubmissionHeartbeatLocked()
	session.mu.Unlock()
	require.NoError(t, model.DB.Model(&model.TaskSubmission{}).Where("submission_id = ?", submissionID).
		Update("lease_expires_at", model.GetDBTimestamp()-1).Error)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var recoveryErr, handoffErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		recoveryErr = model.RecoverExpiredTaskSubmissions(context.Background(), 100)
	}()
	go func() {
		defer wg.Done()
		<-start
		handoffErr = HandoffTaskBilling(c, info, task, model.TaskStatusQueued, 200)
	}()
	close(start)
	wg.Wait()
	require.NoError(t, recoveryErr)

	submission, err := model.GetTaskSubmission(submissionID)
	require.NoError(t, err)
	switch submission.State {
	case model.TaskSubmissionStateTransferred:
		require.NoError(t, handoffErr)
		quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
		require.Equal(t, 800, quota)
		require.Equal(t, 800, remain)
		require.Equal(t, 200, used)
	case model.TaskSubmissionStateReleased:
		require.Error(t, handoffErr)
		session.Refund(c)
		quota, remain, used := readGroupBillingWallet(t, user.Id, token.Id)
		require.Equal(t, 1000, quota)
		require.Equal(t, 1000, remain)
		require.Zero(t, used)
		var storedTask model.Task
		require.NoError(t, model.DB.First(&storedTask, task.ID).Error)
		require.Equal(t, model.TaskStatusFailure, storedTask.Status)
	default:
		t.Fatalf("unexpected submission state after race: %s", submission.State)
	}
}
