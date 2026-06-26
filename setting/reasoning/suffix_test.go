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
		{name: "max", in: "max", want: "xhigh"},
		{name: "keeps supported", in: "High", want: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeOpenAIReasoningEffort(tt.in))
		})
	}
}
