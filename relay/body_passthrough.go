package relay

import (
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func shouldUseClaudeCodeRequestFingerprint(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.ChannelMeta != nil &&
		info.ApiType == constant.APITypeAnthropic &&
		(info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled ||
			info.ChannelOtherSettings.ClaudeCodeTransportFingerprintEnabled)
}

func shouldPassThroughRequestBody(info *relaycommon.RelayInfo) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return true
	}
	return info != nil &&
		info.ChannelMeta != nil &&
		info.ChannelSetting.PassThroughBodyEnabled
}
