package channel

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type replayTaskAdaptor struct {
	TaskAdaptor
	baseURL string
	request *http.Request
}

func (a *replayTaskAdaptor) BuildRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.baseURL + "/v1/videos", nil
}

func (a *replayTaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	a.request = req
	return nil
}

func TestDoTaskApiRequestPreservesNativeBodyFactory(t *testing.T) {
	service.InitHttpClient()
	payload := []byte(`{"model":"video-model","prompt":"replay all bytes"}`)
	received := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		got, _ := io.ReadAll(req.Body)
		received <- got
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(payload))
	adaptor := &replayTaskAdaptor{baseURL: server.URL}
	resp, err := DoTaskApiRequest(adaptor, c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, bytes.NewReader(payload))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, payload, <-received)

	require.NotNil(t, adaptor.request.GetBody)
	for i := 0; i < 2; i++ {
		replay, err := adaptor.request.GetBody()
		require.NoError(t, err)
		got, err := io.ReadAll(replay)
		require.NoError(t, err)
		require.NoError(t, replay.Close())
		require.Equal(t, payload, got)
	}
}
