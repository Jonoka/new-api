package reasoning

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "extra high", in: "extra high", want: "xhigh"},
		{name: "extra hyphen high", in: "extra-high", want: "xhigh"},
		{name: "extra underscore high", in: "extra_high", want: "xhigh"},
		{name: "max", in: "max", want: "max"},
		{name: "ultra", in: "Ultra", want: "ultra"},
		{name: "keeps supported", in: "High", want: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeOpenAIReasoningEffort(tt.in))
		})
	}
}

func TestParseOpenAIReasoningEffortFromModelSuffixSupportsMaxAndUltra(t *testing.T) {
	effort, baseModel := ParseOpenAIReasoningEffortFromModelSuffix("gpt-5.6-sol-max")
	require.Equal(t, "max", effort)
	require.Equal(t, "gpt-5.6-sol", baseModel)

	effort, baseModel = ParseOpenAIReasoningEffortFromModelSuffix("gpt-5.6-sol-ultra")
	require.Equal(t, "ultra", effort)
	require.Equal(t, "gpt-5.6-sol", baseModel)
}
