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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertImageRequestDefaultsGPTImage2ToSync(t *testing.T) {
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
	}, dto.ImageRequest{Model: "gpt-image-2", Prompt: "test", Size: "768x1360", Quality: "low"})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, false, got["async"])
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
	assert.Equal(t, true, got["async"])
}

func TestConvertImageRequestDoesNotAddAsyncForOtherModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "dall-e-3",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "dall-e-3",
		},
	}, dto.ImageRequest{Model: "dall-e-3", Prompt: "test"})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	_, exists := got["async"]
	assert.False(t, exists)
}
