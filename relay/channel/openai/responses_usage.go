package openai

import (
	"github.com/QuantumNous/new-api/dto"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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
		cacheCreationTokens := respUsage.GetCacheCreationTokens()
		if respUsage.HasAnyCacheCreationTokensField() || cacheCreationTokens > 0 {
			usage.SetCacheCreationTokensWithPresence(cacheCreationTokens)
		}
		usage.PromptTokensDetails.ImageTokens = respUsage.InputTokensDetails.ImageTokens
		usage.PromptTokensDetails.AudioTokens = respUsage.InputTokensDetails.AudioTokens
		usage.PromptTokensDetails.TextTokens = respUsage.InputTokensDetails.TextTokens
	}
	if respUsage.PromptTokensDetails.CachedTokens != 0 {
		usage.PromptTokensDetails.CachedTokens = respUsage.PromptTokensDetails.CachedTokens
	}
	if !usage.PromptTokensDetails.HasAnyCacheCreationTokensField() && respUsage.HasAnyCacheCreationTokensField() {
		usage.SetCacheCreationTokensWithPresence(respUsage.GetCacheCreationTokens())
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

func patchResponsesUsageCacheCreationFields(data string, usage *dto.Usage) string {
	if usage == nil || !usage.HasAnyCacheCreationTokensField() || data == "" {
		return data
	}
	cacheCreationTokens := usage.PromptTokensDetails.CachedCreationTokens
	usageRoots := []string{}
	if gjson.Get(data, "usage").Exists() {
		usageRoots = append(usageRoots, "usage")
	}
	if gjson.Get(data, "response.usage").Exists() {
		usageRoots = append(usageRoots, "response.usage")
	}
	if len(usageRoots) == 0 {
		return data
	}
	updated := data
	for _, root := range usageRoots {
		patches := []string{
			root + ".input_tokens_details.cache_creation_tokens",
			root + ".input_tokens_details.cached_creation_tokens",
			root + ".input_tokens_details.cache_write_tokens",
			root + ".prompt_tokens_details.cache_creation_tokens",
			root + ".prompt_tokens_details.cached_creation_tokens",
			root + ".prompt_tokens_details.cache_write_tokens",
			root + ".cache_creation_input_tokens",
			root + ".cache_write_input_tokens",
			root + ".cache_creation_tokens",
			root + ".cache_write_tokens",
		}
		for _, path := range patches {
			next, err := sjson.Set(updated, path, cacheCreationTokens)
			if err != nil {
				return data
			}
			updated = next
		}
	}
	return updated
}
