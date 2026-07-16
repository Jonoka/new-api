package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type imageTaskBillingSpy struct{ refunds, settles int }

func (s *imageTaskBillingSpy) Settle(int) error        { s.settles++; return nil }
func (s *imageTaskBillingSpy) Refund(*gin.Context)      { s.refunds++ }
func (s *imageTaskBillingSpy) NeedsRefund() bool        { return true }
func (s *imageTaskBillingSpy) GetPreConsumedQuota() int { return 123 }
func (s *imageTaskBillingSpy) Reserve(int) error        { return nil }

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

func TestRelayImageTaskSubmitRefundsPersistenceFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelay, originalInsert := relayImageTaskRelay, insertImageTask
	t.Cleanup(func() { relayImageTaskRelay, insertImageTask = originalRelay, originalInsert })
	spy := &imageTaskBillingSpy{}
	relayImageTaskRelay = func(c *gin.Context, _ types.RelayFormat) {
		c.Set("relay_info", &relaycommon.RelayInfo{Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}})
		c.JSON(http.StatusAccepted, gin.H{"task_id": "task_persist_fail", "status": "queued"})
	}
	insertImageTask = func(*model.Task) error { return errors.New("db unavailable") }
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	RelayImageTaskSubmit(ctx)
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, 1, spy.refunds)
}

func TestRelayImageTaskSubmitRefundsImmediateTerminalFailureOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelay, originalInsert := relayImageTaskRelay, insertImageTask
	t.Cleanup(func() { relayImageTaskRelay, insertImageTask = originalRelay, originalInsert })
	spy := &imageTaskBillingSpy{}
	relayImageTaskRelay = func(c *gin.Context, _ types.RelayFormat) {
		c.Set("relay_info", &relaycommon.RelayInfo{Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{}})
		c.JSON(http.StatusOK, gin.H{"task_id": "task_failed", "status": "failed"})
	}
	insertImageTask = func(*model.Task) error { return errors.New("db unavailable too") }
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	RelayImageTaskSubmit(ctx)
	require.Equal(t, 1, spy.refunds)
}

func TestRelayImageTaskSubmitRejectsAndRefundsMissingTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelay := relayImageTaskRelay
	t.Cleanup(func() { relayImageTaskRelay = originalRelay })
	spy := &imageTaskBillingSpy{}
	relayImageTaskRelay = func(c *gin.Context, _ types.RelayFormat) {
		c.Set("relay_info", &relaycommon.RelayInfo{Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}})
		c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	RelayImageTaskSubmit(ctx)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, 1, spy.refunds)
}

func TestRelayImageTaskSubmitPassesThroughSynchronousResponseWithoutTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelay := relayImageTaskRelay
	t.Cleanup(func() { relayImageTaskRelay = originalRelay })
	spy := &imageTaskBillingSpy{}
	relayImageTaskRelay = func(c *gin.Context, _ types.RelayFormat) {
		c.Set("relay_info", &relaycommon.RelayInfo{Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{}})
		c.JSON(http.StatusOK, gin.H{"created": 123, "data": []gin.H{{"url": "https://example.invalid/image.png"}}})
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	RelayImageTaskSubmit(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"created":123,"data":[{"url":"https://example.invalid/image.png"}]}`, recorder.Body.String())
	require.Zero(t, spy.refunds)
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

func TestImageTaskResponseCaptureDoesNotCommitUnderlyingWriterOnFlush(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	capture := &imageTaskResponseCapture{ResponseWriter: ctx.Writer}
	capture.WriteHeader(http.StatusAccepted)
	_, err := capture.WriteString(`{"task_id":"queued"}`)
	require.NoError(t, err)
	capture.Flush()
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, recorder.Body.String())
	capture.statusCode = http.StatusInternalServerError
	capture.buf.Reset()
	_, _ = capture.buf.WriteString(`{"error":"persist failed"}`)
	capture.flush()
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.JSONEq(t, `{"error":"persist failed"}`, recorder.Body.String())
}

func TestRelayImageTaskSubmitFreezesTasksEndpointPollingProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelay, originalInsert := relayImageTaskRelay, insertImageTask
	t.Cleanup(func() { relayImageTaskRelay, insertImageTask = originalRelay, originalInsert })
	relayImageTaskRelay = func(c *gin.Context, _ types.RelayFormat) {
		c.Set("relay_info", &relaycommon.RelayInfo{UserId: 7, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 34, ChannelType: 1,
			ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint, ImageTasksEndpoint: "/v1/images/tasks"}}})
		c.JSON(http.StatusAccepted, gin.H{"task_id": "upstream-task-id", "status": "queued"})
	}
	var inserted *model.Task
	insertImageTask = func(task *model.Task) error { inserted = task; return nil }
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	RelayImageTaskSubmit(ctx)
	require.NotNil(t, inserted)
	require.EqualValues(t, constant.TaskPlatformImage, inserted.Platform)
	require.Equal(t, "/v1/images/tasks/{task_id}", inserted.PrivateData.PollPath)
	require.Equal(t, model.TaskProtocolOpenAIImageTasks, inserted.PrivateData.TaskProtocol)
}

func TestBuildRelayImageTaskResponseUsesCanvasResultForHandedOffTask(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_canvas_handoff",
		UserId:   7,
		Platform: constant.TaskPlatformImage,
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		PrivateData: model.TaskPrivateData{
			ClientPlatform: string(constant.TaskPlatformCanvasImage),
		},
		Data: []byte(`{"created":123,"data":[{"b64_json":"aGVsbG8="}]}`),
	}

	response := buildRelayImageTaskResponse(task)
	data, ok := response["data"].([]gin.H)
	require.True(t, ok)
	require.Len(t, data, 1)
	require.Contains(t, data[0]["url"], "/canvas/v1/images/tasks/task_canvas_handoff/content/0")
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
