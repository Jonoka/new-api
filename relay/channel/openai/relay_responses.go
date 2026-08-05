package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const responsesPreambleBufferLimit = 16

type responsesPendingEvent struct {
	response dto.ResponsesStreamResponse
	data     string
}

func isResponsesPreambleEvent(eventType string) bool {
	switch eventType {
	case "response.created", "response.queued", "response.in_progress":
		return true
	default:
		return false
	}
}

func responsesStreamError(streamResponse dto.ResponsesStreamResponse) *types.OpenAIError {
	if streamResponse.Response != nil {
		if openAIError := streamResponse.Response.GetOpenAIError(); openAIError != nil {
			return openAIError
		}
	}
	if openAIError := dto.GetOpenAIError(streamResponse.Error); openAIError != nil {
		return openAIError
	}
	if streamResponse.Type != "error" && streamResponse.Code == nil && streamResponse.Message == "" && streamResponse.Param == "" {
		return nil
	}
	return &types.OpenAIError{
		Type:    "error",
		Code:    streamResponse.Code,
		Message: streamResponse.Message,
		Param:   streamResponse.Param,
	}
}

func isResponsesCapacityError(openAIError *types.OpenAIError) bool {
	if openAIError == nil {
		return false
	}
	code := strings.ToLower(fmt.Sprint(openAIError.Code))
	message := strings.Join(strings.Fields(strings.ToLower(openAIError.Message)), " ")
	return strings.Contains(code, "capacity") ||
		strings.Contains(code, "overloaded") ||
		strings.Contains(message, "selected model is at capacity")
}

func isResponsesErrorEvent(eventType string) bool {
	switch eventType {
	case "error", "response.error", "response.failed":
		return true
	default:
		return false
	}
}

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if isResponsesSSEBody(responseBody) {
		converted, convErr := convertResponsesSSEToJSON(responseBody)
		if convErr != nil {
			return nil, types.NewOpenAIError(convErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseBody = converted
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	service.BindOpenAIResponsesContinuationResponseIDFromInfo(info, responsesResponse.ID)

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := dto.Usage{}
	applyResponsesUsageToOpenAIUsage(&usage, &responsesResponse)
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	var streamErr *types.NewAPIError
	pending := make([]responsesPendingEvent, 0, 3)
	clientOutputCommitted := false
	flushPending := func() {
		for _, event := range pending {
			sendResponsesStreamData(c, event.response, event.data)
		}
		pending = nil
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		if c.GetBool("sensitive_response_stream_blocked") {
			sr.Stop(service.ErrSensitiveResponseBlocked)
			return
		}

		// 检查当前数据是否包含 completed 状态和 usage 信息
		streamResponse, normalizedData, ok, err := parseResponsesStreamEventData(data)
		if !ok {
			return
		}
		if err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		if isResponsesErrorEvent(streamResponse.Type) {
			if openAIError := responsesStreamError(streamResponse); isResponsesCapacityError(openAIError) && !clientOutputCommitted && c.Writer.Size() <= 0 {
				c.Set(string(constant.ContextKeyResponsesPreOutputRetry), true)
				pending = nil
				streamErr = types.WithOpenAIError(*openAIError, http.StatusServiceUnavailable)
				sr.Stop(streamErr)
				return
			}
		}
		if !clientOutputCommitted {
			if isResponsesPreambleEvent(streamResponse.Type) && len(pending) < responsesPreambleBufferLimit {
				pending = append(pending, responsesPendingEvent{response: streamResponse, data: normalizedData})
				return
			}
			flushPending()
			clientOutputCommitted = true
		}
		sendResponsesStreamData(c, streamResponse, normalizedData)
		if c.GetBool("sensitive_response_stream_blocked") {
			sr.Stop(service.ErrSensitiveResponseBlocked)
			return
		}
		switch streamResponse.Type {
		case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
			if streamResponse.Response != nil {
				if streamResponse.Type == "response.completed" || streamResponse.Type == "response.done" {
					service.BindOpenAIResponsesContinuationResponseIDFromInfo(info, streamResponse.Response.ID)
				}
				applyResponsesUsageToOpenAIUsage(usage, streamResponse.Response)
				if streamResponse.Response.HasImageGenerationCall() {
					c.Set("image_generation_call", true)
					c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
					c.Set("image_generation_call_size", streamResponse.Response.GetSize())
				}
			}
		case "response.output_text.delta":
			// 处理输出文本
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			// 函数调用处理
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
						if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
							webSearchTool.CallCount++
						}
					}
				}
			}
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}
	if !clientOutputCommitted {
		flushPending()
	}

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return usage, nil
}
