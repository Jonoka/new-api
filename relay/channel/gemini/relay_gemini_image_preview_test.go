package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiImagePreviewConvertImageRequestUsesGenerateContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:   "gemini-3-pro-image-preview",
		Prompt:  "生成一张蓝色小猫图片",
		Size:    "16:9",
		Quality: "4K",
		N:       common.GetPointer(uint(1)),
	})
	require.NoError(t, err)
	require.False(t, info.IsStream)

	req, ok := converted.(dto.GeminiChatRequest)
	require.True(t, ok)
	require.Equal(t, []string{"TEXT", "IMAGE"}, req.GenerationConfig.ResponseModalities)
	require.JSONEq(t, `{"aspectRatio":"16:9","imageSize":"4K"}`, string(req.GenerationConfig.ImageConfig))
	require.Len(t, req.Contents, 1)
	require.Equal(t, "user", req.Contents[0].Role)
	require.Len(t, req.Contents[0].Parts, 1)
	require.Equal(t, "生成一张蓝色小猫图片", req.Contents[0].Parts[0].Text)
}

func TestGeminiImagePreviewConvertsPixelSizeToAspectRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		size string
		want string
	}{
		{name: "canvas three by four", size: "864x1152", want: "3:4"},
		{name: "existing landscape mapping", size: "1536x1024", want: "3:2"},
		{name: "legacy portrait mapping", size: "1024x1792", want: "9:16"},
		{name: "supported ratio passthrough", size: "4:3", want: "4:3"},
		{name: "unknown size defaults square", size: "123x456", want: "1:1"},
		{name: "unknown ratio defaults square", size: "7:5", want: "1:1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "gemini-3-pro-image-preview",
				},
			}

			converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
				Model:  "gemini-3-pro-image-preview",
				Prompt: "test",
				Size:   tt.size,
			})
			require.NoError(t, err)

			req, ok := converted.(dto.GeminiChatRequest)
			require.True(t, ok)
			expected, err := common.Marshal(map[string]string{
				"aspectRatio": tt.want,
				"imageSize":   "1K",
			})
			require.NoError(t, err)
			require.JSONEq(t, string(expected), string(req.GenerationConfig.ImageConfig))
		})
	}
}

func TestGeminiImagenConvertImageRequestKeepsLegacySizeMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "imagen-3.0-generate-002",
		},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "imagen-3.0-generate-002",
		Prompt: "test",
		Size:   "1536x1024",
	})
	require.NoError(t, err)

	req, ok := converted.(dto.GeminiImageRequest)
	require.True(t, ok)
	require.Equal(t, "3:2", req.Parameters.AspectRatio)
}

func TestGeminiImagePreviewHandlerReturnsOpenAIImageResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-pro-image-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}
	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{{
			Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{
				{Text: "ok"},
				{InlineData: &dto.GeminiInlineData{MimeType: "image/jpeg", Data: "abcd1234"}},
			}},
		}},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:     12,
			CandidatesTokenCount: 34,
			TotalTokenCount:      46,
			CandidatesTokensDetails: []dto.GeminiPromptTokensDetails{{
				Modality:   "IMAGE",
				TokenCount: 34,
			}},
		},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	usage, apiErr := GeminiImagePreviewHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 46, usage.TotalTokens)
	require.Equal(t, 34, usage.CompletionTokenDetails.ImageTokens)

	var imageResp dto.ImageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &imageResp))
	require.Len(t, imageResp.Data, 1)
	require.Equal(t, "abcd1234", imageResp.Data[0].B64Json)
}

func TestGeminiImagePreviewHandlerAcceptsMarkdownImageURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"}}
	payload := dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "![generated image 1](<https://images.example/one.png?x=1>) and ![generated image 2](https://images.example/two.png)"}}}}}}
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	usage, apiErr := GeminiImagePreviewHandler(c, info, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	var imageResp dto.ImageResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &imageResp))
	require.Len(t, imageResp.Data, 2)
	require.Equal(t, []string{"https://images.example/one.png?x=1", "https://images.example/two.png"}, []string{imageResp.Data[0].Url, imageResp.Data[1].Url})
}

func TestExtractGeminiMarkdownImageURLsRejectsInvalidDestinations(t *testing.T) {
	require.Equal(t, []string{"https://images.example/ok.png"}, extractGeminiMarkdownImageURLs("![ok](<https://images.example/ok.png>) ![bad](javascript:alert(1)) ![relative](/image.png) ![broken](<https://images.example/missing.png)"))
}

func TestGeminiImagePreviewHandlerReturnsNoImagesForInvalidMarkdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload := dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "![bad](data:image/png;base64,AAAA) plain text"}}}}}}
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	usage, apiErr := GeminiImagePreviewHandler(c, &relaycommon.RelayInfo{}, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, usage)
	require.EqualError(t, apiErr, "no images generated")
}
