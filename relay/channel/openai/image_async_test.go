package openai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertImageRequestForcesGPTImage2HighTierToAsyncTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "gpt-image-2",
		},
	}, dto.ImageRequest{Model: "gpt-image-2", Prompt: "test", Size: "3840x2160", Quality: "high"})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, true, got["async"])
	require.Equal(t, false, got["wait_for_result"])
}

func TestConvertImageRequestDefaultsGPTImage2LowTierToSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "gpt-image-2",
		},
	}, dto.ImageRequest{Model: "gpt-image-2", Prompt: "test", Size: "1024x1024", Quality: "low"})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, false, got["async"])
	_, exists := got["wait_for_result"]
	require.False(t, exists)
}

func TestConvertImageRequestPreservesExplicitGPTImage2Async(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	async := true
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "gpt-image-2",
		},
	}, dto.ImageRequest{Model: "gpt-image-2", Prompt: "test", Async: &async})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, true, got["async"])
}
