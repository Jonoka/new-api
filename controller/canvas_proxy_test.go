package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInjectCanvasGroupIntoJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/canvas/v1/chat/completions?group=vip", strings.NewReader(`{"model":"gpt-4o"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	err := injectCanvasGroup(ctx)
	require.NoError(t, err)

	body, err := io.ReadAll(ctx.Request.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-4o","group":"vip"}`, string(body))
	require.Equal(t, int64(len(body)), ctx.Request.ContentLength)
}

func TestInjectCanvasGroupIntoMultipartBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "test"))
	require.NoError(t, writer.Close())

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/canvas/v1/images/edits?group=vip", bytes.NewReader(body.Bytes()))
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	err := injectCanvasGroup(ctx)
	require.NoError(t, err)

	reader, err := ctx.Request.MultipartReader()
	require.NoError(t, err)
	form, err := reader.ReadForm(32 << 20)
	require.NoError(t, err)
	defer form.RemoveAll()

	require.Equal(t, []string{"vip"}, form.Value["group"])
	require.Equal(t, []string{"gpt-image-1"}, form.Value["model"])
	require.NotEmpty(t, ctx.Request.Header.Get("Content-Type"))
	require.NotEqual(t, writer.FormDataContentType(), ctx.Request.Header.Get("Content-Type"))
	require.Greater(t, ctx.Request.ContentLength, int64(0))
}

func TestBuildCanvasImageTaskResponseReturnsLightweightContentURLs(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_canvas",
		UserId:   1,
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     json.RawMessage(`{"data":[{"b64_json":"abc"}]}`),
	}

	response := buildCanvasImageTaskResponse(task)

	require.Equal(t, "task_canvas", response["task_id"])
	require.Equal(t, "succeeded", response["status"])
	result, ok := response["result"].(gin.H)
	require.True(t, ok)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	items := result["data"].([]gin.H)
	require.Len(t, items, 1)
	contentURL, ok := items[0]["url"].(string)
	require.True(t, ok)
	require.Contains(t, contentURL, "/canvas/v1/images/tasks/task_canvas/content/0?user_id=1&expires=")
	require.Contains(t, contentURL, "&token=")
	require.NotContains(t, string(encoded), "b64_json")
}

func TestBuildCanvasImageTaskResponsePreservesLegacyStringFailure(t *testing.T) {
	task := &model.Task{TaskID: "task_legacy_failed", Status: model.TaskStatusFailure, Progress: "100%", FailReason: "legacy failure"}
	response := buildCanvasImageTaskResponse(task)
	require.Equal(t, "legacy failure", response["error"])
	require.Equal(t, "legacy failure", response["msg"])
}

func TestBuildCanvasImageTaskResponsePreservesStructuredFailure(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_failed",
		Status:     model.TaskStatusFailure,
		Progress:   "100%",
		FailReason: "Lite pool unavailable",
		Data:       json.RawMessage(`{"error":{"message":"Lite pool unavailable","type":"new_api_error","code":"lite_pool_exhausted"}}`),
	}

	response := buildCanvasImageTaskResponse(task)

	errorPayload, ok := response["error"].(gin.H)
	require.True(t, ok)
	require.Equal(t, "Lite pool unavailable", errorPayload["message"])
	require.Equal(t, "new_api_error", errorPayload["type"])
	require.Equal(t, "lite_pool_exhausted", errorPayload["code"])
}

func TestBuildAPIImageTaskResponseReturnsRegularContentURLs(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_api",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     json.RawMessage(`{"data":[{"b64_json":"abc"}]}`),
	}

	response := buildAPIImageTaskResponse(task)

	result, ok := response["result"].(gin.H)
	require.True(t, ok)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":[{"url":"/v1/images/tasks/task_api/content/0"}]}`, string(encoded))
}

func TestBuildAPIImageTaskResponsePrefersStableContentURLWhenBase64Exists(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_api_with_cdn",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     json.RawMessage(`{"data":[{"url":"https://cdn.example.com/image.png","b64_json":"abc"}]}`),
	}

	response := buildAPIImageTaskResponse(task)

	result, ok := response["result"].(gin.H)
	require.True(t, ok)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":[{"url":"/v1/images/tasks/task_api_with_cdn/content/0"}]}`, string(encoded))
}

