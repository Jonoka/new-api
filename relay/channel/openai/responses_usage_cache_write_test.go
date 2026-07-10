package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func TestApplyResponsesUsageCopiesCacheWriteTokens(t *testing.T) {
	usage := &dto.Usage{}
	resp := &dto.OpenAIResponsesResponse{
		Usage: &dto.Usage{
			InputTokens: 4096,
			InputTokensDetails: &dto.InputTokenDetails{
				CachedCreationTokens:        2048,
				CachedCreationTokensPresent: true,
			},
		},
	}

	applyResponsesUsageToOpenAIUsage(usage, resp)

	if usage.PromptTokensDetails.CachedCreationTokens != 2048 {
		t.Fatalf("CachedCreationTokens = %d, want 2048", usage.PromptTokensDetails.CachedCreationTokens)
	}
	if !usage.PromptTokensDetails.CachedCreationTokensPresent {
		t.Fatal("CachedCreationTokensPresent = false, want true")
	}
}

func TestApplyResponsesUsagePreservesExplicitZeroCacheWriteTokens(t *testing.T) {
	usage := &dto.Usage{}
	resp := &dto.OpenAIResponsesResponse{
		Usage: &dto.Usage{
			InputTokens: 4096,
			InputTokensDetails: &dto.InputTokenDetails{
				CachedCreationTokensPresent: true,
			},
		},
	}

	applyResponsesUsageToOpenAIUsage(usage, resp)

	if !usage.PromptTokensDetails.CachedCreationTokensPresent {
		t.Fatal("CachedCreationTokensPresent = false, want true")
	}
	if usage.PromptTokensDetails.CachedCreationTokens != 0 {
		t.Fatalf("CachedCreationTokens = %d, want 0", usage.PromptTokensDetails.CachedCreationTokens)
	}
}
