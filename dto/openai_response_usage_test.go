package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

func TestResponsesUsageCacheCreationAliases(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{
			name: "input cache write tokens",
			body: `{"input_tokens":100,"input_tokens_details":{"cache_write_tokens":20,"cache_creation_tokens":30},"cache_write_tokens":40}`,
			want: 20,
		},
		{
			name: "prompt cache write tokens",
			body: `{"input_tokens":100,"prompt_tokens_details":{"cache_write_tokens":21},"input_tokens_details":{"cache_creation_tokens":30}}`,
			want: 21,
		},
		{
			name: "input cache creation tokens",
			body: `{"input_tokens":100,"input_tokens_details":{"cache_creation_tokens":30},"cache_write_tokens":40}`,
			want: 30,
		},
		{
			name: "legacy cached creation tokens",
			body: `{"input_tokens":100,"input_tokens_details":{"cached_creation_tokens":31}}`,
			want: 31,
		},
		{
			name: "top level cache write tokens",
			body: `{"input_tokens":100,"cache_write_tokens":40,"cache_creation_input_tokens":50,"cache_write_input_tokens":60,"cache_creation_tokens":70}`,
			want: 40,
		},
		{
			name: "top level cache creation input tokens",
			body: `{"input_tokens":100,"cache_creation_input_tokens":50,"cache_write_input_tokens":60,"cache_creation_tokens":70}`,
			want: 50,
		},
		{
			name: "top level cache write input tokens",
			body: `{"input_tokens":100,"cache_write_input_tokens":60,"cache_creation_tokens":70}`,
			want: 60,
		},
		{
			name: "top level cache creation tokens",
			body: `{"input_tokens":100,"cache_creation_tokens":70}`,
			want: 70,
		},
		{
			name: "missing cache creation",
			body: `{"input_tokens":100}`,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var usage Usage
			require.NoError(t, common.Unmarshal([]byte(tt.body), &usage))
			require.Equal(t, tt.want, usage.GetCacheCreationTokens())
		})
	}
}

func TestResponsesUsageSetCacheCreationTokensWritesAliases(t *testing.T) {
	usage := Usage{
		InputTokensDetails: &InputTokenDetails{},
	}

	usage.SetCacheCreationTokens(30)

	require.Equal(t, 30, usage.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CacheCreationTokens)
	require.Equal(t, 30, usage.PromptTokensDetails.CacheWriteTokens)
	require.Equal(t, 30, usage.InputTokensDetails.CachedCreationTokens)
	require.Equal(t, 30, usage.InputTokensDetails.CacheCreationTokens)
	require.Equal(t, 30, usage.InputTokensDetails.CacheWriteTokens)
	require.Equal(t, 30, usage.CacheCreationInputTokens)
	require.Equal(t, 30, usage.CacheWriteInputTokens)
	require.Equal(t, 30, usage.CacheWriteTokens)
	require.Equal(t, 30, usage.CacheCreationTokens)
}

func TestResponsesUsageTopLevelZeroAliasIsNotDetailCacheCreation(t *testing.T) {
	var usage Usage
	require.NoError(t, common.Unmarshal([]byte(`{"cache_creation_input_tokens":0,"input_tokens_details":{"cached_tokens":10}}`), &usage))

	require.True(t, usage.HasAnyCacheCreationTokensField())
	require.False(t, usage.HasAnyDetailCacheCreationTokensField())
	require.Equal(t, 0, usage.GetCacheCreationTokens())
}
