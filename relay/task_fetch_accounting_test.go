package relay

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAsyncTaskGeminiFetchOwnsTerminalAccounting(t *testing.T) {
	for _, scenario := range []string{"success_after_progress", "failure_refund", "database_read_failure"} {
		t.Run(scenario, func(t *testing.T) {
			oldDB, oldLog := model.DB, model.LOG_DB
			oldSQLite, oldPG, oldMySQL := common.UsingSQLite, common.UsingPostgreSQL, common.UsingMySQL
			oldRedis, oldBatch, oldMemory, oldLogEnabled := common.RedisEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fetch-accounting.db")), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Task{}, &model.TaskAccounting{}, &model.TaskAccountingEvent{}, &model.TaskAccountingLogReceipt{}, &model.Log{}))
			model.DB, model.LOG_DB = db, db
			common.UsingSQLite, common.UsingPostgreSQL, common.UsingMySQL = true, false, false
			common.RedisEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = false, false, false, true
			t.Cleanup(func() {
				model.DB, model.LOG_DB = oldDB, oldLog
				common.UsingSQLite, common.UsingPostgreSQL, common.UsingMySQL = oldSQLite, oldPG, oldMySQL
				common.RedisEnabled, common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = oldRedis, oldBatch, oldMemory, oldLogEnabled
				if sqlDB, err := db.DB(); err == nil {
					_ = sqlDB.Close()
				}
			})
			service.InitHttpClient()
			user := &model.User{Username: "fetch-user", AffCode: "fetch-user", Quota: 1000}
			require.NoError(t, db.Create(user).Error)
			token := &model.Token{UserId: user.Id, Key: "fetch-synthetic-token", RemainQuota: 1000}
			require.NoError(t, db.Create(token).Error)
			var calls atomic.Int32
			var failReads atomic.Bool
			var task *model.Task
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if scenario == "success_after_progress" {
					if err := db.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusInProgress).Error; err != nil {
						t.Error(err)
					}
				}
				if scenario == "database_read_failure" {
					failReads.Store(true)
				}
				if scenario == "failure_refund" {
					_, _ = w.Write([]byte(`{"done":true,"error":{"message":"provider failed"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"name":"models/veo/operations/fixture","done":true,"response":{}}`))
			}))
			t.Cleanup(upstream.Close)
			baseURL := upstream.URL
			channel := &model.Channel{Type: constant.ChannelTypeGemini, Name: "fetch-channel", Key: "synthetic-provider-key", BaseURL: &baseURL, Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(channel).Error)
			task = &model.Task{TaskID: "fetch-public-id", UserId: user.Id, ChannelId: channel.Id, Group: "default", Status: model.TaskStatusQueued,
				PrivateData: model.TaskPrivateData{BillingSource: "wallet", TokenId: token.Id, UpstreamTaskID: taskcommon.EncodeLocalTaskID("models/veo/operations/fixture")}}
			_, err = model.WithReconciledGroupReservation(model.GroupReservationRequest{Source: "wallet", UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TargetReserved: 300}, func(tx *gorm.DB, _ *model.GroupReservationResult) error {
				_, err := model.PersistAsyncTaskHandoffTx(tx, model.AsyncTaskHandoffRequest{Task: task, ChargedQuota: 300, InitialLog: model.TaskAccountingLogFacts{UserID: user.Id, Username: user.Username, TokenID: token.Id, ChannelID: channel.Id, ModelName: "veo", Group: "default", Other: map[string]any{"is_task": true}}})
				return err
			})
			require.NoError(t, err)
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fetch-read-failure", func(tx *gorm.DB) {
				if failReads.Load() && tx.Statement.Table == "tasks" {
					tx.AddError(errors.New("injected unavailable task read"))
				}
			}))
			body := tryRealtimeFetch(task, false)
			require.EqualValues(t, 1, calls.Load())
			if scenario == "database_read_failure" {
				require.Nil(t, body)
				require.EqualValues(t, model.TaskStatusQueued, task.Status)
				require.Empty(t, task.PrivateData.ResultURL)
				failReads.Store(false)
			} else {
				require.NotEmpty(t, body)
				require.Nil(t, tryRealtimeFetch(task, false))
				require.EqualValues(t, 1, calls.Load())
			}
			require.NoError(t, db.First(user, user.Id).Error)
			require.NoError(t, db.First(token, token.Id).Error)
			require.Equal(t, 1, user.RequestCount)
			if scenario == "failure_refund" {
				require.EqualValues(t, model.TaskStatusFailure, task.Status)
				require.Equal(t, 1000, user.Quota)
				require.Equal(t, 1000, token.RemainQuota)
				require.Zero(t, user.UsedQuota)
			} else {
				require.Equal(t, 700, user.Quota)
				require.Equal(t, 700, token.RemainQuota)
				require.Equal(t, 300, user.UsedQuota)
			}
			if scenario == "success_after_progress" {
				require.EqualValues(t, model.TaskStatusSuccess, task.Status)
			}
		})
	}
}
