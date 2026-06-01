package sora

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultRunningStatus(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info, err := adaptor.ParseTaskResult([]byte(`{"id":"upstream_task","task_id":"upstream_task","status":"running","progress":5}`))

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, model.TaskStatusInProgress, info.Status)
	assert.Equal(t, "5%", info.Progress)
}

func TestParseTaskResultSuccessAliases(t *testing.T) {
	adaptor := &TaskAdaptor{}
	for _, status := range []string{"completed", "succeeded", "success"} {
		t.Run(status, func(t *testing.T) {
			info, err := adaptor.ParseTaskResult([]byte(`{"id":"upstream_task","status":"` + status + `","progress":100}`))

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, model.TaskStatusSuccess, info.Status)
			assert.Equal(t, "100%", info.Progress)
		})
	}
}
