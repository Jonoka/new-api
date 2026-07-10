package openaicompat

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

func isGPT56ResponsesCacheCreationModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(normalized, "gpt-5.6-sol") ||
		strings.Contains(normalized, "gpt-5.6-terra") ||
		strings.Contains(normalized, "gpt-5.6-luna")
}

func inferGPT56ResponsesCacheCreationTokens(model string, inputTokens int, details *dto.InputTokenDetails) int {
	if details == nil || details.HasAnyCacheCreationTokensField() || !isGPT56ResponsesCacheCreationModel(model) {
		return 0
	}
	if inputTokens <= details.CachedTokens {
		return 0
	}
	return inputTokens - details.CachedTokens
}

func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}

	text := ExtractOutputTextFromResponses(resp)

	usage := &dto.Usage{}
	if resp.Usage != nil {
		inputTokens := resp.Usage.InputTokens
		if inputTokens == 0 {
			inputTokens = resp.Usage.PromptTokens
		}
		outputTokens := resp.Usage.OutputTokens
		if outputTokens == 0 {
			outputTokens = resp.Usage.CompletionTokens
		}
		if inputTokens != 0 {
			usage.PromptTokens = inputTokens
			usage.InputTokens = inputTokens
		}
		if outputTokens != 0 {
			usage.CompletionTokens = outputTokens
			usage.OutputTokens = outputTokens
		}
		if resp.Usage.TotalTokens != 0 {
			usage.TotalTokens = resp.Usage.TotalTokens
		} else {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if resp.Usage.InputTokensDetails != nil {
			usage.InputTokensDetails = resp.Usage.InputTokensDetails
			usage.PromptTokensDetails.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
			cacheCreationTokens := resp.Usage.GetCacheCreationTokens()
			if cacheCreationTokens == 0 && !resp.Usage.HasAnyDetailCacheCreationTokensField() {
				cacheCreationTokens = inferGPT56ResponsesCacheCreationTokens(resp.Model, usage.PromptTokens, resp.Usage.InputTokensDetails)
			}
			if resp.Usage.HasAnyCacheCreationTokensField() || cacheCreationTokens > 0 {
				usage.SetCacheCreationTokensWithPresence(cacheCreationTokens)
			}
			usage.PromptTokensDetails.ImageTokens = resp.Usage.InputTokensDetails.ImageTokens
			usage.PromptTokensDetails.AudioTokens = resp.Usage.InputTokensDetails.AudioTokens
			usage.PromptTokensDetails.TextTokens = resp.Usage.InputTokensDetails.TextTokens
		}
		if resp.Usage.PromptTokensDetails.CachedTokens != 0 {
			usage.PromptTokensDetails.CachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
		}
		if !usage.PromptTokensDetails.HasAnyCacheCreationTokensField() && resp.Usage.HasAnyCacheCreationTokensField() {
			usage.SetCacheCreationTokensWithPresence(resp.Usage.GetCacheCreationTokens())
		}
		if resp.Usage.PromptTokensDetails.ImageTokens != 0 {
			usage.PromptTokensDetails.ImageTokens = resp.Usage.PromptTokensDetails.ImageTokens
		}
		if resp.Usage.PromptTokensDetails.AudioTokens != 0 {
			usage.PromptTokensDetails.AudioTokens = resp.Usage.PromptTokensDetails.AudioTokens
		}
		if resp.Usage.PromptTokensDetails.TextTokens != 0 {
			usage.PromptTokensDetails.TextTokens = resp.Usage.PromptTokensDetails.TextTokens
		}
		if resp.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
			usage.CompletionTokenDetails.ReasoningTokens = resp.Usage.CompletionTokenDetails.ReasoningTokens
		}
		if resp.Usage.CompletionTokenDetails.TextTokens != 0 {
			usage.CompletionTokenDetails.TextTokens = resp.Usage.CompletionTokenDetails.TextTokens
		}
		if resp.Usage.CompletionTokenDetails.AudioTokens != 0 {
			usage.CompletionTokenDetails.AudioTokens = resp.Usage.CompletionTokenDetails.AudioTokens
		}
		if resp.Usage.CompletionTokenDetails.ImageTokens != 0 {
			usage.CompletionTokenDetails.ImageTokens = resp.Usage.CompletionTokenDetails.ImageTokens
		}
	}

	created := resp.CreatedAt

	var toolCalls []dto.ToolCallResponse
	if text == "" && len(resp.Output) > 0 {
		for _, out := range resp.Output {
			if out.Type != "function_call" {
				continue
			}
			name := strings.TrimSpace(out.Name)
			if name == "" {
				continue
			}
			callId := strings.TrimSpace(out.CallId)
			if callId == "" {
				callId = strings.TrimSpace(out.ID)
			}
			toolCalls = append(toolCalls, dto.ToolCallResponse{
				ID:   callId,
				Type: "function",
				Function: dto.FunctionResponse{
					Name:      name,
					Arguments: out.ArgumentsString(),
				},
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	msg := dto.Message{
		Role:    "assistant",
		Content: text,
	}
	if len(toolCalls) > 0 {
		msg.SetToolCalls(toolCalls)
		msg.Content = ""
	}

	out := &dto.OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   resp.Model,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: *usage,
	}

	return out, usage, nil
}

func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Output) == 0 {
		return ""
	}

	var sb strings.Builder

	// Prefer assistant message outputs.
	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		if out.Role != "" && out.Role != "assistant" {
			continue
		}
		for _, c := range out.Content {
			if c.Type == "output_text" && c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	for _, out := range resp.Output {
		for _, c := range out.Content {
			if c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}
