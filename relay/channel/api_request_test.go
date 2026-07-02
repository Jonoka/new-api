package channel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_ClaudeCodeFingerprintSkipsPassiveProtectedHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("User-Agent", "CherryStudio/1.0")
	ctx.Request.Header.Set("Anthropic-Beta", "client-beta")
	ctx.Request.Header.Set("X-App", "browser")
	ctx.Request.Header.Set("X-Stainless-Lang", "python")
	ctx.Request.Header.Set("X-Client-Request-Id", "client-request-id")
	ctx.Request.Header.Set("X-Claude-Code-Session-Id", "client-session-id")
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])
	require.NotContains(t, headers, "user-agent")
	require.NotContains(t, headers, "anthropic-beta")
	require.NotContains(t, headers, "x-app")
	require.NotContains(t, headers, "x-stainless-lang")
	require.NotContains(t, headers, "x-client-request-id")
	require.NotContains(t, headers, "x-claude-code-session-id")
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestProcessHeaderOverride_ClaudeCodeFingerprintSkipsPassHeadersProtectedHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("User-Agent", "CherryStudio/1.0")
	ctx.Request.Header.Set("Anthropic-Beta", "client-beta")
	ctx.Request.Header.Set("X-App", "browser")
	ctx.Request.Header.Set("X-Stainless-Lang", "python")
	ctx.Request.Header.Set("X-Client-Request-Id", "client-request-id")
	ctx.Request.Header.Set("X-Claude-Code-Session-Id", "client-session-id")
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"User-Agent":               "CherryStudio/1.0",
			"Anthropic-Beta":           "client-beta",
			"X-App":                    "browser",
			"X-Stainless-Lang":         "python",
			"X-Client-Request-Id":      "client-request-id",
			"X-Claude-Code-Session-Id": "client-session-id",
			"X-Trace-Id":               "trace-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"User-Agent", "Anthropic-Beta", "X-App", "X-Stainless-Lang", "X-Client-Request-Id", "X-Claude-Code-Session-Id", "X-Trace-Id"},
					},
				},
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"claude-sonnet-4-6"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "trace-123", info.RuntimeHeadersOverride["x-trace-id"])
	require.NotContains(t, info.RuntimeHeadersOverride, "user-agent")
	require.NotContains(t, info.RuntimeHeadersOverride, "anthropic-beta")
	require.NotContains(t, info.RuntimeHeadersOverride, "x-app")
	require.NotContains(t, info.RuntimeHeadersOverride, "x-stainless-lang")
	require.NotContains(t, info.RuntimeHeadersOverride, "x-client-request-id")
	require.NotContains(t, info.RuntimeHeadersOverride, "x-claude-code-session-id")

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])
	require.NotContains(t, headers, "user-agent")
	require.NotContains(t, headers, "anthropic-beta")
	require.NotContains(t, headers, "x-app")
	require.NotContains(t, headers, "x-stainless-lang")
	require.NotContains(t, headers, "x-client-request-id")
	require.NotContains(t, headers, "x-claude-code-session-id")
}

func TestProcessHeaderOverride_ClaudeCodeFingerprintSkipsExplicitProtectedHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("User-Agent", "CherryStudio/1.0")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
			HeadersOverride: map[string]any{
				"User-Agent":               "custom-agent",
				"Anthropic-Beta":           "custom-beta",
				"X-App":                    "custom-app",
				"X-Stainless-Lang":         "custom-lang",
				"X-Client-Request-Id":      "custom-request-id",
				"X-Claude-Code-Session-Id": "custom-session-id",
				"X-Custom-Header":          "custom-value",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.NotContains(t, headers, "user-agent")
	require.NotContains(t, headers, "anthropic-beta")
	require.NotContains(t, headers, "x-app")
	require.NotContains(t, headers, "x-stainless-lang")
	require.NotContains(t, headers, "x-client-request-id")
	require.NotContains(t, headers, "x-claude-code-session-id")
	require.Equal(t, "custom-value", headers["x-custom-header"])
}

func TestProcessHeaderOverride_RealClaudeCodeSkipsProtectedHeadersWithoutFingerprintSetting(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("User-Agent", "claude-cli/2.1.156 (Claude Code)")
	ctx.Request.Header.Set("Anthropic-Beta", "client-beta")
	ctx.Request.Header.Set("X-App", "claude-code")
	ctx.Request.Header.Set("X-Stainless-Lang", "js")
	ctx.Request.Header.Set("X-Stainless-Package-Version", "0.72.0")
	ctx.Request.Header.Set("X-Client-Request-Id", "client-request-id")
	ctx.Request.Header.Set("X-Claude-Code-Session-Id", "client-session-id")
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RelayFormat:   types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
			HeadersOverride: map[string]any{
				"*":                           "",
				"User-Agent":                  "custom-agent",
				"Anthropic-Beta":              "custom-beta",
				"X-App":                       "custom-app",
				"X-Stainless-Lang":            "custom-lang",
				"X-Stainless-Package-Version": "custom-package",
				"X-Client-Request-Id":         "custom-request-id",
				"X-Claude-Code-Session-Id":    "custom-session-id",
				"X-Custom-Header":             "custom-value",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])
	require.Equal(t, "custom-value", headers["x-custom-header"])
	require.NotContains(t, headers, "user-agent")
	require.NotContains(t, headers, "anthropic-beta")
	require.NotContains(t, headers, "x-app")
	require.NotContains(t, headers, "x-stainless-lang")
	require.NotContains(t, headers, "x-stainless-package-version")
	require.NotContains(t, headers, "x-client-request-id")
	require.NotContains(t, headers, "x-claude-code-session-id")
}

