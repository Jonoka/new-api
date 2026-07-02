package claude

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Adaptor struct {
}

const (
	claudeCodeSystemText               = "You are Claude Code, Anthropic's official CLI for Claude."
	claudeCodeAnthropicBeta            = "interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14,claude-code-20250219,oauth-2025-04-20,context-management-2025-06-27,extended-cache-ttl-2025-04-11,prompt-caching-scope-2026-01-05"
	claudeCodeUserAgentFallbackVersion = "2.1.169"
	claudeCodeStainlessVersion         = "0.94.0"
	claudeCodeStainlessRuntime         = "node"
	claudeCodeStainlessRuntimeVer      = "v24.13.0"
	claudeCodeStainlessRetryCount      = "0"
	claudeCodeStainlessTimeoutSecs     = "600"
	billingFingerprintSalt             = "59cf53e54c78"
	cchSeed                            = uint64(0x6E52736AC806831E)
)

var claudeCodeUserAgentPattern = regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`)

var realClaudeCodeHeaderPassthroughNames = []string{
	"User-Agent",
	"X-App",
	"anthropic-version",
	"anthropic-beta",
	"anthropic-dangerous-direct-browser-access",
	"X-Client-Request-Id",
	"X-Claude-Code-Session-Id",
}

func getClaudeCodeVersion(info *relaycommon.RelayInfo) string {
	if info != nil && strings.TrimSpace(info.ChannelOtherSettings.ClaudeCodeVersion) != "" {
		return strings.TrimSpace(info.ChannelOtherSettings.ClaudeCodeVersion)
	}
	return claudeCodeUserAgentFallbackVersion
}

func mapStainlessOS(goos string) string {
	switch goos {
	case "linux":
		return "Linux"
	case "darwin":
		return "MacOS"
	case "windows":
		return "Windows"
	default:
		return "Linux"
	}
}

func mapStainlessArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return "x64"
	}
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if err := applyClaudeCodeRequestFingerprint(c, info, request); err != nil {
		return nil, err
	}
	if info.ReasoningEffort == "" {
		info.ReasoningEffort = extractClaudeThinkingEffort(request)
	}
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	requestURL := fmt.Sprintf("%s/v1/messages", info.ChannelBaseUrl)
	if !shouldAppendClaudeBetaQuery(info) {
		return requestURL, nil
	}

	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	query := parsedURL.Query()
	query.Set("beta", "true")
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

func shouldAppendClaudeBetaQuery(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if info.IsClaudeBetaQuery {
		return true
	}
	if info.ChannelOtherSettings.ClaudeBetaQuery {
		return true
	}
	return false
}

func CommonClaudeHeadersOperation(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	// common headers operation
	anthropicBeta := c.Request.Header.Get("anthropic-beta")
	if anthropicBeta != "" {
		req.Set("anthropic-beta", anthropicBeta)
	}
	model_setting.GetClaudeSettings().WriteHeaders(info.OriginModelName, req)
}

func shouldUseClaudeCodeFingerprint(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		(info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled ||
			info.ChannelOtherSettings.ClaudeCodeTransportFingerprintEnabled)
}

func shouldUseClaudeCodeOriginalPassThrough(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil || info.RelayFormat != types.RelayFormatClaude {
		return false
	}
	return isRealClaudeCodeClient(c)
}

func isRealClaudeCodeClient(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	userAgent := c.Request.Header.Get("User-Agent")
	if claudeCodeUserAgentPattern.MatchString(userAgent) {
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

func applyClaudeCodeHeaderFingerprint(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	if req == nil || !shouldApplyClaudeCodeSyntheticFingerprint(c, info) || shouldUseClaudeCodeOriginalPassThrough(c, info) {
		return
	}
	req.Set("User-Agent", fmt.Sprintf("claude-cli/%s (external, cli)", getClaudeCodeVersion(info)))
	req.Set("X-Stainless-Lang", "js")
	req.Set("X-Stainless-Package-Version", claudeCodeStainlessVersion)
	req.Set("X-Stainless-OS", mapStainlessOS(runtime.GOOS))
	req.Set("X-Stainless-Arch", mapStainlessArch(runtime.GOARCH))
	req.Set("X-Stainless-Runtime", claudeCodeStainlessRuntime)
	req.Set("X-Stainless-Runtime-Version", claudeCodeStainlessRuntimeVer)
	req.Set("X-Stainless-Retry-Count", claudeCodeStainlessRetryCount)
	req.Set("X-Stainless-Timeout", claudeCodeStainlessTimeoutSecs)
	req.Set("X-App", "cli")
	req.Set("anthropic-version", "2023-06-01")
	req.Set("anthropic-beta", claudeCodeAnthropicBeta)
	req.Set("anthropic-dangerous-direct-browser-access", "true")
	req.Set("x-client-request-id", uuid.NewString())
	if info.ApiKey != "" {
		req.Set("Authorization", "Bearer "+info.ApiKey)
	}
}

func applyRealClaudeCodeHeaderPassthrough(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	if req == nil || !shouldPassThroughRealClaudeCodeHeaders(c, info) {
		return
	}
	for _, name := range realClaudeCodeHeaderPassthroughNames {
		passIncomingHeader(c.Request.Header, req, name)
	}
	for name := range c.Request.Header {
		lowerName := strings.ToLower(name)
		if strings.HasPrefix(lowerName, "anthropic-") ||
			strings.HasPrefix(lowerName, "x-stainless-") ||
			strings.HasPrefix(lowerName, "x-claude-") ||
			strings.HasPrefix(lowerName, "x-client-") {
			passIncomingHeader(c.Request.Header, req, name)
		}
	}
}

func shouldPassThroughRealClaudeCodeHeaders(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if c == nil || c.Request == nil || info == nil || info.ChannelMeta == nil {
		return false
	}
	if info.RelayFormat != types.RelayFormatClaude {
		return false
	}
	if shouldUseClaudeCodeFingerprint(info) && !shouldUseClaudeCodeOriginalPassThrough(c, info) {
		return false
	}
	return isRealClaudeCodeClient(c)
}

func passIncomingHeader(src http.Header, dst *http.Header, name string) {
	value := strings.TrimSpace(src.Get(name))
	if value == "" {
		return
	}
	dst.Set(name, value)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-api-key", info.ApiKey)
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Set("anthropic-version", anthropicVersion)
	if shouldUseClaudeCodeOriginalPassThrough(c, info) {
		applyRealClaudeCodeHeaderPassthrough(c, req, info)
		if req.Get("anthropic-version") == "" {
			req.Set("anthropic-version", "2023-06-01")
		}
		return nil
	}
	CommonClaudeHeadersOperation(c, req, info)
	applyClaudeCodeHeaderFingerprint(c, req, info)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	claudeRequest, err := RequestOpenAI2ClaudeMessage(c, *request)
	if err != nil {
		return nil, err
	}
	if err := applyClaudeCodeRequestFingerprint(c, info, claudeRequest); err != nil {
		return nil, err
	}
	return claudeRequest, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	if info.IsStream {
		return ClaudeStreamHandler(c, resp, info)
	} else {
		return ClaudeHandler(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

const (
	claudeCodeUserDeviceID  = "0000000000000000000000000000000000000000000000000000000000000000"
	claudeCodeUserSessionID = "00000000-0000-0000-0000-000000000000"
	claudeCodeUserID        = "user_" + claudeCodeUserDeviceID + "_account__session_" + claudeCodeUserSessionID
)

var claudeCodeLegacySub2APIUserIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[\w-]+$`)

