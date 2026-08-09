package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestImageTaskDataRetentionOptionValidation(t *testing.T) {
	const key = "performance_setting.image_task_data_retention_hours"
	require.NoError(t, validateOptionValue(key, "0"))
	require.NoError(t, validateOptionValue(key, "1"))
	require.NoError(t, validateOptionValue(
		key,
		strconv.Itoa(common.MaxImageTaskDataRetentionHours),
	))
	require.Error(t, validateOptionValue(key, "-1"))
	require.Error(t, validateOptionValue(key, "1.5"))
	require.Error(t, validateOptionValue(
		key,
		strconv.Itoa(common.MaxImageTaskDataRetentionHours+1),
	))
}

func TestUpdateImageTaskDataRetentionOptionUpdatesRuntimeValue(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	previous := common.GetImageTaskDataRetentionHours()
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		_ = updateOptionMap("performance_setting.image_task_data_retention_hours", strconv.Itoa(previous))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		_ = DB.Where("key = ?", "performance_setting.image_task_data_retention_hours").Delete(&Option{}).Error
	})

	const key = "performance_setting.image_task_data_retention_hours"
	require.NoError(t, UpdateOption(key, "2"))
	require.Equal(t, 2, common.GetImageTaskDataRetentionHours())

	var option Option
	require.NoError(t, DB.Where("key = ?", key).First(&option).Error)
	require.Equal(t, "2", option.Value)

	require.Error(t, UpdateOption(key, "invalid"))
	require.NoError(t, DB.Where("key = ?", key).First(&option).Error)
	require.Equal(t, "2", option.Value, "非法配置不能覆盖已持久化的有效值")
}
