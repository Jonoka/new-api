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
