package openai

import (
	"bytes"
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

const (
	responsesPreOutputMaxEvents = 64
	responsesPreOutputMaxBytes  = 256 << 10
)

type responsesStreamEventClass string

const (
	responsesEventStructural      responsesStreamEventClass = "structural"
	responsesEventMeaningful      responsesStreamEventClass = "meaningful_output"
	responsesEventTerminalSuccess responsesStreamEventClass = "terminal_success"
	responsesEventError           responsesStreamEventClass = "error"
	responsesEventOther           responsesStreamEventClass = "other"
)

type responsesPendingEvent struct {
	response dto.ResponsesStreamResponse
	data     string
}

type responsesCommittedEventHandler func(dto.ResponsesStreamResponse)

type responsesPreOutputGate struct {
	pending               []responsesPendingEvent
	pendingBytes          int
	clientOutputCommitted bool
}

func newResponsesPreOutputGate() *responsesPreOutputGate {
	return &responsesPreOutputGate{
		pending: make([]responsesPendingEvent, 0, 8),
	}
}

func (g *responsesPreOutputGate) canBuffer(data string) bool {
	return len(g.pending) < responsesPreOutputMaxEvents &&
		g.pendingBytes+len(data) <= responsesPreOutputMaxBytes
}

func (g *responsesPreOutputGate) buffer(streamResponse dto.ResponsesStreamResponse, data string) {
	g.pending = append(g.pending, responsesPendingEvent{response: streamResponse, data: data})
	g.pendingBytes += len(data)
}

func (g *responsesPreOutputGate) flush(c *gin.Context, handleCommitted responsesCommittedEventHandler) {
	for _, event := range g.pending {
		sendResponsesStreamData(c, event.response, event.data)
		if c.GetBool("sensitive_response_stream_blocked") {
			break
		}
		if handleCommitted != nil {
			handleCommitted(event.response)
		}
	}
	g.discard()
}

func (g *responsesPreOutputGate) discard() {
	g.pending = nil
	g.pendingBytes = 0
}

func (g *responsesPreOutputGate) commit(c *gin.Context, eventClass responsesStreamEventClass, reason string, handleCommitted responsesCommittedEventHandler) {
	bufferedEvents := len(g.pending)
	bufferedBytes := g.pendingBytes
	g.flush(c, handleCommitted)
	g.clientOutputCommitted = true
	logger.LogDebug(c, "responses pre-output gate transition=committed event_class=%s reason=%s buffered_events=%d buffered_bytes=%d",
		eventClass, reason, bufferedEvents, bufferedBytes)
}

func responsesOutputHasMeaningfulPayload(output *dto.ResponsesOutput) bool {
	if output == nil {
		return false
	}
	if output.ArgumentsString() != "" ||
		responsesRawPayloadIsMeaningful(output.Input) ||
		responsesRawPayloadIsMeaningful(output.Result) ||
		responsesRawPayloadIsMeaningful(output.Action) ||
		responsesRawPayloadIsMeaningful(output.Output) {
		return true
	}
	for _, content := range output.Content {
		if content.Text != "" || content.Refusal != "" {
			return true
		}
	}
	return false
}

func responsesRawPayloadIsMeaningful(payload []byte) bool {
	normalized := bytes.TrimSpace(payload)
	return len(normalized) > 0 && !bytes.Equal(normalized, []byte("null")) && !bytes.Equal(normalized, []byte(`""`))
}

func responsesStreamHasMeaningfulPayload(streamResponse dto.ResponsesStreamResponse) bool {
	if streamResponse.Delta != "" || streamResponse.Text != "" || streamResponse.Refusal != "" ||
		responsesRawPayloadIsMeaningful(streamResponse.Arguments) || responsesRawPayloadIsMeaningful(streamResponse.Input) ||
		streamResponse.Transcript != "" || streamResponse.PartialImageB64 != "" ||
		responsesRawPayloadIsMeaningful(streamResponse.Output) {
		return true
	}
	if streamResponse.Part != nil && (streamResponse.Part.Text != "" || streamResponse.Part.Refusal != "") {
		return true
	}
	if !isResponsesErrorEvent(streamResponse.Type) &&
		(streamResponse.Type == "response.code_interpreter_call.code.done" || streamResponse.Type == "response.code_interpreter_call.code.delta") {
		if code, ok := streamResponse.Code.(string); ok && code != "" {
			return true
		}
	}
	if responsesOutputHasMeaningfulPayload(streamResponse.Item) {
		return true
	}
	if streamResponse.Response != nil {
		for i := range streamResponse.Response.Output {
			if responsesOutputHasMeaningfulPayload(&streamResponse.Response.Output[i]) {
				return true
			}
		}
	}
	return false
}

func isResponsesToolLifecycleEvent(eventType string) bool {
	prefixes := []string{
		"response.web_search_call.",
		"response.file_search_call.",
		"response.code_interpreter_call.",
		"response.computer_tool_call.",
		"response.image_generation_call.",
	}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(eventType, prefix) {
			continue
		}
		switch strings.TrimPrefix(eventType, prefix) {
		case "in_progress", "searching", "interpreting", "generating", "completed":
			return true
		}
	}
	return false
}

