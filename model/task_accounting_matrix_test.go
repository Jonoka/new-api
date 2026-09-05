package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func taskAccountingMatrixDatabase(t *testing.T, fixture groupReservationDatabase) *gorm.DB {
	t.Helper()
	db := useGroupReservationDatabase(t, fixture)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Task{}, &TaskSubmission{}, &TaskAccounting{}, &TaskAccountingEvent{}))
	openLog := func() (*gorm.DB, error) {
		return gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "separate-log.db")), &gorm.Config{})
	}
	if fixture.mysql {
		openLog = fixture.open
	} else if fixture.sqlite {
		for _, candidate := range groupReservationDatabases(t) {
			if candidate.postgres {
				openLog = candidate.open
				break
			}
		}
	}
	logDB, err := openLog()
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&Log{}, &TaskAccountingLogReceipt{}))
	oldLog, oldEnabled := LOG_DB, common.LogConsumeEnabled
	LOG_DB, common.LogConsumeEnabled = logDB, true
	t.Cleanup(func() {
		LOG_DB, common.LogConsumeEnabled = oldLog, oldEnabled
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedTaskAccountingMatrix(t *testing.T, db *gorm.DB) (*Task, *User, *Token, *Channel) {
	t.Helper()
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	user := &User{Username: "m" + suffix, Password: "ci-only", Quota: 2000}
	user.AffCode = user.Username
	require.NoError(t, db.Create(user).Error)
	token := &Token{UserId: user.Id, Key: "matrix-token-" + suffix, Name: "matrix", RemainQuota: 2000}
	require.NoError(t, db.Create(token).Error)
	channel := &Channel{Name: "matrix-" + suffix}
	require.NoError(t, db.Create(channel).Error)
	task := &Task{TaskID: "matrix-" + suffix, UserId: user.Id, ChannelId: channel.Id, Group: "paid", Status: TaskStatusQueued, Platform: constant.TaskPlatformImage,
		PrivateData: TaskPrivateData{BillingSource: TaskAccountingFundingWallet, TokenId: token.Id}}
	_, err := WithReconciledGroupReservation(GroupReservationRequest{Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TargetReserved: 300}, func(tx *gorm.DB, _ *GroupReservationResult) error {
		_, err := PersistAsyncTaskHandoffTx(tx, AsyncTaskHandoffRequest{Task: task, ChargedQuota: 300,
			InitialLog: TaskAccountingLogFacts{UserID: user.Id, Username: user.Username, TokenID: token.Id, ChannelID: channel.Id, ModelName: "frozen-model", Group: "paid", Quota: 300, LogType: LogTypeConsume, Other: map[string]any{"is_task": true, "group_ratio": 2.0}}})
		return err
	})
	require.NoError(t, err)
	return task, user, token, channel
}

func assertTaskAccountingMatrix(t *testing.T, db *gorm.DB, task *Task, user *User, token *Token, channel *Channel, target int) {
	t.Helper()
	require.NoError(t, db.First(user, user.Id).Error)
	require.NoError(t, db.First(token, token.Id).Error)
	require.NoError(t, db.First(channel, channel.Id).Error)
	require.NoError(t, db.First(task, task.ID).Error)
	require.Equal(t, 2000-target, user.Quota)
	require.Equal(t, 2000-target, token.RemainQuota)
	require.Equal(t, target, token.UsedQuota)
	require.Equal(t, target, user.UsedQuota)
	require.EqualValues(t, target, channel.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Equal(t, target, task.Quota)
}

func TestTaskAccountingFinalQuotaCrossDatabase(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) {
			db := taskAccountingMatrixDatabase(t, fixture)
			oldBatch := common.BatchUpdateEnabled
			common.BatchUpdateEnabled = true
			t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })
			for _, target := range []int{0, 100, 300, 500} {
				t.Run(fmt.Sprint(target), func(t *testing.T) {
					task, user, token, channel := seedTaskAccountingMatrix(t, db)
					missingIdentity := *task
					missingIdentity.ID = 0
					missingIdentity.Status = TaskStatusFailure
					wonMissing, err := missingIdentity.UpdateWithStatus(TaskStatusQueued)
					require.Error(t, err)
					require.False(t, wonMissing)
					decision := TaskTerminalDecision{TaskRowID: task.ID, ExpectedStatus: TaskStatusQueued, Status: TaskStatusSuccess, FinalQuota: target, Progress: "100%", Reason: "frozen actual usage"}
					accepted, err := AcceptTaskTerminalDecision(context.Background(), decision)
					require.NoError(t, err)
					require.True(t, accepted.Won)
					require.NoError(t, db.First(task, task.ID).Error)
					require.EqualValues(t, TaskStatusQueued, task.Status)
					stale := *task
					stale.Progress = "90%"
					won, err := stale.UpdateWithStatus(TaskStatusQueued)
					require.NoError(t, err)
					require.False(t, won)
					require.NoError(t, RecoverTaskAccounting(context.Background(), 100))
					assertTaskAccountingMatrix(t, db, task, user, token, channel, target)
					require.EqualValues(t, TaskStatusSuccess, task.Status)
					decision.Status, decision.FinalQuota = TaskStatusFailure, 0
					duplicate, err := AcceptTaskTerminalDecision(context.Background(), decision)
					require.NoError(t, err)
					require.False(t, duplicate.Won)
					require.NoError(t, RecoverTaskAccounting(context.Background(), 100))
					assertTaskAccountingMatrix(t, db, task, user, token, channel, target)
					var logs int64
					require.NoError(t, LOG_DB.Model(&Log{}).Where("username = ?", user.Username).Count(&logs).Error)
					wantLogs := int64(2)
					if target == 300 {
						wantLogs = 1
					}
					require.Equal(t, wantLogs, logs)
				})
			}
		})
	}
}