func shouldApplyClaudeCodeSyntheticFingerprint(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return shouldUseClaudeCodeFingerprint(info) && isRealClaudeCodeClient(c)
}

func applyClaudeCodeRequestFingerprint(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) error {
	if request == nil || !shouldApplyClaudeCodeSyntheticFingerprint(c, info) {
		return nil
	}
	ensureClaudeCodeSystem(request, info)
	return ensureClaudeCodeMetadata(request)
}

func ensureClaudeCodeMetadata(request *dto.ClaudeRequest) error {
	metadata := make(map[string]interface{})
	if len(request.Metadata) > 0 {
		if err := common.Unmarshal(request.Metadata, &metadata); err != nil {
			return err
		}
	}
	userID := strings.TrimSpace(common.Interface2String(metadata["user_id"]))
	if userID == "" || !claudeCodeLegacySub2APIUserIDPattern.MatchString(userID) {
		metadata["user_id"] = claudeCodeUserID
	}
	metadataBytes, err := common.Marshal(metadata)
	if err != nil {
		return err
	}
	request.Metadata = metadataBytes
	return nil
}

func ensureClaudeCodeSystem(request *dto.ClaudeRequest, info *relaycommon.RelayInfo) {
	version := getClaudeCodeVersion(info)

	// Build billing attribution block (no cache_control)
	billingText := buildBillingBlockText(request, version)
	billingBlock := dto.ClaudeMediaMessage{Type: "text"}
	billingBlock.SetText(billingText)

	// Build Claude Code prompt block (with cache_control: ephemeral)
	ccBlock := dto.ClaudeMediaMessage{Type: "text", CacheControl: json.RawMessage(`{"type":"ephemeral"}`)}
	ccBlock.SetText(claudeCodeSystemText)

	// Always set system to [billing, cc_prompt] — original system is discarded
	// from system field (sub2api's mimicry moves it to messages if needed)
	request.System = []dto.ClaudeMediaMessage{billingBlock, ccBlock}
}

