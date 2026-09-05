package relay

import (
	"io"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/claude"
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
		info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled &&
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

func buildClaudeCodeAwarePassthroughBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, int64, func() (io.ReadCloser, error), io.Closer, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, 0, nil, nil, err
	}
	if shouldPassThroughRealClaudeCodeRequest(c, info) {
		return common.ReaderOnly(storage), storage.Size(), storage.NewReader, nil, nil
	}
	if !shouldApplyClaudeCodePassthroughBodyFingerprint(info) {
		return common.ReaderOnly(storage), storage.Size(), storage.NewReader, nil, nil
	}
	bodyBytes, err := storage.Bytes()
	if err != nil {
		return nil, 0, nil, nil, err
	}
	jsonData, err := claude.ApplyClaudeCodePassthroughBodyFingerprint(info, bodyBytes)
	if err != nil {
		return nil, 0, nil, nil, err
	}
	if len(jsonData) == len(bodyBytes) && string(jsonData) == string(bodyBytes) {
		return common.ReaderOnly(storage), storage.Size(), storage.NewReader, nil, nil
	}
	return relaycommon.NewOutboundJSONBody(jsonData)
}

func shouldApplyClaudeCodePassthroughBodyFingerprint(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.ChannelMeta != nil &&
		info.ApiType == constant.APITypeAnthropic &&
		info.GetFinalRequestRelayFormat() == types.RelayFormatClaude &&
		(info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled ||
			info.ChannelOtherSettings.ClaudeCodeTransportFingerprintEnabled)
}
