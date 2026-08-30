package openaicompat

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const (
	chatFinishReasonLength        = "length"
	chatFinishReasonContentFilter = "content_filter"
)

// ChatCompletionsResponseToResponsesResponse 将 Chat 非流响应转换为 Responses 响应。
func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse, id string) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}
	if id == "" {
		id = resp.Id
	}

	usage := UsageFromChatUsage(&resp.Usage)
	out := &dto.OpenAIResponsesResponse{
		ID:        id,
		Object:    "response",
		CreatedAt: chatCreatedAt(resp.Created),
		Status:    []byte(`"completed"`),
		Model:     resp.Model,
		Output:    make([]dto.ResponsesOutput, 0),
		Usage:     usage,
	}
	if len(resp.Choices) == 0 {
		return out, usage, nil
	}

	choice := resp.Choices[0]
	status, details := ResponsesStatusFromChatFinishReason(choice.FinishReason)
	out.Status = mustJSONString(status)
	out.IncompleteDetails = details
	outputStatus := responseOutputStatus(out)

	if text := choice.Message.StringContent(); text != "" {
		out.Output = append(out.Output, dto.ResponsesOutput{
			Type:   responsesOutputTypeMessage,
			ID:     fmt.Sprintf("%s_msg_0", id),
			Status: outputStatus,
			Role:   "assistant",
			Content: []dto.ResponsesOutputContent{{
				Type:        "output_text",
				Text:        text,
				Annotations: []interface{}{},
			}},
		})
	}
	if reasoning := choice.Message.GetReasoningContent(); reasoning != "" {
		out.Output = append(out.Output, dto.ResponsesOutput{
			Type:   responsesOutputTypeReasoning,
			ID:     fmt.Sprintf("%s_reasoning_0", id),
			Status: outputStatus,
			Content: []dto.ResponsesOutputContent{{
				Type: "summary_text",
				Text: reasoning,
			}},
		})
	}
	for index, toolCall := range choice.Message.ParseToolCalls() {
		toolOutput, err := chatToolCallToResponsesOutput(toolCall, id, index, outputStatus)
		if err != nil {
			return nil, nil, err
		}
		out.Output = append(out.Output, toolOutput)
	}

	return out, usage, nil
}

func ResponsesStatusFromChatFinishReason(finishReason string) (string, *dto.IncompleteDetails) {
	switch strings.TrimSpace(finishReason) {
	case chatFinishReasonLength:
		return "incomplete", &dto.IncompleteDetails{Reason: responsesIncompleteReasonMaxTokens}
	case chatFinishReasonContentFilter:
		return "incomplete", &dto.IncompleteDetails{Reason: responsesIncompleteReasonContentFilter}
	default:
		return "completed", nil
	}
}

// UsageFromChatUsage 同时保留标准 token、缓存 token、推理 token 及项目扩展字段。
func UsageFromChatUsage(src *dto.Usage) *dto.Usage {
	usage := &dto.Usage{}
	if src == nil {
		return usage
	}

	*usage = *src
	if src.InputTokensDetails != nil {
		details := *src.InputTokensDetails
		usage.InputTokensDetails = &details
	}
	if usage.InputTokens == 0 {
		usage.InputTokens = src.PromptTokens
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = src.CompletionTokens
	}
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}

	if usage.InputTokensDetails == nil && inputTokenDetailsPresent(src.PromptTokensDetails) {
		details := src.PromptTokensDetails
		usage.InputTokensDetails = &details
	}
	if src.HasAnyCacheCreationTokensField() {
		if usage.InputTokensDetails == nil {
			usage.InputTokensDetails = &dto.InputTokenDetails{}
		}
		usage.SetCacheCreationTokensWithPresence(src.GetCacheCreationTokens())
	}
	return usage
}

func inputTokenDetailsPresent(details dto.InputTokenDetails) bool {
	return details.CachedTokens != 0 ||
		details.CacheWriteTokens != 0 ||
		details.CacheCreationTokens != 0 ||
		details.CachedCreationTokens != 0 ||
		details.TextTokens != 0 ||
		details.AudioTokens != 0 ||
		details.ImageTokens != 0 ||
		details.HasAnyCacheCreationTokensField()
}

