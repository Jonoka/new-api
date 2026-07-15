package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunImageTaskDataCleanupOnceHonorsRetentionSetting(t *testing.T) {
	truncate(t)
	previous := common.GetImageTaskDataRetentionHours()
	t.Cleanup(func() {
		common.SetImageTaskDataRetentionHours(previous)
		imageTaskDataCleanupRunning.Store(false)
	})

	task := &model.Task{
		TaskID:     "cleanup_setting",
		Platform:   constant.TaskPlatformImage,
		Status:     model.TaskStatusSuccess,
		FinishTime: time.Now().Add(-2 * time.Hour).Unix(),
		Data:       json.RawMessage(`{"data":[{"b64_json":"large"}]}`),
	}
	require.NoError(t, model.DB.Create(task).Error)

	common.SetImageTaskDataRetentionHours(0)
	runImageTaskDataCleanupOnce()
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.NotEmpty(t, reloaded.Data)

	common.SetImageTaskDataRetentionHours(1)
	runImageTaskDataCleanupOnce()
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Empty(t, reloaded.Data)
}
