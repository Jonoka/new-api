package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type sessionBridgeOutboundAdaptor struct {
	captureResponsesAdaptor
	upstreamURL string
}

func (a *sessionBridgeOutboundAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.upstreamURL, nil
}

func (a *sessionBridgeOutboundAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, body)
}

type capturedSessionBridgeRequest struct {
	header http.Header
	body   []byte
}

type sessionBridgeUpstreamCapture struct {
	mu       sync.Mutex
	requests []capturedSessionBridgeRequest
}

func (c *sessionBridgeUpstreamCapture) handler(w http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	c.mu.Lock()
	c.requests = append(c.requests, capturedSessionBridgeRequest{
		header: request.Header.Clone(),
		body:   append([]byte(nil), body...),
	})
	c.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"id":"resp_session_bridge","model":"gpt-5","created_at":1800000000,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}`))
}

func (c *sessionBridgeUpstreamCapture) snapshot(t *testing.T) []capturedSessionBridgeRequest {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]capturedSessionBridgeRequest, len(c.requests))
	copy(result, c.requests)
	return result
}

func executeClaudeViaResponsesForSessionBridgeTest(
	t *testing.T,
	adaptor channel.Adaptor,
	requestID string,
	channelID int,
	promptCacheKey string,
	stableHeaders map[string]string,
) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("X-Client-Request-Id", requestID)
	for name, value := range stableHeaders {
		ctx.Request.Header.Set(name, value)
	}
	ctx.Set(common.RequestIdKey, requestID)

	requestHeaders := map[string]string{
		"X-Client-Request-Id": requestID,
	}
	for name, value := range stableHeaders {
		requestHeaders[name] = value
	}
	info := &relaycommon.RelayInfo{
		RequestId:       requestID,
		RequestHeaders:  requestHeaders,
		OriginModelName: "gpt-5",
		RelayFormat:     types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           constant.APITypeOpenAI,
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelId:         channelID,
			UpstreamModelName: "gpt-5",
		},
	}

	claudeRequest := dto.ClaudeRequest{
		Model: "gpt-5",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}
	openAIRequest, err := service.ClaudeToOpenAIRequest(claudeRequest, info)
	require.NoError(t, err)
	if promptCacheKey != "" {
		openAIRequest.PromptCacheKey = promptCacheKey
	}

	usage, apiErr := chatCompletionsViaResponses(ctx, info, adaptor, openAIRequest)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
}

func TestClaudeViaResponsesFinalRequestIgnoresChangingClientRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	capture := &sessionBridgeUpstreamCapture{}
	server := httptest.NewServer(http.HandlerFunc(capture.handler))
	t.Cleanup(server.Close)
	adaptor := &sessionBridgeOutboundAdaptor{upstreamURL: server.URL}

	executeClaudeViaResponsesForSessionBridgeTest(
		t,
		adaptor,
		"11111111-1111-4111-8111-111111111111",
		7001,
		"",
		nil,
	)
	executeClaudeViaResponsesForSessionBridgeTest(
		t,
		adaptor,
		"22222222-2222-4222-8222-222222222222",
		7001,
		"",
		nil,
	)

	requests := capture.snapshot(t)
	require.Len(t, requests, 2)
	for _, request := range requests {
		require.Empty(t, request.header.Get("session_id"))
		require.False(t, gjson.GetBytes(request.body, "prompt_cache_key").Exists())
	}
	require.JSONEq(t, string(requests[0].body), string(requests[1].body))
}

func TestClaudeViaResponsesFinalRequestPreservesExplicitPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	capture := &sessionBridgeUpstreamCapture{}
	server := httptest.NewServer(http.HandlerFunc(capture.handler))
	t.Cleanup(server.Close)
	adaptor := &sessionBridgeOutboundAdaptor{upstreamURL: server.URL}

	executeClaudeViaResponsesForSessionBridgeTest(t, adaptor, "request-id-one", 7002, "explicit-cache-key", nil)
	executeClaudeViaResponsesForSessionBridgeTest(t, adaptor, "request-id-two", 7002, "explicit-cache-key", nil)

	requests := capture.snapshot(t)
	require.Len(t, requests, 2)
	for _, request := range requests {
		require.Equal(t, "explicit-cache-key", gjson.GetBytes(request.body, "prompt_cache_key").String())
		require.NotEmpty(t, request.header.Get("session_id"))
	}
	require.Equal(t, requests[0].header.Get("session_id"), requests[1].header.Get("session_id"))
}

func TestClaudeViaResponsesFinalSessionHeaderUsesStableSessionSeeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	for _, headerName := range []string{"X-Codex-Session-Id", "X-Session-Id"} {
		t.Run(headerName, func(t *testing.T) {
			capture := &sessionBridgeUpstreamCapture{}
			server := httptest.NewServer(http.HandlerFunc(capture.handler))
			t.Cleanup(server.Close)
			adaptor := &sessionBridgeOutboundAdaptor{upstreamURL: server.URL}
			stableHeaders := map[string]string{headerName: "stable-session-seed"}

			executeClaudeViaResponsesForSessionBridgeTest(t, adaptor, "request-id-one", 7003, "", stableHeaders)
			executeClaudeViaResponsesForSessionBridgeTest(t, adaptor, "request-id-two", 7003, "", stableHeaders)

			requests := capture.snapshot(t)
			require.Len(t, requests, 2)
			require.NotEmpty(t, requests[0].header.Get("session_id"))
			require.Equal(t, requests[0].header.Get("session_id"), requests[1].header.Get("session_id"))
			require.Equal(t, "stable-session-seed", gjson.GetBytes(requests[0].body, "prompt_cache_key").String())
			require.Equal(t, "stable-session-seed", gjson.GetBytes(requests[1].body, "prompt_cache_key").String())
		})
	}
}

func TestClaudeViaResponsesFinalSessionHeaderIsIsolatedByChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	capture := &sessionBridgeUpstreamCapture{}
	server := httptest.NewServer(http.HandlerFunc(capture.handler))
	t.Cleanup(server.Close)
	adaptor := &sessionBridgeOutboundAdaptor{upstreamURL: server.URL}

	executeClaudeViaResponsesForSessionBridgeTest(t, adaptor, "request-id-one", 7101, "shared-cache-key", nil)
	executeClaudeViaResponsesForSessionBridgeTest(t, adaptor, "request-id-two", 7102, "shared-cache-key", nil)

	requests := capture.snapshot(t)
	require.Len(t, requests, 2)
	require.NotEmpty(t, requests[0].header.Get("session_id"))
	require.NotEmpty(t, requests[1].header.Get("session_id"))
	require.NotEqual(t, requests[0].header.Get("session_id"), requests[1].header.Get("session_id"))
	require.Equal(t, "shared-cache-key", gjson.GetBytes(requests[0].body, "prompt_cache_key").String())
	require.Equal(t, "shared-cache-key", gjson.GetBytes(requests[1].body, "prompt_cache_key").String())
}

func executeDirectOpenAISessionBridgeRequest(
	t *testing.T,
	adaptor channel.Adaptor,
	info *relaycommon.RelayInfo,
	body []byte,
) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	for name, value := range info.RequestHeaders {
		ctx.Request.Header.Set(name, value)
	}

	relaycommon.MergeOpenAISessionBridgeOverride(info, body)
	response, err := channel.DoApiRequest(adaptor, ctx, info, bytes.NewReader(body))
	require.NoError(t, err)
	require.NotNil(t, response)
	_ = response.Body.Close()
}

func TestNativeResponsesAndCompactFinalRequestsIgnoreClientRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	tests := []struct {
		name        string
		relayMode   int
		relayFormat types.RelayFormat
	}{
		{
			name:        "native responses",
			relayMode:   relayconstant.RelayModeResponses,
			relayFormat: types.RelayFormatOpenAIResponses,
		},
		{
			name:        "responses compact",
			relayMode:   relayconstant.RelayModeResponsesCompact,
			relayFormat: types.RelayFormatOpenAIResponsesCompaction,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capture := &sessionBridgeUpstreamCapture{}
			server := httptest.NewServer(http.HandlerFunc(capture.handler))
			t.Cleanup(server.Close)
			adaptor := &sessionBridgeOutboundAdaptor{upstreamURL: server.URL}
			body := []byte(`{"model":"gpt-5","input":"hello"}`)

			for _, requestID := range []string{"request-id-one", "request-id-two"} {
				info := &relaycommon.RelayInfo{
					RelayMode:   test.relayMode,
					RelayFormat: test.relayFormat,
					RequestHeaders: map[string]string{
						"X-Client-Request-Id": requestID,
					},
					ChannelMeta: &relaycommon.ChannelMeta{
						ApiType:   constant.APITypeOpenAI,
						ChannelId: 7201,
					},
				}
				executeDirectOpenAISessionBridgeRequest(t, adaptor, info, body)
			}

			requests := capture.snapshot(t)
			require.Len(t, requests, 2)
			for _, request := range requests {
				require.Empty(t, request.header.Get("session_id"))
				require.JSONEq(t, string(body), string(request.body))
				require.False(t, gjson.GetBytes(request.body, "prompt_cache_key").Exists())
			}
		})
	}
}

func TestNativeResponsesFinalRequestIgnoresNullSessionHeaderOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	capture := &sessionBridgeUpstreamCapture{}
	server := httptest.NewServer(http.HandlerFunc(capture.handler))
	t.Cleanup(server.Close)
	adaptor := &sessionBridgeOutboundAdaptor{upstreamURL: server.URL}
	body := []byte(`{"model":"gpt-5","input":"hello"}`)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:   constant.APITypeOpenAI,
			ChannelId: 7202,
			HeadersOverride: map[string]interface{}{
				"session_id":      nil,
				"conversation_id": nil,
			},
		},
	}

	executeDirectOpenAISessionBridgeRequest(t, adaptor, info, body)

	requests := capture.snapshot(t)
	require.Len(t, requests, 1)
	require.Empty(t, requests[0].header.Get("session_id"))
	require.Empty(t, requests[0].header.Get("conversation_id"))
	require.False(t, gjson.GetBytes(requests[0].body, "prompt_cache_key").Exists())
}

func TestOfficialCodexFinalSessionHeaderRemainsStable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	capture := &sessionBridgeUpstreamCapture{}
	server := httptest.NewServer(http.HandlerFunc(capture.handler))
	t.Cleanup(server.Close)
	adaptor := &sessionBridgeOutboundAdaptor{upstreamURL: server.URL}
	body := []byte(`{"model":"gpt-5","input":"hello"}`)

	for _, requestID := range []string{"request-id-one", "request-id-two"} {
		info := &relaycommon.RelayInfo{
			RelayMode:   relayconstant.RelayModeResponses,
			RelayFormat: types.RelayFormatOpenAIResponses,
			RequestHeaders: map[string]string{
				"Session_id":          "official-codex-session",
				"X-Client-Request-Id": requestID,
			},
			ChannelMeta: &relaycommon.ChannelMeta{
				ApiType:   constant.APITypeCodex,
				ChannelId: 7301,
			},
		}
		executeDirectOpenAISessionBridgeRequest(t, adaptor, info, body)
	}

	requests := capture.snapshot(t)
	require.Len(t, requests, 2)
	require.NotEmpty(t, requests[0].header.Get("session_id"))
	require.Equal(t, requests[0].header.Get("session_id"), requests[1].header.Get("session_id"))
	for _, request := range requests {
		require.JSONEq(t, string(body), string(request.body))
	}
}
