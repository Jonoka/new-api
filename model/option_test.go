package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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

func TestValidateOptionValueModelPriceUnit(t *testing.T) {
	require.NoError(t, validateOptionValue("ModelPriceUnit", `{"video":"second","image":"request"}`))
	require.Error(t, validateOptionValue("ModelPriceUnit", `{"video":"minute"}`))
}

func TestValidateOptionValueModelPriceVariants(t *testing.T) {
	require.NoError(t, validateOptionValue("ModelPriceVariants", `{
		"video":{"resolution_enabled":true,"rules":[{"resolution":"720p","price":0.07}]}
	}`))
	require.Error(t, validateOptionValue("ModelPriceVariants", `{
		"video":{"resolution_enabled":true,"rules":[{"resolution":"","price":0.07}]}
	}`))
}

func TestUpdateModelPriceUnitInvalidatesPricingCache(t *testing.T) {
	originalOptionMap := common.OptionMap
	originalUnits := ratio_setting.ModelPriceUnit2JSONString()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, ratio_setting.UpdateModelPriceUnitByJSONString(originalUnits))
		InvalidatePricingCache()
	})

	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	pricingMap = []Pricing{{ModelName: "cached-model"}}
	lastGetPricingTime = time.Now()

	require.NoError(t, updateOptionMap("ModelPriceUnit", `{"video":"second"}`))
	require.Empty(t, pricingMap)
	require.True(t, lastGetPricingTime.IsZero())
}

func TestUpdateModelPriceReturnsEffectiveDefaultsToAdmin(t *testing.T) {
	originalOptionMap := common.OptionMap
	originalPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalPrices))
		InvalidatePricingCache()
	})

	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	pricingMap = []Pricing{{ModelName: "cached-model"}}
	lastGetPricingTime = time.Now()

	require.NoError(t, updateOptionMap("ModelPrice", `{"custom-video":0.12}`))

	common.OptionMapRWMutex.RLock()
	effective := common.OptionMap["ModelPrice"]
	common.OptionMapRWMutex.RUnlock()
	var prices map[string]float64
	require.NoError(t, common.UnmarshalJsonStr(effective, &prices))
	require.Equal(t, 0.12, prices["custom-video"])
	require.Equal(t, 0.08, prices["grok-imagine-video-1.5"])
	require.Empty(t, pricingMap)
	require.True(t, lastGetPricingTime.IsZero())
}
