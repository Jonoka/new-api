package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIResponsesRequestToGeminiMultimodalAndFunctionConversation(t *testing.T) {
	request := dto.OpenAIResponsesRequest{
		Model:        "gemini-test",
		Instructions: mustGeminiResponsesRaw(t, "system rules"),
		Input: mustGeminiResponsesRaw(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "look"},
					{"type": "input_image", "image_url": "data:image/png;base64,YQ=="},
				},
			},
			{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": map[string]any{"q": "x"}},
			{"type": "function_call_output", "call_id": "call_1", "output": map[string]any{"ok": true}},
		}),
		Tools: mustGeminiResponsesRaw(t, []map[string]any{
			{"type": "function", "name": "lookup", "description": "Lookup", "parameters": map[string]any{"type": "object"}},
		}),
		ToolChoice: mustGeminiResponsesRaw(t, map[string]any{"type": "function", "name": "lookup"}),
		Reasoning:  &dto.Reasoning{Effort: "medium"},
	}

	converted := mustConvertGeminiResponses(t, request)
	require.NotNil(t, converted.SystemInstructions)
	assert.Equal(t, "system rules", converted.SystemInstructions.Parts[0].Text)
	require.Len(t, converted.Contents, 3)
	assert.Equal(t, "user", converted.Contents[0].Role)
	assert.Equal(t, "model", converted.Contents[1].Role)
	require.NotNil(t, converted.Contents[1].Parts[0].FunctionCall)
	assert.Equal(t, "lookup", converted.Contents[1].Parts[0].FunctionCall.FunctionName)
	require.NotNil(t, converted.Contents[2].Parts[0].FunctionResponse)
	assert.Equal(t, "lookup", converted.Contents[2].Parts[0].FunctionResponse.Name)
	require.NotNil(t, converted.ToolConfig)
	assert.Equal(t, []string{"lookup"}, converted.ToolConfig.FunctionCallingConfig.AllowedFunctionNames)
	assert.NotEmpty(t, converted.GetTools())
}

func TestConvertOpenAIResponsesRequestToGeminiDropsCustomToolAndPairedOutputs(t *testing.T) {
	converted := mustConvertGeminiResponses(t, dto.OpenAIResponsesRequest{
		Model: "gemini-test",
		Input: mustGeminiResponsesRaw(t, []map[string]any{
			{"role": "assistant", "content": "before custom"},
			{"type": "custom_tool_call", "call_id": "call_custom", "name": "apply_patch", "input": "patch"},
			{"type": "custom_tool_call_output", "call_id": "call_custom", "output": "ok"},
			{"type": "function_call_output", "call_id": "call_custom", "output": "legacy output"},
			{"role": "user", "content": "next"},
		}),
		Tools: mustGeminiResponsesRaw(t, []map[string]any{{"type": "custom", "name": "apply_patch"}}),
	})

	assert.Empty(t, converted.GetTools())
	require.Len(t, converted.Contents, 2)
	assert.Equal(t, "model", converted.Contents[0].Role)
	assert.Equal(t, "before custom", converted.Contents[0].Parts[0].Text)
	assert.Equal(t, "user", converted.Contents[1].Role)
	assert.Equal(t, "next", converted.Contents[1].Parts[0].Text)
}

func mustConvertGeminiResponses(t *testing.T, request dto.OpenAIResponsesRequest) *dto.GeminiChatRequest {
	t.Helper()
	info := &relaycommon.RelayInfo{
		OriginModelName: request.Model,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: request.Model,
		},
	}
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	geminiRequest, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	return geminiRequest
}

func mustGeminiResponsesRaw(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}