func TestApplyHeaderOverrideKeepsUserHeadersHighestPriority(t *testing.T) {
	t.Parallel()

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	upstreamReq.Header.Set("anthropic-beta", "claude-code-20250219")
	upstreamReq.Header.Set("User-Agent", "claude-cli/2.1.114 (external, sdk-cli)")

	applyHeaderOverrideToRequest(upstreamReq, map[string]string{
		"anthropic-beta": "custom-beta",
		"user-agent":     "custom-agent",
	})

	require.Equal(t, "custom-beta", upstreamReq.Header.Get("anthropic-beta"))
	require.Equal(t, "custom-agent", upstreamReq.Header.Get("User-Agent"))
}

func TestEnforceFinalStreamHeadersKeepsCodexResponsesAcceptSSE(t *testing.T) {
	t.Parallel()

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	upstreamReq.Header.Set("Accept", "application/json")

	info := &relaycommon.RelayInfo{
		IsStream:  true,
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeCodex,
		},
	}

	enforceFinalStreamHeaders(upstreamReq, info)

	require.Equal(t, "text/event-stream", upstreamReq.Header.Get("Accept"))
}

func TestMergeOpenAISessionBridgeOverrideUsesPromptCacheKeyFromResponsesBody(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	relaycommon.MergeOpenAISessionBridgeOverride(info, []byte(`{"model":"gpt-5","prompt_cache_key":"cache-key-123"}`))

	require.True(t, info.UseRuntimeHeadersOverride)
	sessionID, _ := info.RuntimeHeadersOverride["session_id"].(string)
	require.NotEmpty(t, sessionID)
	require.Len(t, strings.Split(sessionID, "-"), 5)
}

func TestMergeOpenAISessionBridgeOverrideUsesClaudeCodeSessionHeader(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAI,
		},
		RequestHeaders: map[string]string{
			"X-Claude-Code-Session-Id": "cc-session-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	relaycommon.MergeOpenAISessionBridgeOverride(info, []byte(`{"model":"gpt-5"}`))

	require.True(t, info.UseRuntimeHeadersOverride)
	sessionID, _ := info.RuntimeHeadersOverride["session_id"].(string)
	require.NotEmpty(t, sessionID)
	require.Len(t, strings.Split(sessionID, "-"), 5)
}

func TestMergeOpenAISessionBridgeOverrideIgnoresClientRequestIDSeed(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAI,
		},
		RequestHeaders: map[string]string{
			"X-Client-Request-Id": "req-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	relaycommon.MergeOpenAISessionBridgeOverride(info, []byte(`{"model":"gpt-5"}`))

	require.False(t, info.UseRuntimeHeadersOverride)
	require.Empty(t, info.RuntimeHeadersOverride)
}

func TestMergeOpenAISessionBridgeOverrideSkipsNilSessionSeed(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	relaycommon.MergeOpenAISessionBridgeOverride(info, []byte(`{"model":"gpt-5"}`))

	require.False(t, info.UseRuntimeHeadersOverride)
	require.Empty(t, info.RuntimeHeadersOverride)
}

func TestMergeOpenAISessionBridgeOverrideKeepsExplicitSessionID(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		RequestHeaders: map[string]string{
			"X-Claude-Code-Session-Id": "cc-session-123",
		},
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"session_id": "explicit-session",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	relaycommon.MergeOpenAISessionBridgeOverride(info, []byte(`{"model":"gpt-5","prompt_cache_key":"cache-key-123"}`))

	require.True(t, info.UseRuntimeHeadersOverride)
	require.NotEqual(t, "cc-session-123", info.RuntimeHeadersOverride["session_id"])
	require.NotEqual(t, "cache-key-123", info.RuntimeHeadersOverride["session_id"])
}

func TestMergeOpenAISessionBridgeOverrideStableForSameSeed(t *testing.T) {
	t.Parallel()

	infoA := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:   constant.APITypeOpenAI,
			ChannelId: 1001,
		},
	}
	infoB := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:   constant.APITypeOpenAI,
			ChannelId: 1001,
		},
	}

	relaycommon.MergeOpenAISessionBridgeOverride(infoA, []byte(`{"model":"gpt-5","prompt_cache_key":"cache-key-123"}`))
	relaycommon.MergeOpenAISessionBridgeOverride(infoB, []byte(`{"model":"gpt-5","prompt_cache_key":"cache-key-123"}`))

	require.Equal(t, infoA.RuntimeHeadersOverride["session_id"], infoB.RuntimeHeadersOverride["session_id"])
}

