package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/require"
)

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

func TestShouldPassThroughRequestBodyAllowsClaudeCodeFingerprintWithClaudePassThrough(t *testing.T) {
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

	require.True(t, shouldPassThroughRequestBody(info))
}

func TestShouldPassThroughRequestBodyAllowsClaudeCodeTransportFingerprintWithClaudePassThrough(t *testing.T) {
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

	require.True(t, shouldPassThroughRequestBody(info))
}

func TestClaudeCodeFingerprintAllowsClaudePassThroughWhenGlobalPassthroughEnabled(t *testing.T) {
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

	require.True(t, shouldPassThroughRequestBody(info))
}

func TestClaudeCodeFingerprintDoesNotPassThroughWhenPassThroughDisabled(t *testing.T) {
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
