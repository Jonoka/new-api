package controller

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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
