package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const (
	responsesInputTypeFunctionCall       = "function_call"
	responsesInputTypeFunctionCallOutput = "function_call_output"
	responsesInputTypeCustomToolCall     = "custom_tool_call"
	responsesInputTypeCustomToolOutput   = "custom_tool_call_output"
)

// ResponsesRequestToChatCompletionsRequest 将无状态 Responses 请求转换为
// Chat Completions 请求。无法在 Chat 协议中可靠表达的会话状态字段会被明确拒绝，
// 避免静默丢失上下文。
func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("model is required")
	}
	if err := validateResponsesRequestChatUnsupportedFields(req); err != nil {
		return nil, err
	}

	messages, err := responsesRequestMessagesToChat(req)
	if err != nil {
		return nil, err
	}
	tools, err := responsesRequestToolsToChat(req.Tools)
	if err != nil {
		return nil, err
	}
	toolChoice, err := responsesRequestToolChoiceToChat(req.ToolChoice)
	if err != nil {
		return nil, err
	}
	responseFormat, verbosity, err := responsesRequestTextToChat(req.Text)
	if err != nil {
		return nil, err
	}

	out := &dto.GeneralOpenAIRequest{
		Model:                req.Model,
		Messages:             messages,
		Stream:               req.Stream,
		StreamOptions:        req.StreamOptions,
		MaxCompletionTokens:  req.MaxOutputTokens,
		Temperature:          req.Temperature,
		TopP:                 req.TopP,
		TopLogProbs:          req.TopLogProbs,
		ResponseFormat:       responseFormat,
		Verbosity:            verbosity,
		Tools:                tools,
		ToolChoice:           toolChoice,
		User:                 req.User,
		Store:                req.Store,
		Metadata:             req.Metadata,
		SafetyIdentifier:     req.SafetyIdentifier,
		PromptCacheRetention: cloneRawMessage(req.PromptCacheRetention),
		EnableThinking:       req.EnableThinking,
	}

	if req.Reasoning != nil {
		out.ReasoningEffort = req.Reasoning.Effort
	}
	if req.ServiceTier != "" {
		out.ServiceTier, err = common.Marshal(req.ServiceTier)
		if err != nil {
			return nil, fmt.Errorf("invalid service_tier: %w", err)
		}
	}
	if rawJSONPresent(req.ParallelToolCalls) {
		if common.GetJsonType(req.ParallelToolCalls) != "boolean" {
			return nil, errors.New("parallel_tool_calls must be a boolean")
		}
		var parallelToolCalls bool
		if err := common.Unmarshal(req.ParallelToolCalls, &parallelToolCalls); err != nil {
			return nil, fmt.Errorf("invalid parallel_tool_calls: %w", err)
		}
		out.ParallelTooCalls = &parallelToolCalls
	}
	if rawJSONPresent(req.PromptCacheKey) {
		if common.GetJsonType(req.PromptCacheKey) != "string" {
			return nil, errors.New("prompt_cache_key must be a string")
		}
		if err := common.Unmarshal(req.PromptCacheKey, &out.PromptCacheKey); err != nil {
			return nil, fmt.Errorf("invalid prompt_cache_key: %w", err)
		}
	}

	return out, nil
}

func validateResponsesRequestChatUnsupportedFields(req *dto.OpenAIResponsesRequest) error {
	unsupported := make([]string, 0, 5)
	if rawJSONPresent(req.Conversation) {
		unsupported = append(unsupported, "conversation")
	}
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		unsupported = append(unsupported, "previous_response_id")
	}
	if rawJSONPresent(req.Prompt) {
		unsupported = append(unsupported, "prompt")
	}
	if rawJSONPresent(req.ContextManagement) {
		unsupported = append(unsupported, "context_management")
	}
	if rawJSONPresent(req.PromptCacheOptions) {
		unsupported = append(unsupported, "prompt_cache_options")
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("responses to chat conversion cannot safely represent fields: %s", strings.Join(unsupported, ", "))
	}
	return nil
}

