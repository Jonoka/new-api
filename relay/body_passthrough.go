package relay

import (
	"regexp"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var claudeCodeRequestUserAgentPattern = regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`)

func shouldUseClaudeCodeRequestFingerprint(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.ChannelMeta != nil &&
		info.ApiType == constant.APITypeAnthropic &&
		(info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled ||
			info.ChannelOtherSettings.ClaudeCodeTransportFingerprintEnabled)
}

func shouldPassThroughRequestBody(info *relaycommon.RelayInfo) bool {
	return shouldPassThroughRequestBodyForContext(nil, info)
}

func shouldPassThroughRequestBodyForContext(c *gin.Context, info *relaycommon.RelayInfo) bool {
	passThroughEnabled := isRequestBodyPassThroughSettingEnabled(info)
	if shouldUseClaudeCodeRequestFingerprint(info) {
		return shouldPassThroughClaudeCodeOriginalRequest(c, info, passThroughEnabled)
	}
	return passThroughEnabled
}

func shouldPassThroughClaudeCodeOriginalRequest(c *gin.Context, info *relaycommon.RelayInfo, passThroughEnabled bool) bool {
	if info == nil || info.RelayFormat != types.RelayFormatClaude {
		return false
	}
	if passThroughEnabled {
		return true
	}
	return isRealClaudeCodeRequest(c)
}

func isRealClaudeCodeRequest(c *gin.Context) bool {
	return c != nil &&
		c.Request != nil &&
		claudeCodeRequestUserAgentPattern.MatchString(c.Request.Header.Get("User-Agent"))
}

func isRequestBodyPassThroughSettingEnabled(info *relaycommon.RelayInfo) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return true
	}
	return info != nil &&
		info.ChannelMeta != nil &&
		info.ChannelSetting.PassThroughBodyEnabled
}
