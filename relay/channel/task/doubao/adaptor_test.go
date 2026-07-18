package doubao

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newBillingContext(req relaycommon.TaskSubmitReq) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("task_request", req)
	return c
}

func TestEstimateBillingPerSecondUsesDurationAndResolutionVariantPrice(t *testing.T) {
	savedPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(savedPrices))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(
		`{"seedance-test":0.06,"seedance-test-1080p":0.09}`,
	))

	c := newBillingContext(relaycommon.TaskSubmitReq{
		Duration:   10,
		Resolution: "FHD",
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "seedance-test",
		PriceData: types.PriceData{
			UsePrice:       true,
			ModelPrice:     0.06,
			ModelPriceUnit: types.ModelPriceUnitSecond,
		},
	}

	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	require.Equal(t, float64(10), ratios["seconds"])
	require.InDelta(t, 1.5, ratios["resolution-1080p"], 1e-12)
}

func TestEstimateBillingPerSecondDoesNotDoubleChargeEncodedResolutionModel(t *testing.T) {
	c := newBillingContext(relaycommon.TaskSubmitReq{
		Duration:   6,
		Resolution: "1080p",
	})
	info := &relaycommon.RelayInfo{
		OriginModelName: "seedance-test-1080p",
		PriceData: types.PriceData{
			UsePrice:       true,
			ModelPrice:     0.09,
			ModelPriceUnit: types.ModelPriceUnitSecond,
		},
	}

	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	require.Equal(t, float64(6), ratios["seconds"])
	_, hasResolutionRatio := ratios["resolution-1080p"]
	require.False(t, hasResolutionRatio)
}

func TestEstimateBillingTokenModeDoesNotMultiplyDuration(t *testing.T) {
	c := newBillingContext(relaycommon.TaskSubmitReq{Duration: 12, Resolution: "1080p"})
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-1-5-pro-251215",
		PriceData:       types.PriceData{UsePrice: false},
	}

	ratios := (&TaskAdaptor{}).EstimateBilling(c, info)
	_, hasSeconds := ratios["seconds"]
	require.False(t, hasSeconds)
}

func TestConvertToRequestPayloadAcceptsTopLevelDurationAndResolution(t *testing.T) {
	payload, err := (&TaskAdaptor{}).convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:      "doubao-seedance-1-5-pro-251215",
		Prompt:     "测试视频",
		Duration:   7,
		Resolution: "1920x1080",
	})
	require.NoError(t, err)
	require.NotNil(t, payload.Duration)
	require.Equal(t, 7, int(*payload.Duration))
	require.Equal(t, "1080p", payload.Resolution)
}

func TestParseTaskResultCarriesActualDurationForSettlement(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id":"task_upstream",
		"status":"succeeded",
		"duration":9,
		"content":{"video_url":"https://example.com/video.mp4"}
	}`))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, result.Status)
	require.Equal(t, 9, result.DurationSeconds)
}
