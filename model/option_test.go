package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestInitOptionMapMigratesLegacyAutomaticRetryStatusCodes(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where("key = ?", "AutomaticRetryStatusCodes").Delete(&Option{}).Error)

	legacyValue := "100-199,300-399,401-407,409-499,500-503,505-523,525-599"
	currentValue := "100-199,300-399,401-407,409-499,500-599"
	require.NoError(t, DB.Create(&Option{
		Key:   "AutomaticRetryStatusCodes",
		Value: legacyValue,
	}).Error)

	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, DB.Where("key = ?", "AutomaticRetryStatusCodes").Delete(&Option{}).Error)
	})

	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(currentValue))
	InitOptionMap()

	var option Option
	require.NoError(t, DB.Where("key = ?", "AutomaticRetryStatusCodes").First(&option).Error)
	require.Equal(t, currentValue, option.Value)

	common.OptionMapRWMutex.RLock()
	require.Equal(t, currentValue, common.OptionMap["AutomaticRetryStatusCodes"])
	common.OptionMapRWMutex.RUnlock()
	require.True(t, operation_setting.ShouldRetryByStatusCode(504))
	require.True(t, operation_setting.ShouldRetryByStatusCode(524))
}