func TestBuildAPIImageTaskResponseMarksExpiredData(t *testing.T) {
	previous := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previous) })

	task := &model.Task{
		TaskID:     "task_expired",
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		FinishTime: time.Now().Add(-2 * time.Hour).Unix(),
		Data:       json.RawMessage(`{"data":[{"b64_json":"abc"}]}`),
	}

	response := buildAPIImageTaskResponse(task)
	require.Equal(t, true, response["result_expired"])
	require.NotContains(t, response, "result")
	require.Equal(t, task.FinishTime+3600, response["expires_at"])
}

func TestCanvasImageTaskContentReturnsStoredBase64Image(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	imageBytes := testPNGImageBytes

	require.NoError(t, (&model.Task{
		TaskID:   "task_image",
		Platform: constant.TaskPlatformCanvasImage,
		UserId:   1,
		Status:   model.TaskStatusSuccess,
		Data:     json.RawMessage(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(imageBytes) + `"}]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_image"}, {Key: "index", Value: "0"}}
	expires := time.Now().Add(time.Hour).Unix()
	token := signCanvasImageTaskContentToken("task_image", 1, 0, expires)
	ctx.Request = httptest.NewRequest("GET", fmt.Sprintf("/canvas/v1/images/tasks/task_image/content/0?user_id=1&expires=%d&token=%s", expires, token), nil)

	CanvasImageTaskContent(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, imageBytes, recorder.Body.Bytes())
}

func TestImageTaskContentReturnsGoneAfterRetentionExpires(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	previous := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previous) })

	require.NoError(t, (&model.Task{
		TaskID:     "task_expired_content",
		Platform:   constant.TaskPlatformImage,
		UserId:     1,
		Status:     model.TaskStatusSuccess,
		FinishTime: time.Now().Add(-2 * time.Hour).Unix(),
		Data:       json.RawMessage(`{"data":[{"b64_json":"aW1hZ2U="}]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_expired_content"}, {Key: "index", Value: "0"}}
	ctx.Request = httptest.NewRequest("GET", "/v1/images/tasks/task_expired_content/content/0", nil)

	ImageTaskContent(ctx)

	require.Equal(t, http.StatusGone, recorder.Code)
	require.Contains(t, recorder.Body.String(), "image task data has expired")
}

func TestCanvasImageTaskFetchRejectsOtherUsersTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)

	require.NoError(t, (&model.Task{
		TaskID: "task_other",
		UserId: 2,
		Status: model.TaskStatusSuccess,
		Data:   json.RawMessage(`{"data":[]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_other"}}
	ctx.Request = httptest.NewRequest("GET", "/canvas/v1/images/tasks/task_other?group=vip", nil)

	CanvasImageTaskFetch(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "task not found")
}

func TestImageTaskFetchRejectsNonImageTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)

	require.NoError(t, (&model.Task{
		TaskID:   "task_video",
		Platform: constant.TaskPlatform("video"),
		UserId:   1,
		Status:   model.TaskStatusSuccess,
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_video"}}
	ctx.Request = httptest.NewRequest("GET", "/v1/images/tasks/task_video", nil)

	ImageTaskFetch(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestImageTaskFetchAcceptsGenericImageTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)

	require.NoError(t, (&model.Task{
		TaskID:   "task_api_image",
		Platform: constant.TaskPlatformImage,
		UserId:   1,
		Status:   model.TaskStatusQueued,
		Progress: "0%",
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_api_image"}}
	ctx.Request = httptest.NewRequest("GET", "/v1/images/tasks/task_api_image", nil)

	ImageTaskFetch(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"task_id":"task_api_image"`)
}

func TestFinishCanvasImageTaskStoresSuccessfulRelayResponse(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	_, err := recorder.WriteString(`{"data":[{"url":"https://example.com/image.png"}]}`)
	require.NoError(t, err)

	task := &model.Task{TaskID: "task_ok", UserId: 1, Status: model.TaskStatusInProgress}
	require.NoError(t, task.Insert())

	finishCanvasImageTask(task, 12, recorder, &relaycommon.RelayInfo{UserId: 1, UsingGroup: "default", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 12}})

	reloaded, exists, err := model.GetByTaskId(1, "task_ok")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, 12, reloaded.ChannelId)
	require.JSONEq(t, `{"data":[{"url":"https://example.com/image.png"}]}`, string(reloaded.Data))
	require.Empty(t, reloaded.FailReason)
}

func TestFinishCanvasImageTaskHandsOffQueuedTasksEndpointResponse(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusAccepted)
	_, err := recorder.WriteString(`{"task_id":"upstream_async","status":"queued"}`)
	require.NoError(t, err)

	task := &model.Task{TaskID: "task_canvas_async", Platform: constant.TaskPlatformCanvasImage, UserId: 1, Status: model.TaskStatusInProgress}
	require.NoError(t, task.Insert())
	relayInfo := &relaycommon.RelayInfo{UserId: 1, UsingGroup: "default", ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId:   34,
		ChannelType: constant.ChannelTypeOpenAI,
		ChannelOtherSettings: dto.ChannelOtherSettings{
			ImageAsyncMode:     dto.ImageAsyncModeTasksEndpoint,
			ImageTasksEndpoint: "/v1/images/tasks",
		},
	}}

	finishCanvasImageTask(task, 34, recorder, relayInfo)

	reloaded, exists, err := model.GetByTaskId(1, "task_canvas_async")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, constant.TaskPlatformImage, reloaded.Platform)
	require.Equal(t, string(constant.TaskPlatformCanvasImage), reloaded.PrivateData.ClientPlatform)
	require.Equal(t, "upstream_async", reloaded.PrivateData.UpstreamTaskID)
	require.EqualValues(t, model.TaskStatusQueued, reloaded.Status)
	require.Equal(t, "/v1/images/tasks/{task_id}", reloaded.PrivateData.PollPath)
}

func TestFinishCanvasImageTaskRejectsAndRefundsMissingTaskID(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	spy := &imageTaskBillingSpy{}
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusAccepted)
	_, _ = recorder.WriteString(`{"status":"queued"}`)
	task := &model.Task{TaskID: "task_canvas_missing", UserId: 1, Status: model.TaskStatusInProgress}
	require.NoError(t, task.Insert())
	relayInfo := &relaycommon.RelayInfo{UserId: 1, Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 34, ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}}
	finishCanvasImageTask(task, 34, recorder, relayInfo)
	reloaded, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, 1, spy.refunds)
}

