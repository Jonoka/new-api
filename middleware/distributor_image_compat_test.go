package middleware

import "testing"

func TestResolveImageCompatibilityModelRoutesLegacyGPTImage2ToLite(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		model string
		want  string
	}{
		{name: "generation", path: "/v1/images/generations", model: "gpt-image-2", want: "gpt-image-2-lite"},
		{name: "canvas generation", path: "/canvas/v1/images/generations", model: "gpt-image-2", want: "gpt-image-2-lite"},
		{name: "edit", path: "/v1/images/edits", model: "gpt-image-2", want: "gpt-image-2-lite"},
		{name: "canvas edit", path: "/canvas/v1/images/edits", model: "gpt-image-2", want: "gpt-image-2-lite"},
		{name: "lite unchanged", path: "/v1/images/generations", model: "gpt-image-2-lite", want: "gpt-image-2-lite"},
		{name: "pro unchanged", path: "/v1/images/generations", model: "gpt-image-2-pro", want: "gpt-image-2-pro"},
		{name: "non image unchanged", path: "/v1/chat/completions", model: "gpt-image-2", want: "gpt-image-2"},
		{name: "task polling unchanged", path: "/v1/images/generations/task_123", model: "gpt-image-2", want: "gpt-image-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveImageCompatibilityModel(tt.path, tt.model); got != tt.want {
				t.Fatalf("resolveImageCompatibilityModel(%q, %q) = %q, want %q", tt.path, tt.model, got, tt.want)
			}
		})
	}
}
