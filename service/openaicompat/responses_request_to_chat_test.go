package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesRequestToChatPreservesCoreAndCacheFields(t *testing.T) {
	stream := true
	zeroTemperature := 0.0
	maxOutputTokens := uint(128)

	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:                "gpt-test",
		Instructions:         mustResponsesRaw(t, "system rules"),
		Input:                mustResponsesRaw(t, "hello"),
		Stream:               &stream,
		StreamOptions:        &dto.StreamOptions{IncludeUsage: true},
		MaxOutputTokens:      &maxOutputTokens,
		Temperature:          &zeroTemperature,
		ParallelToolCalls:    mustResponsesRaw(t, false),
		PromptCacheKey:       mustResponsesRaw(t, "cache-key"),
		PromptCacheRetention: mustResponsesRaw(t, "24h"),
		ServiceTier:          "flex",
		Reasoning:            &dto.Reasoning{Effort: "medium", Summary: "auto"},
		Text: mustResponsesRaw(t, map[string]any{
			"verbosity": "low",
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "answer",
				"schema": map[string]any{"type": "object"},
				"strict": true,
			},
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, "gpt-test", got.Model)
	require.Len(t, got.Messages, 2)
	assert.Equal(t, "system", got.Messages[0].Role)
	assert.Equal(t, "system rules", got.Messages[0].StringContent())
	assert.Equal(t, "hello", got.Messages[1].StringContent())
	assert.Same(t, &stream, got.Stream)
	assert.True(t, got.StreamOptions.IncludeUsage)
	assert.Equal(t, maxOutputTokens, lo.FromPtr(got.MaxCompletionTokens))
	assert.Equal(t, 0.0, lo.FromPtr(got.Temperature))
	assert.False(t, lo.FromPtr(got.ParallelTooCalls))
	assert.Equal(t, "cache-key", got.PromptCacheKey)
	assert.JSONEq(t, `"24h"`, string(got.PromptCacheRetention))
	assert.JSONEq(t, `"flex"`, string(got.ServiceTier))
	assert.Equal(t, "medium", got.ReasoningEffort)
	assert.JSONEq(t, `"low"`, string(got.Verbosity))
	require.NotNil(t, got.ResponseFormat)
	assert.Equal(t, "json_schema", got.ResponseFormat.Type)
	assert.Contains(t, string(got.ResponseFormat.JsonSchema), `"name":"answer"`)
}

func TestResponsesRequestToChatConvertsMultimodalAndPreservesCacheControl(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustResponsesRaw(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "look", "cache_control": map[string]any{"type": "ephemeral"}},
					{"type": "input_image", "image_url": "https://example.test/a.png", "detail": "low"},
					{"type": "input_file", "file_id": "file_1", "filename": "a.txt"},
					{"type": "input_audio", "input_audio": map[string]any{"data": "abc", "format": "wav"}},
					{"type": "input_video", "video_url": map[string]any{"url": "https://example.test/v.mp4"}},
				},
			},
		}),
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 1)

	parts, ok := got.Messages[0].Content.([]any)
	require.True(t, ok)
	require.Len(t, parts, 5)
	textPart := parts[0].(map[string]any)
	assert.Equal(t, dto.ContentTypeText, textPart["type"])
	assert.Equal(t, "ephemeral", textPart["cache_control"].(map[string]any)["type"])
	imagePart := parts[1].(map[string]any)
	imageURL := imagePart["image_url"].(map[string]any)
	assert.Equal(t, "https://example.test/a.png", imageURL["url"])
	assert.Equal(t, "low", imageURL["detail"])

	parsed := got.Messages[0].ParseContent()
	require.Len(t, parsed, 5)
	assert.Equal(t, "file_1", parsed[2].GetFile().FileId)
	assert.Equal(t, "wav", parsed[3].GetInputAudio().Format)
	assert.Equal(t, "https://example.test/v.mp4", parsed[4].GetVideoUrl().Url)
}

func TestResponsesRequestToChatPreservesTextFunctionAndCustomToolHistory(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustResponsesRaw(t, []map[string]any{
			{"role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "calling"}}},
			{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": map[string]any{"q": "x"}},
			{"type": "custom_tool_call", "call_id": "call_2", "name": "apply_patch", "input": "patch"},
			{"type": "function_call_output", "call_id": "call_1", "output": map[string]any{"ok": true}},
			{"type": "custom_tool_call_output", "call_id": "call_2", "output": "done"},
		}),
	})
	require.NoError(t, err)
	require.Len(t, got.Messages, 3)

	assert.Equal(t, "calling", got.Messages[0].StringContent())
	toolCalls := got.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 2)
	assert.Equal(t, "function", toolCalls[0].Type)
	assert.JSONEq(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
	assert.Equal(t, dto.CustomType, toolCalls[1].Type)
	assert.Equal(t, "patch", toolCalls[1].Function.Arguments)
	assert.Contains(t, string(toolCalls[1].Custom), `"type":"custom_tool_call"`)
	assert.Equal(t, "tool", got.Messages[1].Role)
	assert.Equal(t, "call_1", got.Messages[1].ToolCallId)
	assert.JSONEq(t, `{"ok":true}`, got.Messages[1].StringContent())
	assert.Equal(t, "call_2", got.Messages[2].ToolCallId)
	assert.Equal(t, "done", got.Messages[2].StringContent())
}

func TestResponsesRequestToChatRejectsUnsafeStateAndBuiltInTools(t *testing.T) {
	_, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:              "gpt-test",
		PreviousResponseID: "resp_1",
	})
	require.ErrorContains(t, err, "previous_response_id")

	_, err = ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustResponsesRaw(t, "search"),
		Tools: mustResponsesRaw(t, []map[string]any{{"type": "web_search_preview"}}),
	})
	require.ErrorContains(t, err, "cannot be safely represented")
}

func mustResponsesRaw(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}