func TestFinishCanvasImageTaskRefundsHandoffPersistenceFailure(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	originalUpdate := updateCanvasImageTask
	t.Cleanup(func() { updateCanvasImageTask = originalUpdate })
	updateCanvasImageTask = func(*model.Task) error { return errors.New("db unavailable") }
	spy := &imageTaskBillingSpy{}
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusAccepted)
	_, _ = recorder.WriteString(`{"task_id":"upstream_async","status":"queued"}`)
	task := &model.Task{TaskID: "task_canvas_update_fail", UserId: 1, Status: model.TaskStatusInProgress}
	require.NoError(t, task.Insert())
	relayInfo := &relaycommon.RelayInfo{UserId: 1, Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 34, ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}}
	finishCanvasImageTask(task, 34, recorder, relayInfo)
	require.Equal(t, 1, spy.refunds)
}

func TestFinishCanvasImageTaskRefundsImmediateTerminalFailure(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	spy := &imageTaskBillingSpy{}
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	_, _ = recorder.WriteString(`{"task_id":"failed_async","status":"failed","error":{"message":"rejected"}}`)
	task := &model.Task{TaskID: "task_canvas_terminal_fail", UserId: 1, Status: model.TaskStatusInProgress}
	require.NoError(t, task.Insert())
	relayInfo := &relaycommon.RelayInfo{UserId: 1, Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 34, ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}}
	finishCanvasImageTask(task, 34, recorder, relayInfo)

	reloaded, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, 1, spy.refunds)
}