func responsesRequestMessagesToChat(req *dto.OpenAIResponsesRequest) ([]dto.Message, error) {
	messages := make([]dto.Message, 0)
	if rawJSONPresent(req.Instructions) {
		instructions, err := responsesJSONString(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("invalid instructions: %w", err)
		}
		if strings.TrimSpace(instructions) != "" {
			messages = append(messages, dto.Message{Role: "system", Content: instructions})
		}
	}

	if !rawJSONPresent(req.Input) {
		return messages, nil
	}
	switch common.GetJsonType(req.Input) {
	case "string":
		input, err := responsesJSONString(req.Input)
		if err != nil {
			return nil, fmt.Errorf("invalid input string: %w", err)
		}
		return append(messages, dto.Message{Role: "user", Content: input}), nil
	case "array":
		var items []any
		if err := common.Unmarshal(req.Input, &items); err != nil {
			return nil, fmt.Errorf("invalid input array: %w", err)
		}
		for index, rawItem := range items {
			switch item := rawItem.(type) {
			case string:
				messages = append(messages, dto.Message{Role: "user", Content: item})
			case map[string]any:
				next, err := responsesInputItemToChatMessages(item, messages)
				if err != nil {
					return nil, fmt.Errorf("invalid input item %d: %w", index, err)
				}
				messages = next
			default:
				return nil, fmt.Errorf("unsupported responses input item %d type %T", index, rawItem)
			}
		}
		return messages, nil
	default:
		return nil, fmt.Errorf("unsupported responses input type %q", common.GetJsonType(req.Input))
	}
}

func responsesInputItemToChatMessages(item map[string]any, messages []dto.Message) ([]dto.Message, error) {
	itemType := strings.TrimSpace(common.Interface2String(item["type"]))
	switch itemType {
	case responsesInputTypeFunctionCall:
		toolCall, err := responsesFunctionCallItemToChatToolCall(item)
		if err != nil {
			return nil, err
		}
		return appendToolCallToLastAssistant(messages, toolCall)
	case responsesInputTypeCustomToolCall:
		toolCall, err := responsesCustomToolCallItemToChatToolCall(item)
		if err != nil {
			return nil, err
		}
		return appendToolCallToLastAssistant(messages, toolCall)
	case responsesInputTypeFunctionCallOutput, responsesInputTypeCustomToolOutput:
		callID := responsesCallID(item)
		if callID == "" {
			return nil, fmt.Errorf("%s item is missing call_id", itemType)
		}
		content := responseToolOutputToChatContent(item["output"])
		return append(messages, dto.Message{Role: "tool", ToolCallId: callID, Content: content}), nil
	}

	role := strings.TrimSpace(common.Interface2String(item["role"]))
	if role == "" {
		role = "user"
	}
	content, err := responsesInputContentToChatContent(item["content"])
	if err != nil {
		return nil, err
	}
	return append(messages, dto.Message{Role: role, Content: content}), nil
}

func responsesInputContentToChatContent(content any) (any, error) {
	if content == nil {
		return "", nil
	}
	switch value := content.(type) {
	case string:
		return value, nil
	case []any:
		return responsesContentPartsToChatContent(value)
	case []map[string]any:
		parts := make([]any, 0, len(value))
		for _, part := range value {
			parts = append(parts, part)
		}
		return responsesContentPartsToChatContent(parts)
	default:
		return content, nil
	}
}

func responsesContentPartsToChatContent(parts []any) (any, error) {
	chatParts := make([]any, 0, len(parts))
	var textOnly strings.Builder
	onlyText := true

	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			onlyText = false
			chatParts = append(chatParts, rawPart)
			continue
		}

		partType := strings.TrimSpace(common.Interface2String(part["type"]))
		var converted map[string]any
		switch partType {
		case "input_text", "output_text", "text":
			text := common.Interface2String(part["text"])
			textOnly.WriteString(text)
			converted = map[string]any{"type": dto.ContentTypeText, "text": text}
		case "input_image":
			onlyText = false
			converted = map[string]any{"type": dto.ContentTypeImageURL, "image_url": responsesImagePartToChatImageURL(part)}
		case "input_file":
			onlyText = false
			converted = map[string]any{"type": dto.ContentTypeFile, "file": responsesFilePartToChatFile(part)}
		case "input_audio":
			onlyText = false
			converted = map[string]any{"type": dto.ContentTypeInputAudio, "input_audio": responsesPartPayload(part, "input_audio")}
		case "input_video":
			onlyText = false
			converted = map[string]any{"type": dto.ContentTypeVideoUrl, "video_url": responsesVideoPartToChatVideoURL(part)}
		default:
			onlyText = false
			chatParts = append(chatParts, part)
			continue
		}
		if cacheControl, ok := part["cache_control"]; ok {
			onlyText = false
			converted["cache_control"] = cacheControl
		}
		chatParts = append(chatParts, converted)
	}

	if onlyText {
		return textOnly.String(), nil
	}
	return chatParts, nil
}

