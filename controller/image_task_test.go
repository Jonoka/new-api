package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestParseImageTaskSubmitResponseBodyMarksImmediateURLResultSuccess(t *testing.T) {
	body := []byte(`{
		"created": 1780224045,
		"data": [{"url": "https://example.invalid/generated.png", "width": 1024, "height": 1024}],
		"task_id": "task_sync_url",
		"usage": {"total_cost": 8, "total_points": 0.08}
	}`)

	taskID, upstreamTaskID, status, progress := parseImageTaskSubmitResponseBody(body)

	assert.Equal(t, "task_sync_url", taskID)
	assert.Equal(t, "task_sync_url", upstreamTaskID)
	assert.Equal(t, string(model.TaskStatusSuccess), status)
	assert.Equal(t, "100%", progress)
}

func TestParseImageTaskSubmitResponseBodyMarksImmediateB64ResultSuccess(t *testing.T) {
	body := []byte(`{
		"id": "provider_task_123",
		"task_id": "task_sync_b64",
		"data": [{"b64_json": "iVBORw0KGgo="}]
	}`)

	taskID, upstreamTaskID, status, progress := parseImageTaskSubmitResponseBody(body)

	assert.Equal(t, "task_sync_b64", taskID)
	assert.Equal(t, "provider_task_123", upstreamTaskID)
	assert.Equal(t, string(model.TaskStatusSuccess), status)
	assert.Equal(t, "100%", progress)
}

func TestParseImageTaskSubmitResponseBodyPreservesQueuedTaskWithoutResult(t *testing.T) {
	body := []byte(`{
		"id": "provider_task_queued",
		"task_id": "task_queued",
		"status": "queued",
		"progress": 0
	}`)

	taskID, upstreamTaskID, status, progress := parseImageTaskSubmitResponseBody(body)

	assert.Equal(t, "task_queued", taskID)
	assert.Equal(t, "provider_task_queued", upstreamTaskID)
	assert.Equal(t, string(model.TaskStatusQueued), status)
	assert.Equal(t, "0%", progress)
}

func TestParseImageTaskSubmitResponseBodyCompletedStatusProgress(t *testing.T) {
	body := []byte(`{
		"id": "provider_task_done",
		"task_id": "task_done",
		"status": "completed",
		"progress": 100
	}`)

	_, _, status, progress := parseImageTaskSubmitResponseBody(body)

	assert.Equal(t, string(model.TaskStatusSuccess), status)
	assert.Equal(t, "100%", progress)
}