type ChatToResponsesStreamEvent struct {
	Type    string
	Payload dto.ResponsesStreamResponse
}

type ChatToResponsesStreamState struct {
	ID      string
	Model   string
	Created int64

	usage             *dto.Usage
	status            string
	incompleteDetails *dto.IncompleteDetails
	sentCreated       bool
	textOutputIndex   int
	textStarted       bool
	textDone          bool
	reasoningIndex    int
	reasoningStarted  bool
	reasoningDone     bool
	finalized         bool
	nextOutputIndex   int
	toolsByIndex      map[int]*chatToResponsesStreamTool
	outputOrder       []chatToResponsesOutputRef
	text              strings.Builder
	reasoning         strings.Builder
}

type chatToResponsesStreamTool struct {
	ChatIndex   int
	OutputIndex int
	ID          string
	Type        string
	Name        string
	Arguments   strings.Builder
	Done        bool
}

type chatToResponsesOutputRef struct {
	Kind      string
	ToolIndex int
}

func NewChatToResponsesStreamState(id string, model string) *ChatToResponsesStreamState {
	return &ChatToResponsesStreamState{
		ID:              id,
		Model:           model,
		Created:         time.Now().Unix(),
		usage:           &dto.Usage{},
		status:          "completed",
		textOutputIndex: -1,
		reasoningIndex:  -1,
		toolsByIndex:    make(map[int]*chatToResponsesStreamTool),
	}
}

func (s *ChatToResponsesStreamState) GetUsage() *dto.Usage {
	if s == nil {
		return nil
	}
	return s.usage
}

func (s *ChatToResponsesStreamState) SetUsage(usage *dto.Usage) {
	if s != nil {
		s.usage = UsageFromChatUsage(usage)
	}
}

func (s *ChatToResponsesStreamState) UsageText() string {
	if s == nil {
		return ""
	}
	return s.text.String()
}

func ChatCompletionsStreamChunkToResponsesEvents(chunk *dto.ChatCompletionsStreamResponse, state *ChatToResponsesStreamState) ([]ChatToResponsesStreamEvent, error) {
	if chunk == nil || state == nil {
		return nil, nil
	}
	if state.ID == "" {
		state.ID = chunk.Id
	}
	if state.Model == "" {
		state.Model = chunk.Model
	}
	if chunk.Created != 0 {
		state.Created = chunk.Created
	}
	if chunk.Usage != nil {
		state.SetUsage(chunk.Usage)
	}

	events := state.ensureCreated()
	for _, choice := range chunk.Choices {
		if reasoning := choice.Delta.GetReasoningContent(); reasoning != "" {
			events = append(events, state.appendReasoningDelta(reasoning)...)
		}
		if text := choice.Delta.GetContentString(); text != "" {
			events = append(events, state.appendTextDelta(text)...)
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			toolEvents, err := state.appendToolCallDelta(toolCall)
			if err != nil {
				return nil, err
			}
			events = append(events, toolEvents...)
		}
		if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
			state.applyFinishReason(*choice.FinishReason)
			events = append(events, state.doneDeltaEvents()...)
		}
	}
	return events, nil
}

func FinalizeChatCompletionsStreamToResponses(state *ChatToResponsesStreamState) []ChatToResponsesStreamEvent {
	if state == nil || state.finalized {
		return nil
	}
	events := state.ensureCreated()
	events = append(events, state.doneDeltaEvents()...)
	state.finalized = true
	eventType := responsesEventCompleted
	if state.status == "incomplete" {
		eventType = responsesEventIncomplete
	}
	events = append(events, responsesStreamEvent(eventType, dto.ResponsesStreamResponse{
		Response: state.finalResponse(),
	}))
	return events
}

func (s *ChatToResponsesStreamState) ensureCreated() []ChatToResponsesStreamEvent {
	if s.sentCreated {
		return nil
	}
	s.sentCreated = true
	return []ChatToResponsesStreamEvent{responsesStreamEvent(responsesEventCreated, dto.ResponsesStreamResponse{
		Response: s.createdResponse(),
	})}
}

