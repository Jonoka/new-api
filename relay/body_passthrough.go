package relay

import (
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var claudeCodeRequestUserAgentPattern = regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`)

func shouldPassThroughRequestBody(info *relaycommon.RelayInfo) bool {
	return shouldPassThroughRequestBodyForContext(nil, info)
}

func shouldPassThroughRequestBodyForContext(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if shouldPassThroughRealClaudeCodeRequest(c, info) {
		return true
	}
	if shouldSynthesizeClaudeCodeBodyForCompatibleClient(c, info) {
		return false
	}
	if shouldConvertRequestBodyBeforeAnthropicUpstream(info) {
		return false
	}
	return isRequestBodyPassThroughSettingEnabled(info)
}

func shouldSynthesizeClaudeCodeBodyForCompatibleClient(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.ChannelMeta != nil &&
		info.ApiType == constant.APITypeAnthropic &&
		info.RelayFormat == types.RelayFormatClaude &&
		(info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled ||
			info.ChannelOtherSettings.ClaudeCodeTransportFingerprintEnabled) &&
		!isRealClaudeCodeRequest(c)
}

func shouldConvertRequestBodyBeforeAnthropicUpstream(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.ChannelMeta != nil &&
		info.ApiType == constant.APITypeAnthropic &&
		info.RelayFormat != types.RelayFormatClaude
}

func shouldPassThroughRealClaudeCodeRequest(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return isClaudeNativeRequest(info) && isRealClaudeCodeRequest(c)
}

func isClaudeNativeRequest(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.RelayFormat == types.RelayFormatClaude
}

func isRealClaudeCodeRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	userAgent := c.Request.Header.Get("User-Agent")
	if claudeCodeRequestUserAgentPattern.MatchString(userAgent) {
		return true
	}
	xApp := c.Request.Header.Get("X-App")
	if strings.EqualFold(xApp, "claude-code") {
		return true
	}
	if strings.EqualFold(xApp, "cli") &&
		c.Request.Header.Get("X-Stainless-Package-Version") != "" &&
		strings.EqualFold(c.Request.Header.Get("X-Stainless-Lang"), "js") {
		return true
	}
	return c.Request.Header.Get("X-Claude-Code-Session-Id") != "" &&
		strings.TrimSpace(userAgent) == "" &&
		strings.TrimSpace(xApp) == ""
}

func isRequestBodyPassThroughSettingEnabled(info *relaycommon.RelayInfo) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return true
	}
	return info != nil &&
		info.ChannelMeta != nil &&
		info.ChannelSetting.PassThroughBodyEnabled
}
