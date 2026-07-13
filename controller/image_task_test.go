package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayImageTaskSubmitUsesRetryCapableRelayPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelay := relayImageTaskRelay
	t.Cleanup(func() { relayImageTaskRelay = originalRelay })

	called := false
	relayImageTaskRelay = func(c *gin.Context, relayFormat types.RelayFormat) {
		called = true
		require.EqualValues(t, types.RelayFormatOpenAIImage, relayFormat)
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream failed"})
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	RelayImageTaskSubmit(ctx)

	require.True(t, called)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestRelayImageTaskSubmitDoesNotPersistErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelay := relayImageTaskRelay
	originalInsert := insertImageTask
	t.Cleanup(func() {
		relayImageTaskRelay = originalRelay
		insertImageTask = originalInsert
	})

	relayImageTaskRelay = func(c *gin.Context, _ types.RelayFormat) {
		c.Set("relay_info", &relaycommon.RelayInfo{})
		c.JSON(http.StatusBadGateway, gin.H{"task_id": "task_must_not_persist"})
	}
	insertImageTask = func(task *model.Task) error {
		t.Fatalf("unexpected task persistence: %s", task.TaskID)
		return nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	RelayImageTaskSubmit(ctx)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestRelayImageTaskSubmitPersistsSuccessfulFinalRelayMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelay := relayImageTaskRelay
	originalInsert := insertImageTask
	t.Cleanup(func() {
		relayImageTaskRelay = originalRelay
		insertImageTask = originalInsert
	})

	relayImageTaskRelay = func(c *gin.Context, relayFormat types.RelayFormat) {
		require.EqualValues(t, types.RelayFormatOpenAIImage, relayFormat)
		c.Set("relay_info", &relaycommon.RelayInfo{
			UserId:          42,
			UsingGroup:      "image-group",
			OriginModelName: "gpt-image-1",
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelId:         29,
				UpstreamModelName: "upstream-image-model",
			},
			PriceData: types.PriceData{Quota: 123},
		})
		c.JSON(http.StatusAccepted, gin.H{
			"id":      "provider-final-id",
			"task_id": "task_public_id",
			"status":  "queued",
		})
	}

	var inserted *model.Task
	insertImageTask = func(task *model.Task) error {
		inserted = task
		return nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	ctx.Set("platform", "openai")

	RelayImageTaskSubmit(ctx)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.JSONEq(t, `{"id":"provider-final-id","task_id":"task_public_id","status":"queued"}`, recorder.Body.String())
	require.NotNil(t, inserted)
	require.Equal(t, "task_public_id", inserted.TaskID)
	require.Equal(t, "provider-final-id", inserted.PrivateData.UpstreamTaskID)
	require.Equal(t, 29, inserted.ChannelId)
	require.Equal(t, 42, inserted.UserId)
	require.Equal(t, 123, inserted.Quota)
	require.Equal(t, "gpt-image-1", inserted.Properties.OriginModelName)
	require.Equal(t, "upstream-image-model", inserted.Properties.UpstreamModelName)
	require.EqualValues(t, model.TaskStatusQueued, inserted.Status)
}

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
