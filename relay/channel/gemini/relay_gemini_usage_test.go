package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiUsageTraceCacheFieldPresence(t *testing.T) {
	tests := []struct {
		name     string
		cache    *int
		present  bool
	}{
		{name: "non-zero", cache: common.GetPointer(24431), present: true},
		{name: "zero", cache: common.GetPointer(0), present: true},
		{name: "omitted", present: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]any{
				"usageMetadata": map[string]any{
					"promptTokenCount":     2826,
					"candidatesTokenCount": 532,
					"totalTokenCount":      3358,
				},
			}
			if tt.cache != nil {
				payload["usageMetadata"].(map[string]any)["cachedContentTokenCount"] = *tt.cache
			}
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			require.Equal(t, tt.present, geminiUsageTraceCacheFieldPresent(string(body)))
		})
	}
}

func TestGeminiUsageTraceGateAndFormat(t *testing.T) {
	t.Setenv(geminiUsageTraceChannelEnv, "")
	require.False(t, geminiUsageTraceEnabled(nil))
	require.False(t, geminiUsageTraceEnabled(&relaycommon.RelayInfo{}))
	require.False(t, geminiUsageTraceEnabled(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 100},
	}))
	t.Setenv(geminiUsageTraceChannelEnv, "not-a-channel")
	require.False(t, geminiUsageTraceEnabled(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 100},
	}))

	t.Setenv(geminiUsageTraceChannelEnv, "100")
	info := &relaycommon.RelayInfo{
		RequestId: "request-1",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 100},
	}
	metadata := dto.GeminiUsageMetadata{
		PromptTokenCount:        2826,
		CachedContentTokenCount: 24431,
		CandidatesTokenCount:    532,
		TotalTokenCount:         3358,
	}

	require.True(t, geminiUsageTraceEnabled(info))
	require.False(t, geminiUsageTraceEnabled(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 78},
	}))
	t.Setenv(geminiUsageTraceChannelEnv, "78")
	require.False(t, geminiUsageTraceEnabled(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 78},
	}))

	line := formatGeminiUsageTrace(info.RequestId, info.ChannelId, 1, metadata, true)
	require.Equal(t, "gemini_usage_trace request_id=request-1 channel_id=100 chunk=1 usage_present=true prompt_tokens=2826 cached_content_tokens=24431 candidates_tokens=532 total_tokens=3358", line)
	require.NotContains(t, line, "usageMetadata")
	require.NotContains(t, line, "{")
	require.NotContains(t, line, "}")
}

func TestTraceGeminiUsageLogsSanitizedMetadataOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RequestId:   "request-1",
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 100},
	}

	t.Setenv(geminiUsageTraceChannelEnv, "100")

	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultWriter
	gin.DefaultWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	traceGeminiUsage(c, info, `{"usageMetadata":{"cachedContentTokenCount":0},"candidates":[{"content":{"parts":[{"text":"do-not-log"}]}}]}`, 7, dto.GeminiUsageMetadata{
		PromptTokenCount:        2826,
		CachedContentTokenCount: 0,
		CandidatesTokenCount:    532,
		TotalTokenCount:         3358,
	})

	line := logs.String()
	require.Contains(t, line, "gemini_usage_trace request_id=request-1 channel_id=100 chunk=7")
	require.Contains(t, line, "usage_present=true prompt_tokens=2826 cached_content_tokens=0 candidates_tokens=532 total_tokens=3358")
	require.NotContains(t, line, "usageMetadata")
	require.NotContains(t, line, "do-not-log")
}

func TestGeminiStreamHandlerTracesEachUsageChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		RequestId:   "stream-request-1",
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 100, UpstreamModelName: "gemini-test"},
	}

	t.Setenv(geminiUsageTraceChannelEnv, "100")
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })

	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultWriter
	gin.DefaultWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	streamBody := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"one\"}]}}],\"usageMetadata\":{\"promptTokenCount\":10,\"cachedContentTokenCount\":24431,\"candidatesTokenCount\":1,\"totalTokenCount\":11}}\n" +
		"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"two\"}]}}],\"usageMetadata\":{\"promptTokenCount\":10,\"cachedContentTokenCount\":0,\"candidatesTokenCount\":2,\"totalTokenCount\":12}}\n" +
		"data: {\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":10,\"candidatesTokenCount\":3,\"totalTokenCount\":13}}\n" +
		"data: [DONE]\n"

	resp := &http.Response{Body: io.NopCloser(bytes.NewBufferString(streamBody))}
	usage, relayErr := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool { return true })
	require.Nil(t, relayErr)
	require.Equal(t, 13, usage.TotalTokens)

	lines := strings.Split(logs.String(), "\n")
	traceLines := make([]string, 0, 3)
	for _, line := range lines {
		if strings.Contains(line, "gemini_usage_trace ") {
			traceLines = append(traceLines, line)
		}
	}
	require.Len(t, traceLines, 3)
	require.Contains(t, traceLines[0], "chunk=1 usage_present=true cached_content_tokens=24431")
	require.Contains(t, traceLines[1], "chunk=2 usage_present=true cached_content_tokens=0")
	require.Contains(t, traceLines[2], "chunk=3 usage_present=false cached_content_tokens=0")
	require.NotContains(t, logs.String(), "one")
	require.NotContains(t, logs.String(), "two")
}

func TestGeminiChatHandlerCompletionTokensExcludeToolUsePromptTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiStreamHandlerCompletionTokensExcludeToolUsePromptTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiTextGenerationHandlerPromptTokensIncludeToolUsePromptTokens(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        151,
			ToolUsePromptTokenCount: 18329,
			CandidatesTokenCount:    1089,
			ThoughtsTokenCount:      1120,
			TotalTokenCount:         20689,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 18480, usage.PromptTokens)
	require.Equal(t, 2209, usage.CompletionTokens)
	require.Equal(t, 20689, usage.TotalTokens)
	require.Equal(t, 1120, usage.CompletionTokenDetails.ReasoningTokens)
}

func TestGeminiChatHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatGemini,
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiStreamHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() {
		constant.StreamingTimeout = oldStreamingTimeout
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	chunk := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "partial"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	chunkData, err := common.Marshal(chunk)
	require.NoError(t, err)

	streamBody := []byte("data: " + string(chunkData) + "\n" + "data: [DONE]\n")
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(streamBody)),
	}

	usage, newAPIError := geminiStreamHandler(c, info, resp, func(_ string, _ *dto.GeminiChatResponse) bool {
		return true
	})
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}

func TestGeminiTextGenerationHandlerUsesEstimatedPromptTokensWhenUsagePromptMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3-flash-preview:generateContent", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gemini-3-flash-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-flash-preview",
		},
	}
	info.SetEstimatePromptTokens(20)

	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "ok"},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:        0,
			ToolUsePromptTokenCount: 0,
			CandidatesTokenCount:    90,
			ThoughtsTokenCount:      10,
			TotalTokenCount:         110,
		},
	}

	body, err := common.Marshal(payload)
	require.NoError(t, err)

	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}

	usage, newAPIError := GeminiTextGenerationHandler(c, info, resp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 100, usage.CompletionTokens)
	require.Equal(t, 110, usage.TotalTokens)
}
