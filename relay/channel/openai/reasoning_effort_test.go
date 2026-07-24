package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertOpenAIRequestNormalizesReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}
	req := &dto.GeneralOpenAIRequest{
		Model:           "gpt-5.5",
		ReasoningEffort: "extra high",
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, req)

	require.NoError(t, err)
	convertedReq := converted.(*dto.GeneralOpenAIRequest)
	require.Equal(t, "xhigh", convertedReq.ReasoningEffort)
	require.Equal(t, "xhigh", info.ReasoningEffort)
}

func TestConvertOpenAIRequestNormalizesRawReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			UpstreamModelName: "gpt-5.5",
		},
	}
	req := &dto.GeneralOpenAIRequest{
		Model:     "gpt-5.5",
		Reasoning: mustMarshalOpenAITestRaw(t, map[string]any{"effort": "extra high", "summary": "auto"}),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, req)

	require.NoError(t, err)
	convertedReq := converted.(*dto.GeneralOpenAIRequest)
	require.Equal(t, "xhigh", gjson.GetBytes(convertedReq.Reasoning, "effort").String())
	require.Equal(t, "xhigh", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequestNormalizesReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	req := dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Reasoning: &dto.Reasoning{
			Effort: "extra-high",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, req)

	require.NoError(t, err)
	convertedReq := converted.(dto.OpenAIResponsesRequest)
	require.NotNil(t, convertedReq.Reasoning)
	require.Equal(t, "xhigh", convertedReq.Reasoning.Effort)
	require.Equal(t, "xhigh", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequestPreservesMaxReasoningEffort(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	req := dto.OpenAIResponsesRequest{
		Model: "gpt-5.6-sol",
		Reasoning: &dto.Reasoning{
			Effort: "max",
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, req)

	require.NoError(t, err)
	convertedReq := converted.(dto.OpenAIResponsesRequest)
	require.NotNil(t, convertedReq.Reasoning)
	require.Equal(t, "max", convertedReq.Reasoning.Effort)
	require.Equal(t, "max", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequestParsesUltraReasoningSuffix(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	req := dto.OpenAIResponsesRequest{
		Model: "gpt-5.6-sol-ultra",
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, req)

	require.NoError(t, err)
	convertedReq := converted.(dto.OpenAIResponsesRequest)
	require.Equal(t, "gpt-5.6-sol", convertedReq.Model)
	require.NotNil(t, convertedReq.Reasoning)
	require.Equal(t, "ultra", convertedReq.Reasoning.Effort)
	require.Equal(t, "ultra", info.ReasoningEffort)
}

func TestConvertOpenAIResponsesRequestDropsCodexClientMetadata(t *testing.T) {
	info := &relaycommon.RelayInfo{}
	req := dto.OpenAIResponsesRequest{
		Model:          "gpt-5.6-sol",
		ClientMetadata: mustMarshalOpenAITestRaw(t, map[string]any{"thread_id": "thread-123"}),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, req)

	require.NoError(t, err)
	convertedReq := converted.(dto.OpenAIResponsesRequest)
	require.Empty(t, convertedReq.ClientMetadata)
	require.NotEmpty(t, req.ClientMetadata)
}

func mustMarshalOpenAITestRaw(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}
