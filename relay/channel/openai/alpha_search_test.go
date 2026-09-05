package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAlphaSearchGatewayURLNormalizesV1Base(t *testing.T) {
	for _, baseURL := range []string{"https://gateway.example", "https://gateway.example/", "https://gateway.example/v1", "https://gateway.example/v1/"} {
		info := &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeAlphaSearch,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeOpenAI,
				ChannelBaseUrl: baseURL,
			},
		}
		got, err := (&Adaptor{}).GetRequestURL(info)
		require.NoError(t, err)
		require.Equal(t, "https://gateway.example/v1/alpha/search", got)
	}
}

func TestAlphaSearchGatewayUsesBearerKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept", "text/event-stream")
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAlphaSearch,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ApiKey:      "gateway-key",
		},
	}
	headers := http.Header{}
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &headers, info))
	require.Equal(t, "Bearer gateway-key", headers.Get("Authorization"))
	require.Empty(t, headers.Get("chatgpt-account-id"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))
	require.Equal(t, "application/json", headers.Get("Accept"))
}
