package codex

import (
	"net/http"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCodexHeaderTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request, _ = http.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func TestCodexSetupRequestHeaderPreservesClientStreamingHeaders(t *testing.T) {
	c := newCodexHeaderTestContext()
	c.Request.Header.Set("Accept", "text/event-stream")
	c.Request.Header.Set("OpenAI-Beta", "client-beta")
	c.Request.Header.Set("Originator", "Codex CLI")
	c.Request.Header.Set("Session_id", "sess-123")
	c.Request.Header.Set("User-Agent", "codex-test/1.0")
	c.Request.Header.Set("X-Codex-Beta-Features", "client-feature")
	c.Request.Header.Set("X-Codex-Turn-Metadata", "turn-meta")
	c.Request.Header.Set("X-Codex-Installation-Id", "install-123")

	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: `{"access_token":"access-token","account_id":"account-id"}`,
		},
	}

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(c, &headers, info)
	require.NoError(t, err)

	require.Equal(t, "Bearer access-token", headers.Get("Authorization"))
	require.Equal(t, "account-id", headers.Get("chatgpt-account-id"))
	require.Equal(t, "text/event-stream", headers.Get("Accept"))
	require.Equal(t, "client-beta", headers.Get("OpenAI-Beta"))
	require.Equal(t, "Codex CLI", headers.Get("Originator"))
	require.Equal(t, "sess-123", headers.Get("Session_id"))
	require.Equal(t, "codex-test/1.0", headers.Get("User-Agent"))
	require.Equal(t, "client-feature", headers.Get("X-Codex-Beta-Features"))
	require.Equal(t, "turn-meta", headers.Get("X-Codex-Turn-Metadata"))
	require.Equal(t, "install-123", headers.Get("X-Codex-Installation-Id"))
}

func TestCodexSetupRequestHeaderUsesCurrentCodexDefaults(t *testing.T) {
	c := newCodexHeaderTestContext()
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: `{"access_token":"access-token","account_id":"account-id"}`,
		},
	}

	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(c, &headers, info)
	require.NoError(t, err)

	require.Equal(t, "text/event-stream", headers.Get("Accept"))
	require.Equal(t, defaultOpenAIBetaHeaderValue, headers.Get("OpenAI-Beta"))
	require.Equal(t, defaultCodexOriginatorHeaderValue, headers.Get("Originator"))
	require.Equal(t, defaultCodexBetaFeaturesValue, headers.Get("X-Codex-Beta-Features"))
}
