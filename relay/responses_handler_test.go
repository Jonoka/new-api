package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesRequestFromCompactionPreservesSupportedFields(t *testing.T) {
	reasoning := &dto.Reasoning{Effort: "high"}
	compact := &dto.OpenAIResponsesCompactionRequest{
		Model:                "gpt-5",
		Input:                []byte(`[{"role":"user","content":"hi"}]`),
		Instructions:         []byte(`"system"`),
		PreviousResponseID:   "resp_prev",
		Tools:                []byte(`[{"type":"function","name":"lookup"}]`),
		ParallelToolCalls:    []byte(`true`),
		Reasoning:            reasoning,
		ServiceTier:          "priority",
		PromptCacheKey:       []byte(`"cache-key"`),
		PromptCacheOptions:   []byte(`{"retention":"24h"}`),
		PromptCacheRetention: []byte(`"24h"`),
		Text:                 []byte(`{"format":{"type":"text"}}`),
	}

	request, err := responsesRequestFromRelayInput(compact)
	require.NoError(t, err)
	require.Equal(t, compact.Model, request.Model)
	require.JSONEq(t, string(compact.Input), string(request.Input))
	require.JSONEq(t, string(compact.Tools), string(request.Tools))
	require.JSONEq(t, string(compact.ParallelToolCalls), string(request.ParallelToolCalls))
	require.Same(t, reasoning, request.Reasoning)
	require.Equal(t, compact.ServiceTier, request.ServiceTier)
	require.JSONEq(t, string(compact.PromptCacheKey), string(request.PromptCacheKey))
	require.JSONEq(t, string(compact.PromptCacheOptions), string(request.PromptCacheOptions))
	require.JSONEq(t, string(compact.PromptCacheRetention), string(request.PromptCacheRetention))
	require.JSONEq(t, string(compact.Text), string(request.Text))
}

func TestSyncResponsesStreamStateFromBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name     string
		body     []byte
		initial  bool
		expected bool
	}{
		{name: "stream true", body: []byte(`{"stream":true}`), initial: false, expected: true},
		{name: "stream false", body: []byte(`{"stream":false}`), initial: true, expected: false},
		{name: "stream absent", body: []byte(`{"model":"gpt-5"}`), initial: true, expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{IsStream: tc.initial}

			syncResponsesStreamStateFromBody(c, info, tc.body)

			require.Equal(t, tc.expected, info.IsStream)
			if _, ok := common.GetContextKey(c, appconstant.ContextKeyIsStream); ok {
				require.Equal(t, tc.expected, common.GetContextKeyBool(c, appconstant.ContextKeyIsStream))
			}
		})
	}
}

func TestResponsesHelperNormalizesHistoryUnlessBodyPassThroughIsEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	globalSettings := model_setting.GetGlobalSettings()
	originalGlobalPassThrough := globalSettings.PassThroughRequestEnabled
	globalSettings.PassThroughRequestEnabled = false
	t.Cleanup(func() {
		globalSettings.PassThroughRequestEnabled = originalGlobalPassThrough
	})

	originalBody := []byte(`{"model":"gpt-5","input":[{"type":"message","role":"assistant","id":"item_11a43ee66145a174de8027ea","status":"completed","namespace":"codex","content":[{"type":"output_text","text":"hello"}]}]}`)
	tests := []struct {
		name               string
		relayMode          int
		requestPath        string
		channelPassThrough bool
		globalPassThrough  bool
		wantNormalized     bool
	}{
		{name: "responses", relayMode: relayconstant.RelayModeResponses, requestPath: "/v1/responses", wantNormalized: true},
		{name: "compact", relayMode: relayconstant.RelayModeResponsesCompact, requestPath: "/v1/responses/compact", wantNormalized: true},
		{name: "channel pass through", relayMode: relayconstant.RelayModeResponses, requestPath: "/v1/responses", channelPassThrough: true},
		{name: "global pass through", relayMode: relayconstant.RelayModeResponses, requestPath: "/v1/responses", globalPassThrough: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			globalSettings.PassThroughRequestEnabled = test.globalPassThrough
			t.Cleanup(func() {
				globalSettings.PassThroughRequestEnabled = false
			})

			bodyCh := make(chan []byte, 1)
			pathCh := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				bodyCh <- body
				pathCh <- r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"test stop after request capture","type":"invalid_request_error"}}`))
			}))
			t.Cleanup(server.Close)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, test.requestPath, bytes.NewReader(originalBody))
			c.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(c) })

			common.SetContextKey(c, appconstant.ContextKeyChannelType, appconstant.ChannelTypeOpenAI)
			common.SetContextKey(c, appconstant.ContextKeyChannelId, 9101)
			common.SetContextKey(c, appconstant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(c, appconstant.ContextKeyChannelKey, "test-key")
			common.SetContextKey(c, appconstant.ContextKeyOriginalModel, "gpt-5")
			common.SetContextKey(c, appconstant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: test.channelPassThrough})

			input := []byte(`[{"type":"message","role":"assistant","id":"item_11a43ee66145a174de8027ea","status":"completed","namespace":"codex","content":[{"type":"output_text","text":"hello"}]}]`)
			var request dto.Request = &dto.OpenAIResponsesRequest{Model: "gpt-5", Input: input}
			var relayFormat types.RelayFormat = types.RelayFormatOpenAIResponses
			if test.relayMode == relayconstant.RelayModeResponsesCompact {
				request = &dto.OpenAIResponsesCompactionRequest{Model: "gpt-5", Input: input}
				relayFormat = types.RelayFormatOpenAIResponsesCompaction
			}
			info := &relaycommon.RelayInfo{
				Request:         request,
				RelayMode:       test.relayMode,
				RelayFormat:     relayFormat,
				RequestURLPath:  test.requestPath,
				OriginModelName: "gpt-5",
			}

			apiErr := ResponsesHelper(c, info)
			require.NotNil(t, apiErr)
			require.Equal(t, test.requestPath, <-pathCh)
			outboundBody := <-bodyCh

			if test.wantNormalized {
				require.False(t, gjson.GetBytes(outboundBody, "input.0.id").Exists(), string(outboundBody))
				require.False(t, gjson.GetBytes(outboundBody, "input.0.status").Exists(), string(outboundBody))
				require.False(t, gjson.GetBytes(outboundBody, "input.0.namespace").Exists(), string(outboundBody))
				require.Equal(t, "input_text", gjson.GetBytes(outboundBody, "input.0.content.0.type").String())
			} else {
				require.Equal(t, originalBody, outboundBody)
			}
			require.Equal(t, "item_11a43ee66145a174de8027ea", gjson.GetBytes(input, "0.id").String())
		})
	}
}