func newClaudeCodeSystemBlock() dto.ClaudeMediaMessage {
	return newTextSystemBlock(claudeCodeSystemText)
}

func newTextSystemBlock(text string) dto.ClaudeMediaMessage {
	block := dto.ClaudeMediaMessage{Type: "text"}
	block.SetText(text)
	return block
}

// buildBillingBlockText constructs the billing attribution header text
// matching real Claude Code CLI format.
func buildBillingBlockText(request *dto.ClaudeRequest, version string) string {
	fp := computeBillingFingerprint(request, version)
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=cli; cch=00000;", version, fp)
}

// computeBillingFingerprint replicates the real Claude Code CLI fingerprint algorithm:
// 1. Take the first user message's text content
// 2. Extract characters at positions 4, 7, 20 (pad with '0' if shorter)
// 3. SHA256(salt + chars + version), take first 3 hex chars
func computeBillingFingerprint(request *dto.ClaudeRequest, version string) string {
	firstText := extractFirstUserText(request)
	indices := []int{4, 7, 20}
	chars := make([]byte, 0, 3)
	for _, i := range indices {
		if i < len(firstText) {
			chars = append(chars, firstText[i])
		} else {
			chars = append(chars, '0')
		}
	}
	sum := sha256.Sum256([]byte(billingFingerprintSalt + string(chars) + version))
	return hex.EncodeToString(sum[:])[:3]
}

// extractFirstUserText extracts the text content from the first user message.
func extractFirstUserText(request *dto.ClaudeRequest) string {
	if request == nil {
		return ""
	}
	for _, msg := range request.Messages {
		if msg.Role != "user" {
			continue
		}
		switch content := msg.Content.(type) {
		case string:
			return content
		case []interface{}:
			for _, block := range content {
				blockMap, ok := block.(map[string]interface{})
				if !ok {
					continue
				}
				if blockMap["type"] == "text" {
					if text, ok := blockMap["text"].(string); ok {
						return text
					}
				}
			}
		}
		break
	}
	return ""
}

// SignBillingHeaderCCH replaces the cch=00000 placeholder in the serialized body
// with an xxHash64-based signature. Must be called after Marshal.
func SignBillingHeaderCCH(body []byte) []byte {
	placeholder := []byte("cch=00000;")
	idx := bytes.Index(body, placeholder)
	if idx < 0 {
		return body
	}
	h := xxhash.NewWithSeed(cchSeed)
	_, _ = h.Write(body)
	cch := fmt.Sprintf("cch=%05x;", h.Sum64()&0xFFFFF)
	result := make([]byte, len(body))
	copy(result, body)
	copy(result[idx:], []byte(cch))
	return result
}

// ApplyClaudeCodeFinalBodyFingerprint 在下游 body 修改后重新写入 Claude Code 指纹字段，
// 并对最终序列化 body 签名。
func ApplyClaudeCodeFinalBodyFingerprint(info *relaycommon.RelayInfo, body []byte) ([]byte, error) {
	return body, nil
}

