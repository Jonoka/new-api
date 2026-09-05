package model

import (
	"context"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func taskAccountingCompatibilityDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("NEW_API_TEST_COMPATIBILITY_DSN")
	if dsn == "" {
		t.Skip("only used by the immutable image compatibility rehearsal")
	}
	require.Equal(t, "true", os.Getenv("GITHUB_ACTIONS"))
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	oldDB, oldLog := DB, LOG_DB
	oldPG, oldSQLite, oldMySQL := common.UsingPostgreSQL, common.UsingSQLite, common.UsingMySQL
	oldRedis, oldBatch := common.RedisEnabled, common.BatchUpdateEnabled
	DB, LOG_DB = db, db
	common.UsingPostgreSQL, common.UsingSQLite, common.UsingMySQL = true, false, false
	common.RedisEnabled, common.BatchUpdateEnabled = false, true
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLog
		common.UsingPostgreSQL, common.UsingSQLite, common.UsingMySQL = oldPG, oldSQLite, oldMySQL
		common.RedisEnabled, common.BatchUpdateEnabled = oldRedis, oldBatch
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// Leave real interrupted reservations and decisions for immutable-image recovery.
func TestTaskAccountingCompatibilitySeed(t *testing.T) {
	db := taskAccountingCompatibilityDatabase(t)
	user := &User{Username: "b-compat-user", Password: "ci-only", Quota: 1000}
	user.AffCode = user.Username
	require.NoError(t, db.Create(user).Error)
	token := &Token{UserId: user.Id, Key: "b-compat-synthetic-token", Name: "b-compat-token", RemainQuota: 1000}
	require.NoError(t, db.Create(token).Error)
	channel := &Channel{Name: "b-compat-channel"}
	require.NoError(t, db.Create(channel).Error)
	task := seedTaskAccountingCompatibilityTask(t, user, token, channel, "b-compat-pending", 300)
	decision, err := AcceptTaskTerminalDecision(context.Background(), TaskTerminalDecision{TaskRowID: task.ID, ExpectedStatus: TaskStatusQueued, Status: TaskStatusFailure, Progress: "100%", FinalQuota: 0, Reason: "CI interrupted refund"})
	require.NoError(t, err)
	require.True(t, decision.Won)
	require.False(t, decision.Applied)
	require.NoError(t, db.First(task, task.ID).Error)
	require.EqualValues(t, TaskStatusQueued, task.Status)
	require.Equal(t, 300, task.Quota)
	require.NoError(t, db.First(user, user.Id).Error)
	require.Equal(t, 700, user.Quota)
	require.Equal(t, 300, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	legacy := &Task{TaskID: "b-compat-legacy-terminal", Platform: constant.TaskPlatformImage, UserId: user.Id, ChannelId: channel.Id, Status: TaskStatusFailure, Quota: 50}
	require.NoError(t, db.Create(legacy).Error)
	legacyPending := &Task{TaskID: "b-compat-legacy-pending", Platform: constant.TaskPlatformImage, UserId: user.Id, ChannelId: channel.Id, Status: TaskStatusQueued, Quota: 50}
	require.NoError(t, db.Create(legacyPending).Error)
	seedTaskAccountingCompatibilityTask(t, user, token, channel, "b-compat-live", 200)
	abandonedID := common.GetUUID()
	_, err = ReconcileGroupReservation(GroupReservationRequest{Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, ModelName: "b-compat-abandoned", TargetReserved: 75,
		SubmissionID: abandonedID, SubmissionLeaseToken: common.GetUUID(), SubmissionOperationID: common.GetUUID()})
	require.NoError(t, err)
	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", abandonedID).Update("lease_expires_at", 1).Error)
	queued := &Task{TaskID: "b-compat-queued", Platform: constant.TaskPlatformCanvasImage, UserId: user.Id, Status: TaskStatusQueued}
	queuedID := common.GetUUID()
	require.NoError(t, CreateQueuedTaskSubmission(queued, queuedID, common.GetUUID()))
	require.NoError(t, db.Model(&TaskSubmission{}).Where("submission_id = ?", queuedID).Update("lease_expires_at", 1).Error)
}

func seedTaskAccountingCompatibilityTask(t *testing.T, user *User, token *Token, channel *Channel, publicID string, quota int) *Task {
	t.Helper()
	submissionID, leaseToken := common.GetUUID(), common.GetUUID()
	task := &Task{TaskID: publicID, Platform: constant.TaskPlatformImage, UserId: user.Id, ChannelId: channel.Id, Group: "default", Status: TaskStatusQueued, Progress: "0%",
		PrivateData: TaskPrivateData{BillingSource: TaskAccountingFundingWallet, TokenId: token.Id}}
	_, err := WithReconciledGroupReservation(GroupReservationRequest{Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, ModelName: "b-compat-model", TargetReserved: quota,
		SubmissionID: submissionID, SubmissionLeaseToken: leaseToken, SubmissionOperationID: common.GetUUID()}, func(tx *gorm.DB, _ *GroupReservationResult) error {
		_, err := PersistAsyncTaskHandoffTx(tx, AsyncTaskHandoffRequest{Task: task, ChargedQuota: quota,
			InitialLog: TaskAccountingLogFacts{UserID: user.Id, Username: user.Username, TokenID: token.Id, TokenName: token.Name, ChannelID: channel.Id, ModelName: "b-compat-model", Group: "default", Quota: quota, LogType: LogTypeConsume, Content: "CI compatibility charge", Other: map[string]any{"is_task": true}}})
		if err != nil {
			return err
		}
		return TransferTaskSubmissionTx(tx, submissionID, leaseToken, task.ID, quota)
	})
	require.NoError(t, err)
	return task
}

func TestTaskAccountingCompatibilityDrain(t *testing.T) {
	db := taskAccountingCompatibilityDatabase(t)
	var task Task
	require.NoError(t, db.Where("task_id = ?", "b-compat-live").First(&task).Error)
	require.EqualValues(t, TaskStatusQueued, task.Status)
	result, err := AcceptTaskTerminalDecision(context.Background(), TaskTerminalDecision{TaskRowID: task.ID, ExpectedStatus: task.Status, Status: TaskStatusSuccess, FinalQuota: task.Quota, Progress: "100%", Reason: "CI live task completed"})
	require.NoError(t, err)
	require.True(t, result.Won)
}
