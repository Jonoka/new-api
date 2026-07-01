package service

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeToOpenAIRequestPreservesSystemCacheControlForResponses(t *testing.T) {
	systemText := "cache this system prompt"
	req := dto.ClaudeRequest{
		Model: "gpt-5.5",
		System: []dto.ClaudeMediaMessage{
			{
				Type:         "text",
				Text:         &systemText,
				CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
			},
		},
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}

	openAIReq, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.NotNil(t, openAIReq)
	require.Len(t, openAIReq.Messages, 2)

	body, err := common.Marshal(openAIReq.Messages[0].Content)
	require.NoError(t, err)
	require.Equal(t, "system", openAIReq.Messages[0].Role)
	require.Equal(t, "cache this system prompt", gjson.GetBytes(body, "0.text").String())
	require.Equal(t, "ephemeral", gjson.GetBytes(body, "0.cache_control.type").String())
}

func TestBuildClaudeUsageFromOpenAIUsageSubtractsCachedTokens(t *testing.T) {
	usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
		PromptTokens:     1114,
		CompletionTokens: 382,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 1108,
		},
	})

	require.NotNil(t, usage)
	require.Equal(t, 6, usage.InputTokens)
	require.Equal(t, 1108, usage.CacheReadInputTokens)
	require.Equal(t, 0, usage.CacheCreationInputTokens)
	require.Equal(t, 382, usage.OutputTokens)
}

func TestBuildClaudeUsageFromOpenAIUsageSubtractsCacheCreationTokens(t *testing.T) {
	usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
		PromptTokens:     2000,
		CompletionTokens: 123,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         700,
			CachedCreationTokens: 300,
		},
		ClaudeCacheCreation5mTokens: 100,
		ClaudeCacheCreation1hTokens: 50,
	})

	require.NotNil(t, usage)
	require.Equal(t, 1000, usage.InputTokens)
	require.Equal(t, 700, usage.CacheReadInputTokens)
	require.Equal(t, 300, usage.CacheCreationInputTokens)
	require.Equal(t, 123, usage.OutputTokens)
	require.NotNil(t, usage.CacheCreation)
	require.Equal(t, 250, usage.CacheCreation.Ephemeral5mInputTokens)
	require.Equal(t, 50, usage.CacheCreation.Ephemeral1hInputTokens)
}

func TestBuildClaudeUsageFromOpenAIUsageKeepsPromptTokensWhenNoCache(t *testing.T) {
	usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
		PromptTokens:     256,
		CompletionTokens: 32,
	})

	require.NotNil(t, usage)
	require.Equal(t, 256, usage.InputTokens)
	require.Equal(t, 0, usage.CacheReadInputTokens)
	require.Equal(t, 0, usage.CacheCreationInputTokens)
	require.Equal(t, 32, usage.OutputTokens)
}

func TestResponseOpenAI2ClaudeNonStreamMessageShape(t *testing.T) {
	message := dto.Message{Role: "assistant"}
	message.SetStringContent("pong")
	openAIResponse := &dto.OpenAITextResponse{
		Id:    "chatcmpl-test",
		Model: "claude-sonnet-4-20250514",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: "stop",
			},
		},
		Usage: dto.Usage{
			PromptTokens:     11,
			CompletionTokens: 3,
			TotalTokens:      14,
		},
	}

	claudeResponse := ResponseOpenAI2Claude(openAIResponse, &relaycommon.RelayInfo{
		OriginModelName: "claude-sonnet-4-20250514",
	})
	body, err := common.Marshal(claudeResponse)
	require.NoError(t, err)

	require.Equal(t, "message", gjson.GetBytes(body, "type").String())
	require.Equal(t, "assistant", gjson.GetBytes(body, "role").String())
	require.Equal(t, "claude-sonnet-4-20250514", gjson.GetBytes(body, "model").String())
	require.Equal(t, "text", gjson.GetBytes(body, "content.0.type").String())
	require.Equal(t, "pong", gjson.GetBytes(body, "content.0.text").String())
	require.Equal(t, "end_turn", gjson.GetBytes(body, "stop_reason").String())
	require.Equal(t, gjson.Null, gjson.GetBytes(body, "stop_sequence").Type)
	require.EqualValues(t, 11, gjson.GetBytes(body, "usage.input_tokens").Int())
	require.EqualValues(t, 0, gjson.GetBytes(body, "usage.cache_creation_input_tokens").Int())
	require.EqualValues(t, 0, gjson.GetBytes(body, "usage.cache_read_input_tokens").Int())
	require.EqualValues(t, 3, gjson.GetBytes(body, "usage.output_tokens").Int())
	require.False(t, gjson.GetBytes(body, "usage.claude_cache_creation_5_m_tokens").Exists())
	require.False(t, gjson.GetBytes(body, "usage.claude_cache_creation_1_h_tokens").Exists())
}

