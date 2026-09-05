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

// This fixture intentionally leaves an accepted, unapplied decision for the
// immutable image startup test. It only runs against its disposable CI database.
func TestTaskAccountingCompatibilitySeed(t *testing.T) {
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
	user := &User{Username: "b-compat-user", Password: "ci-only", Quota: 1000}
	require.NoError(t, db.Create(user).Error)
	token := &Token{UserId: user.Id, Key: "b-compat-synthetic-token", Name: "b-compat-token", RemainQuota: 1000}
	require.NoError(t, db.Create(token).Error)
	channel := &Channel{Name: "b-compat-channel"}
	require.NoError(t, db.Create(channel).Error)
	task := &Task{TaskID: "b-compat-pending", Platform: constant.TaskPlatformImage, UserId: user.Id, ChannelId: channel.Id, Group: "default", Status: TaskStatusQueued, Progress: "0%",
		PrivateData: TaskPrivateData{BillingSource: TaskAccountingFundingWallet, TokenId: token.Id}}
	_, err = WithReconciledGroupReservation(GroupReservationRequest{Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TargetReserved: 300}, func(tx *gorm.DB, _ *GroupReservationResult) error {
		_, err := PersistAsyncTaskHandoffTx(tx, AsyncTaskHandoffRequest{Task: task, ChargedQuota: 300,
			InitialLog: TaskAccountingLogFacts{UserID: user.Id, Username: user.Username, TokenID: token.Id, TokenName: token.Name, ChannelID: channel.Id, ModelName: "b-compat-model", Group: "default", Quota: 300, LogType: LogTypeConsume, Content: "CI compatibility charge", Other: map[string]any{"is_task": true}}})
		return err
	})
	require.NoError(t, err)
	decision, err := AcceptTaskTerminalDecision(context.Background(), TaskTerminalDecision{TaskRowID: task.ID, ExpectedStatus: TaskStatusQueued, Status: TaskStatusFailure, Progress: "100%", FinalQuota: 0, Reason: "CI interrupted refund"})
	require.NoError(t, err)
	require.True(t, decision.Won)
	require.False(t, decision.Applied)
	require.NoError(t, db.First(task, task.ID).Error)
	require.Equal(t, TaskStatusQueued, task.Status)
	require.Equal(t, 300, task.Quota)
	require.NoError(t, db.First(user, user.Id).Error)
	require.Equal(t, 700, user.Quota)
	require.Equal(t, 300, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	legacy := &Task{TaskID: "b-compat-legacy-terminal", Platform: constant.TaskPlatformImage, UserId: user.Id, ChannelId: channel.Id, Status: TaskStatusFailure, Quota: 50}
	require.NoError(t, db.Create(legacy).Error)
}
