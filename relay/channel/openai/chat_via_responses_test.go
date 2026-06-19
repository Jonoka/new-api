package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestConvertResponsesSSEToJSON(t *testing.T) {
	body := []byte(`data: {"type":"response.created","response":{"id":"resp_1","model":"test-model","created_at":1800000000}}
data: {"type":"response.output_text.delta","delta":"Hel"}
data: {"type":"response.output_text.delta","delta":"lo"}
data: {"type":"response.completed","response":{"id":"resp_1","model":"test-model","created_at":1800000000,"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}
data: [DONE]
`)

	converted, err := convertResponsesSSEToJSON(body)
	require.NoError(t, err)

	var resp dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(converted, &resp))
	require.Equal(t, "resp_1", resp.ID)
	require.Equal(t, "test-model", resp.Model)
	require.NotNil(t, resp.Usage)
	require.Equal(t, 5, resp.Usage.TotalTokens)
	require.Len(t, resp.Output, 1)
	require.Equal(t, "Hello", resp.Output[0].Content[0].Text)
}
