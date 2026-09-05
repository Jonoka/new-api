package codex

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCodexAlphaSearchRequestURL(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAlphaSearch,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeCodex,
			ChannelBaseUrl: "https://chatgpt.com",
		},
	}
	got, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://chatgpt.com/backend-api/codex/alpha/search", got)
}

func TestCodexAlphaSearchUsesOAuthHeadersAndJSON(t *testing.T) {
	c := newCodexHeaderTestContext()
	c.Request.URL.Path = "/v1/alpha/search"
	c.Request.Header.Set("Accept", "text/event-stream")
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAlphaSearch,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: `{"access_token":"access-token","account_id":"account-id"}`,
		},
	}
	headers := http.Header{}
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &headers, info))
	require.Equal(t, "Bearer access-token", headers.Get("Authorization"))
	require.Equal(t, "account-id", headers.Get("chatgpt-account-id"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))
	require.Equal(t, "application/json", headers.Get("Accept"))
	require.Equal(t, defaultOpenAIBetaHeaderValue, headers.Get("OpenAI-Beta"))
	require.Equal(t, defaultCodexOriginatorHeaderValue, headers.Get("Originator"))
}

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
	require.Empty(t, headers.Get("X-Codex-Beta-Features"))
}

func TestCodexConvertOpenAIResponsesRequestPreservesCodexClientFields(t *testing.T) {
	info := &relaycommon.RelayInfo{
		IsStream:  true,
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{},
		},
	}
	req := dto.OpenAIResponsesRequest{
		Model:          "gpt-5-codex",
		Input:          json.RawMessage(`"hello"`),
		ClientMetadata: json.RawMessage(`{"thread_id":"thread-123","turn_id":"turn-123"}`),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, req)
	require.NoError(t, err)

	out, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.NotNil(t, out.Stream)
	require.True(t, *out.Stream)
	require.JSONEq(t, `{"thread_id":"thread-123","turn_id":"turn-123"}`, string(out.ClientMetadata))
	require.JSONEq(t, `""`, string(out.Instructions))
	require.JSONEq(t, `false`, string(out.Store))
}