func (s *ChatToResponsesStreamState) appendTextDelta(delta string) []ChatToResponsesStreamEvent {
	events := make([]ChatToResponsesStreamEvent, 0, 2)
	if !s.textStarted {
		s.textStarted = true
		s.textOutputIndex = s.nextIndex("message", -1)
		events = append(events, responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{
			OutputIndex: intPointer(s.textOutputIndex),
			Item: &dto.ResponsesOutput{
				Type:    responsesOutputTypeMessage,
				ID:      s.messageID(),
				Status:  "in_progress",
				Role:    "assistant",
				Content: []dto.ResponsesOutputContent{},
			},
		}))
	}
	s.text.WriteString(delta)
	events = append(events, responsesStreamEvent(responsesEventOutputTextDelta, dto.ResponsesStreamResponse{
		OutputIndex:  intPointer(s.textOutputIndex),
		ContentIndex: intPointer(0),
		Delta:        delta,
		ItemID:       s.messageID(),
	}))
	return events
}

func (s *ChatToResponsesStreamState) appendReasoningDelta(delta string) []ChatToResponsesStreamEvent {
	events := make([]ChatToResponsesStreamEvent, 0, 2)
	if !s.reasoningStarted {
		s.reasoningStarted = true
		s.reasoningIndex = s.nextIndex("reasoning", -1)
		events = append(events, responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{
			OutputIndex: intPointer(s.reasoningIndex),
			Item: &dto.ResponsesOutput{
				Type:    responsesOutputTypeReasoning,
				ID:      s.reasoningID(),
				Status:  "in_progress",
				Content: []dto.ResponsesOutputContent{},
			},
		}))
	}
	s.reasoning.WriteString(delta)
	events = append(events, responsesStreamEvent(responsesEventReasoningSummaryDelta, dto.ResponsesStreamResponse{
		OutputIndex:  intPointer(s.reasoningIndex),
		SummaryIndex: intPointer(0),
		Delta:        delta,
		ItemID:       s.reasoningID(),
	}))
	return events
}

func (s *ChatToResponsesStreamState) appendToolCallDelta(toolCall dto.ToolCallResponse) ([]ChatToResponsesStreamEvent, error) {
	chatIndex := 0
	if toolCall.Index != nil {
		chatIndex = *toolCall.Index
	}
	tool := s.toolsByIndex[chatIndex]
	events := make([]ChatToResponsesStreamEvent, 0, 2)
	if tool == nil {
		toolType := chatStreamToolType(toolCall.Type)
		if toolType == "" {
			toolType = "function"
		}
		if toolType != "function" && toolType != dto.CustomType {
			return nil, fmt.Errorf("chat stream tool type %q cannot be represented by Responses", toolType)
		}
		tool = &chatToResponsesStreamTool{
			ChatIndex:   chatIndex,
			OutputIndex: s.nextIndex("tool", chatIndex),
			ID:          strings.TrimSpace(toolCall.ID),
			Type:        toolType,
			Name:        strings.TrimSpace(toolCall.Function.Name),
		}
		if tool.ID == "" {
			tool.ID = fmt.Sprintf("%s_call_%d", s.ID, chatIndex)
		}
		s.toolsByIndex[chatIndex] = tool
		events = append(events, responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{
			OutputIndex: intPointer(tool.OutputIndex),
			ItemID:      tool.ID,
			Item:        s.toolOutput(tool, "in_progress"),
		}))
	}
	if id := strings.TrimSpace(toolCall.ID); id != "" {
		tool.ID = id
	}
	if name := strings.TrimSpace(toolCall.Function.Name); name != "" {
		tool.Name = name
	}
	if arguments := toolCall.Function.Arguments; arguments != "" {
		tool.Arguments.WriteString(arguments)
		eventType := responsesEventFunctionArgsDelta
		if tool.Type == dto.CustomType {
			eventType = responsesEventCustomToolInputDelta
		}
		events = append(events, responsesStreamEvent(eventType, dto.ResponsesStreamResponse{
			OutputIndex: intPointer(tool.OutputIndex),
			ItemID:      tool.ID,
			Delta:       arguments,
		}))
	}
	return events, nil
}

