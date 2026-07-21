package xai

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newMultipartImageEditContext(t *testing.T) (*gin.Context, dto.ImageRequest) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "client-model"))
	require.NoError(t, writer.WriteField("prompt", "保留参考图"))
	require.NoError(t, writer.WriteField("size", "1024x1024"))
	require.NoError(t, writer.WriteField("custom", "first"))
	require.NoError(t, writer.WriteField("custom", "second"))

	imagePart, err := writer.CreateFormFile("image", "reference.png")
	require.NoError(t, err)
	_, err = imagePart.Write([]byte("image-content"))
	require.NoError(t, err)

	maskPart, err := writer.CreateFormFile("mask", "mask.png")
	require.NoError(t, err)
	_, err = maskPart.Write([]byte("mask-content"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	return c, dto.ImageRequest{
		Model:  "mapped-model",
		Prompt: "保留参考图",
		Size:   "1024x1024",
	}
}

func TestConvertImageEditMultipartPreservesFilesFieldsAndMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, request := newMultipartImageEditContext(t)
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)
	convertedBody, ok := converted.(*bytes.Buffer)
	require.True(t, ok)

	multipartRequest := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(convertedBody.Bytes()))
	multipartRequest.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	require.NoError(t, multipartRequest.ParseMultipartForm(1<<20))
	t.Cleanup(func() { _ = multipartRequest.MultipartForm.RemoveAll() })

	require.Equal(t, []string{"mapped-model"}, multipartRequest.MultipartForm.Value["model"])
	require.Equal(t, []string{"保留参考图"}, multipartRequest.MultipartForm.Value["prompt"])
	require.Equal(t, []string{"1024x1024"}, multipartRequest.MultipartForm.Value["size"])
	require.Equal(t, []string{"first", "second"}, multipartRequest.MultipartForm.Value["custom"])
	require.Equal(t, "image-content", readMultipartFile(t, multipartRequest.MultipartForm.File["image"][0]))
	require.Equal(t, "mask-content", readMultipartFile(t, multipartRequest.MultipartForm.File["mask"][0]))
}

func TestDoRequestForMultipartImageEditForwardsFormContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	c, request := newMultipartImageEditContext(t)
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}

	converted, err := adaptor.ConvertImageRequest(c, info, request)
	require.NoError(t, err)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, c.Request.Header.Get("Content-Type"), r.Header.Get("Content-Type"))
		require.NoError(t, r.ParseMultipartForm(1<<20))
		require.Equal(t, "mapped-model", r.FormValue("model"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(upstream.Close)
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelBaseUrl: upstream.URL,
	}
	info.RequestURLPath = "/v1/images/edits"

	response, err := adaptor.DoRequest(c, info, converted.(io.Reader))
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NoError(t, response.(*http.Response).Body.Close())
}

func readMultipartFile(t *testing.T, header *multipart.FileHeader) string {
	t.Helper()
	file, err := header.Open()
	require.NoError(t, err)
	defer file.Close()
	content, err := io.ReadAll(file)
	require.NoError(t, err)
	return string(content)
}
