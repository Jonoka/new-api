package controller

import (
	"bytes"
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
	require.NotContains(t, string(encoded), "abc")
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

func TestCanvasImageTaskContentReturnsStoredBase64Image(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	imageBytes := []byte("fake image bytes")

	require.NoError(t, (&model.Task{
		TaskID: "task_image",
		UserId: 1,
		Status: model.TaskStatusSuccess,
		Data:   json.RawMessage(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(imageBytes) + `"}]}`),
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

func TestFinishCanvasImageTaskStoresSuccessfulRelayResponse(t *testing.T) {
	setupCanvasImageTaskTestDB(t)
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusOK)
	_, err := recorder.WriteString(`{"data":[{"url":"https://example.com/image.png"}]}`)
	require.NoError(t, err)

	task := &model.Task{TaskID: "task_ok", UserId: 1, Status: model.TaskStatusInProgress}
	require.NoError(t, task.Insert())

	finishCanvasImageTask(task, 12, recorder, nil)

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
	relayInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
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
	relayInfo := &relaycommon.RelayInfo{Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}}
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
	relayInfo := &relaycommon.RelayInfo{Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}}
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
	relayInfo := &relaycommon.RelayInfo{Billing: spy, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}}
	finishCanvasImageTask(task, 34, recorder, relayInfo)
	reloaded, exists, err := model.GetByTaskId(1, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, 1, spy.refunds)
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

func TestNormalizeCanvasImageTaskActionAcceptsShortEditAction(t *testing.T) {
	require.Equal(t, canvasImageTaskActionEdits, normalizeCanvasImageTaskAction("edits"))
	require.Equal(t, canvasImageTaskActionEdits, normalizeCanvasImageTaskAction("images/edits"))
	require.Equal(t, canvasImageTaskActionGenerations, normalizeCanvasImageTaskAction(""))
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

	oldDB := model.DB
	oldUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		model.DB = oldDB
		common.UsingSQLite = oldUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	common.UsingSQLite = true
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.Task{}))
}
