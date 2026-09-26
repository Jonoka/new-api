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
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

	ctx, info = newInfo("tiered-overflow-model", "default")
	_, err = ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	var clamp *common.QuotaClamp
	require.ErrorAs(t, err, &clamp)
	require.Equal(t, "QuotaRound", clamp.Op)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
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

func TestRefreshSelectedGroupPricingPreservesFrozenFacts(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	oldFree := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFree
	})
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":          `{"frozen-group-test":"tiered_expr"}`,
		"billing_setting.billing_expr":          `{"frozen-group-test":"param(\"stream\") == true ? tier(\"stream\", p * 3 + c * 10) : tier(\"other\", p * 9)"}`,
		"group_ratio_setting.group_ratio":       `{"initial":0.22,"final":0.7,"free":0}`,
		"group_ratio_setting.group_group_ratio": `{"customer":{"special":0.4,"zero-special":0}}`,
	}))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		OriginModelName: "frozen-group-test", UsingGroup: "initial", UserGroup: "customer",
		BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"stream":true}`)},
	}
	_, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{MaxTokens: 100})
	require.NoError(t, err)
	frozen := *info.TieredBillingSnapshot
	requestInput := info.BillingRequestInput
	info.FinalPreConsumedQuota = 321 // deliberately not equal to the estimate
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"frozen-group-test":"ratio"}`,
		"billing_setting.billing_expr": `{"frozen-group-test":"p * 9999"}`,
	}))
	for _, tc := range []struct {
		group   string
		ratio   float64
		special bool
	}{
		{"final", .7, false}, {"final", .7, false}, {"free", 0, false},
		{"special", .4, true}, {"zero-special", 0, true}, {"initial", .22, false},
	} {
		ctx.Set("auto_group", tc.group)
		require.NoError(t, RefreshSelectedGroupPricing(ctx, info))
		snap := *info.TieredBillingSnapshot
		require.Equal(t, tc.group, info.UsingGroup)
		require.Equal(t, tc.ratio, snap.GroupRatio)
		require.Equal(t, tc.ratio, info.PriceData.GroupRatioInfo.GroupRatio)
		require.Equal(t, tc.special, info.PriceData.GroupRatioInfo.HasSpecialRatio)
		if tc.special {
			require.Equal(t, tc.ratio, info.PriceData.GroupRatioInfo.GroupSpecialRatio)
		}
		require.Equal(t, tc.ratio == 0, info.PriceData.FreeModel)
		require.Equal(t, billingexpr.QuotaRound(frozen.EstimatedQuotaBeforeGroup*tc.ratio), snap.EstimatedQuotaAfterGroup)
		require.Equal(t, snap.EstimatedQuotaAfterGroup, info.PriceData.QuotaToPreConsume)
		require.Equal(t, 321, info.FinalPreConsumedQuota)
		require.Same(t, requestInput, info.BillingRequestInput)
		snap.GroupRatio = frozen.GroupRatio
		snap.EstimatedQuotaAfterGroup = frozen.EstimatedQuotaAfterGroup
		require.Equal(t, frozen, snap, "all non-group snapshot facts stay frozen")
	}
}
