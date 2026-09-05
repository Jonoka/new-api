package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestModelPriceHelperTieredUsesCompletionFallbackAndRejectsOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{
			"tiered-fallback-model":"tiered_expr",
			"tiered-overflow-model":"tiered_expr"
		}`,
		"billing_setting.billing_expr": `{
			"tiered-fallback-model":"tier(\"base\", p * 3 + c * 15)",
			"tiered-overflow-model":"tier(\"overflow\", p * 1000000000)"
		}`,
		"group_ratio_setting.group_ratio": `{"default":1,"free":0}`,
	}))

	newInfo := func(model, group string) (*gin.Context, *relaycommon.RelayInfo) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		ctx.Set("group", group)
		return ctx, &relaycommon.RelayInfo{
			OriginModelName: model,
			UserGroup:       group,
			UsingGroup:      group,
			BillingRequestInput: &billingexpr.RequestInput{
				Body: []byte(`{}`),
			},
		}
	}

	ctx, info := newInfo("tiered-fallback-model", "default")
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	// (1000*3 + 8192*15) / 1e6 * 500000 = 62940
	require.Equal(t, 62940, priceData.QuotaToPreConsume)

	ctx, info = newInfo("tiered-fallback-model", "free")
	priceData, err = ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Zero(t, priceData.QuotaToPreConsume)
	require.Equal(t, defaultTieredPreConsumeMaxTokens, info.TieredBillingSnapshot.EstimatedCompletionTokens)
	require.Positive(t, info.TieredBillingSnapshot.EstimatedQuotaBeforeGroup)

	ctx, info = newInfo("tiered-overflow-model", "default")
	_, err = ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	require.Equal(t, "QuotaRound", clamp.Op)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}

func TestModelPriceHelperTieredRefreshesOnlyGroupDependentSnapshotFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":      `{"tiered-switch-model":"tiered_expr"}`,
		"billing_setting.billing_expr":      `{"tiered-switch-model":"tier(\"original\", p * 3 + c * 15)"}`,
		"group_ratio_setting.group_ratio":   `{"free":0,"paid":2}`,
	}))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("auto_group", "free")
	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-switch-model",
		UserGroup:       "tiered-test-user",
		UsingGroup:      "free",
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}
	first, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.True(t, first.FreeModel)
	require.Equal(t, defaultTieredPreConsumeMaxTokens, info.TieredBillingSnapshot.EstimatedCompletionTokens)
	frozenExpr := info.TieredBillingSnapshot.ExprString
	frozenRequest := string(info.BillingRequestInput.Body)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":      `{}`,
		"billing_setting.billing_expr":      `{}`,
		"group_ratio_setting.group_ratio":   `{"free":0,"paid":2}`,
	}))
	ctx.Set("auto_group", "paid")
	second, err := ModelPriceHelper(ctx, info, 9999, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	require.False(t, second.FreeModel)
	require.Equal(t, 125880, second.QuotaToPreConsume)
	require.Equal(t, "paid", info.TieredBillingSnapshot.Group)
	require.Equal(t, float64(2), info.TieredBillingSnapshot.GroupRatio)
	require.Equal(t, frozenExpr, info.TieredBillingSnapshot.ExprString)
	require.Equal(t, 1000, info.TieredBillingSnapshot.EstimatedPromptTokens)
	require.Equal(t, defaultTieredPreConsumeMaxTokens, info.TieredBillingSnapshot.EstimatedCompletionTokens)
	require.Equal(t, frozenRequest, string(info.BillingRequestInput.Body))
	require.Equal(t, "original", info.TieredBillingSnapshot.EstimatedTier)
}

func TestModelPriceHelperAppliesRequestBillingRatiosOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedModelPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedModelPrices))
	})

	modelPrices, err := common.Marshal(map[string]float64{"fixed-image-price": 0.04})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(modelPrices)))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-image-price",
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{
		ImagePriceRatio: 3,
		BillingRatios:   map[string]float64{"n": 3},
	})

	require.NoError(t, err)
	require.Equal(t, 180000, priceData.QuotaToPreConsume)
	require.Equal(t, float64(3), priceData.OtherRatios["n"])
}

func TestModelPriceHelperRatioRefreshesSelectedGroupPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedRatios := ratio_setting.ModelRatio2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(savedRatios))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"final-group-ratio-model":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"cheap":1,"paid":2}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("auto_group", "cheap")
	info := &relaycommon.RelayInfo{OriginModelName: "final-group-ratio-model", UserGroup: "ratio-test-user", UsingGroup: "cheap"}
	cheap, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"final-group-ratio-model":100}`))
	ctx.Set("auto_group", "paid")
	paid, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, cheap.QuotaToPreConsume*2, paid.QuotaToPreConsume)
	require.Equal(t, "paid", info.UsingGroup)
	require.Equal(t, float64(2), paid.GroupRatioInfo.GroupRatio)
}

func TestModelPriceHelperPerCallCarriesConfiguredPriceUnit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedUnits := ratio_setting.ModelPriceUnit2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceUnitByJSONString(savedUnits))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"video-unit-test":0.05}`))
	require.NoError(t, ratio_setting.UpdateModelPriceUnitByJSONString(`{"video-unit-test":"second"}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "video-unit-test",
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	priceData, err := ModelPriceHelperPerCall(ctx, info)

	require.NoError(t, err)
	require.Equal(t, types.ModelPriceUnitSecond, priceData.ModelPriceUnit)
}

func TestModelPriceHelperPerCallFreezesBasePriceAndPreservesTaskContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"final-group-fixed-task":0.01}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"cheap":1,"paid":2}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("auto_group", "cheap")
	info := &relaycommon.RelayInfo{OriginModelName: "final-group-fixed-task", UserGroup: "fixed-task-user", UsingGroup: "cheap"}
	cheap, err := ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	cheap.AddOtherRatio("seconds", 4)
	cheap.AddBillingMeta("resolution", "720p")
	info.PriceData = cheap

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"final-group-fixed-task":9}`))
	ctx.Set("auto_group", "paid")
	paid, err := ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	require.Equal(t, cheap.Quota*2, paid.Quota)
	require.Equal(t, float64(0.01), paid.ModelPrice)
	require.Equal(t, float64(4), paid.OtherRatios["seconds"])
	require.Equal(t, "720p", paid.BillingMeta["resolution"])
}

func TestModelPriceHelperPerCallRejectsSelectedGroupOverflow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	savedPrices := ratio_setting.ModelPrice2JSONString()
	savedGroups := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(savedGroups))
	})
	price := float64(common.MaxQuota-100) / 2 / common.QuotaPerUnit
	prices, err := common.Marshal(map[string]float64{"final-group-overflow-task": price})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(prices)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"cheap":1,"overflow":3}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("auto_group", "cheap")
	info := &relaycommon.RelayInfo{OriginModelName: "final-group-overflow-task", UserGroup: "overflow-user", UsingGroup: "cheap"}
	_, err = ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	ctx.Set("auto_group", "overflow")
	_, err = ModelPriceHelperPerCall(ctx, info)
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}