func TestBuildClaudeUsageFromOpenAIUsageClampsNegativeInputTokens(t *testing.T) {
	usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 20,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         90,
			CachedCreationTokens: 30,
		},
	})

	require.NotNil(t, usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Equal(t, 90, usage.CacheReadInputTokens)
	require.Equal(t, 30, usage.CacheCreationInputTokens)
	require.Equal(t, 20, usage.OutputTokens)
}

func TestClaudeToOpenAIRequestMapsMetadataUserIDToPromptCacheKey(t *testing.T) {
	req := dto.ClaudeRequest{
		Model:    "gpt-5.5",
		Metadata: json.RawMessage(`{"user_id":"claude-cache-user"}`),
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}

	openAIReq, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.NotNil(t, openAIReq)
	require.Equal(t, "claude-cache-user", openAIReq.PromptCacheKey)
}

func TestClaudeToOpenAIRequestExtractsSessionIDFromMetadataUserIDJSON(t *testing.T) {
	req := dto.ClaudeRequest{
		Model:    "gpt-5.5",
		Metadata: json.RawMessage(`{"user_id":"{\"device_id\":\"dev-1\",\"account_uuid\":\"\",\"session_id\":\"sess-json-123\"}"}`),
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}

	openAIReq, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.NotNil(t, openAIReq)
	require.Equal(t, "sess-json-123", openAIReq.PromptCacheKey)
}

func TestClaudeToOpenAIRequestMapsSessionHeaderToPromptCacheKey(t *testing.T) {
	req := dto.ClaudeRequest{
		Model: "gpt-5.5",
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		RequestHeaders: map[string]string{
			"Session_id": "sess-claude-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}

	openAIReq, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.NotNil(t, openAIReq)
	require.Equal(t, "sess-claude-123", openAIReq.PromptCacheKey)
}

func TestClaudeToOpenAIRequestMapsClaudeCodeSessionHeaderToPromptCacheKey(t *testing.T) {
	req := dto.ClaudeRequest{
		Model: "gpt-5.5",
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		RequestHeaders: map[string]string{
			"X-Claude-Code-Session-Id": "cc-session-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}

	openAIReq, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.NotNil(t, openAIReq)
	require.Equal(t, "cc-session-123", openAIReq.PromptCacheKey)
}

func TestClaudeToOpenAIRequestMapsClaudeEffortToOpenAIReasoningEffort(t *testing.T) {
	req := dto.ClaudeRequest{
		Model:        "gpt-5.5",
		OutputConfig: json.RawMessage(`{"effort":"high"}`),
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}

	openAIReq, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.NotNil(t, openAIReq)
	require.Equal(t, "high", openAIReq.ReasoningEffort)
}

func TestClaudeToOpenAIRequestMapsClaudeMaxEffortToXHigh(t *testing.T) {
	req := dto.ClaudeRequest{
		Model:        "gpt-5.5",
		OutputConfig: json.RawMessage(`{"effort":"max"}`),
		Messages: []dto.ClaudeMessage{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}

	openAIReq, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.NotNil(t, openAIReq)
	require.Equal(t, "xhigh", openAIReq.ReasoningEffort)
}
