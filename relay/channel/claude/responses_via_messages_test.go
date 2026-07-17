package claude

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIResponsesRequestToClaudePreservesFingerprintCacheAndReasoning(t *testing.T) {
	c, _ := newClaudeResponsesContext(t)
	info := newClaudeResponsesInfo(false)
	info.ApiType = rootconstant.APITypeAnthropic
	info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled = true
	request := dto.OpenAIResponsesRequest{
		Model: "claude-test",
		Input: mustClaudeResponsesRaw(t, []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text", "text": "cached prompt",
				"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"},
			}},
		}}),
		Tools: mustClaudeResponsesRaw(t, []map[string]any{{
			"type": "function", "name": "lookup", "description": "Lookup",
			"parameters": map[string]any{"type": "object", "properties": map[string]any{}},
		}}),
		Reasoning: &dto.Reasoning{Effort: "medium"},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, request)
	require.NoError(t, err)
	claudeRequest, ok := converted.(*dto.ClaudeRequest)
	require.True(t, ok)
	require.NotNil(t, claudeRequest.Thinking)
	assert.Equal(t, 2048, claudeRequest.Thinking.GetBudgetTokens())
	assert.Equal(t, "medium", info.ReasoningEffort)
	assert.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.FinalRequestRelayFormat)
	require.Len(t, claudeRequest.Tools, 1)

	system := claudeRequest.ParseSystem()
	require.Len(t, system, 2)
	assert.Contains(t, system[0].GetText(), "x-anthropic-billing-header")
	assert.Contains(t, system[1].GetText(), "Claude Code")
	assert.JSONEq(t, `{"type":"ephemeral"}`, string(system[1].CacheControl))

	require.Len(t, claudeRequest.Messages, 1)
	blocks, err := common.Any2Type[[]dto.ClaudeMediaMessage](claudeRequest.Messages[0].Content)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "cached prompt", blocks[0].GetText())
	assert.JSONEq(t, `{"type":"ephemeral","ttl":"1h"}`, string(blocks[0].CacheControl))
}

func TestConvertOpenAIResponsesRequestToClaudeRejectsUnsafeCustomTool(t *testing.T) {
	c, _ := newClaudeResponsesContext(t)
	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, newClaudeResponsesInfo(false), dto.OpenAIResponsesRequest{
		Model: "claude-test",
		Input: mustClaudeResponsesRaw(t, []map[string]any{{
			"type": "custom_tool_call", "call_id": "call_1", "name": "apply_patch", "input": "patch",
		}}),
	})
	require.ErrorContains(t, err, "cannot safely represent")
}

func TestClaudeMessagesToResponsesHandlerPreservesToolsReasoningAndCacheUsage(t *testing.T) {
	c, recorder := newClaudeResponsesContext(t)
	response := map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-test",
		"content": []map[string]any{
			{"type": "thinking", "thinking": "reasoning"},
			{"type": "text", "text": "hello"},
			{"type": "tool_use", "id": "call_1", "name": "lookup", "input": map[string]any{"q": "x"}},
		},
		"stop_reason": "tool_use",
		"usage": map[string]any{
			"input_tokens": 6, "cache_creation_input_tokens": 3, "cache_read_input_tokens": 4, "output_tokens": 2,
			"cache_creation": map[string]any{"ephemeral_5m_input_tokens": 2, "ephemeral_1h_input_tokens": 1},
		},
	}
	body, err := common.Marshal(response)
	require.NoError(t, err)
	usage, relayErr := ClaudeMessagesToResponsesHandler(c, newClaudeResponsesInfo(false), &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	})
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.Equal(t, 6, usage.PromptTokens)
	assert.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 3, usage.GetCacheCreationTokens())
	assert.Equal(t, 2, usage.ClaudeCacheCreation5mTokens)
	assert.Equal(t, 1, usage.ClaudeCacheCreation1hTokens)

	var result dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	assert.Equal(t, "response", result.Object)
	require.Len(t, result.Output, 3)
	assert.Equal(t, "hello", result.Output[0].Content[0].Text)
	assert.Equal(t, "reasoning", result.Output[1].Content[0].Text)
	assert.Equal(t, "lookup", result.Output[2].Name)
	require.NotNil(t, result.Usage)
	assert.Equal(t, 13, result.Usage.InputTokens)
	assert.Equal(t, 2, result.Usage.OutputTokens)
	assert.Equal(t, 3, result.Usage.GetCacheCreationTokens())
}

func TestClaudeMessagesToResponsesStreamHandlerReturnsResponsesSSEAndCacheUsage(t *testing.T) {
	c, recorder := newClaudeResponsesContext(t)
	body := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","usage":{"input_tokens":6,"cache_creation_input_tokens":3,"cache_read_input_tokens":4,"output_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":1}}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`data: {"type":"message_stop"}`,
		`data: [DONE]`,
		``,
	}, "\n")

	oldTimeout := rootconstant.StreamingTimeout
	rootconstant.StreamingTimeout = 30
	t.Cleanup(func() { rootconstant.StreamingTimeout = oldTimeout })
	usage, relayErr := ClaudeMessagesToResponsesStreamHandler(c, newClaudeResponsesInfo(true), &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	})
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.Equal(t, 6, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 3, usage.GetCacheCreationTokens())

	output := recorder.Body.String()
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	requireOrderedClaudeResponsesEvents(t, output,
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.completed",
	)
	assert.Contains(t, output, `"input_tokens":13`)
	assert.Contains(t, output, `"output_tokens":2`)
}

func newClaudeResponsesContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "claude-responses-test")
	return c, recorder
}

func newClaudeResponsesInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		IsStream:        stream,
		RelayMode:       relayconstant.RelayModeResponses,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RequestURLPath:  "/v1/responses",
		DisablePing:     true,
		OriginModelName: "claude-test",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "claude-test",
		},
	}
}

func mustClaudeResponsesRaw(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}

func requireOrderedClaudeResponsesEvents(t *testing.T, body string, events ...string) {
	t.Helper()
	offset := 0
	for _, event := range events {
		index := strings.Index(body[offset:], event)
		require.NotEqualf(t, -1, index, "missing %q after byte %d", event, offset)
		offset += index + len(event)
	}
}