func normalizeClaudeSystemBlocks(value any) []dto.ClaudeMediaMessage {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []dto.ClaudeMediaMessage{newTextSystemBlock(v)}
	case dto.ClaudeMediaMessage:
		if block, ok := normalizeClaudeSystemBlock(v); ok {
			return []dto.ClaudeMediaMessage{block}
		}
		return nil
	case []dto.ClaudeMediaMessage:
		blocks := make([]dto.ClaudeMediaMessage, 0, len(v))
		for _, item := range v {
			if block, ok := normalizeClaudeSystemBlock(item); ok {
				blocks = append(blocks, block)
			}
		}
		return blocks
	case []interface{}:
		blocks := make([]dto.ClaudeMediaMessage, 0, len(v))
		for _, item := range v {
			blocks = append(blocks, normalizeClaudeSystemBlocks(item)...)
		}
		return blocks
	case map[string]interface{}:
		block, err := common.Any2Type[dto.ClaudeMediaMessage](v)
		if err != nil {
			return nil
		}
		if normalized, ok := normalizeClaudeSystemBlock(block); ok {
			return []dto.ClaudeMediaMessage{normalized}
		}
	}

	blocks, err := common.Any2Type[[]dto.ClaudeMediaMessage](value)
	if err == nil {
		return normalizeClaudeSystemBlocks(blocks)
	}
	block, err := common.Any2Type[dto.ClaudeMediaMessage](value)
	if err != nil {
		return nil
	}
	if normalized, ok := normalizeClaudeSystemBlock(block); ok {
		return []dto.ClaudeMediaMessage{normalized}
	}
	return nil
}

func normalizeClaudeSystemBlock(block dto.ClaudeMediaMessage) (dto.ClaudeMediaMessage, bool) {
	if strings.TrimSpace(block.Type) == "" && strings.TrimSpace(block.GetText()) != "" {
		block.Type = dto.ContentTypeText
	}
	if block.Type == dto.ContentTypeText && strings.TrimSpace(block.GetText()) == "" {
		if content, ok := block.Content.(string); ok {
			content = strings.TrimSpace(content)
			if content != "" {
				block.SetText(content)
				block.Content = nil
			}
		}
	}
	if strings.TrimSpace(block.Type) == "" && strings.TrimSpace(block.GetText()) != "" {
		block.Type = dto.ContentTypeText
	}
	if strings.TrimSpace(block.Type) == "" && strings.TrimSpace(block.GetText()) == "" {
		return dto.ClaudeMediaMessage{}, false
	}
	return block, true
}

func containsClaudeCodeTextBlock(blocks []dto.ClaudeMediaMessage) bool {
	for _, block := range blocks {
		if block.Type == dto.ContentTypeText && containsClaudeCodeMarker(block.GetText()) {
			return true
		}
	}
	return false
}

func containsClaudeCodeMarker(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.Contains(strings.ToLower(v), "claude code")
	case []dto.ClaudeMediaMessage:
		for _, item := range v {
			if containsClaudeCodeMarker(item.GetText()) ||
				containsClaudeCodeMarker(item.Content) ||
				containsClaudeCodeMarker(item.Input) {
				return true
			}
		}
	case []interface{}:
		for _, item := range v {
			if containsClaudeCodeMarker(item) {
				return true
			}
		}
	case map[string]interface{}:
		for _, item := range v {
			if containsClaudeCodeMarker(item) {
				return true
			}
		}
	}
	return false
}

func extractClaudeThinkingEffort(request *dto.ClaudeRequest) string {
	if effort := request.GetEfforts(); effort != "" {
		return effort
	}
	if request.Thinking != nil {
		if request.Thinking.Type == "disabled" {
			return ""
		}
		if budget := request.Thinking.GetBudgetTokens(); budget > 0 {
			return fmt.Sprintf("thinking:%d", budget)
		}
		if request.Thinking.Type == "enabled" || request.Thinking.Type == "adaptive" {
			return "thinking"
		}
	}
	return ""
}
