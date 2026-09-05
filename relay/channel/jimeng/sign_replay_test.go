package jimeng

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSignReopensReplayableBodyAfterStreamingHash(t *testing.T) {
	payload := []byte(`{"model":"jimeng-model","prompt":"large replayable body"}`)
	body, size, factory, owner, err := relaycommon.NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer owner.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	req, err := http.NewRequest(http.MethodPost, "https://example.com/?Action=CVProcess", body)
	require.NoError(t, err)
	req.ContentLength = size
	factoryCalled := false
	req.GetBody = func() (io.ReadCloser, error) {
		factoryCalled = true
		return factory()
	}

	require.NoError(t, Sign(c, req, "access|secret"))
	require.True(t, factoryCalled)
	got, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())
	require.Equal(t, payload, got)
}
