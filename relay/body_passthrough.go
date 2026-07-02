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
	if shouldPassThroughRealClaudeCodeRequest(c, info) {
		return true
	}
	if shouldUseClaudeCodeRequestFingerprint(info) {
		return shouldPassThroughClaudeCodeOriginalRequest(c, info, passThroughEnabled)
	}
	return passThroughEnabled
}

func shouldPassThroughClaudeCodeOriginalRequest(c *gin.Context, info *relaycommon.RelayInfo, passThroughEnabled bool) bool {
	if !isClaudeNativeRequest(info) {
		return false
	}
	if isRealClaudeCodeRequest(c) {
		return true
	}
	if shouldUseClaudeCodeRequestFingerprint(info) {
		return false
	}
	return passThroughEnabled
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