func TestFinishCanvasImageTaskDoesNotOverwriteTimedOutTask(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	task := &model.Task{TaskID: "task_timeout_race", UserId: 1, Status: model.TaskStatusInProgress, Progress: "10%"}
	require.NoError(t, task.Insert())

	timedOut, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	timedOut.Status = model.TaskStatusFailure
	timedOut.Progress = "100%"
	timedOut.FailReason = "image generation timed out"
	won, err := timedOut.UpdateWithStatus(model.TaskStatusInProgress)
	require.NoError(t, err)
	require.True(t, won)

	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	_, err = recorder.WriteString(`{"data":[{"url":"https://example.com/late.png"}]}`)
	require.NoError(t, err)
	finishCanvasImageTask(task, 12, recorder, nil)

	reloaded, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, "image generation timed out", reloaded.FailReason)
}

func TestExecuteCanvasImageRelayRoutesEditTasks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "edit"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	relayReq := canvasImageTaskRelayRequest{
		Action: canvasImageTaskActionEdits,
		Body:   body.Bytes(),
		Header: http.Header{"Content-Type": []string{writer.FormDataContentType()}},
	}

	recorder, _, _ := executeCanvasImageRelayWithHandler(relayReq, func(c *gin.Context) {
		imageCount := 0
		if form, err := c.MultipartForm(); err == nil && form != nil {
			imageCount = len(form.File["image"])
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "path": c.Request.URL.Path, "imageCount": imageCount})
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"ok":true,"path":"/canvas/v1/images/edits","imageCount":1}`, recorder.Body.String())
}

func TestRelayImageTaskResponseFlattensSuccessfulImageData(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_openai_image",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     []byte(`{"id":"upstream_task","status":"succeeded","data":[{"url":"https://example.com/result.png"}]}`),
	}

	response := buildRelayImageTaskResponse(task)

	require.Equal(t, "task_openai_image", response["id"])
	require.Equal(t, "task_openai_image", response["task_id"])
	require.Equal(t, "succeeded", response["status"])
	items, ok := response["data"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
}

func TestRelayImageTaskResponseSignsCanvasImageContentURL(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_canvas_image",
		UserId:   7,
		Platform: "canvas_image",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     []byte(`{"data":[{"url":"data:image/png;base64,abc"}]}`),
	}

	response := buildRelayImageTaskResponse(task)
	items, ok := response["data"].([]gin.H)
	require.True(t, ok)
	require.Len(t, items, 1)
	urlValue, _ := items[0]["url"].(string)
	require.Contains(t, urlValue, "/canvas/v1/images/tasks/task_canvas_image/content/0")
	require.Contains(t, urlValue, "user_id=7")
	require.Contains(t, urlValue, "expires=")
	require.Contains(t, urlValue, "token=")
}

func TestExecuteAPIImageRelayUsesRegularRequestPath(t *testing.T) {
	relayReq := canvasImageTaskRelayRequest{
		Action:      canvasImageTaskActionGenerations,
		RelayPrefix: apiImageTaskRelayPrefix,
		Body:        []byte(`{"model":"gpt-image-1","prompt":"test"}`),
		Header:      http.Header{"Content-Type": []string{"application/json"}},
	}

	recorder, _, _ := executeCanvasImageRelayWithHandler(relayReq, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"path": c.Request.URL.Path})
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"path":"/v1/images/generations"}`, recorder.Body.String())
}

func TestExecuteCanvasImageRelayPropagatesTaskCancellation(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()

	relayReq := canvasImageTaskRelayRequest{
		Action:  canvasImageTaskActionGenerations,
		Body:    []byte(`{"model":"grok-imagine-image","prompt":"test"}`),
		Context: requestContext,
	}
	contextCanceled := false

	executeCanvasImageRelayWithHandler(relayReq, func(c *gin.Context) {
		contextCanceled = errors.Is(c.Request.Context().Err(), context.Canceled)
		c.Status(http.StatusGatewayTimeout)
	})

	require.True(t, contextCanceled)
}

