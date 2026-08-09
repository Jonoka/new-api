package openai

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/tidwall/sjson"
)

func normalizeOpenAIUsageTokenCounts(usage *dto.Usage) bool {
	if usage == nil {
		return false
	}

	modified := false
	if usage.PromptTokens == 0 && usage.InputTokens > 0 {
		usage.PromptTokens = usage.InputTokens
		modified = true
	}

	if usage.CompletionTokens == 0 {
		switch {
		case usage.OutputTokens > 0:
			usage.CompletionTokens = usage.OutputTokens
			modified = true
		case usage.PromptTokens > 0 && usage.TotalTokens > usage.PromptTokens:
			usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens
			modified = true
		}
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens > 0 && usage.TotalTokens > usage.CompletionTokens {
		usage.PromptTokens = usage.TotalTokens - usage.CompletionTokens
		modified = true
	}

	calculatedTotal := usage.PromptTokens + usage.CompletionTokens
	if calculatedTotal > 0 && usage.TotalTokens != calculatedTotal {
		usage.TotalTokens = calculatedTotal
		modified = true
	}

	return modified
}

func normalizeAndValidateOpenAIUsage(usage *dto.Usage) bool {
	normalizeOpenAIUsageTokenCounts(usage)
	return usage != nil && (usage.PromptTokens != 0 || usage.CompletionTokens != 0)
}

func fillMissingOpenAIChatUsage(usage *dto.Usage, promptTokens, completionTokens int) bool {
	modified := normalizeOpenAIUsageTokenCounts(usage)
	if usage == nil {
		return modified
	}

	if usage.PromptTokens == 0 && promptTokens > 0 {
		usage.PromptTokens = promptTokens
		modified = true
	}

	// 输入补齐后，优先使用上游 total_tokens 推导输出，最后才采用本地估算。
	if normalizeOpenAIUsageTokenCounts(usage) {
		modified = true
	}
	if usage.CompletionTokens == 0 && completionTokens > 0 {
		usage.CompletionTokens = completionTokens
		modified = true
	}
	if normalizeOpenAIUsageTokenCounts(usage) {
		modified = true
	}

	return modified
}

func patchOpenAIChatUsage(data []byte, usage *dto.Usage) ([]byte, error) {
	if usage == nil {
		return data, nil
	}

	patched, err := sjson.SetBytes(data, "usage.prompt_tokens", usage.PromptTokens)
	if err != nil {
		return nil, err
	}
	patched, err = sjson.SetBytes(patched, "usage.completion_tokens", usage.CompletionTokens)
	if err != nil {
		return nil, err
	}
	return sjson.SetBytes(patched, "usage.total_tokens", usage.TotalTokens)
}

func normalizeOpenAIStreamUsageData(data string) string {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil || streamResponse.Usage == nil {
		return data
	}
	if !normalizeOpenAIUsageTokenCounts(streamResponse.Usage) {
		return data
	}
	patched, err := patchOpenAIChatUsage(common.StringToByteSlice(data), streamResponse.Usage)
	if err != nil {
		return data
	}
	return string(patched)
}
