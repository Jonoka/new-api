package controller

import (
	"context"
	"strings"
	"testing"

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
}