func (s *ChatToResponsesStreamState) doneDeltaEvents() []ChatToResponsesStreamEvent {
	events := make([]ChatToResponsesStreamEvent, 0)
	status := s.outputStatus()
	if s.textStarted && !s.textDone {
		s.textDone = true
		events = append(events,
			responsesStreamEvent("response.output_text.done", dto.ResponsesStreamResponse{
				OutputIndex: intPointer(s.textOutputIndex), ContentIndex: intPointer(0), ItemID: s.messageID(), Text: s.text.String(),
			}),
			responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
				OutputIndex: intPointer(s.textOutputIndex), Item: s.messageOutput(status),
			}),
		)
	}
	if s.reasoningStarted && !s.reasoningDone {
		s.reasoningDone = true
		events = append(events,
			responsesStreamEvent(responsesEventReasoningSummaryDone, dto.ResponsesStreamResponse{
				OutputIndex: intPointer(s.reasoningIndex), SummaryIndex: intPointer(0), ItemID: s.reasoningID(),
				Part: &dto.ResponsesReasoningSummaryPart{Type: "summary_text", Text: s.reasoning.String()},
			}),
			responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
				OutputIndex: intPointer(s.reasoningIndex), Item: s.reasoningOutput(status),
			}),
		)
	}
	for _, tool := range s.sortedTools() {
		if tool.Done {
			continue
		}
		tool.Done = true
		eventType := responsesEventFunctionArgsDone
		if tool.Type == dto.CustomType {
			eventType = responsesEventCustomToolInputDone
		}
		events = append(events,
			responsesStreamEvent(eventType, dto.ResponsesStreamResponse{
				OutputIndex: intPointer(tool.OutputIndex), ItemID: tool.ID,
			}),
			responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
				OutputIndex: intPointer(tool.OutputIndex), Item: s.toolOutput(tool, status),
			}),
		)
	}
	return events
}

func (s *ChatToResponsesStreamState) applyFinishReason(finishReason string) {
	s.status, s.incompleteDetails = ResponsesStatusFromChatFinishReason(finishReason)
}

func (s *ChatToResponsesStreamState) finalResponse() *dto.OpenAIResponsesResponse {
	output := make([]dto.ResponsesOutput, 0, len(s.outputOrder))
	status := s.outputStatus()
	for _, ref := range s.outputOrder {
		switch ref.Kind {
		case "message":
			output = append(output, *s.messageOutput(status))
		case "reasoning":
			output = append(output, *s.reasoningOutput(status))
		case "tool":
			if tool := s.toolsByIndex[ref.ToolIndex]; tool != nil {
				output = append(output, *s.toolOutput(tool, status))
			}
		}
	}
	return &dto.OpenAIResponsesResponse{
		ID:                s.ID,
		Object:            "response",
		CreatedAt:         int(s.Created),
		Status:            mustJSONString(s.status),
		IncompleteDetails: s.incompleteDetails,
		Model:             s.Model,
		Output:            output,
		Usage:             s.usage,
	}
}

func (s *ChatToResponsesStreamState) createdResponse() *dto.OpenAIResponsesResponse {
	return &dto.OpenAIResponsesResponse{
		ID: s.ID, Object: "response", CreatedAt: int(s.Created), Status: []byte(`"in_progress"`),
		Model: s.Model, Output: []dto.ResponsesOutput{},
	}
}

func (s *ChatToResponsesStreamState) nextIndex(kind string, toolIndex int) int {
	index := s.nextOutputIndex
	s.nextOutputIndex++
	s.outputOrder = append(s.outputOrder, chatToResponsesOutputRef{Kind: kind, ToolIndex: toolIndex})
	return index
}

func (s *ChatToResponsesStreamState) sortedTools() []*chatToResponsesStreamTool {
	indexes := make([]int, 0, len(s.toolsByIndex))
	for index := range s.toolsByIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	tools := make([]*chatToResponsesStreamTool, 0, len(indexes))
	for _, index := range indexes {
		tools = append(tools, s.toolsByIndex[index])
	}
	return tools
}

func (s *ChatToResponsesStreamState) outputStatus() string {
	if s.status == "incomplete" {
		return "incomplete"
	}
	return "completed"
}

