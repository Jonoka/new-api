package dto

import "testing"

func TestMapGPT2APIImageSizeUsesModelSpecificTables(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		quality string
		size    string
		want    string
	}{
		{name: "banana high 16x9 display size", model: "nano-banana-pro", quality: "high", size: "3840x2160", want: "5504x3072"},
		{name: "banana 4k explicit ratio", model: "nano-banana-v2", quality: "4k", size: "16:9", want: "5504x3072"},
		{name: "banana medium portrait", model: "nano-banana", quality: "medium", size: "2160x3840", want: "1536x2752"},
		{name: "gpt-image-2 high 16x9 display size", model: "gpt-image-2", quality: "high", size: "3840x2160", want: "3328x1872"},
		{name: "gpt-image-2 high portrait", model: "gpt-image-2", quality: "high", size: "2160x3840", want: "1872x3328"},
		{name: "gpt-image-2 medium 4x5", model: "gpt-image-2", quality: "medium", size: "1024x1280", want: "1792x2240"},
		{name: "unknown model unchanged", model: "dall-e-3", quality: "high", size: "3840x2160", want: "3840x2160"},
		{name: "auto quality unchanged", model: "nano-banana-pro", quality: "auto", size: "3840x2160", want: "3840x2160"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapGPT2APIImageSize(tt.model, tt.quality, tt.size); got != tt.want {
				t.Fatalf("MapGPT2APIImageSize(%q,%q,%q) = %q, want %q", tt.model, tt.quality, tt.size, got, tt.want)
			}
		})
	}
}