func responsesFunctionCallItemToChatToolCall(item map[string]any) (dto.ToolCallRequest, error) {
	name := strings.TrimSpace(common.Interface2String(item["name"]))
	if name == "" {
		return dto.ToolCallRequest{}, errors.New("function_call item is missing name")
	}
	callID := responsesCallID(item)
	if callID == "" {
		return dto.ToolCallRequest{}, errors.New("function_call item is missing call_id")
	}
	return dto.ToolCallRequest{
		ID:   callID,
		Type: "function",
		Function: dto.FunctionRequest{
			Name:      name,
			Arguments: responsesArgumentsString(item["arguments"]),
		},
	}, nil
}

func responsesCustomToolCallItemToChatToolCall(item map[string]any) (dto.ToolCallRequest, error) {
	name := strings.TrimSpace(common.Interface2String(item["name"]))
	if name == "" {
		return dto.ToolCallRequest{}, errors.New("custom_tool_call item is missing name")
	}
	callID := responsesCallID(item)
	if callID == "" {
		return dto.ToolCallRequest{}, errors.New("custom_tool_call item is missing call_id")
	}
	raw, err := common.Marshal(item)
	if err != nil {
		return dto.ToolCallRequest{}, err
	}
	return dto.ToolCallRequest{
		ID:     callID,
		Type:   dto.CustomType,
		Custom: raw,
		Function: dto.FunctionRequest{
			Name:      name,
			Arguments: responsesArgumentsString(item["input"]),
		},
	}, nil
}

func appendToolCallToLastAssistant(messages []dto.Message, toolCall dto.ToolCallRequest) ([]dto.Message, error) {
	if len(messages) == 0 || messages[len(messages)-1].Role != "assistant" {
		messages = append(messages, dto.Message{Role: "assistant"})
	}
	idx := len(messages) - 1
	toolCalls := messages[idx].ParseToolCalls()
	toolCalls = append(toolCalls, toolCall)
	raw, err := common.Marshal(toolCalls)
	if err != nil {
		return nil, err
	}
	messages[idx].ToolCalls = raw
	return messages, nil
}

func responsesRequestToolsToChat(raw json.RawMessage) ([]dto.ToolCallRequest, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}
	if common.GetJsonType(raw) != "array" {
		return nil, errors.New("tools must be an array")
	}

	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil, fmt.Errorf("invalid tools: %w", err)
	}
	out := make([]dto.ToolCallRequest, 0, len(tools))
	for index, tool := range tools {
		toolType := strings.TrimSpace(common.Interface2String(tool["type"]))
		switch toolType {
		case "function":
			name := strings.TrimSpace(common.Interface2String(tool["name"]))
			if name == "" {
				return nil, fmt.Errorf("function tool %d is missing name", index)
			}
			out = append(out, dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        name,
					Description: common.Interface2String(tool["description"]),
					Parameters:  tool["parameters"],
				},
			})
		case dto.CustomType:
			rawTool, err := common.Marshal(tool)
			if err != nil {
				return nil, err
			}
			out = append(out, dto.ToolCallRequest{
				Type:   dto.CustomType,
				Custom: rawTool,
				Function: dto.FunctionRequest{
					Name:        strings.TrimSpace(common.Interface2String(tool["name"])),
					Description: common.Interface2String(tool["description"]),
				},
			})
		default:
			return nil, fmt.Errorf("responses tool type %q cannot be safely represented by Chat Completions", toolType)
		}
	}
	return out, nil
}

func responsesRequestToolChoiceToChat(raw json.RawMessage) (any, error) {
	if !rawJSONPresent(raw) {
		return nil, nil
	}
	if common.GetJsonType(raw) == "string" {
		var choice string
		if err := common.Unmarshal(raw, &choice); err != nil {
			return nil, fmt.Errorf("invalid tool_choice: %w", err)
		}
		return choice, nil
	}
	if common.GetJsonType(raw) != "object" {
		return nil, errors.New("tool_choice must be a string or object")
	}
	var choice map[string]any
	if err := common.Unmarshal(raw, &choice); err != nil {
		return nil, fmt.Errorf("invalid tool_choice: %w", err)
	}
	choiceType := strings.TrimSpace(common.Interface2String(choice["type"]))
	if choiceType == "function" {
		name := strings.TrimSpace(common.Interface2String(choice["name"]))
		if name == "" {
			return nil, errors.New("function tool_choice is missing name")
		}
		return map[string]any{"type": "function", "function": map[string]any{"name": name}}, nil
	}
	if choiceType == dto.CustomType {
		return choice, nil
	}
	return nil, fmt.Errorf("responses tool_choice type %q cannot be safely represented by Chat Completions", choiceType)
}

