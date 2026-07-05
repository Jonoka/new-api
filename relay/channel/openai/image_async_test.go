package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
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
	assert.NotContains(t, got, "wait_for_result")
}

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
	assert.Equal(t, true, got["async"])
	assert.Equal(t, false, got["wait_for_result"])
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

func TestConvertImageRequestDefaultsGPTImage2EditToSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(`{"model":"gpt-image-2","prompt":"test"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "gpt-image-2",
		},
	}, dto.ImageRequest{Model: "gpt-image-2", Prompt: "test"})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, false, got["async"])
}

func TestConvertImageRequestMapsChannel23OpenAIQualityToGPT2APITier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "nano-banana-v2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         23,
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "nano-banana-v2",
		},
	}, dto.ImageRequest{Model: "nano-banana-v2", Prompt: "test", Size: "3840x2160", Quality: "high"})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, "4k", got["quality"])
}

func TestConvertImageEditMultipartMapsChannel23QualityAndAsync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("model", "nano-banana-v2"))
	require.NoError(t, writer.WriteField("prompt", "test"))
	require.NoError(t, writer.WriteField("quality", "medium"))
	require.NoError(t, writer.WriteField("async", "true"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, _ = part.Write([]byte("png"))
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &buf)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, c.Request.ParseMultipartForm(32<<20))
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		RequestURLPath:  "/v1/images/edits",
		OriginModelName: "nano-banana-v2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         23,
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "nano-banana-v2",
		},
	}, dto.ImageRequest{Model: "nano-banana-v2", Prompt: "test", Quality: "medium", Async: func() *bool { v := true; return &v }()})

	require.NoError(t, err)
	body, ok := converted.(*bytes.Buffer)
	require.True(t, ok)
	multipartBody := body.String()
	assert.Contains(t, multipartBody, "2k")
	assert.Contains(t, multipartBody, "true")
	assert.NotContains(t, multipartBody, "medium")
}

func TestConvertImageEditMultipartMapsChannel23HighTo4K(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("model", "nano-banana-pro"))
	require.NoError(t, writer.WriteField("prompt", "test"))
	require.NoError(t, writer.WriteField("quality", "high"))
	require.NoError(t, writer.WriteField("size", "5504x3072"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, _ = part.Write([]byte("png"))
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &buf)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, c.Request.ParseMultipartForm(32<<20))
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		RequestURLPath:  "/v1/images/edits",
		OriginModelName: "nano-banana-pro",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         23,
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "nano-banana-pro",
		},
	}, dto.ImageRequest{Model: "nano-banana-pro", Prompt: "test", Size: "5504x3072", Quality: "high"})

	require.NoError(t, err)
	body, ok := converted.(*bytes.Buffer)
	require.True(t, ok)
	multipartBody := body.String()
	assert.Contains(t, multipartBody, "4k")
	assert.Contains(t, multipartBody, "5504x3072")
	assert.NotContains(t, multipartBody, "high")
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

func TestChannel25ImageEditRoutesToGenerations(t *testing.T) {
	adaptor := &Adaptor{}
	url, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeImagesEdits,
		RequestURLPath: "/v1/images/edits",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:      25,
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: "https://img-api.xn--1ys141f4ks.com",
			ApiType:        constant.APITypeOpenAI,
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "https://img-api.xn--1ys141f4ks.com/v1/images/generations", url)
}

func TestConvertChannel25GeminiImageRequestAddsGoogleImageConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gemini_3.1_flash_image_preview_4K","prompt":"test"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gemini_3.1_flash_image_preview_4K",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         25,
			ChannelBaseUrl:    "https://img-api.xn--1ys141f4ks.com",
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "gemini_3.1_flash_image_preview_4K",
		},
	}, dto.ImageRequest{Model: "gemini_3.1_flash_image_preview_4K", Prompt: "test", Size: "3840x2160", Quality: "high"})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, "gemini_3.1_flash_image_preview_4K", got["model"])
	assert.Equal(t, "b64_json", got["response_format"])
	extraBody := got["extra_body"].(map[string]any)
	google := extraBody["google"].(map[string]any)
	imageConfig := google["image_config"].(map[string]any)
	assert.Equal(t, "16:9", imageConfig["aspect_ratio"])
	assert.Equal(t, "4K", imageConfig["image_size"])
	assert.Equal(t, "3840x2160", imageConfig["size"])
}

func TestConvertChannel25ImageEditMultipartUsesDataURLImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "edit test"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, _ = part.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &buf)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, c.Request.ParseMultipartForm(32<<20))
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		RequestURLPath:  "/v1/images/edits",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         25,
			ChannelBaseUrl:    "https://img-api.xn--1ys141f4ks.com",
			ApiType:           constant.APITypeOpenAI,
			UpstreamModelName: "gpt-image-2",
		},
	}, dto.ImageRequest{Model: "gpt-image-2", Prompt: "edit test"})

	require.NoError(t, err)
	body, err := json.Marshal(converted)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, "gpt-image-2", got["model"])
	image, _ := got["image"].(string)
	assert.True(t, strings.HasPrefix(image, "data:image/png;base64,"))
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
		Body:       io.NopCloser(strings.NewReader(`{"created":1,"data":[{"url":"` + imageServer.URL + `/img.png","width":720,"height":1280}],"usage":{"total_tokens":1,"total_cost":15,"total_points":0.15}}`)),
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
	_, hasUsage := got["usage"]
	assert.False(t, hasUsage)
}
