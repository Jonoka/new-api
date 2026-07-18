package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetEndpointTypesByChannelTypeDefaultOpenAIIncludesResponses(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-5.4")

	if len(got) != 2 {
		t.Fatalf("endpoint types len = %d, want 2: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAI {
		t.Fatalf("first endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAI)
	}
	if got[1] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("second endpoint type = %q, want %q", got[1], constant.EndpointTypeOpenAIResponse)
	}
}

func TestGetEndpointTypesByChannelTypeResponseOnlyModel(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "o3-pro")

	if len(got) != 1 {
		t.Fatalf("endpoint types len = %d, want 1: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAIResponse)
	}
}

func TestGetEndpointTypesByChannelTypeCodexUsesResponses(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeCodex, "gpt-5.4")

	if len(got) != 1 {
		t.Fatalf("endpoint types len = %d, want 1: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAIResponse)
	}
}

func TestGetEndpointTypesByChannelTypeCodexCompactUsesResponsesCompact(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeCodex, "gpt-5.4-openai-compact")

	if len(got) != 1 {
		t.Fatalf("endpoint types len = %d, want 1: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIResponseCompact {
		t.Fatalf("endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAIResponseCompact)
	}
}

func TestGetEndpointTypesByChannelTypeXAIVideoIncludesVideoEndpoint(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-imagine-video-1.5")

	if len(got) != 3 {
		t.Fatalf("endpoint types len = %d, want 3: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAIVideo {
		t.Fatalf("first endpoint type = %q, want %q", got[0], constant.EndpointTypeOpenAIVideo)
	}
}

func TestGetEndpointTypesByChannelTypeXAITextRemainsTextOnly(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-4-1-fast-reasoning")

	if len(got) != 2 {
		t.Fatalf("endpoint types len = %d, want 2: %v", len(got), got)
	}
	if got[0] != constant.EndpointTypeOpenAI || got[1] != constant.EndpointTypeOpenAIResponse {
		t.Fatalf("endpoint types = %v, want OpenAI and Responses", got)
	}
}
