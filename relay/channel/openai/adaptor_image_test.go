package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func imageGenerationInfo(enabled bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{
			GeminiImageParamCompatEnabled: enabled,
		}},
	}
}

func convertImageRequestJSON(t *testing.T, request dto.ImageRequest) map[string]json.RawMessage {
	t.Helper()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := (&Adaptor{}).ConvertImageRequest(ctx, imageGenerationInfo(true), request)
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)
	fields := make(map[string]json.RawMessage)
	require.NoError(t, common.Unmarshal(body, &fields))
	return fields
}

func requireJSONString(t *testing.T, fields map[string]json.RawMessage, key, want string) {
	t.Helper()
	var got string
	require.NoError(t, common.Unmarshal(fields[key], &got))
	require.Equal(t, want, got)
}

func TestConvertImageGenerationRequestPreservesGPTImage2Size(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "生成一张方形图片",
		Size:   "1024x1024",
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(ctx, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}, request)
	require.NoError(t, err)

	body, err := common.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-image-2","prompt":"生成一张方形图片","size":"1024x1024"}`, string(body))
}

func TestGeminiImageParamCompatDisabledPreservesRequest(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := dto.ImageRequest{Model: "model", Prompt: "prompt", Size: "16:9", Quality: "4K"}

	converted, err := (&Adaptor{}).ConvertImageRequest(ctx, imageGenerationInfo(false), request)
	require.NoError(t, err)
	require.Equal(t, request, converted)
}

func TestGeminiImageParamCompatOnlyAppliesToGenerations(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	request := dto.ImageRequest{Model: "model", Prompt: "edit", Size: "16:9", Quality: "4K"}
	info := imageGenerationInfo(true)
	info.RelayMode = relayconstant.RelayModeImagesEdits

	converted, err := (&Adaptor{}).ConvertImageRequest(ctx, info, request)
	require.NoError(t, err)
	require.Equal(t, request, converted)
}

func TestNormalizeGeminiImageRequestRatiosAndQualityAliases(t *testing.T) {
	ratios := []string{"2:3", "1:1", "16:9", "4:3", "4:5", "9:16", "21:9"}
	aliases := map[string]string{
		"4K": "4k", "ultra": "4k", "ultra-high": "4k", "超清": "4k",
		"2K": "2k", "hd": "2k", "high": "2k",
		"1K": "1k", "standard": "1k", "medium": "1k", "low": "1k", "auto": "1k", "": "1k", "unknown": "1k",
	}
	for _, ratio := range ratios {
		for quality, tier := range aliases {
			t.Run(ratio+"/"+quality, func(t *testing.T) {
				fields := convertImageRequestJSON(t, dto.ImageRequest{Model: "model", Prompt: "prompt", Size: ratio, Quality: quality})
				requireJSONString(t, fields, "size", tier)
				requireJSONString(t, fields, "aspect_ratio", ratio)
				require.NotContains(t, fields, "quality")
			})
		}
	}
}

func TestNormalizeGeminiImageRequestSizePrecedence(t *testing.T) {
	tests := []struct {
		name    string
		size    string
		quality string
		want    string
	}{
		{name: "tier", size: " 4K ", quality: "low", want: "4k"},
		{name: "official dimensions", size: "5504x3072", quality: "low", want: "5504x3072"},
		{name: "nonstandard dimensions", size: "1024X576", quality: "4K", want: "1024x576"},
		{name: "empty size uses quality", quality: "high", want: "2k"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := convertImageRequestJSON(t, dto.ImageRequest{
				Model: "model", Prompt: "prompt", Size: test.size, Quality: test.quality,
				ResponseFormat: "b64_json", Background: json.RawMessage(`"transparent"`),
			})
			requireJSONString(t, fields, "size", test.want)
			require.NotContains(t, fields, "aspect_ratio")
			require.NotContains(t, fields, "quality")
			requireJSONString(t, fields, "response_format", "b64_json")
			requireJSONString(t, fields, "background", "transparent")
		})
	}
}

func TestNormalizeGeminiImageRequestRejectsInvalidSizes(t *testing.T) {
	invalid := []string{"3:2", "1024", "1024x", "x576", "1024xx576", "0x576", "-1x576", "1x-2", "999999999999999999999999x576", "1024×576"}
	for _, size := range invalid {
		t.Run(size, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			_, err := (&Adaptor{}).ConvertImageRequest(ctx, imageGenerationInfo(true), dto.ImageRequest{Model: "model", Prompt: "prompt", Size: size})
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
			require.True(t, types.IsSkipRetryError(apiErr))
		})
	}
}
