package controller

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var testPNGImageBytes = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func TestPrepareImageTaskLogBuildsLightweightPreviewURLs(t *testing.T) {
	task := &model.Task{
		TaskID:     "task image/with space",
		Platform:   constant.TaskPlatformImage,
		Status:     model.TaskStatusSuccess,
		FinishTime: time.Now().Unix(),
		Data: json.RawMessage(`{"data":[` +
			`{"url":"https://example.com/image.png"},` +
			`{"b64_json":"aW1hZ2U="},` +
			`{"url":"data:image/png;base64,aW1hZ2U="}]}`),
	}
	item := &dto.TaskDto{Data: append(json.RawMessage(nil), task.Data...)}

	prepareImageTaskLog(item, task)

	require.Nil(t, item.Data)
	require.Equal(t, []string{
		"https://example.com/image.png",
		"/api/task/task%20image%2Fwith%20space/content/1",
		"/api/task/task%20image%2Fwith%20space/content/2",
	}, item.ImageURLs)
	require.False(t, item.ResultExpired)
}

func TestPrepareImageTaskLogMarksExpiredResult(t *testing.T) {
	previous := common.GetImageTaskDataRetentionHours()
	common.SetImageTaskDataRetentionHours(1)
	t.Cleanup(func() { common.SetImageTaskDataRetentionHours(previous) })

	task := &model.Task{
		TaskID:     "task_expired_log",
		Platform:   constant.TaskPlatformCanvasImage,
		Status:     model.TaskStatusSuccess,
		FinishTime: time.Now().Add(-2 * time.Hour).Unix(),
		Data:       json.RawMessage(`{"data":[{"b64_json":"aW1hZ2U="}]}`),
	}
	item := &dto.TaskDto{Data: append(json.RawMessage(nil), task.Data...)}

	prepareImageTaskLog(item, task)

	require.Nil(t, item.Data)
	require.Empty(t, item.ImageURLs)
	require.True(t, item.ResultExpired)
}

func TestGetTaskImageContentReturnsOwnStoredBase64Image(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	imageBytes := testPNGImageBytes
	require.NoError(t, (&model.Task{
		TaskID:   "task_log_image",
		Platform: constant.TaskPlatformImage,
		UserId:   7,
		Status:   model.TaskStatusSuccess,
		Data: json.RawMessage(`{"data":[{"b64_json":"` +
			base64.StdEncoding.EncodeToString(imageBytes) + `"}]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 7)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_log_image"}, {Key: "index", Value: "0"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/task_log_image/content/0", nil)

	GetTaskImageContent(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, imageBytes, recorder.Body.Bytes())
}

func TestGetTaskImageContentAllowsCurrentAdminToViewOtherUserTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	require.NoError(t, model.DB.Create(&model.User{
		Id:       9,
		Username: "task-admin",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, (&model.Task{
		TaskID:   "task_other_image",
		Platform: constant.TaskPlatformCanvasImage,
		UserId:   8,
		Status:   model.TaskStatusSuccess,
		Data: json.RawMessage(`{"data":[{"b64_json":"` +
			base64.StdEncoding.EncodeToString(testPNGImageBytes) + `"}]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 9)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_other_image"}, {Key: "index", Value: "0"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/task_other_image/content/0", nil)

	GetTaskImageContent(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, testPNGImageBytes, recorder.Body.Bytes())
}

func TestGetTaskImageContentRejectsOtherCommonUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	require.NoError(t, model.DB.Create(&model.User{
		Id:       9,
		Username: "task-user",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, (&model.Task{
		TaskID:   "task_private_image",
		Platform: constant.TaskPlatformCanvasImage,
		UserId:   8,
		Status:   model.TaskStatusSuccess,
		Data:     json.RawMessage(`{"data":[{"b64_json":"aW1hZ2U="}]}`),
	}).Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 9)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_private_image"}, {Key: "index", Value: "0"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/task_private_image/content/0", nil)

	GetTaskImageContent(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "task not found")
}

func TestDecodeCanvasImageDataRejectsActiveContentMIMEType(t *testing.T) {
	_, _, err := decodeCanvasImageData("data:text/html;base64,PGh0bWw+")
	require.ErrorContains(t, err, "unsupported image MIME type")
}

func TestDecodeCanvasImageDataDetectsRawImageMIMEType(t *testing.T) {
	tests := []struct {
		name     string
		image    []byte
		expected string
	}{
		{name: "png", image: testPNGImageBytes, expected: "image/png"},
		{name: "jpeg", image: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, expected: "image/jpeg"},
		{name: "webp", image: []byte{'R', 'I', 'F', 'F', 0x04, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}, expected: "image/webp"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, mimeType, err := decodeCanvasImageData(base64.StdEncoding.EncodeToString(test.image))
			require.NoError(t, err)
			require.Equal(t, test.expected, mimeType)
		})
	}
}

func TestDecodeCanvasImageDataRejectsDeclaredMIMEMismatch(t *testing.T) {
	value := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<html>"))
	_, _, err := decodeCanvasImageData(value)
	require.Error(t, err)
}

func TestReadCanvasImageTaskContentSupportsDataURLInURLField(t *testing.T) {
	task := &model.Task{Data: json.RawMessage(`{"data":[{"url":"data:image/png;base64,` +
		base64.StdEncoding.EncodeToString(testPNGImageBytes) + `"}]}`)}

	image, mimeType, err := readCanvasImageTaskContent(task, 0)

	require.NoError(t, err)
	require.Equal(t, "image/png", mimeType)
	require.Equal(t, testPNGImageBytes, image)
}
