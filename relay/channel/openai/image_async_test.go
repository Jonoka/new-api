package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestOpenaiHandlerWithUsageConvertsURLToB64WhenRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer imageServer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"created":1,"data":[{"url":"` + imageServer.URL + `/img.png","width":720,"height":1280}],"usage":{"total_tokens":1}}`)),
	}

	usage, apiErr := OpenaiHandlerWithUsage(c, &relaycommon.RelayInfo{
		Request:     &dto.ImageRequest{ResponseFormat: "b64_json"},
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
	}, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	data := got["data"].([]any)
	item := data[0].(map[string]any)
	assert.Equal(t, "aW1hZ2UtYnl0ZXM=", item["b64_json"])
	_, hasURL := item["url"]
	assert.False(t, hasURL)
}