func responsesRequestTextToChat(raw json.RawMessage) (*dto.ResponseFormat, json.RawMessage, error) {
	if !rawJSONPresent(raw) {
		return nil, nil, nil
	}
	if common.GetJsonType(raw) != "object" {
		return nil, nil, errors.New("text must be an object")
	}
	var textConfig map[string]any
	if err := common.Unmarshal(raw, &textConfig); err != nil {
		return nil, nil, fmt.Errorf("invalid text config: %w", err)
	}

	var verbosity json.RawMessage
	if value, ok := textConfig["verbosity"]; ok {
		var err error
		verbosity, err = common.Marshal(value)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid text.verbosity: %w", err)
		}
	}
	format, ok := textConfig["format"].(map[string]any)
	if !ok {
		return nil, verbosity, nil
	}
	formatType := strings.TrimSpace(common.Interface2String(format["type"]))
	if formatType == "" {
		return nil, verbosity, nil
	}
	out := &dto.ResponseFormat{Type: formatType}
	if formatType == "json_schema" {
		schemaRaw, err := common.Marshal(format)
		if err != nil {
			return nil, nil, err
		}
		out.JsonSchema = schemaRaw
	}
	return out, verbosity, nil
}

func responsesImagePartToChatImageURL(part map[string]any) any {
	if imageURL, ok := part["image_url"]; ok {
		if detail, hasDetail := part["detail"]; hasDetail {
			switch value := imageURL.(type) {
			case string:
				return map[string]any{"url": value, "detail": detail}
			case map[string]any:
				copyValue := cloneMap(value)
				if _, exists := copyValue["detail"]; !exists {
					copyValue["detail"] = detail
				}
				return copyValue
			}
		}
		return imageURL
	}
	imageURL := map[string]any{}
	for _, key := range []string{"url", "file_id", "detail"} {
		if value, ok := part[key]; ok {
			imageURL[key] = value
		}
	}
	if len(imageURL) == 0 {
		return part
	}
	return imageURL
}

func responsesFilePartToChatFile(part map[string]any) any {
	if file, ok := part["file"]; ok {
		return file
	}
	file := map[string]any{}
	for _, key := range []string{"file_id", "file_data", "filename", "file_url"} {
		if value, ok := part[key]; ok {
			file[key] = value
		}
	}
	if len(file) == 0 {
		return part
	}
	return file
}

func responsesVideoPartToChatVideoURL(part map[string]any) any {
	if videoURL, ok := part["video_url"]; ok {
		if videoURLMap, ok := videoURL.(map[string]any); ok {
			if url := common.Interface2String(videoURLMap["url"]); url != "" {
				return url
			}
		}
		return videoURL
	}
	if url := common.Interface2String(part["url"]); url != "" {
		return url
	}
	return responsesPartPayload(part, "video_url")
}

func responsesPartPayload(part map[string]any, key string) any {
	if value, ok := part[key]; ok {
		return value
	}
	payload := make(map[string]any, len(part))
	for name, value := range part {
		if name != "type" && name != "cache_control" {
			payload[name] = value
		}
	}
	return payload
}

func responsesCallID(item map[string]any) string {
	if callID := strings.TrimSpace(common.Interface2String(item["call_id"])); callID != "" {
		return callID
	}
	return strings.TrimSpace(common.Interface2String(item["id"]))
}

func responsesArgumentsString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		raw, err := common.Marshal(typed)
		if err != nil {
			return common.Interface2String(typed)
		}
		return string(raw)
	}
}

func responseToolOutputToChatContent(value any) any {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		raw, err := common.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(raw)
	}
}

func responsesJSONString(raw json.RawMessage) (string, error) {
	if common.GetJsonType(raw) != "string" {
		return string(raw), nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func rawJSONPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && common.GetJsonType(raw) != "null"
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func cloneMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
