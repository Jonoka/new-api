package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageEditUpload struct {
	field       string
	contentType string
	data        []byte
}

func newGeminiImageEditContext(t *testing.T, uploads ...imageEditUpload) *gin.Context {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gemini-3-pro-image-preview"))
	require.NoError(t, writer.WriteField("prompt", "preserve the source image"))
	for i, upload := range uploads {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="image-%d"`, upload.field, i))
		header.Set("Content-Type", upload.contentType)
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(upload.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(1<<20))
	t.Cleanup(func() { _ = req.MultipartForm.RemoveAll() })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

func geminiImageEditInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3-pro-image-preview",
		},
	}
}

func TestGeminiImageEditCollectsMultipartImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pngA := []byte("\x89PNG\r\n\x1a\nfirst")
	pngB := []byte("\x89PNG\r\n\x1a\nsecond")

	tests := []struct {
		name    string
		uploads []imageEditUpload
		want    [][]byte
	}{
		{
			name:    "image",
			uploads: []imageEditUpload{{field: "image", contentType: "application/octet-stream", data: pngA}},
			want:    [][]byte{pngA},
		},
		{
			name:    "image array",
			uploads: []imageEditUpload{{field: "image[]", contentType: "image/png", data: pngA}},
			want:    [][]byte{pngA},
		},
		{
			name: "indexed images",
			uploads: []imageEditUpload{
				{field: "image[1]", contentType: "image/png", data: pngB},
				{field: "image[0]", contentType: "image/png", data: pngA},
			},
			want: [][]byte{pngA, pngB},
		},
		{
			name: "image takes precedence over aliases",
			uploads: []imageEditUpload{
				{field: "image[0]", contentType: "image/png", data: pngB},
				{field: "image[]", contentType: "image/png", data: pngB},
				{field: "image", contentType: "image/png", data: pngA},
			},
			want: [][]byte{pngA},
		},
		{
			name: "image array takes precedence over indexed aliases",
			uploads: []imageEditUpload{
				{field: "image[0]", contentType: "image/png", data: pngB},
				{field: "image[]", contentType: "image/png", data: pngA},
			},
			want: [][]byte{pngA},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted, err := (&Adaptor{}).ConvertImageRequest(
				newGeminiImageEditContext(t, tt.uploads...),
				geminiImageEditInfo(),
				dto.ImageRequest{Prompt: "preserve the source image"},
			)
			require.NoError(t, err)

			req, ok := converted.(dto.GeminiChatRequest)
			require.True(t, ok)
			parts := req.Contents[0].Parts
			require.Len(t, parts, len(tt.want)+1)
			require.Equal(t, "preserve the source image", parts[0].Text)
			for i, want := range tt.want {
				require.NotNil(t, parts[i+1].InlineData)
				require.Equal(t, "image/png", parts[i+1].InlineData.MimeType)
				got, err := base64.StdEncoding.DecodeString(parts[i+1].InlineData.Data)
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		})
	}
}

func TestGeminiImageEditRejectsMissingOrInvalidImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	_, err := (&Adaptor{}).ConvertImageRequest(
		newGeminiImageEditContext(t),
		geminiImageEditInfo(),
		dto.ImageRequest{Prompt: "edit"},
	)
	require.EqualError(t, err, "image is required for image edit")

	_, err = (&Adaptor{}).ConvertImageRequest(
		newGeminiImageEditContext(t, imageEditUpload{field: "image", contentType: "text/plain", data: []byte("not an image")}),
		geminiImageEditInfo(),
		dto.ImageRequest{Prompt: "edit"},
	)
	require.ErrorContains(t, err, "unsupported content type")

	_, err = (&Adaptor{}).ConvertImageRequest(
		newGeminiImageEditContext(t, imageEditUpload{field: "image", contentType: "image/png"}),
		geminiImageEditInfo(),
		dto.ImageRequest{Prompt: "edit"},
	)
	require.ErrorContains(t, err, "is empty")
}

func TestGeminiImageEditPreservesScalarImageInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	png := []byte("\x89PNG\r\n\x1a\nscalar")
	image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	converted, err := (&Adaptor{}).ConvertImageRequest(c, geminiImageEditInfo(), dto.ImageRequest{
		Prompt: "edit",
		Image:  json.RawMessage(fmt.Sprintf("%q", image)),
	})
	require.NoError(t, err)

	req := converted.(dto.GeminiChatRequest)
	require.Len(t, req.Contents[0].Parts, 2)
	require.Equal(t, "image/png", req.Contents[0].Parts[1].InlineData.MimeType)
	got, err := base64.StdEncoding.DecodeString(req.Contents[0].Parts[1].InlineData.Data)
	require.NoError(t, err)
	require.Equal(t, png, got)
}

func TestGeminiSetupRequestHeaderUsesJSONForMultipartIngress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=inbound")
	info := geminiImageEditInfo()
	info.ApiKey = "test-key"
	header := make(http.Header)

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	require.Equal(t, "application/json", header.Get("Content-Type"))
	require.Equal(t, "test-key", header.Get("x-goog-api-key"))
	require.Equal(t, "multipart/form-data; boundary=inbound", c.Request.Header.Get("Content-Type"))
}
