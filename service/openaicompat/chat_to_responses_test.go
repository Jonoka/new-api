package openaicompat

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatCompletionsRequestToResponsesRequestPreservesStream(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(strconv.FormatBool(stream), func(t *testing.T) {
			req := &dto.GeneralOpenAIRequest{
				Model:  "test-model",
				Stream: common.GetPointer(stream),
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: "hello",
					},
				},
			}

			respReq, err := ChatCompletionsRequestToResponsesRequest(req)
			require.NoError(t, err)
			require.NotNil(t, respReq.Stream)
			require.Equal(t, stream, *respReq.Stream)
		})
	}
}

func TestChatCompletionsRequestToResponsesRequestPreservesCacheControl(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "test-model",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":          "text",
						"text":          "cache me",
						"cache_control": map[string]any{"type": "ephemeral"},
					},
				},
			},
		},
	}

	respReq, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)

	cacheControl := gjson.GetBytes(respReq.Input, "0.content.0.cache_control.type")
	require.True(t, cacheControl.Exists(), string(respReq.Input))
	require.Equal(t, "ephemeral", cacheControl.String())
}

func TestChatCompletionsRequestToResponsesRequestKeepsCachedSystemInInput(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "test-model",
		Messages: []dto.Message{
			{
				Role: "system",
				Content: []any{
					map[string]any{
						"type":          "text",
						"text":          "cached system",
						"cache_control": map[string]any{"type": "ephemeral"},
					},
				},
			},
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}

	respReq, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)
	require.Empty(t, respReq.Instructions)
	require.Equal(t, "system", gjson.GetBytes(respReq.Input, "0.role").String())
	require.Equal(t, "cached system", gjson.GetBytes(respReq.Input, "0.content.0.text").String())
	require.Equal(t, "ephemeral", gjson.GetBytes(respReq.Input, "0.content.0.cache_control.type").String())
	require.Equal(t, "user", gjson.GetBytes(respReq.Input, "1.role").String())
}
