package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestTaskSubmissionWalletResizeAndExpiredRecoveryCrossDatabase(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			require.NoError(t, db.AutoMigrate(&Task{}, &TaskSubmission{}, &TaskAccounting{}))
			user, token := seedGroupReservationWallet(t, db, fmt.Sprintf("submission-%s-%d", fixture.name, time.Now().UnixNano()), 1000, 1000)
			req := GroupReservationRequest{
				Source:               GroupReservationWallet,
				UserId:               user.Id,
				ModelName:            "submission-model",
				TokenId:              token.Id,
				TokenKey:             token.Key,
				TargetReserved:       300,
				SubmissionID:         common.GetUUID(),
				SubmissionLeaseToken: common.GetUUID(),
			}
			_, err := ReconcileGroupReservation(req)
			require.NoError(t, err)
			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 700, userQuota)
			require.Equal(t, 700, tokenRemain)
			require.Equal(t, 300, tokenUsed)

			// A live lease is not recovered.
			require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))
			userQuota, tokenRemain, tokenUsed = readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 700, userQuota)
			require.Equal(t, 700, tokenRemain)
			require.Equal(t, 300, tokenUsed)

			// The same logical submission resizes without creating a second row.
			req.ExpectedReserved, req.TargetReserved = 300, 500
			_, err = ReconcileGroupReservation(req)
			require.NoError(t, err)
			var count int64
			require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).Count(&count).Error)
			require.EqualValues(t, 1, count)
			userQuota, tokenRemain, tokenUsed = readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 500, userQuota)
			require.Equal(t, 500, tokenRemain)
			require.Equal(t, 500, tokenUsed)

			require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).
				Update("lease_expires_at", GetDBTimestamp()-1).Error)
			require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))
			require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))
			userQuota, tokenRemain, tokenUsed = readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 1000, userQuota)
			require.Equal(t, 1000, tokenRemain)
			require.Zero(t, tokenUsed)
			submission, err := GetTaskSubmission(req.SubmissionID)
			require.NoError(t, err)
			require.Equal(t, TaskSubmissionStateReleased, submission.State)
			require.Zero(t, submission.ReservedQuota)
		})
	}
}

func TestTaskSubmissionQueuedTaskCrashFailsAtomically(t *testing.T) {
	fixture := groupReservationDatabases(t)[0]
	db := useGroupReservationDatabase(t, fixture)
	require.NoError(t, db.AutoMigrate(&Task{}, &TaskSubmission{}, &TaskAccounting{}))
	user, _ := seedGroupReservationWallet(t, db, fmt.Sprintf("queued-%d", time.Now().UnixNano()), 1000, 1000)
	task := &Task{
		TaskID: GenerateTaskID(), Platform: constant.TaskPlatformCanvasImage,
		UserId: user.Id, Status: TaskStatusQueued, Progress: "0%", SubmitTime: GetDBTimestamp(),
	}
	submissionID, leaseToken := common.GetUUID(), common.GetUUID()
	require.NoError(t, CreateQueuedTaskSubmission(task, submissionID, leaseToken))
	require.Positive(t, task.ID)
	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", submissionID).
		Update("lease_expires_at", GetDBTimestamp()-1).Error)
	require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))

	var stored Task
	require.NoError(t, db.First(&stored, task.ID).Error)
	require.Equal(t, TaskStatusFailure, stored.Status)
	require.Contains(t, stored.FailReason, "submission lease expired")
	submission, err := GetTaskSubmission(submissionID)
	require.NoError(t, err)
	require.Equal(t, TaskSubmissionStateReleased, submission.State)
	require.NotNil(t, submission.TaskRowID)
	require.Equal(t, task.ID, *submission.TaskRowID)
}

func TestTaskSubmissionRecoveryKeepsSoftDeletedTokenDeleted(t *testing.T) {
	fixture := groupReservationDatabases(t)[0]
	db := useGroupReservationDatabase(t, fixture)
	require.NoError(t, db.AutoMigrate(&Task{}, &TaskSubmission{}, &TaskAccounting{}))
	user, token := seedGroupReservationWallet(t, db, fmt.Sprintf("deleted-token-%d", time.Now().UnixNano()), 1000, 1000)
	req := GroupReservationRequest{
		Source: GroupReservationWallet, UserId: user.Id, ModelName: "deleted-token-submission",
		TokenId: token.Id, TokenKey: token.Key, TargetReserved: 250,
		SubmissionID: common.GetUUID(), SubmissionLeaseToken: common.GetUUID(),
	}
	_, err := ReconcileGroupReservation(req)
	require.NoError(t, err)
	require.NoError(t, db.Delete(token).Error)
	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).
		Update("lease_expires_at", GetDBTimestamp()-1).Error)
	require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))
	var stored Token
	require.NoError(t, db.Unscoped().First(&stored, token.Id).Error)
	require.Equal(t, 1000, stored.RemainQuota)
	require.Zero(t, stored.UsedQuota)
	require.True(t, stored.DeletedAt.Valid)
}

func TestTaskSubmissionSubscriptionResetRecoveryWithoutToken(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			require.NoError(t, db.AutoMigrate(&Task{}, &TaskSubmission{}, &TaskAccounting{}))
			user, _ := seedGroupReservationWallet(t, db, fmt.Sprintf("sub-no-token-%s-%d", fixture.name, time.Now().UnixNano()), 1000, 1000)
			now := GetDBTimestamp()
			plan := &SubscriptionPlan{Title: "submission reset", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 2000, QuotaResetPeriod: SubscriptionResetNever}
			require.NoError(t, db.Create(plan).Error)
			InvalidateSubscriptionPlanCache(plan.Id)
			sub := &UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 2000, StartTime: now - 60, EndTime: now + 3600, Status: "active", LastResetTime: now - 60}
			require.NoError(t, db.Create(sub).Error)
			req := GroupReservationRequest{
				Source:               GroupReservationSubscription,
				UserId:               user.Id,
				ModelName:            "subscription-submission",
				SkipTokenQuota:       true,
				TargetReserved:       300,
				SubmissionID:         common.GetUUID(),
				SubmissionLeaseToken: common.GetUUID(),
			}
			result, err := ReconcileGroupReservation(req)
			require.NoError(t, err)
			require.Equal(t, sub.Id, result.SubscriptionId)
			require.NoError(t, db.Model(&UserSubscription{}).Where("id = ?", sub.Id).
				Updates(map[string]any{"last_reset_time": now, "amount_used": 600}).Error)
			require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", req.SubmissionID).
				Update("lease_expires_at", GetDBTimestamp()-1).Error)
			require.NoError(t, RecoverExpiredTaskSubmissions(context.Background(), 100))
			require.NoError(t, db.First(sub, sub.Id).Error)
			require.EqualValues(t, 600, sub.AmountUsed)
			var record SubscriptionPreConsumeRecord
			require.NoError(t, db.Where("request_id = ?", req.SubmissionID).First(&record).Error)
			require.Equal(t, "refunded", record.Status)
		})
	}
}
