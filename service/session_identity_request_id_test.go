package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaudeToOpenAIRequestDoesNotUseClientRequestIDAsPromptCacheKey(t *testing.T) {
	t.Parallel()

	request := dto.ClaudeRequest{
		Model: "gpt-5",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	for _, requestID := range []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	} {
		info := &relaycommon.RelayInfo{
			RequestHeaders: map[string]string{
				"X-Client-Request-Id": requestID,
			},
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:       constant.ChannelTypeOpenAI,
				UpstreamModelName: "gpt-5",
			},
		}

		converted, err := ClaudeToOpenAIRequest(request, info)
		require.NoError(t, err)
		require.NotNil(t, converted)
		require.Empty(t, converted.PromptCacheKey)
	}
}

func TestClaudeToOpenAIRequestUsesStableSessionHeaders(t *testing.T) {
	t.Parallel()

	request := dto.ClaudeRequest{
		Model: "gpt-5",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	for _, headerName := range []string{
		"X-Claude-Code-Session-Id",
		"X-Codex-Session-Id",
		"Conversation_id",
		"X-Session-Id",
		"Session_id",
	} {
		t.Run(headerName, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RequestHeaders: map[string]string{
					headerName: "stable-session",
				},
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       constant.ChannelTypeOpenAI,
					UpstreamModelName: "gpt-5",
				},
			}

			converted, err := ClaudeToOpenAIRequest(request, info)
			require.NoError(t, err)
			require.Equal(t, "stable-session", converted.PromptCacheKey)
		})
	}
}

func TestPromptCacheKeyAffinityIgnoresClientRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	source := operation_setting.ChannelAffinityKeySource{
		Type: "gjson",
		Path: "prompt_cache_key",
	}

	for _, requestID := range []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(
			http.MethodPost,
			"/v1/messages",
			strings.NewReader(`{"model":"gpt-5","messages":[{"role":"user","content":"hello"}]}`),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Request.Header.Set("X-Client-Request-Id", requestID)

		require.Empty(t, extractChannelAffinityValue(ctx, source))
	}
}

func TestExplicitPromptCacheKeyAffinityWinsOverSessionHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		strings.NewReader(`{"model":"gpt-5","prompt_cache_key":"explicit-cache-key"}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Client-Request-Id", "request-only-id")
	ctx.Request.Header.Set("X-Session-Id", "stable-session-header")

	value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{
		Type: "gjson",
		Path: "prompt_cache_key",
	})

	require.Equal(t, "explicit-cache-key", value)
}

func TestNullPromptCacheKeyAffinityFallsBackToStableSessionHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		strings.NewReader(`{"model":"gpt-5","prompt_cache_key":null}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Session-Id", "stable-session-header")

	value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{
		Type: "gjson",
		Path: "prompt_cache_key",
	})

	require.Equal(t, "stable-session-header", value)
}

func TestNullPromptCacheKeyAffinityWithoutStableSessionIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		strings.NewReader(`{"model":"gpt-5","prompt_cache_key":null}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("X-Client-Request-Id", "request-only-id")

	value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{
		Type: "gjson",
		Path: "prompt_cache_key",
	})

	require.Empty(t, value)
}