func TestMergeOpenAISessionBridgeOverrideStableAcrossMultiKeyIndexWithinChannel(t *testing.T) {
	t.Parallel()

	infoA := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:              constant.APITypeOpenAI,
			ChannelId:            2001,
			ChannelMultiKeyIndex: 1,
		},
	}
	infoB := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:              constant.APITypeOpenAI,
			ChannelId:            2001,
			ChannelMultiKeyIndex: 2,
		},
	}

	relaycommon.MergeOpenAISessionBridgeOverride(infoA, []byte(`{"model":"gpt-5","prompt_cache_key":"cache-key-123"}`))
	relaycommon.MergeOpenAISessionBridgeOverride(infoB, []byte(`{"model":"gpt-5","prompt_cache_key":"cache-key-123"}`))

	require.Equal(t, infoA.RuntimeHeadersOverride["session_id"], infoB.RuntimeHeadersOverride["session_id"])
}

func TestMergeOpenAISessionBridgeOverrideDiffersAcrossChannelID(t *testing.T) {
	t.Parallel()

	infoA := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:   constant.APITypeOpenAI,
			ChannelId: 3001,
		},
	}
	infoB := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAIResponses,
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:   constant.APITypeOpenAI,
			ChannelId: 3002,
		},
	}

	relaycommon.MergeOpenAISessionBridgeOverride(infoA, []byte(`{"model":"gpt-5","prompt_cache_key":"cache-key-123"}`))
	relaycommon.MergeOpenAISessionBridgeOverride(infoB, []byte(`{"model":"gpt-5","prompt_cache_key":"cache-key-123"}`))

	require.NotEqual(t, infoA.RuntimeHeadersOverride["session_id"], infoB.RuntimeHeadersOverride["session_id"])
}

func TestShouldUseClaudeCodeTransportFingerprint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info *relaycommon.RelayInfo
		want bool
	}{
		{
			name: "nil info",
			info: nil,
			want: false,
		},
		{
			name: "non anthropic keeps normal transport",
			info: &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ApiType: constant.APITypeOpenAI,
				ChannelOtherSettings: dto.ChannelOtherSettings{
					ClaudeCodeTransportFingerprintEnabled: true,
				},
			}},
			want: false,
		},
		{
			name: "anthropic without switch keeps normal transport",
			info: &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ApiType: constant.APITypeAnthropic,
			}},
			want: false,
		},
		{
			name: "anthropic with switch uses claude code transport",
			info: &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ApiType: constant.APITypeAnthropic,
				ChannelOtherSettings: dto.ChannelOtherSettings{
					ClaudeCodeTransportFingerprintEnabled: true,
				},
			}},
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, shouldUseClaudeCodeTransportFingerprint(tt.info))
		})
	}
}

func TestShouldUseClaudeCodeTransportForRealClaudeCodeRequest(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("User-Agent", "claude-cli/2.1.156 (Claude Code)")

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	require.True(t, shouldUseClaudeCodeTransport(ctx, info))
}

func TestShouldUseClaudeCodeTransportForRealClaudeCodeSessionHeader(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("X-Claude-Code-Session-Id", "session-123")

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	require.True(t, shouldUseClaudeCodeTransport(ctx, info))
}

func TestSelectRelayHTTPClientUsesClaudeCodeTransportFingerprint(t *testing.T) {
	service.InitHttpClient()

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ApiType: constant.APITypeAnthropic,
		ChannelOtherSettings: dto.ChannelOtherSettings{
			ClaudeCodeTransportFingerprintEnabled: true,
		},
	}}

	client, err := selectRelayHTTPClient(nil, info)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotSame(t, service.GetHttpClient(), client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.False(t, transport.ForceAttemptHTTP2)
	require.NotNil(t, transport.DialTLSContext)
	require.Nil(t, transport.TLSClientConfig)
}

func TestSelectRelayHTTPClientKeepsProxyWithClaudeCodeTransportFingerprint(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ApiType: constant.APITypeAnthropic,
		ChannelSetting: dto.ChannelSettings{
			Proxy: "http://127.0.0.1:18080",
		},
		ChannelOtherSettings: dto.ChannelOtherSettings{
			ClaudeCodeTransportFingerprintEnabled: true,
		},
	}}

	client, err := selectRelayHTTPClient(nil, info)
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.Proxy)
	require.False(t, transport.ForceAttemptHTTP2)
	require.NotNil(t, transport.DialTLSContext)
	require.Nil(t, transport.TLSClientConfig)
}
