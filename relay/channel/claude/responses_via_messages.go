package claude

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/openaicompat"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// restoreChatCacheControlToClaude 补回旧转换链在 MediaContent -> ClaudeMediaMessage
// 过程中遗漏的 cache_control。Claude Code 指纹生成后的 system 内容不会被覆盖。
func restoreChatCacheControlToClaude(chatRequest *dto.GeneralOpenAIRequest, claudeRequest *dto.ClaudeRequest) {
	if chatRequest == nil || claudeRequest == nil {
		return
	}
	for messageIndex := range chatRequest.Messages {
		message := &chatRequest.Messages[messageIndex]
		role := message.Role
		if role == "developer" {
			role = "system"
		}
		for _, part := range message.ParseContent() {
			if len(part.CacheControl) == 0 {
				continue
			}
			if role == "system" {
				blocks := normalizeClaudeSystemBlocks(claudeRequest.System)
				if applyCacheControlToClaudeBlocks(blocks, part) {
					claudeRequest.System = blocks
				}
				continue
			}
			for index := range claudeRequest.Messages {
				if claudeRequest.Messages[index].Role != role {
					continue
				}
				if applyCacheControlToClaudeMessage(&claudeRequest.Messages[index], part) {
					break
				}
			}
		}
	}
}

func applyCacheControlToClaudeMessage(message *dto.ClaudeMessage, source dto.MediaContent) bool {
	if message == nil {
		return false
	}
	if text, ok := message.Content.(string); ok {
		if source.Type != dto.ContentTypeText || text != source.Text {
			return false
		}
		message.Content = []dto.ClaudeMediaMessage{{
			Type: dto.ContentTypeText, Text: common.GetPointer(text), CacheControl: cloneClaudeRaw(source.CacheControl),
		}}
		return true
	}
	blocks, err := common.Any2Type[[]dto.ClaudeMediaMessage](message.Content)
	if err != nil || len(blocks) == 0 {
		return false
	}
	if !applyCacheControlToClaudeBlocks(blocks, source) {
		return false
	}
	message.Content = blocks
	return true
}

func applyCacheControlToClaudeBlocks(blocks []dto.ClaudeMediaMessage, source dto.MediaContent) bool {
	for index := range blocks {
		if len(blocks[index].CacheControl) != 0 {
			continue
		}
		if source.Type == dto.ContentTypeText {
			if blocks[index].Type != dto.ContentTypeText || blocks[index].GetText() != source.Text {
				continue
			}
		} else if blocks[index].Type != "image" && blocks[index].Type != "document" {
			continue
		}
		blocks[index].CacheControl = cloneClaudeRaw(source.CacheControl)
		return true
	}
	return false
}

func cloneClaudeRaw(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return append([]byte(nil), raw...)
}

func ClaudeMessagesToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	logger.LogDebug(c, "Claude responses response body: %s", body)

	var claudeResponse dto.ClaudeResponse
	if err := common.Unmarshal(body, &claudeResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		return nil, types.WithClaudeError(*claudeError, resp.StatusCode)
	}
	maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	usage := usageFromClaudeMessage(&claudeResponse)
	if claudeResponse.Usage != nil && claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}

	chatResponse := ResponseClaude2OpenAI(&claudeResponse)
	chatResponse.Usage = buildOpenAIStyleUsageFromClaudeUsage(usage)
	responsesResponse, _, err := service.ChatCompletionsResponseToResponsesResponse(chatResponse, helper.GetResponseID(c))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if usage.TotalTokens == 0 {
		fallback := service.ResponseText2Usage(c, service.ExtractOutputTextFromResponses(responsesResponse), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage = fallback
		usage.UsageSemantic = "anthropic"
		usage.UsageSource = "anthropic"
		responsesResponse.Usage = openaicompat.UsageFromChatUsage(fallback)
	}
	responseBody, err := common.Marshal(responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func ClaudeMessagesToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	claudeInfo := &ClaudeResponseInfo{
		ResponseId: helper.GetResponseID(c), Created: common.GetTimestamp(), Model: info.UpstreamModelName,
		ResponseText: strings.Builder{}, Usage: &dto.Usage{},
	}
	state := openaicompat.NewChatToResponsesStreamState(helper.GetResponseID(c), info.UpstreamModelName)
	state.Created = claudeInfo.Created
	var streamErr *types.NewAPIError

	sendEvent := func(event openaicompat.ChatToResponsesStreamEvent) bool {
		data, err := common.Marshal(event.Payload)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: event.Type}, string(data)); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, result *helper.StreamResult) {
		var claudeResponse dto.ClaudeResponse
		if err := common.UnmarshalJsonStr(data, &claudeResponse); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			result.Stop(streamErr)
			return
		}
		if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
			streamErr = types.WithClaudeError(*claudeError, resp.StatusCode)
			result.Stop(streamErr)
			return
		}
		if claudeResponse.StopReason != "" {
			maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
		}
		if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
			maybeMarkClaudeRefusal(c, *claudeResponse.Delta.StopReason)
		}

		chatChunk := StreamResponseClaude2OpenAI(&claudeResponse)
		if !FormatClaudeResponseInfo(&claudeResponse, chatChunk, claudeInfo) || chatChunk == nil {
			return
		}
		events, err := openaicompat.ChatCompletionsStreamChunkToResponsesEvents(chatChunk, state)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			result.Stop(streamErr)
			return
		}
		for _, event := range events {
			if !sendEvent(event) {
				result.Stop(streamErr)
				return
			}
		}
	})
	if streamErr != nil {
		return nil, streamErr
	}

	// 复用 Claude 原有兜底与 cache usage 汇总；Responses 格式不会触发它的下游写入分支。
	HandleStreamFinalResponse(c, info, claudeInfo)
	if claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens == 0 {
		claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens =
			claudeInfo.Usage.ClaudeCacheCreation5mTokens + claudeInfo.Usage.ClaudeCacheCreation1hTokens
	}
	openAIUsage := buildOpenAIStyleUsageFromClaudeUsage(claudeInfo.Usage)
	state.SetUsage(&openAIUsage)
	for _, event := range openaicompat.FinalizeChatCompletionsStreamToResponses(state) {
		if !sendEvent(event) {
			return nil, streamErr
		}
	}
	return claudeInfo.Usage, nil
}

func usageFromClaudeMessage(response *dto.ClaudeResponse) *dto.Usage {
	usage := &dto.Usage{UsageSemantic: "anthropic", UsageSource: "anthropic"}
	if response == nil || response.Usage == nil {
		return usage
	}
	usage.PromptTokens = response.Usage.InputTokens
	usage.CompletionTokens = response.Usage.OutputTokens
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.PromptTokensDetails.CachedTokens = response.Usage.CacheReadInputTokens
	usage.PromptTokensDetails.CachedCreationTokens = response.Usage.GetCacheCreationTotalTokens()
	usage.ClaudeCacheCreation5mTokens = response.Usage.GetCacheCreation5mTokens()
	usage.ClaudeCacheCreation1hTokens = response.Usage.GetCacheCreation1hTokens()
	return usage
}

func validateClaudeResponsesChatTools(request *dto.GeneralOpenAIRequest) error {
	if request == nil {
		return nil
	}
	for _, tool := range request.Tools {
		if tool.Type != "" && tool.Type != "function" {
			return fmt.Errorf("Claude Messages cannot safely represent Chat tool type %q", tool.Type)
		}
	}
	for _, message := range request.Messages {
		for _, toolCall := range message.ParseToolCalls() {
			if toolCall.Type != "" && toolCall.Type != "function" {
				return fmt.Errorf("Claude Messages cannot safely represent Chat tool call type %q", toolCall.Type)
			}
		}
	}
	return nil
}
