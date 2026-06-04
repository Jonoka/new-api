package billingexpr_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

func TestImageTierQualityOverridesSize(t *testing.T) {
	tests := []struct {
		name    string
		size    string
		quality string
		want    string
	}{
		{name: "high is 4k", size: "1024x1024", quality: "high", want: "4k"},
		{name: "explicit 4K is 4k", size: "1024x1024", quality: "4K", want: "4k"},
		{name: "ultra is 4k", size: "1024x1024", quality: "ultra", want: "4k"},
		{name: "medium is 2k", size: "1024x1024", quality: "medium", want: "2k"},
		{name: "explicit 2K is 2k", size: "1024x1024", quality: "2K", want: "2k"},
		{name: "low is 1k", size: "2160x3840", quality: "low", want: "1k"},
		{name: "explicit 1K is 1k", size: "2160x3840", quality: "1K", want: "1k"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := billingexpr.ImageTier(tt.size, tt.quality); got != tt.want {
				t.Fatalf("ImageTier(%q,%q)=%q want %q", tt.size, tt.quality, got, tt.want)
			}
		})
	}
}

func TestImageTierAutoUsesResolution(t *testing.T) {
	tests := []struct {
		name string
		size string
		want string
	}{
		{name: "empty defaults 2k", size: "", want: "2k"},
		{name: "auto defaults 2k", size: "auto", want: "2k"},
		{name: "openai square 1024 is 1k", size: "1024x1024", want: "1k"},
		{name: "gpt2api low landscape is 1k", size: "1280x720", want: "1k"},
		{name: "gpt2api low portrait is 1k", size: "720x1280", want: "1k"},
		{name: "openai 1536 landscape is 2k", size: "1536x1024", want: "2k"},
		{name: "openai 1536 portrait is 2k", size: "1024x1536", want: "2k"},
		{name: "legacy dalle 1792 landscape is 2k", size: "1792x1024", want: "2k"},
		{name: "legacy dalle 1792 portrait is 2k", size: "1024x1792", want: "2k"},
		{name: "reported problematic portrait is 2k", size: "1024x1824", want: "2k"},
		{name: "gpt2api result portrait is 2k", size: "1440x2560", want: "2k"},
		{name: "gpt2api result landscape is 2k", size: "2560x1440", want: "2k"},
		{name: "4k portrait is 4k", size: "2160x3840", want: "4k"},
		{name: "4k landscape is 4k", size: "3840x2160", want: "4k"},
		{name: "custom under 2k max pixels is 2k", size: "1600x1600", want: "2k"},
		{name: "custom over 2k max pixels is 4k", size: "2500x2000", want: "4k"},
		{name: "invalid defaults 2k", size: "nonsense", want: "2k"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := billingexpr.ImageTier(tt.size, "auto"); got != tt.want {
				t.Fatalf("ImageTier(%q, auto)=%q want %q", tt.size, got, tt.want)
			}
		})
	}
}

func TestImageTierExpressionHelper(t *testing.T) {
	expr := `image_tier(param("size"), param("quality")) == "4k"
? tier("4k", 300000.0 * max(1.0, param("n") == nil ? 1.0 : param("n")))
: image_tier(param("size"), param("quality")) == "2k"
  ? tier("2k", 250000.0 * max(1.0, param("n") == nil ? 1.0 : param("n")))
  : tier("1k", 200000.0 * max(1.0, param("n") == nil ? 1.0 : param("n")))`
	tests := []struct {
		name      string
		body      string
		wantTier  string
		wantQuota float64
	}{
		{name: "auto 1024x1824 bills 2k", body: `{"size":"1024x1824","quality":"auto","n":1}`, wantTier: "2k", wantQuota: 250000},
		{name: "auto 1280x720 bills 1k", body: `{"size":"1280x720","quality":"auto","n":1}`, wantTier: "1k", wantQuota: 200000},
		{name: "high remains 4k", body: `{"size":"1024x1024","quality":"high","n":1}`, wantTier: "4k", wantQuota: 300000},
		{name: "n multiplier", body: `{"size":"1024x1824","quality":"auto","n":2}`, wantTier: "2k", wantQuota: 500000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, trace, err := billingexpr.RunExprWithRequest(expr, billingexpr.TokenParams{}, billingexpr.RequestInput{Body: []byte(tt.body)})
			if err != nil {
				t.Fatalf("RunExprWithRequest error: %v", err)
			}
			if trace.MatchedTier != tt.wantTier {
				t.Fatalf("tier=%s want %s", trace.MatchedTier, tt.wantTier)
			}
			if got != tt.wantQuota {
				t.Fatalf("quota=%v want %v", got, tt.wantQuota)
			}
		})
	}
}
