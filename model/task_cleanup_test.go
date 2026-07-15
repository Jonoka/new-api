package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClearExpiredImageTaskDataOnlyClearsExpiredTerminalCanvasTasks(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	tasks := []*Task{
		{
			TaskID: "expired_success", Platform: constant.TaskPlatformCanvasImage,
			Status: TaskStatusSuccess, FinishTime: now - 7200, UpdatedAt: 1234,
			Data: json.RawMessage(`{"data":[{"b64_json":"large"}]}`),
		},
		{
			TaskID: "expired_failure", Platform: constant.TaskPlatformCanvasImage,
			Status: TaskStatusFailure, FinishTime: now - 7200, Data: json.RawMessage(`{"error":"failed"}`),
		},
		{
			TaskID: "expired_api_image", Platform: constant.TaskPlatformImage,
			Status: TaskStatusSuccess, FinishTime: now - 7200, Data: json.RawMessage(`{"data":[{"b64_json":"api"}]}`),
		},
		{
			TaskID: "recent_success", Platform: constant.TaskPlatformCanvasImage,
			Status: TaskStatusSuccess, FinishTime: now - 1800, Data: json.RawMessage(`{"data":[{"b64_json":"recent"}]}`),
		},
		{
			TaskID: "unfinished", Platform: constant.TaskPlatformCanvasImage,
			Status: TaskStatusInProgress, FinishTime: now - 7200, Data: json.RawMessage(`{"data":"working"}`),
		},
		{
			TaskID: "other_platform", Platform: constant.TaskPlatform("video"),
			Status: TaskStatusSuccess, FinishTime: now - 7200, Data: json.RawMessage(`{"data":"video"}`),
		},
	}
	for _, task := range tasks {
		insertTask(t, task)
	}
	var beforeCleanup Task
	require.NoError(t, DB.Where("task_id = ?", "expired_success").First(&beforeCleanup).Error)

	cleared, err := ClearExpiredImageTaskData(now-3600, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 3, cleared)

	for _, taskID := range []string{"expired_success", "expired_failure", "expired_api_image"} {
		var task Task
		require.NoError(t, DB.Where("task_id = ?", taskID).First(&task).Error)
		assert.Empty(t, task.Data, taskID)
		if taskID == "expired_success" {
			assert.EqualValues(t, beforeCleanup.UpdatedAt, task.UpdatedAt, "清理数据不应改写任务更新时间")
		}
	}
	for _, taskID := range []string{"recent_success", "unfinished", "other_platform"} {
		var task Task
		require.NoError(t, DB.Where("task_id = ?", taskID).First(&task).Error)
		assert.NotEmpty(t, task.Data, taskID)
	}
}

func TestClearExpiredImageTaskDataRespectsBatchLimit(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	for i := range 3 {
		insertTask(t, &Task{
			TaskID:     "expired_batch_" + string(rune('a'+i)),
			Platform:   constant.TaskPlatformCanvasImage,
			Status:     TaskStatusSuccess,
			FinishTime: now - 7200 + int64(i),
			Data:       json.RawMessage(`{"data":"large"}`),
		})
	}

	cleared, err := ClearExpiredImageTaskData(now-3600, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 2, cleared)

	var remaining int64
	require.NoError(t, DB.Model(&Task{}).Where("data IS NOT NULL").Count(&remaining).Error)
	assert.EqualValues(t, 1, remaining)

	var remainingTask Task
	require.NoError(t, DB.Where("data IS NOT NULL").First(&remainingTask).Error)
	assert.Equal(t, "expired_batch_a", remainingTask.TaskID, "应优先清理刚过期的数据")
}
