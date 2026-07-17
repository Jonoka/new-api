package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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
