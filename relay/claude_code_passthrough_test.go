package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/require"
)

func newClaudeCodePassthroughTestContext(userAgent string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if userAgent != "" {
		ctx.Request.Header.Set("User-Agent", userAgent)
	}
	return ctx
}

func TestShouldPassThroughRequestBodyRespectsPassThroughSettings(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()

	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	require.False(t, shouldPassThroughRequestBody(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}))

	model_setting.GetGlobalSettings().PassThroughRequestEnabled = true
	require.True(t, shouldPassThroughRequestBody(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}))

	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false
	require.True(t, shouldPassThroughRequestBody(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				PassThroughBodyEnabled: true,
			},
		},
	}))
}

func TestShouldPassThroughRequestBodyDoesNotPassThroughSyntheticClaudeCodeFingerprint(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = true

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelSetting: dto.ChannelSettings{
				PassThroughBodyEnabled: true,
			},
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	require.False(t, shouldPassThroughRequestBody(info))
}

func TestShouldPassThroughRequestBodyDoesNotPassThroughSyntheticClaudeCodeTransportFingerprint(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = true

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelSetting: dto.ChannelSettings{
				PassThroughBodyEnabled: true,
			},
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeTransportFingerprintEnabled: true,
			},
		},
	}

	require.False(t, shouldPassThroughRequestBody(info))
}

func TestClaudeCodeFingerprintDoesNotPassThroughSyntheticBodyWhenGlobalPassthroughEnabled(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = true

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	require.False(t, shouldPassThroughRequestBody(info))
}

func TestClaudeCodeFingerprintDoesNotPassThroughSyntheticBodyWhenPassThroughDisabled(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	require.False(t, shouldPassThroughRequestBody(info))
}

func TestClaudeCodeFingerprintAutoPassesThroughRealClaudeCodeClient(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	ctx := newClaudeCodePassthroughTestContext("claude-cli/2.1.156 (Claude Code)")
	require.True(t, shouldPassThroughRequestBodyForContext(ctx, info))
}

func TestRealClaudeCodeClientAutoPassesThroughNonAnthropicClaudeRequest(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	ctx := newClaudeCodePassthroughTestContext("claude-cli/2.1.156 (Claude Code)")
	require.True(t, shouldPassThroughRequestBodyForContext(ctx, info))
}

func TestRealClaudeCodeClientAutoPassesThroughBySessionHeader(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeOpenAI,
		},
	}

	ctx := newClaudeCodePassthroughTestContext("")
	ctx.Request.Header.Set("X-Claude-Code-Session-Id", "session-123")
	require.True(t, shouldPassThroughRequestBodyForContext(ctx, info))
}

func TestClaudeCodeFingerprintDoesNotPassThroughSyntheticBodyForNonClaudeCodeClient(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = false

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	ctx := newClaudeCodePassthroughTestContext("Hermes/1.0")
	require.False(t, shouldPassThroughRequestBodyForContext(ctx, info))
}

func TestClaudeCodeFingerprintDoesNotPassThroughOpenAICompatibleBody(t *testing.T) {
	originalGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = originalGlobal
	}()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = true

	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled: true,
			},
		},
	}

	require.False(t, shouldPassThroughRequestBody(info))
}