func (s *ChatToResponsesStreamState) messageID() string {
	return fmt.Sprintf("%s_msg_0", s.ID)
}

func (s *ChatToResponsesStreamState) reasoningID() string {
	return fmt.Sprintf("%s_reasoning_0", s.ID)
}

func (s *ChatToResponsesStreamState) messageOutput(status string) *dto.ResponsesOutput {
	return &dto.ResponsesOutput{
		Type: responsesOutputTypeMessage, ID: s.messageID(), Status: status, Role: "assistant",
		Content: []dto.ResponsesOutputContent{{Type: "output_text", Text: s.text.String(), Annotations: []interface{}{}}},
	}
}

func (s *ChatToResponsesStreamState) reasoningOutput(status string) *dto.ResponsesOutput {
	return &dto.ResponsesOutput{
		Type: responsesOutputTypeReasoning, ID: s.reasoningID(), Status: status,
		Content: []dto.ResponsesOutputContent{{Type: "summary_text", Text: s.reasoning.String()}},
	}
}

func (s *ChatToResponsesStreamState) toolOutput(tool *chatToResponsesStreamTool, status string) *dto.ResponsesOutput {
	outputType := responsesOutputTypeFunctionCall
	if tool.Type == dto.CustomType {
		outputType = responsesOutputTypeCustomToolCall
	}
	return &dto.ResponsesOutput{
		Type: outputType, ID: tool.ID, Status: status, CallId: tool.ID, Name: tool.Name,
		Arguments: chatArgumentsRawMessage(tool.Arguments.String()),
	}
}

func responseOutputStatus(resp *dto.OpenAIResponsesResponse) string {
	if resp != nil && responseStatusString(resp) == "incomplete" {
		return "incomplete"
	}
	return "completed"
}

func chatToolCallToResponsesOutput(toolCall dto.ToolCallRequest, responseID string, index int, status string) (dto.ResponsesOutput, error) {
	callID := strings.TrimSpace(toolCall.ID)
	if callID == "" {
		callID = fmt.Sprintf("%s_call_%d", responseID, index)
	}
	toolType := strings.TrimSpace(toolCall.Type)
	if toolType == "" || toolType == "function" {
		return dto.ResponsesOutput{
			Type: responsesOutputTypeFunctionCall, ID: callID, Status: status, CallId: callID,
			Name: toolCall.Function.Name, Arguments: chatArgumentsRawMessage(toolCall.Function.Arguments),
		}, nil
	}
	if toolType == dto.CustomType {
		name := strings.TrimSpace(toolCall.Function.Name)
		input := toolCall.Function.Arguments
		if len(toolCall.Custom) > 0 {
			var custom map[string]any
			if err := common.Unmarshal(toolCall.Custom, &custom); err == nil {
				if name == "" {
					name = strings.TrimSpace(common.Interface2String(custom["name"]))
				}
				if input == "" {
					input = responsesArgumentsString(custom["input"])
				}
			}
		}
		return dto.ResponsesOutput{
			Type: responsesOutputTypeCustomToolCall, ID: callID, Status: status, CallId: callID,
			Name: name, Arguments: chatArgumentsRawMessage(input),
		}, nil
	}
	return dto.ResponsesOutput{}, fmt.Errorf("chat tool type %q cannot be represented by Responses", toolType)
}

func chatArgumentsRawMessage(arguments string) []byte {
	raw, err := common.Marshal(arguments)
	if err != nil {
		return []byte(`""`)
	}
	return raw
}

func chatCreatedAt(created any) int {
	switch value := created.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case string:
		if parsed := common.String2Int(value); parsed != 0 {
			return parsed
		}
	}
	return int(time.Now().Unix())
}

func responsesStreamEvent(eventType string, payload dto.ResponsesStreamResponse) ChatToResponsesStreamEvent {
	payload.Type = eventType
	return ChatToResponsesStreamEvent{Type: eventType, Payload: payload}
}

func intPointer(value int) *int {
	return &value
}

func mustJSONString(value string) []byte {
	raw, err := common.Marshal(value)
	if err != nil {
		return []byte(`""`)
	}
	return raw
}

func chatStreamToolType(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(common.Interface2String(typed))
	}
}
