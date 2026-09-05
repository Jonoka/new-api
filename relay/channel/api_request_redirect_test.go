package channel

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestAlphaRedirectPolicyIsScopedAndPreservesClientSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/redirect" {
			http.Redirect(w, req, "/success", http.StatusTemporaryRedirect)
			return
		}
		_, _ = io.WriteString(w, "followed")
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	base := server.Client()
	base.Timeout = 3 * time.Second
	base.Jar = jar

	ordinary := applyRelayRedirectPolicy(base, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses})
	require.Same(t, base, ordinary)
	ordinaryResp, err := ordinary.Get(server.URL + "/redirect")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, ordinaryResp.StatusCode)
	require.NoError(t, ordinaryResp.Body.Close())

	alpha := applyRelayRedirectPolicy(base, &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAlphaSearch})
	require.NotSame(t, base, alpha)
	require.Same(t, base.Transport, alpha.Transport)
	require.Same(t, base.Jar, alpha.Jar)
	require.Equal(t, base.Timeout, alpha.Timeout)
	require.Nil(t, base.CheckRedirect)
	require.NotNil(t, alpha.CheckRedirect)

	alphaResp, err := alpha.Get(server.URL + "/redirect")
	require.NoError(t, err)
	defer alphaResp.Body.Close()
	require.Equal(t, http.StatusTemporaryRedirect, alphaResp.StatusCode)
	require.Equal(t, "/success", alphaResp.Header.Get("Location"))
}