func classifyResponsesStreamEvent(streamResponse dto.ResponsesStreamResponse) responsesStreamEventClass {
	if isResponsesErrorEvent(streamResponse.Type) {
		return responsesEventError
	}
	if streamResponse.Type == "response.completed" || streamResponse.Type == "response.done" {
		return responsesEventTerminalSuccess
	}
	if responsesStreamHasMeaningfulPayload(streamResponse) {
		return responsesEventMeaningful
	}

	switch streamResponse.Type {
	case "response.created", "response.queued", "response.in_progress",
		dto.ResponsesOutputTypeItemAdded, dto.ResponsesOutputTypeItemDone,
		"response.content_part.added", "response.content_part.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done",
		"response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.reasoning_text.done",
		"response.output_text.delta", "response.output_text.done",
		"response.refusal.delta", "response.refusal.done",
		"response.function_call_arguments.delta", "response.function_call_arguments.done",
		"response.custom_tool_call_input.delta", "response.custom_tool_call_input.done",
		"response.mcp_call_arguments.delta", "response.mcp_call_arguments.done",
		"response.audio.delta", "response.audio.done",
		"response.audio.transcript.delta", "response.audio.transcript.done",
		"response.code_interpreter_call.code.delta", "response.code_interpreter_call.code.done",
		"response.image_generation_call.partial_image", "response.output_text.annotation.added":
		return responsesEventStructural
	default:
		if isResponsesToolLifecycleEvent(streamResponse.Type) {
			return responsesEventStructural
		}
		return responsesEventOther
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
	gate := newResponsesPreOutputGate()
	handleCommittedEvent := func(streamResponse dto.ResponsesStreamResponse) {
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
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			if streamResponse.Item != nil && streamResponse.Item.Type == dto.BuildInCallWebSearchCall &&
				info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
				if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
					webSearchTool.CallCount++
				}
			}
		}
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
		eventClass := classifyResponsesStreamEvent(streamResponse)
		if !gate.clientOutputCommitted && c.Writer.Size() > 0 {
			gate.commit(c, eventClass, "downstream_write_detected", handleCommittedEvent)
		}
		if eventClass == responsesEventError {
			if openAIError := responsesStreamError(streamResponse); isResponsesCapacityError(openAIError) &&
				!responsesStreamHasMeaningfulPayload(streamResponse) &&
				!gate.clientOutputCommitted && c.Writer.Size() <= 0 {
				bufferedEvents := len(gate.pending)
				bufferedBytes := gate.pendingBytes
				c.Set(string(constant.ContextKeyResponsesPreOutputRetry), true)
				gate.discard()
				logger.LogInfo(c, fmt.Sprintf("responses pre-output gate transition=capacity_fallback event_class=error buffered_events=%d buffered_bytes=%d", bufferedEvents, bufferedBytes))
				streamErr = types.WithOpenAIError(*openAIError, http.StatusServiceUnavailable)
				sr.Stop(streamErr)
				return
			}
		}
		if !gate.clientOutputCommitted {
			if eventClass == responsesEventStructural && gate.canBuffer(normalizedData) {
				gate.buffer(streamResponse, normalizedData)
				return
			}
			if eventClass == responsesEventStructural {
				logger.LogWarn(c, fmt.Sprintf("responses pre-output gate transition=buffer_limit event_class=structural buffered_events=%d buffered_bytes=%d next_event_bytes=%d",
					len(gate.pending), gate.pendingBytes, len(normalizedData)))
			}
			gate.commit(c, eventClass, "event_commit", handleCommittedEvent)
		}
		sendResponsesStreamData(c, streamResponse, normalizedData)
		if c.GetBool("sensitive_response_stream_blocked") {
			sr.Stop(service.ErrSensitiveResponseBlocked)
			return
		}
		handleCommittedEvent(streamResponse)
	})

	if streamErr != nil {
		return nil, streamErr
	}
	if !gate.clientOutputCommitted {
		gate.commit(c, responsesEventStructural, "stream_end", handleCommittedEvent)
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
