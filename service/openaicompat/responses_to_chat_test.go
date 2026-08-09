package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/stretchr/testify/require"
)

func TestResponsesResponseToChatPreservesTextReasoningAndTools(t *testing.T) {
	resp := &dto.OpenAIResponsesResponse{
		CreatedAt: 123,
		Model:     "gpt-test",
		Status:    []byte(`"completed"`),
		Output: []dto.ResponsesOutput{
			{
				Type: "reasoning",
				Content: []dto.ResponsesOutputContent{
					{Type: "summary_text", Text: "first"},
					{Type: "summary_text", Text: "\n\nsecond"},
				},
			},
			{
				Type: "message",
				Role: "assistant",
				Content: []dto.ResponsesOutputContent{
					{Type: "output_text", Text: "先说明，再调用工具。"},
				},
			},
			{
				Type:      "function_call",
				ID:        "fc_1",
				CallId:    "call_1",
				Name:      "lookup",
				Arguments: []byte(`{"q":"x"}`),
			},
		},
		Usage: &dto.Usage{InputTokens: 3, OutputTokens: 4, TotalTokens: 7},
	}

	chat, usage, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl_1")
	require.NoError(t, err)
	require.Equal(t, "tool_calls", chat.Choices[0].FinishReason)
	require.Equal(t, "先说明，再调用工具。", chat.Choices[0].Message.StringContent())
	require.Equal(t, "first\n\nsecond", chat.Choices[0].Message.GetReasoningContent())
	toolCalls := chat.Choices[0].Message.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	require.Equal(t, "call_1", toolCalls[0].ID)
	require.Equal(t, "lookup", toolCalls[0].Function.Name)
	require.Equal(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
	require.Equal(t, 7, usage.TotalTokens)
}

func TestResponsesFinishReasonFromIncompleteStatus(t *testing.T) {
	tests := []struct {
		reason string
		want   string
	}{
		{reason: "max_output_tokens", want: "length"},
		{reason: "content_filter", want: "content_filter"},
		{reason: "other", want: "length"},
	}

	for _, tt := range tests {
		got, ok := ResponsesFinishReasonFromStatus(&dto.OpenAIResponsesResponse{
			Status:            []byte(`"incomplete"`),
			IncompleteDetails: &dto.IncompleteDetails{Reason: tt.reason},
		})
		require.True(t, ok)
		require.Equal(t, tt.want, got)
	}
}

func TestUsageFromResponsesUsagePreservesCacheWriteAliases(t *testing.T) {
	var src dto.Usage
	require.NoError(t, common.Unmarshal([]byte(`{
		"input_tokens":100,
		"output_tokens":2,
		"total_tokens":102,
		"input_tokens_details":{"cached_tokens":80,"cache_write_tokens":30}
	}`), &src))

	usage := UsageFromResponsesUsage(&src)

	require.Equal(t, 100, usage.PromptTokens)
	require.Equal(t, 80, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 30, usage.GetCacheCreationTokens())

	direct := UsageFromResponsesUsage(&dto.Usage{
		InputTokens: 20,
		InputTokensDetails: &dto.InputTokenDetails{
			CacheWriteTokens: 7,
		},
	})
	require.Equal(t, 7, direct.GetCacheCreationTokens())
}

func TestResponsesStreamStateHandlesOutOfOrderToolArguments(t *testing.T) {
	state := NewResponsesToChatStreamState("gpt-test", false)
	state.ID = "chatcmpl_test"
	state.Created = 123
	outputIndex := 1

	var chunks []dto.ChatCompletionsStreamResponse
	appendEvent := func(event *dto.ResponsesStreamResponse) {
		converted, err := ResponsesStreamEventToChatChunks(event, state)
		require.NoError(t, err)
		chunks = append(chunks, converted...)
	}

	appendEvent(&dto.ResponsesStreamResponse{Type: responsesEventCreated})
	appendEvent(&dto.ResponsesStreamResponse{Type: responsesEventOutputTextDelta, Delta: "text before tool"})
	appendEvent(&dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		Delta:       `{"cmd":"ls"}`,
	})
	appendEvent(&dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		Item: &dto.ResponsesOutput{
			Type:   "function_call",
			ID:     "fc_1",
			CallId: "call_1",
			Name:   "exec",
		},
	})
	appendEvent(&dto.ResponsesStreamResponse{
		Type: responsesEventCompleted,
		Response: &dto.OpenAIResponsesResponse{
			Status: []byte(`"completed"`),
			Usage:  &dto.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
		},
	})

	require.Len(t, chunks, 4)
	require.Equal(t, "text before tool", chunks[1].Choices[0].Delta.GetContentString())
	tool := chunks[2].Choices[0].Delta.ToolCalls[0]
	require.Equal(t, "call_1", tool.ID)
	require.Equal(t, "exec", tool.Function.Name)
	require.Equal(t, `{"cmd":"ls"}`, tool.Function.Arguments)
	require.Equal(t, "tool_calls", *chunks[3].Choices[0].FinishReason)
	require.Equal(t, 3, state.Usage.TotalTokens)
}

func TestResponsesStreamStateSupportsCustomToolAndIncompleteStatus(t *testing.T) {
	state := NewResponsesToChatStreamState("gpt-test", false)
	state.ID = "chatcmpl_test"
	state.Created = 123
	outputIndex := 0

	var chunks []dto.ChatCompletionsStreamResponse
	for _, event := range []*dto.ResponsesStreamResponse{
		{Type: responsesEventReasoningTextDelta, Delta: "thinking"},
		{
			Type:        responsesEventOutputItemAdded,
			OutputIndex: &outputIndex,
			Item: &dto.ResponsesOutput{
				Type:   "custom_tool_call",
				ID:     "ct_1",
				CallId: "call_custom",
				Name:   "apply_patch",
			},
		},
		{Type: responsesEventCustomToolInputDelta, OutputIndex: &outputIndex, Delta: "patch body"},
		{
			Type: responsesEventIncomplete,
			Response: &dto.OpenAIResponsesResponse{
				IncompleteDetails: &dto.IncompleteDetails{Reason: "content_filter"},
			},
		},
	} {
		converted, err := ResponsesStreamEventToChatChunks(event, state)
		require.NoError(t, err)
		chunks = append(chunks, converted...)
	}

	require.Len(t, chunks, 5)
	require.Equal(t, "thinking", chunks[1].Choices[0].Delta.GetReasoningContent())
	require.Equal(t, "apply_patch", chunks[2].Choices[0].Delta.ToolCalls[0].Function.Name)
	require.Equal(t, "patch body", chunks[3].Choices[0].Delta.ToolCalls[0].Function.Arguments)
	require.Equal(t, "content_filter", *chunks[4].Choices[0].FinishReason)
}
