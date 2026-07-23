package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestResolveOpenAIResponsesContinuationSessionIDWithoutIdentityIsEmpty(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: 9530,
		},
	}

	require.Empty(t, resolveOpenAIResponsesContinuationSessionID(info, &dto.OpenAIResponsesRequest{}))
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

func TestRemoveIncompleteOpenAIResponsesReasoningHistoryFromJSON(t *testing.T) {
	data := []byte(`{
		"model":"gpt-5",
		"previous_response_id":"resp_prev_123",
		"input":[
			{"type":"message","role":"user","content":"hello"},
			{"type":"reasoning","summary":[{"text":"old"}]},
			{"type":"reasoning","encrypted_content":"enc_123","summary":[{"text":"kept"}]},
			{"type":"message","role":"user","content":"next"}
		]
	}`)

	cleaned, removed := RemoveIncompleteOpenAIResponsesReasoningHistoryFromJSON(data)

	require.True(t, removed)
	require.False(t, gjson.GetBytes(cleaned, "previous_response_id").Exists(), string(cleaned))
	require.Len(t, gjson.GetBytes(cleaned, "input").Array(), 3, string(cleaned))
	require.Equal(t, "message", gjson.GetBytes(cleaned, "input.0.type").String())
	require.Equal(t, "reasoning", gjson.GetBytes(cleaned, "input.1.type").String())
	require.Equal(t, "enc_123", gjson.GetBytes(cleaned, "input.1.encrypted_content").String())
	require.Equal(t, "next", gjson.GetBytes(cleaned, "input.2.content").String())
}

func TestNormalizeOpenAIResponsesInputHistoryForUpstream(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{Input: []byte(`[
		{"type":"message","role":"assistant","id":"item_message_1","status":"completed","namespace":"codex","phase":"commentary","content":[{"type":"output_text","id":"content_1","status":"completed","annotations":[],"text":"hello"},{"type":"refusal","refusal":"cannot comply"}]},
		{"role":"user","id":"item_message_2","status":"completed","content":"next"},
		{"type":"function_call","id":"item_call_1","status":"completed","namespace":"codex","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"x\"}"},
		{"type":"function_call_output","id":"item_output_1","status":"completed","call_id":"call_1","output":"ok"},
		{"type":"custom_tool_call","id":"item_custom_1","status":"completed","namespace":"codex","call_id":"call_2","name":"apply_patch","input":"patch"},
		{"type":"custom_tool_call_output","id":"item_custom_output_1","status":"completed","call_id":"call_2","output":"done"},
		{"type":"reasoning","id":"item_reasoning_1","status":"completed","namespace":"codex","encrypted_content":"enc_1"},
		{"type":"item_reference","id":"item_reference_1","status":"completed","namespace":"codex"},
		{"type":"vendor_private","role":"assistant","id":"item_vendor_1","status":"completed","namespace":"codex","content":[{"type":"output_text","text":"vendor"}]},
		"literal input"
	]`)}

	normalized, err := NormalizeOpenAIResponsesInputHistoryForUpstream(req)
	require.NoError(t, err)
	require.Equal(t, 6, normalized)

	for _, index := range []int{0, 1, 2, 3, 4, 5} {
		path := fmt.Sprintf("%d", index)
		require.False(t, gjson.GetBytes(req.Input, path+".id").Exists(), string(req.Input))
		require.False(t, gjson.GetBytes(req.Input, path+".status").Exists(), string(req.Input))
		require.False(t, gjson.GetBytes(req.Input, path+".namespace").Exists(), string(req.Input))
	}

	require.Equal(t, "commentary", gjson.GetBytes(req.Input, "0.phase").String())
	require.Equal(t, "input_text", gjson.GetBytes(req.Input, "0.content.0.type").String())
	require.Equal(t, "hello", gjson.GetBytes(req.Input, "0.content.0.text").String())
	require.False(t, gjson.GetBytes(req.Input, "0.content.0.id").Exists(), string(req.Input))
	require.False(t, gjson.GetBytes(req.Input, "0.content.0.status").Exists(), string(req.Input))
	require.False(t, gjson.GetBytes(req.Input, "0.content.0.annotations").Exists(), string(req.Input))
	require.Equal(t, "input_text", gjson.GetBytes(req.Input, "0.content.1.type").String())
	require.Equal(t, "cannot comply", gjson.GetBytes(req.Input, "0.content.1.text").String())
	require.Equal(t, "call_1", gjson.GetBytes(req.Input, "2.call_id").String())
	require.Equal(t, "lookup", gjson.GetBytes(req.Input, "2.name").String())
	require.Equal(t, "ok", gjson.GetBytes(req.Input, "3.output").String())
	require.Equal(t, "call_2", gjson.GetBytes(req.Input, "4.call_id").String())
	require.Equal(t, "done", gjson.GetBytes(req.Input, "5.output").String())

	require.Equal(t, "item_reasoning_1", gjson.GetBytes(req.Input, "6.id").String())
	require.Equal(t, "item_reference_1", gjson.GetBytes(req.Input, "7.id").String())
	require.Equal(t, "item_vendor_1", gjson.GetBytes(req.Input, "8.id").String())
	require.Equal(t, "completed", gjson.GetBytes(req.Input, "8.status").String())
	require.Equal(t, "output_text", gjson.GetBytes(req.Input, "8.content.0.type").String())
	require.Equal(t, "literal input", gjson.GetBytes(req.Input, "9").String())

	firstPass := append([]byte(nil), req.Input...)
	normalized, err = NormalizeOpenAIResponsesInputHistoryForUpstream(req)
	require.NoError(t, err)
	require.Zero(t, normalized)
	require.Equal(t, firstPass, []byte(req.Input))
}

func TestNormalizeOpenAIResponsesInputHistoryForUpstreamKeepsNonArrayInput(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{Input: []byte(`"hello"`)}
	original := append([]byte(nil), req.Input...)

	normalized, err := NormalizeOpenAIResponsesInputHistoryForUpstream(req)

	require.NoError(t, err)
	require.Zero(t, normalized)
	require.Equal(t, original, []byte(req.Input))
}

func mustMarshalRaw(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}
