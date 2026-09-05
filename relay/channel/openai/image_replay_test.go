package openai

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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageMultipartFinalBodyKeepsNativeReplayFactory(t *testing.T) {
	var inbound bytes.Buffer
	writer := multipart.NewWriter(&inbound)
	require.NoError(t, writer.WriteField("model", "public-model"))
	require.NoError(t, writer.WriteField("prompt", "edit this"))
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("image-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(inbound.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mapped-model",
		},
	}, dto.ImageRequest{Model: "mapped-model", Prompt: "edit this"})
	require.NoError(t, err)
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}

	body, ok := converted.(io.Reader)
	require.True(t, ok)
	req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/images/edits", body)
	require.NoError(t, err)
	require.NotNil(t, req.GetBody)
	require.Greater(t, req.ContentLength, int64(0))

	first, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	replay, err := req.GetBody()
	require.NoError(t, err)
	replayed, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.NoError(t, replay.Close())
	require.Equal(t, first, replayed)
	require.EqualValues(t, len(first), req.ContentLength)
}
