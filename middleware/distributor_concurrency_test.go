package middleware

import (
	"bytes"
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

func TestSetupContextForSelectedChannelReleasesConcurrencyWhenKeySelectionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	limit := 1
	channel := &model.Channel{
		Id:               990001,
		Key:              "disabled-key",
		ConcurrencyLimit: &limit,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled},
			MultiKeyMode:       constant.MultiKeyModeRandom,
		},
	}
	t.Cleanup(func() {
		model.ReleaseChannelConcurrency(channel.Id)
	})

	err := SetupContextForSelectedChannel(c, channel, "test-model")

	require.NotNil(t, err)
	require.True(t, model.IsChannelConcurrencyAvailable(channel))
}

func TestDistributeSkipsChannelSetupWhenRouteDoesNotSelectChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(Distribute())
	router.GET("/mj/task/:id/fetch", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/mj/task/task_123/fetch", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestGetModelRequestReadsCanvasMultipartImageEditModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "edit image"))
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/canvas/v1/images/edits?group=Image2", bytes.NewReader(body.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	req, shouldSelectChannel, err := getModelRequest(c)

	require.NoError(t, err)
	require.True(t, shouldSelectChannel)
	require.Equal(t, "gpt-image-2", req.Model)
}