func TestRunCanvasImageTaskRelayMarksBlockedTaskFailedAfterTimeout(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	task := &model.Task{
		TaskID:   "task_blocked_timeout",
		UserId:   1,
		Platform: constant.TaskPlatformImage,
		Status:   model.TaskStatusQueued,
		Progress: "0%",
	}
	require.NoError(t, task.Insert())

	relayReq := canvasImageTaskRelayRequest{
		TaskID: task.TaskID,
		Keys: map[string]any{
			string(constant.ContextKeyUserId): 1,
		},
	}
	runCanvasImageTaskRelayWithExecutor(relayReq, 20*time.Millisecond, func(req canvasImageTaskRelayRequest) (*httptest.ResponseRecorder, int, *relaycommon.RelayInfo) {
		<-req.Context.Done()
		recorder := httptest.NewRecorder()
		recorder.WriteHeader(http.StatusGatewayTimeout)
		return recorder, 0, nil
	})

	reloaded, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, "image generation timed out", reloaded.FailReason)
	require.NotZero(t, reloaded.FinishTime)
}

func TestNormalizeCanvasImageTaskActionAcceptsShortEditAction(t *testing.T) {
	require.Equal(t, canvasImageTaskActionEdits, normalizeCanvasImageTaskAction("edits"))
	require.Equal(t, canvasImageTaskActionEdits, normalizeCanvasImageTaskAction("images/edits"))
	require.Equal(t, canvasImageTaskActionGenerations, normalizeCanvasImageTaskAction(""))
	_, ok := parseImageTaskAction("unsupported")
	require.False(t, ok)
}

func TestImageTaskRelayRawQueryDropsControlAction(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/images/tasks?action=edits&response_format=b64_json", nil)

	require.Equal(t, "response_format=b64_json", imageTaskRelayRawQuery(ctx))
}

func TestAPIImageTaskGroupIgnoresUntrustedQueryOverride(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/images/tasks?group=untrusted", nil)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "token-group")

	require.Equal(t, "token-group", imageTaskGroup(ctx, apiImageTaskRelayPrefix))
	require.Equal(t, "untrusted", imageTaskGroup(ctx, canvasImageTaskRelayPrefix))
}

func TestImageTaskContentMaxAgeUsesRemainingRetention(t *testing.T) {
	previous := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previous) })

	task := &model.Task{FinishTime: 1000}
	require.EqualValues(t, 3000, imageTaskContentMaxAge(task, 1600))
}

func TestCanvasImageTaskActionFallsBackToEditPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/edits?group=auto", nil)

	require.Equal(t, canvasImageTaskActionEdits, canvasImageTaskAction(ctx))
}

func TestBuildCanvasImageActualBillingInputUsesReturnedSizeQuality(t *testing.T) {
	expr := `param("quality") == "high" || has(param("size"), "3840") ? tier("4k", 300000 * param("n")) : tier("standard", 100000 * param("n"))`
	snap := &billingexpr.BillingSnapshot{
		BillingMode:               "tiered_expr",
		ExprString:                expr,
		ExprHash:                  billingexpr.ExprHashString(expr),
		GroupRatio:                1,
		EstimatedPromptTokens:     1,
		EstimatedCompletionTokens: 0,
		EstimatedTier:             "4k",
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               1,
	}

	input, actual, ok := buildCanvasImageActualBillingInput(snap, []byte(`{"size":"1536x1024","quality":"medium","data":[{"url":"data:image/png;base64,abc"}]}`), &billingexpr.RequestInput{Body: []byte(`{"size":"3840x2160","quality":"high","n":1,"response_format":"url"}`)})
	require.True(t, ok)
	require.Equal(t, "1536x1024", actual.Size)
	require.Equal(t, "medium", actual.Quality)
	require.Equal(t, 1, actual.N)

	result, err := billingexpr.ComputeTieredQuotaWithRequest(snap, billingexpr.TokenParams{P: 1, Len: 1}, input)
	require.NoError(t, err)
	require.Equal(t, "standard", result.MatchedTier)
	require.Equal(t, 50000, result.ActualQuotaAfterGroup)
}

func setupCanvasImageTaskTestDB(t *testing.T) {
	t.Helper()

	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.UsingSQLite = oldUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	common.UsingSQLite = true
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskAccounting{}, &model.TaskAccountingEvent{}, &model.TaskAccountingLogReceipt{}, &model.User{}, &model.Channel{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "canvas-user", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 12, Name: "canvas-12", Key: "test", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 34, Name: "canvas-34", Key: "test", Status: common.ChannelStatusEnabled}).Error)
}
