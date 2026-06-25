package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestShouldClaudeUseOpenAIResponsesForCacheControlOnResponsesChannel(t *testing.T) {
	systemText := "cached system"
	request := &dto.ClaudeRequest{
		System: []dto.ClaudeMediaMessage{
			{
				Type:         "text",
				Text:         &systemText,
				CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
		},
	}

	require.True(t, shouldClaudeUseOpenAIResponses(info, request))
}

func TestShouldClaudeUseOpenAIResponsesKeepsRegularOpenAICompatibleByDefault(t *testing.T) {
	request := &dto.ClaudeRequest{
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-4o",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
		},
	}

	require.False(t, shouldClaudeUseOpenAIResponses(info, request))
}
