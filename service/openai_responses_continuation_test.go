package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestAttachOpenAIResponsesContinuationDoesNotAutoInjectCachedPreviousResponseID(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 9527,
		},
	}
	req := &dto.OpenAIResponsesRequest{
		PromptCacheKey: mustMarshalRaw(t, "sess-123"),
		Input: mustMarshalRaw(t, []map[string]any{
			{
				"role": "system",
				"content": []map[string]any{
					{"type": "input_text", "text": "system"},
				},
			},
			{
				"role": "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": "prior"},
				},
			},
			{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "toolA",
				"arguments": `{"x":1}`,
			},
			{
				"type":    "function_call_output",
				"call_id": "call_1",
				"output":  "ok",
			},
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "next"},
				},
			},
		}),
	}

	BindOpenAIResponsesContinuationResponseID(info, req, "resp_prev_123")

	attached := AttachOpenAIResponsesContinuation(info, req)
	require.False(t, attached)
	require.Empty(t, req.PreviousResponseID)
	require.JSONEq(t, `[
		{"role":"system","content":[{"type":"input_text","text":"system"}]},
		{"role":"assistant","content":[{"type":"output_text","text":"prior"}]},
		{"type":"function_call","call_id":"call_1","name":"toolA","arguments":"{\"x\":1}"},
		{"type":"function_call_output","call_id":"call_1","output":"ok"},
		{"role":"user","content":[{"type":"input_text","text":"next"}]}
	]`, string(req.Input))
}

func TestAttachOpenAIResponsesContinuationDoesNotOverrideExplicitPreviousResponseID(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 9528,
		},
	}
	req := &dto.OpenAIResponsesRequest{
		PromptCacheKey:     mustMarshalRaw(t, "sess-456"),
		PreviousResponseID: "resp_explicit_456",
		Input:              mustMarshalRaw(t, []map[string]any{{"role": "user", "content": "hello"}}),
	}

	BindOpenAIResponsesContinuationResponseID(info, req, "resp_cached_456")

	attached := AttachOpenAIResponsesContinuation(info, req)
	require.False(t, attached)
	require.Equal(t, "resp_explicit_456", req.PreviousResponseID)
}

func TestResolveOpenAIResponsesContinuationSessionIDUsesRuntimeSessionID(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 9529,
		},
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]interface{}{
			"session_id": "123e4567-e89b-12d3-a456-426614174000",
		},
	}
	req := &dto.OpenAIResponsesRequest{}

	sessionID := resolveOpenAIResponsesContinuationSessionID(info, req)
	require.Equal(t, relaycommon.NormalizeOpenAIBridgeSessionIDForCache(info, "123e4567-e89b-12d3-a456-426614174000"), sessionID)
}

func TestIsOpenAIResponsesPreviousResponseRetryable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
		want       bool
	}{
		{
			name:       "unsupported parameter",
			statusCode: 400,
			message:    "status_code=400, Unsupported parameter: previous_response_id",
			want:       true,
		},
		{
			name:       "websocket v2 only supported",
			statusCode: 400,
			message:    "status_code=400, previous_response_id is only supported on Responses WebSocket v2",
			want:       true,
		},
		{
			name:       "previous response not found",
			statusCode: 404,
			message:    "status_code=404, previous response not found",
			want:       true,
		},
		{
			name:       "other bad request",
			statusCode: 400,
			message:    "status_code=400, max_output_tokens is not supported for this model",
			want:       false,
		},
		{
			name:       "server error",
			statusCode: 500,
			message:    "status_code=500, previous_response_id is only supported on Responses WebSocket v2",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsOpenAIResponsesPreviousResponseRetryable(tt.statusCode, tt.message))
		})
	}
}

func mustMarshalRaw(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}
