package claude

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

const (
	claudeCodeSystemText    = "You are Claude Code, Anthropic's official CLI for Claude."
	claudeCodeUserID        = "user_0000000000000000000000000000000000000000000000000000000000000000_account_00000000-0000-0000-0000-000000000000_session_00000000-0000-0000-0000-000000000000"
	claudeCodeAnthropicBeta = "claude-code-20250219,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advisor-tool-2026-03-01"
)

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if err := applyClaudeCodeRequestFingerprint(info, request); err != nil {
		return nil, err
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
	return info != nil && info.ChannelOtherSettings.ClaudeCodeFingerprintEnabled
}

func applyClaudeCodeHeaderFingerprint(req *http.Header, info *relaycommon.RelayInfo) {
	if req == nil || !shouldUseClaudeCodeFingerprint(info) {
		return
	}
	req.Set("User-Agent", "claude-cli/2.1.114 (external, sdk-cli)")
	req.Set("X-App", "cli")
	req.Set("anthropic-version", "2023-06-01")
	req.Set("anthropic-beta", claudeCodeAnthropicBeta)
	req.Set("anthropic-dangerous-direct-browser-access", "true")
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-api-key", info.ApiKey)
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Set("anthropic-version", anthropicVersion)
	CommonClaudeHeadersOperation(c, req, info)
	applyClaudeCodeHeaderFingerprint(req, info)
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
	if err := applyClaudeCodeRequestFingerprint(info, claudeRequest); err != nil {
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

func applyClaudeCodeRequestFingerprint(info *relaycommon.RelayInfo, request *dto.ClaudeRequest) error {
	if request == nil || !shouldUseClaudeCodeFingerprint(info) {
		return nil
	}
	ensureClaudeCodeSystem(request)
	return ensureClaudeCodeMetadata(request)
}

func ensureClaudeCodeSystem(request *dto.ClaudeRequest) {
	if request.System == nil {
		request.System = []dto.ClaudeMediaMessage{newClaudeCodeSystemBlock()}
		return
	}
	if containsClaudeCodeMarker(request.System) {
		return
	}
	if request.IsStringSystem() {
		if strings.TrimSpace(request.GetStringSystem()) == "" {
			request.System = []dto.ClaudeMediaMessage{newClaudeCodeSystemBlock()}
			return
		}
		request.System = []dto.ClaudeMediaMessage{
			newClaudeCodeSystemBlock(),
			newTextSystemBlock(request.GetStringSystem()),
		}
		return
	}
	systemContents := request.ParseSystem()
	if len(systemContents) == 0 {
		request.System = []dto.ClaudeMediaMessage{newClaudeCodeSystemBlock()}
		return
	}
	request.System = append([]dto.ClaudeMediaMessage{newClaudeCodeSystemBlock()}, systemContents...)
}

func ensureClaudeCodeMetadata(request *dto.ClaudeRequest) error {
	metadata := make(map[string]interface{})
	if len(request.Metadata) > 0 {
		if err := common.Unmarshal(request.Metadata, &metadata); err != nil {
			return err
		}
	}
	if _, ok := metadata["user_id"]; !ok || metadata["user_id"] == nil || strings.TrimSpace(common.Interface2String(metadata["user_id"])) == "" {
		metadata["user_id"] = claudeCodeUserID
	}
	metadataBytes, err := common.Marshal(metadata)
	if err != nil {
		return err
	}
	request.Metadata = metadataBytes
	return nil
}

func newClaudeCodeSystemBlock() dto.ClaudeMediaMessage {
	return newTextSystemBlock(claudeCodeSystemText)
}

func newTextSystemBlock(text string) dto.ClaudeMediaMessage {
	block := dto.ClaudeMediaMessage{Type: "text"}
	block.SetText(text)
	return block
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
