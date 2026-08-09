package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatResponseToResponsesPreservesTextReasoningToolsAndUsage(t *testing.T) {
	reasoning := "thinking"
	toolCalls, err := common.Marshal([]dto.ToolCallRequest{
		{ID: "call_1", Type: "function", Function: dto.FunctionRequest{Name: "lookup", Arguments: `{"q":"x"}`}},
		{ID: "call_2", Type: dto.CustomType, Function: dto.FunctionRequest{Name: "apply_patch", Arguments: "patch"}},
	})
	require.NoError(t, err)
	chat := &dto.OpenAITextResponse{
		Id:      "chatcmpl_1",
		Model:   "gpt-test",
		Created: 456,
		Choices: []dto.OpenAITextResponseChoice{{
			Message: dto.Message{
				Role:             "assistant",
				Content:          "I will call.",
				ReasoningContent: &reasoning,
				ToolCalls:        toolCalls,
			},
			FinishReason: "tool_calls",
		}},
		Usage: dto.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         4,
				CachedCreationTokens: 2,
			},
			ClaudeCacheCreation5mTokens: 2,
		},
	}
	chat.Usage.PromptTokensDetails.HasCachedCreationTokens = true

	resp, usage, err := ChatCompletionsResponseToResponsesResponse(chat, "resp_1")
	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, "resp_1", resp.ID)
	assert.Equal(t, `"completed"`, string(resp.Status))
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
	require.NotNil(t, usage.InputTokensDetails)
	assert.Equal(t, 4, usage.InputTokensDetails.CachedTokens)
	assert.Equal(t, 2, usage.GetCacheCreationTokens())
	assert.Equal(t, 2, usage.ClaudeCacheCreation5mTokens)

	require.Len(t, resp.Output, 4)
	assert.Equal(t, responsesOutputTypeMessage, resp.Output[0].Type)
	assert.Equal(t, "I will call.", resp.Output[0].Content[0].Text)
	assert.Equal(t, responsesOutputTypeReasoning, resp.Output[1].Type)
	assert.Equal(t, "thinking", resp.Output[1].Content[0].Text)
	assert.Equal(t, responsesOutputTypeFunctionCall, resp.Output[2].Type)
	assert.Equal(t, "lookup", resp.Output[2].Name)
	assert.Equal(t, `"{\"q\":\"x\"}"`, string(resp.Output[2].Arguments))
	assert.Equal(t, responsesOutputTypeCustomToolCall, resp.Output[3].Type)
	assert.Equal(t, "apply_patch", resp.Output[3].Name)
	assert.Equal(t, `"patch"`, string(resp.Output[3].Arguments))
}

func TestChatResponseToResponsesMapsIncompleteFinishReasons(t *testing.T) {
	for _, test := range []struct {
		finishReason string
		wantReason   string
	}{
		{finishReason: "length", wantReason: responsesIncompleteReasonMaxTokens},
		{finishReason: "content_filter", wantReason: responsesIncompleteReasonContentFilter},
	} {
		t.Run(test.finishReason, func(t *testing.T) {
			resp, _, err := ChatCompletionsResponseToResponsesResponse(&dto.OpenAITextResponse{
				Choices: []dto.OpenAITextResponseChoice{{
					Message:      dto.Message{Role: "assistant", Content: "partial"},
					FinishReason: test.finishReason,
				}},
			}, "resp_1")
			require.NoError(t, err)
			assert.Equal(t, `"incomplete"`, string(resp.Status))
			require.NotNil(t, resp.IncompleteDetails)
			assert.Equal(t, test.wantReason, resp.IncompleteDetails.Reason)
			assert.Equal(t, "incomplete", resp.Output[0].Status)
		})
	}
}

func TestChatStreamToResponsesOrdersTextToolAndUsageEvents(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_1", "gpt-test")
	created := int64(1710000000)
	toolIndex := 0
	finishReason := "tool_calls"

	events := mustChatToResponsesEvents(t, state, &dto.ChatCompletionsStreamResponse{
		Created: created,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: common.GetPointer("hello")},
		}},
	})
	assertEventTypes(t, events, responsesEventCreated, responsesEventOutputItemAdded, responsesEventOutputTextDelta)

	events = mustChatToResponsesEvents(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{{
				Index: &toolIndex,
				ID:    "call_1",
				Type:  "function",
				Function: dto.FunctionResponse{
					Name:      "lookup",
					Arguments: `{"q":"x"}`,
				},
			}}},
		}},
	})
	assertEventTypes(t, events, responsesEventOutputItemAdded, responsesEventFunctionArgsDelta)

	events = mustChatToResponsesEvents(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{FinishReason: &finishReason}},
	})
	assertEventTypes(t, events,
		"response.output_text.done",
		responsesEventOutputItemDone,
		responsesEventFunctionArgsDone,
		responsesEventOutputItemDone,
	)

	mustChatToResponsesEvents(t, state, &dto.ChatCompletionsStreamResponse{
		Usage: &dto.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
	})
	finalEvents := FinalizeChatCompletionsStreamToResponses(state)
	require.Len(t, finalEvents, 1)
	assert.Equal(t, responsesEventCompleted, finalEvents[0].Type)
	final := finalEvents[0].Payload.Response
	require.NotNil(t, final)
	assert.Equal(t, int(created), final.CreatedAt)
	require.NotNil(t, final.Usage)
	assert.Equal(t, 2, final.Usage.InputTokens)
	assert.Equal(t, 3, final.Usage.OutputTokens)
	require.Len(t, final.Output, 2)
	assert.Equal(t, "hello", final.Output[0].Content[0].Text)
	assert.Equal(t, "lookup", final.Output[1].Name)
}

func TestChatStreamToResponsesSupportsCustomToolAndEmptyStream(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_custom", "gpt-test")
	toolIndex := 0
	finishReason := "tool_calls"
	events := mustChatToResponsesEvents(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ToolCalls: []dto.ToolCallResponse{
						{
							Index: &toolIndex,
							ID:    "custom_1",
							Type:  dto.CustomType,
							Function: dto.FunctionResponse{
								Name:      "apply_patch",
								Arguments: "patch",
							},
						},
					},
				},
			},
		},
	})
	assertEventTypes(t, events, responsesEventCreated, responsesEventOutputItemAdded, responsesEventCustomToolInputDelta)
	events = mustChatToResponsesEvents(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{FinishReason: &finishReason}},
	})
	assert.Equal(t, responsesEventCustomToolInputDone, events[0].Type)
	final := FinalizeChatCompletionsStreamToResponses(state)
	assert.Equal(t, responsesOutputTypeCustomToolCall, final[0].Payload.Response.Output[0].Type)

	empty := NewChatToResponsesStreamState("resp_empty", "gpt-test")
	emptyFinal := FinalizeChatCompletionsStreamToResponses(empty)
	assertEventTypes(t, emptyFinal, responsesEventCreated, responsesEventCompleted)
}

func mustChatToResponsesEvents(t *testing.T, state *ChatToResponsesStreamState, chunk *dto.ChatCompletionsStreamResponse) []ChatToResponsesStreamEvent {
	t.Helper()
	events, err := ChatCompletionsStreamChunkToResponsesEvents(chunk, state)
	require.NoError(t, err)
	return events
}

func assertEventTypes(t *testing.T, events []ChatToResponsesStreamEvent, expected ...string) {
	t.Helper()
	actual := make([]string, 0, len(events))
	for _, event := range events {
		actual = append(actual, event.Type)
	}
	assert.Equal(t, expected, actual)
}
