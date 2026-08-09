package zhipu

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestZhipuResponseDataUnmarshalKeepsEnvelopeAndUsage(t *testing.T) {
	body := []byte(`{"code":200,"msg":"ok","success":true,"data":{"task_id":"task-test","request_id":"request-test","task_status":"SUCCESS","choices":[{"role":"assistant","content":"ok"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}}`)
	var response ZhipuResponse

	require.NoError(t, common.Unmarshal(body, &response))
	require.True(t, response.Success)
	require.Equal(t, "task-test", response.Data.TaskId)
	require.Equal(t, "request-test", response.Data.RequestId)
	require.Equal(t, "SUCCESS", response.Data.TaskStatus)
	require.Len(t, response.Data.Choices, 1)
	require.Equal(t, 10, response.Data.Usage.PromptTokens)
	require.Equal(t, 20, response.Data.Usage.CompletionTokens)
	require.Equal(t, 30, response.Data.Usage.TotalTokens)
}

func TestZhipuStreamMetaResponseUnmarshalKeepsEnvelopeAndUsage(t *testing.T) {
	body := []byte(`{"request_id":"request-test","task_id":"task-test","task_status":"SUCCESS","usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
	var response ZhipuStreamMetaResponse

	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, "request-test", response.RequestId)
	require.Equal(t, "task-test", response.TaskId)
	require.Equal(t, "SUCCESS", response.TaskStatus)
	require.Equal(t, 10, response.Usage.PromptTokens)
	require.Equal(t, 20, response.Usage.CompletionTokens)
	require.Equal(t, 30, response.Usage.TotalTokens)
}
