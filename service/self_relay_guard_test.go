package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func buildSelfRelayGuardTestContext(host string, headers map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/v1/chat/completions", nil)
	req.Host = host
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	c.Request = req
	return c
}

func withSelfRelayGuardSettings(t *testing.T, serverAddress string, apiInfo string) {
	t.Helper()

	originServerAddress := system_setting.ServerAddress
	originApiInfo := console_setting.GetConsoleSetting().ApiInfo
	system_setting.ServerAddress = serverAddress
	console_setting.GetConsoleSetting().ApiInfo = apiInfo
	t.Cleanup(func() {
		system_setting.ServerAddress = originServerAddress
		console_setting.GetConsoleSetting().ApiInfo = originApiInfo
	})
}

func TestChannelBaseURLMatchesLocalEndpointFromApiInfo(t *testing.T) {
	withSelfRelayGuardSettings(t, "", `[{"url":"https://api.maolaoapi.cc","route":"主线路","description":"公开 API","color":"blue"}]`)

	c := buildSelfRelayGuardTestContext("panel.maolaoapi.cc", nil)

	require.True(t, ChannelBaseURLMatchesLocalEndpoint(c, "https://api.maolaoapi.cc"))
	require.True(t, IsSelfReferentialChannel(c, &model.Channel{BaseURL: common.GetPointer("https://api.maolaoapi.cc/v1")}))
}

func TestChannelBaseURLMatchesLocalEndpointFromServerAddress(t *testing.T) {
	withSelfRelayGuardSettings(t, "https://maolaoapi.com", "")

	c := buildSelfRelayGuardTestContext("panel.maolaoapi.cc", nil)

	require.True(t, ChannelBaseURLMatchesLocalEndpoint(c, "https://maolaoapi.com/v1"))
}

func TestChannelBaseURLMatchesLocalEndpointFromForwardedHost(t *testing.T) {
	withSelfRelayGuardSettings(t, "", "")

	c := buildSelfRelayGuardTestContext("internal:3000", map[string]string{
		"X-Forwarded-Host": "api.maolaoapi.cc, proxy.local",
	})

	require.True(t, ChannelBaseURLMatchesLocalEndpoint(c, "https://api.maolaoapi.cc"))
}

func TestValidateChannelBaseURLNotSelfAllowsExternalEndpoint(t *testing.T) {
	withSelfRelayGuardSettings(t, "https://maolaoapi.com", `[{"url":"https://api.maolaoapi.cc","route":"主线路","description":"公开 API","color":"blue"}]`)

	c := buildSelfRelayGuardTestContext("panel.maolaoapi.cc", nil)

	err := ValidateChannelBaseURLNotSelf(c, &model.Channel{BaseURL: common.GetPointer("https://api.openai.com/v1")})
	require.NoError(t, err)
}

func TestValidateChannelBaseURLNotSelfRejectsLocalEndpoint(t *testing.T) {
	withSelfRelayGuardSettings(t, "", `[{"url":"https://api.maolaoapi.cc","route":"主线路","description":"公开 API","color":"blue"}]`)

	c := buildSelfRelayGuardTestContext("panel.maolaoapi.cc", nil)

	err := ValidateChannelBaseURLNotSelf(c, &model.Channel{BaseURL: common.GetPointer("https://api.maolaoapi.cc")})
	require.ErrorContains(t, err, "渠道上游地址不能指向当前站点")
}

func TestExcludeSelfReferentialChannelAddsRequestExclusion(t *testing.T) {
	withSelfRelayGuardSettings(t, "", `[{"url":"https://api.maolaoapi.cc","route":"主线路","description":"公开 API","color":"blue"}]`)

	c := buildSelfRelayGuardTestContext("panel.maolaoapi.cc", nil)
	param := &RetryParam{Ctx: c}
	channel := &model.Channel{Id: 384, BaseURL: common.GetPointer("https://api.maolaoapi.cc")}

	require.True(t, excludeSelfReferentialChannel(param, channel, "default"))
	_, excluded := param.ExcludedChannelIDs[384]
	require.True(t, excluded)
}
