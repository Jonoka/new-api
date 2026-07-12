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
		{
			name: "input detail explicit zero overrides top level alias",
			body: `{"input_tokens":20,"output_tokens":2,"cache_creation_input_tokens":19,"input_tokens_details":{"cache_write_tokens":0}}`,
			want: 0,
		},
		{
			name: "prompt detail explicit zero overrides top level alias",
			body: `{"input_tokens":20,"output_tokens":2,"cache_write_tokens":19,"prompt_tokens_details":{"cache_creation_tokens":0}}`,
			want: 0,
		},
		{
			name: "input cache write explicit zero overrides later detail aliases",
			body: `{"input_tokens":20,"output_tokens":2,"input_tokens_details":{"cache_write_tokens":0,"cache_creation_tokens":7,"cached_creation_tokens":8}}`,
			want: 0,
		},
		{
			name: "input cache creation explicit zero overrides old detail alias",
			body: `{"input_tokens":20,"output_tokens":2,"input_tokens_details":{"cache_creation_tokens":0,"cached_creation_tokens":8}}`,
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

func TestOpenAITextResponseUnmarshalKeepsEnvelopeAndUsage(t *testing.T) {
	var response OpenAITextResponse
	body := `{"id":"chatcmpl-test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30,"cache_write_tokens":4}}`

	require.NoError(t, common.Unmarshal([]byte(body), &response))
	require.Equal(t, "chatcmpl-test", response.Id)
	require.Len(t, response.Choices, 1)
	require.Equal(t, 10, response.Usage.PromptTokens)
	require.Equal(t, 20, response.Usage.CompletionTokens)
	require.Equal(t, 30, response.Usage.TotalTokens)
	require.Equal(t, 4, response.Usage.CacheWriteTokens)
	require.True(t, response.Usage.HasCacheWriteTokens)
}

func TestUsageResponseEnvelopesUnmarshalNestedUsage(t *testing.T) {
	body := []byte(`{"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)

	t.Run("simple response", func(t *testing.T) {
		var response SimpleResponse
		require.NoError(t, common.Unmarshal(body, &response))
		require.Equal(t, 10, response.Usage.PromptTokens)
	})

	t.Run("text response", func(t *testing.T) {
		var response TextResponse
		require.NoError(t, common.Unmarshal(body, &response))
		require.Equal(t, 10, response.Usage.PromptTokens)
	})

	t.Run("OpenAI embedding response", func(t *testing.T) {
		var response OpenAIEmbeddingResponse
		require.NoError(t, common.Unmarshal(body, &response))
		require.Equal(t, 10, response.Usage.PromptTokens)
	})

	t.Run("flexible embedding response", func(t *testing.T) {
		var response FlexibleEmbeddingResponse
		require.NoError(t, common.Unmarshal(body, &response))
		require.Equal(t, 10, response.Usage.PromptTokens)
	})

	t.Run("generic embedding response", func(t *testing.T) {
		var response EmbeddingResponse
		embeddingBody := []byte(`{"object":"list","model":"embedding-test","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
		require.NoError(t, common.Unmarshal(embeddingBody, &response))
		require.Equal(t, "list", response.Object)
		require.Equal(t, "embedding-test", response.Model)
		require.Len(t, response.Data, 1)
		require.Equal(t, 10, response.Usage.PromptTokens)
	})
}
