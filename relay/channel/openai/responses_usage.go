package openai

import "github.com/QuantumNous/new-api/dto"

func isResponsesTerminalUsageEvent(eventType string) bool {
	switch eventType {
	case "response.completed",
		"response.done",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled":
		return true
	default:
		return false
	}
}

func applyResponsesUsageToOpenAIUsage(usage *dto.Usage, resp *dto.OpenAIResponsesResponse) {
	if usage == nil || resp == nil || resp.Usage == nil {
		return
	}

	respUsage := resp.Usage
	inputTokens := respUsage.InputTokens
	if inputTokens == 0 {
		inputTokens = respUsage.PromptTokens
	}
	outputTokens := respUsage.OutputTokens
	if outputTokens == 0 {
		outputTokens = respUsage.CompletionTokens
	}

	if inputTokens != 0 {
		usage.PromptTokens = inputTokens
		usage.InputTokens = inputTokens
	}
	if outputTokens != 0 {
		usage.CompletionTokens = outputTokens
		usage.OutputTokens = outputTokens
	}
	if respUsage.TotalTokens != 0 {
		usage.TotalTokens = respUsage.TotalTokens
	} else if inputTokens != 0 || outputTokens != 0 {
		usage.TotalTokens = inputTokens + outputTokens
	}

	if respUsage.InputTokensDetails != nil {
		usage.InputTokensDetails = respUsage.InputTokensDetails
		usage.PromptTokensDetails.CachedTokens = respUsage.InputTokensDetails.CachedTokens
		usage.PromptTokensDetails.CachedCreationTokens = respUsage.InputTokensDetails.CachedCreationTokens
		usage.PromptTokensDetails.CachedCreationTokensPresent = respUsage.InputTokensDetails.CachedCreationTokensPresent
		usage.PromptTokensDetails.ImageTokens = respUsage.InputTokensDetails.ImageTokens
		usage.PromptTokensDetails.AudioTokens = respUsage.InputTokensDetails.AudioTokens
		usage.PromptTokensDetails.TextTokens = respUsage.InputTokensDetails.TextTokens
	}
	if respUsage.PromptTokensDetails.CachedTokens != 0 {
		usage.PromptTokensDetails.CachedTokens = respUsage.PromptTokensDetails.CachedTokens
	}
	if respUsage.PromptTokensDetails.CachedCreationTokensPresent {
		usage.PromptTokensDetails.CachedCreationTokensPresent = true
		usage.PromptTokensDetails.CachedCreationTokens = respUsage.PromptTokensDetails.CachedCreationTokens
	} else if respUsage.PromptTokensDetails.CachedCreationTokens != 0 {
		usage.PromptTokensDetails.CachedCreationTokens = respUsage.PromptTokensDetails.CachedCreationTokens
	}
	if respUsage.PromptTokensDetails.ImageTokens != 0 {
		usage.PromptTokensDetails.ImageTokens = respUsage.PromptTokensDetails.ImageTokens
	}
	if respUsage.PromptTokensDetails.AudioTokens != 0 {
		usage.PromptTokensDetails.AudioTokens = respUsage.PromptTokensDetails.AudioTokens
	}
	if respUsage.PromptTokensDetails.TextTokens != 0 {
		usage.PromptTokensDetails.TextTokens = respUsage.PromptTokensDetails.TextTokens
	}

	if respUsage.CompletionTokenDetails.ReasoningTokens != 0 {
		usage.CompletionTokenDetails.ReasoningTokens = respUsage.CompletionTokenDetails.ReasoningTokens
	}
	if respUsage.CompletionTokenDetails.TextTokens != 0 {
		usage.CompletionTokenDetails.TextTokens = respUsage.CompletionTokenDetails.TextTokens
	}
	if respUsage.CompletionTokenDetails.AudioTokens != 0 {
		usage.CompletionTokenDetails.AudioTokens = respUsage.CompletionTokenDetails.AudioTokens
	}
	if respUsage.CompletionTokenDetails.ImageTokens != 0 {
		usage.CompletionTokenDetails.ImageTokens = respUsage.CompletionTokenDetails.ImageTokens
	}
}
