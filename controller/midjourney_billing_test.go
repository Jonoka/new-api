package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMidjourneyPollerProjectsCanonicalTerminalTaskAfterRestart(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB = oldDB
		_ = sqlDB.Close()
	})
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskAccounting{}, &model.Midjourney{}))

	terminal := dto.MidjourneyDto{
		MjId: "mj-restart", Status: string(model.TaskStatusFailure), Progress: "100%",
		FailReason: "canonical failure", FinishTime: 123000,
	}
	data, err := common.Marshal(terminal)
	require.NoError(t, err)
	task := &model.Task{
		TaskID: "internal-mj-restart", Platform: constant.TaskPlatformMidjourney,
		Status: model.TaskStatusFailure, Progress: "100%", FinishTime: 123,
		FailReason: terminal.FailReason, Data: data,
	}
	require.NoError(t, db.Create(task).Error)
	taskRowID := task.ID
	mj := &model.Midjourney{MjId: terminal.MjId, Status: "", Progress: "50%", TaskRowID: &taskRowID}
	require.NoError(t, db.Create(mj).Error)
	publicJSON, err := common.Marshal(mj)
	require.NoError(t, err)
	require.NotContains(t, string(publicJSON), "task_row_id")
	require.NotContains(t, string(publicJSON), "TaskRowID")

	projected, err := projectCompletedMidjourneyAccounting(context.Background(), mj)
	require.NoError(t, err)
	require.True(t, projected)
	reloaded := model.GetMjByuId(mj.Id)
	require.NotNil(t, reloaded)
	require.Equal(t, string(model.TaskStatusFailure), reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, terminal.FailReason, reloaded.FailReason)

	projection, err := service.MidjourneyTaskProjection(task)
	require.NoError(t, err)
	require.Equal(t, terminal.FinishTime, projection.FinishTime)
}

func TestGenericTaskPollingExcludesInternalMidjourneyRows(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB = oldDB
		_ = sqlDB.Close()
	})
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskAccounting{}))

	now := int64(1000)
	mj := &model.Task{TaskID: "internal-mj", Platform: constant.TaskPlatformMidjourney, Status: model.TaskStatusSubmitted, Progress: "0%", SubmitTime: now - 10}
	video := &model.Task{TaskID: "ordinary-video", Platform: constant.TaskPlatform("video"), Status: model.TaskStatusSubmitted, Progress: "0%", SubmitTime: now - 10}
	require.NoError(t, db.Create(mj).Error)
	require.NoError(t, db.Create(video).Error)

	unfinished := model.GetAllUnFinishSyncTasks(10)
	require.Len(t, unfinished, 1)
	require.Equal(t, video.ID, unfinished[0].ID)
	timedOut := model.GetTimedOutUnfinishedTasks(now, 10)
	require.Len(t, timedOut, 1)
	require.Equal(t, video.ID, timedOut[0].ID)
	require.Len(t, model.TaskGetAllUserTask(0, 0, 10, model.SyncTaskQueryParams{}), 1)
	require.Equal(t, int64(1), model.TaskCountAllUserTask(0, model.SyncTaskQueryParams{}))
	require.Len(t, model.TaskGetAllTasks(0, 10, model.SyncTaskQueryParams{}), 1)
	require.Equal(t, int64(1), model.TaskCountAllTasks(model.SyncTaskQueryParams{}))
	require.Empty(t, model.TaskGetAllUserTask(0, 0, 10, model.SyncTaskQueryParams{Platform: constant.TaskPlatformMidjourney}))
	require.Zero(t, model.TaskCountAllUserTask(0, model.SyncTaskQueryParams{Platform: constant.TaskPlatformMidjourney}))
}

func TestMidjourneyPollerPreservesLegacyWalletOnlyRefund(t *testing.T) {
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSQLite, oldRedis := common.UsingSQLite, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite, common.RedisEnabled = oldSQLite, oldRedis
		_ = sqlDB.Close()
	})
	model.DB, model.LOG_DB = db, db
	common.UsingSQLite, common.RedisEnabled = true, false
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Midjourney{}, &model.Log{}, &model.BalanceCacheRepair{}))

	user := &model.User{Username: fmt.Sprintf("legacy-mj-%d", time.Now().UnixNano()), Password: "test", Quota: 1000}
	user.AffCode = user.Username
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{UserId: user.Id, Name: "legacy-mj", Key: fmt.Sprintf("legacy-mj-%d", time.Now().UnixNano()), RemainQuota: 700, UsedQuota: 300}
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, model.DecreaseUserQuota(user.Id, 300, false), "fixture must contain a real pre-cutover wallet debit")

	legacy := &model.Midjourney{
		UserId: user.Id, Action: constant.MjActionImagine, MjId: "legacy-upstream-id",
		Status: "", Progress: "50%", ChannelId: 9, Quota: 300,
	}
	require.NoError(t, db.Create(legacy).Error)
	preStatus := legacy.Status
	legacy.Status = string(model.TaskStatusFailure)
	legacy.Progress = "100%"
	legacy.FailReason = "legacy provider failure"

	won, err := updateMidjourneyTaskWithLegacyRefund(context.Background(), legacy, preStatus, true)
	require.NoError(t, err)
	require.True(t, won)
	var gotUser model.User
	var gotToken model.Token
	require.NoError(t, db.First(&gotUser, user.Id).Error)
	require.NoError(t, db.First(&gotToken, token.Id).Error)
	require.Equal(t, 1000, gotUser.Quota)
	require.Equal(t, 700, gotToken.RemainQuota, "legacy compatibility must not infer a token refund")
	require.Equal(t, 300, gotToken.UsedQuota)

	won, err = updateMidjourneyTaskWithLegacyRefund(context.Background(), legacy, preStatus, true)
	require.NoError(t, err)
	require.False(t, won)
	require.NoError(t, db.First(&gotUser, user.Id).Error)
	require.Equal(t, 1000, gotUser.Quota, "legacy status CAS must prevent a duplicate wallet refund")
	var refundLogs int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", user.Id, model.LogTypeRefund).Count(&refundLogs).Error)
	require.Equal(t, int64(1), refundLogs)
}
