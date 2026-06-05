package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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

func TestBuildCanvasImageTaskResponseReturnsStoredResult(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_canvas",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Data:     json.RawMessage(`{"data":[{"b64_json":"abc"}]}`),
	}

	response := buildCanvasImageTaskResponse(task)

	require.Equal(t, "task_canvas", response["task_id"])
	require.Equal(t, "succeeded", response["status"])
	require.JSONEq(t, `{"data":[{"b64_json":"abc"}]}`, string(response["result"].(json.RawMessage)))
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

	finishCanvasImageTask(task, 12, recorder)

	reloaded, exists, err := model.GetByTaskId(1, "task_ok")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, 12, reloaded.ChannelId)
	require.JSONEq(t, `{"data":[{"url":"https://example.com/image.png"}]}`, string(reloaded.Data))
	require.Empty(t, reloaded.FailReason)
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

	recorder, _ := executeCanvasImageRelayWithHandler(relayReq, func(c *gin.Context) {
		imageCount := 0
		if form, err := c.MultipartForm(); err == nil && form != nil {
			imageCount = len(form.File["image"])
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "path": c.Request.URL.Path, "imageCount": imageCount})
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"ok":true,"path":"/canvas/v1/images/edits","imageCount":1}`, recorder.Body.String())
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
