package controller

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCanvasImageTaskSubmitPreservesRequestAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	originalStart := startCanvasImageTaskRelay
	t.Cleanup(func() { startCanvasImageTaskRelay = originalStart })

	tests := []struct {
		name     string
		target   string
		expected string
	}{
		{name: "edit path", target: "/canvas/v1/images/edits?group=auto", expected: canvasImageTaskActionEdits},
		{name: "generation path", target: "/canvas/v1/images/generations?group=auto", expected: canvasImageTaskActionGenerations},
		{name: "generic default", target: "/canvas/v1/images/tasks?group=auto", expected: canvasImageTaskActionGenerations},
		{name: "explicit edit", target: "/canvas/v1/images/tasks?action=edits&group=auto", expected: canvasImageTaskActionEdits},
		{name: "full edit action", target: "/canvas/v1/images/tasks?action=images%2Fedits&group=auto", expected: canvasImageTaskActionEdits},
		{name: "explicit generation", target: "/canvas/v1/images/edits?action=generations&group=auto", expected: canvasImageTaskActionGenerations},
		{name: "explicit edit overrides path", target: "/canvas/v1/images/generations?action=edits&group=auto", expected: canvasImageTaskActionEdits},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured canvasImageTaskRelayRequest
			startCanvasImageTaskRelay = func(relayReq canvasImageTaskRelayRequest) {
				captured = relayReq
			}

			body := []byte(`{"model":"gpt-image-2-pro","prompt":"test"}`)
			storage, err := common.CreateBodyStorage(body)
			require.NoError(t, err)
			t.Cleanup(func() { _ = storage.Close() })
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("id", 1)
			ctx.Set(common.KeyBodyStorage, storage)
			ctx.Request = httptest.NewRequest(http.MethodPost, tt.target, bytes.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			CanvasImageTaskSubmit(ctx)

			require.Equal(t, http.StatusAccepted, recorder.Code)
			require.Equal(t, tt.expected, captured.Action)
			task, exists, err := model.GetByTaskId(1, captured.TaskID)
			require.NoError(t, err)
			require.True(t, exists)
			require.Equal(t, tt.expected, task.Action)
			require.Equal(t, constant.TaskPlatform(constant.TaskPlatformCanvasImage), task.Platform)
			require.Equal(t, "auto", task.Group)

			replay, _, _ := executeCanvasImageRelayWithHandler(captured, func(c *gin.Context) {
				require.Equal(t, "/canvas/v1/"+tt.expected, c.Request.URL.Path)
				require.Equal(t, "group=auto", c.Request.URL.RawQuery)
				replayedBody, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.Equal(t, body, replayedBody)
				c.Status(http.StatusNoContent)
			})
			require.Equal(t, http.StatusNoContent, replay.Code)
		})
	}
}

func TestCanvasImageTaskSubmitPreservesMultipartEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCanvasImageTaskTestDB(t)
	originalStart := startCanvasImageTaskRelay
	t.Cleanup(func() { startCanvasImageTaskRelay = originalStart })
	var captured canvasImageTaskRelayRequest
	startCanvasImageTaskRelay = func(relayReq canvasImageTaskRelayRequest) {
		captured = relayReq
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2-pro"))
	require.NoError(t, writer.WriteField("prompt", "edit the reference image"))
	source := []byte("image transport fixture")
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write(source)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	storage, err := common.CreateBodyStorage(body.Bytes())
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Set(common.KeyBodyStorage, storage)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/edits?group=image-test", bytes.NewReader(body.Bytes()))
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	CanvasImageTaskSubmit(ctx)

	require.Equal(t, http.StatusAccepted, recorder.Code)
	task, exists, err := model.GetByTaskId(1, captured.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, canvasImageTaskActionEdits, task.Action)
	require.Equal(t, body.Bytes(), captured.Body)
	replay, _, _ := executeCanvasImageRelayWithHandler(captured, func(c *gin.Context) {
		require.Equal(t, "/canvas/v1/images/edits", c.Request.URL.Path)
		require.Equal(t, "image-test", c.Query("group"))
		require.Equal(t, writer.FormDataContentType(), c.Request.Header.Get("Content-Type"))
		form, err := c.MultipartForm()
		require.NoError(t, err)
		defer form.RemoveAll()
		require.Equal(t, []string{"gpt-image-2-pro"}, form.Value["model"])
		require.Equal(t, []string{"edit the reference image"}, form.Value["prompt"])
		require.Len(t, form.File["image"], 1)
		file, err := form.File["image"][0].Open()
		require.NoError(t, err)
		defer file.Close()
		replayedImage, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, source, replayedImage)
		c.Status(http.StatusNoContent)
	})
	require.Equal(t, http.StatusNoContent, replay.Code)
}