func TestTaskAccountingRollbackAndLogRedelivery(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) {
			db := taskAccountingMatrixDatabase(t, fixture)
			task, user, token, channel := seedTaskAccountingMatrix(t, db)
			_, err := AcceptTaskTerminalDecision(context.Background(), TaskTerminalDecision{TaskRowID: task.ID, ExpectedStatus: TaskStatusQueued, Status: TaskStatusFailure, FinalQuota: 0})
			require.NoError(t, err)
			name := "test:matrix-fail-token"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
				if tx.Statement.Table == "tokens" {
					tx.AddError(errors.New("injected token failure"))
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Update().Remove(name) })
			_, err = ApplyTaskAccountingDecision(context.Background(), task.ID)
			require.Error(t, err)
			assertTaskAccountingMatrix(t, db, task, user, token, channel, 300)
			require.EqualValues(t, TaskStatusQueued, task.Status)
			require.NoError(t, db.Callback().Update().Remove(name))
			_, err = ApplyTaskAccountingDecision(context.Background(), task.ID)
			require.NoError(t, err)
			assertTaskAccountingMatrix(t, db, task, user, token, channel, 0)
			logFailure := "test:matrix-log-unavailable"
			require.NoError(t, LOG_DB.Callback().Create().Before("gorm:create").Register(logFailure, func(tx *gorm.DB) {
				if tx.Statement.Table == "logs" {
					tx.AddError(errors.New("injected independent log failure"))
				}
			}))
			logDB := LOG_DB
			t.Cleanup(func() { _ = logDB.Callback().Create().Remove(logFailure) })
			require.Error(t, DeliverPendingTaskAccountingLogs(context.Background(), 100))
			require.NoError(t, LOG_DB.Callback().Create().Remove(logFailure))
			var beforeRetry int64
			require.NoError(t, LOG_DB.Model(&Log{}).Where("username = ?", user.Username).Count(&beforeRetry).Error)
			require.Zero(t, beforeRetry)
			assertTaskAccountingMatrix(t, db, task, user, token, channel, 0)
			ackName := "test:matrix-fail-log-ack"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(ackName, func(tx *gorm.DB) {
				if tx.Statement.Table == "task_accounting_events" {
					tx.AddError(errors.New("injected primary acknowledgement failure"))
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Update().Remove(ackName) })
			require.Error(t, DeliverPendingTaskAccountingLogs(context.Background(), 100))
			require.NoError(t, db.Callback().Update().Remove(ackName))
			require.NoError(t, DeliverPendingTaskAccountingLogs(context.Background(), 100))
			var logs, receipts int64
			require.NoError(t, LOG_DB.Model(&Log{}).Where("username = ?", user.Username).Count(&logs).Error)
			require.NoError(t, LOG_DB.Table("task_accounting_log_receipts AS receipt").Joins("JOIN logs ON logs.id = receipt.log_id").Where("logs.username = ?", user.Username).Count(&receipts).Error)
			require.EqualValues(t, 2, logs)
			require.EqualValues(t, 2, receipts)
		})
	}
}

func TestTaskAccountingConcurrentTerminalDecisions(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		t.Run(fixture.name, func(t *testing.T) {
			db := taskAccountingMatrixDatabase(t, fixture)
			if fixture.sqlite {
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(1)
			}
			task, user, token, channel := seedTaskAccountingMatrix(t, db)
			var wg sync.WaitGroup
			results := make(chan *TaskTerminalDecisionResult, 8)
			errorsSeen := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func(target int) {
					defer wg.Done()
					result, err := AcceptTaskTerminalDecision(context.Background(), TaskTerminalDecision{TaskRowID: task.ID, ExpectedStatus: TaskStatusQueued, Status: TaskStatusSuccess, FinalQuota: target})
					if err != nil {
						errorsSeen <- err
						return
					}
					results <- result
				}(i * 50)
			}
			wg.Wait()
			close(results)
			close(errorsSeen)
			for err := range errorsSeen {
				require.NoError(t, err)
			}
			wins, finalQuota := 0, -1
			for result := range results {
				if result.Won {
					wins++
					finalQuota = result.Accounting.DecisionQuota
				}
			}
			require.Equal(t, 1, wins)
			require.NoError(t, RecoverTaskAccounting(context.Background(), 100))
			assertTaskAccountingMatrix(t, db, task, user, token, channel, finalQuota)
		})
	}
}
