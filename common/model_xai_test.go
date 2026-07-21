package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestIsImageGenerationModelXAI(t *testing.T) {
	tests := []struct {
		modelName string
		want      bool
	}{
		{modelName: "grok-imagine-image", want: true},
		// IsImageGenerationModel 使用字符串包含匹配，基础模型名会同时覆盖 -pro 变体。
		{modelName: "grok-imagine-image-pro", want: true},
		{modelName: "grok-2-image-1212", want: true},
		{modelName: "grok-4-fast-reasoning", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			require.Equal(t, tt.want, IsImageGenerationModel(tt.modelName))
		})
	}
}

func TestGetEndpointTypesByChannelTypeXAIImageModels(t *testing.T) {
	want := []constant.EndpointType{
		constant.EndpointTypeImageGeneration,
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
	}

	for _, modelName := range []string{
		"grok-imagine-image",
		"grok-imagine-image-pro",
		"grok-2-image-1212",
	} {
		t.Run(modelName, func(t *testing.T) {
			require.Equal(t, want, GetEndpointTypesByChannelType(constant.ChannelTypeXai, modelName))
		})
	}
}

func TestGetEndpointTypesByChannelTypeXAIChatModelDoesNotIncludeImageGeneration(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-4-fast-reasoning")

	require.Equal(t, []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
	}, got)
	require.NotContains(t, got, constant.EndpointTypeImageGeneration)
}
