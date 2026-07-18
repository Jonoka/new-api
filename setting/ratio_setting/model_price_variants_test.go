package ratio_setting

import (
	"math"
	"testing"
)

func TestDefaultModelPriceVariantUsesCurrentBasePrice(t *testing.T) {
	savedPrices := ModelPrice2JSONString()
	savedVariants := ModelPriceVariants2JSONString()
	t.Cleanup(func() {
		_ = UpdateModelPriceByJSONString(savedPrices)
		_ = UpdateModelPriceVariantsByJSONString(savedVariants)
	})

	if err := UpdateModelPriceByJSONString(`{"grok-imagine-video-1.5":0.09}`); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString() error = %v", err)
	}
	if err := UpdateModelPriceVariantsByJSONString(`{}`); err != nil {
		t.Fatalf("UpdateModelPriceVariantsByJSONString() error = %v", err)
	}

	config, ok := GetModelPriceVariantConfig("grok-imagine-video-1.5")
	if !ok || !config.ResolutionEnabled || config.QualityEnabled || !config.Inherited {
		t.Fatalf("default config = %#v, ok = %v", config, ok)
	}
	want := map[string]float64{
		"480p":  0.09,
		"720p":  0.09 * (0.14 / 0.08),
		"1080p": 0.09 * (0.25 / 0.08),
	}
	for _, rule := range config.Rules {
		if rule.Price == nil || math.Abs(*rule.Price-want[rule.Resolution]) > 1e-12 {
			t.Fatalf("rule %s price = %v, want %v", rule.Resolution, rule.Price, want[rule.Resolution])
		}
		delete(want, rule.Resolution)
	}
	if len(want) != 0 {
		t.Fatalf("missing default rules: %#v", want)
	}
}

func TestInheritedModelPriceVariantsAreNotPersistedAsOverrides(t *testing.T) {
	savedPrices := ModelPrice2JSONString()
	savedVariants := ModelPriceVariants2JSONString()
	t.Cleanup(func() {
		_ = UpdateModelPriceByJSONString(savedPrices)
		_ = UpdateModelPriceVariantsByJSONString(savedVariants)
	})

	if err := UpdateModelPriceVariantsByJSONString(`{}`); err != nil {
		t.Fatalf("clear variants error = %v", err)
	}
	effectiveDefaults := ModelPriceVariants2JSONString()
	if err := UpdateModelPriceVariantsByJSONString(effectiveDefaults); err != nil {
		t.Fatalf("round-trip effective variants error = %v", err)
	}
	if err := UpdateModelPriceByJSONString(`{"grok-imagine-video":0.1}`); err != nil {
		t.Fatalf("update base price error = %v", err)
	}
	config, ok := GetModelPriceVariantConfig("grok-imagine-video")
	if !ok || !config.Inherited {
		t.Fatalf("config = %#v, ok = %v", config, ok)
	}
	for _, rule := range config.Rules {
		if rule.Resolution == "480p" && (rule.Price == nil || *rule.Price != 0.1) {
			t.Fatalf("480p price = %v, want dynamic 0.1", rule.Price)
		}
	}
}

func TestModelPriceVariantMatchesResolutionAndQuality(t *testing.T) {
	savedVariants := ModelPriceVariants2JSONString()
	t.Cleanup(func() { _ = UpdateModelPriceVariantsByJSONString(savedVariants) })

	configJSON := `{
		"custom-video": {
			"resolution_enabled": true,
			"quality_enabled": true,
			"rules": [
				{"resolution":"HD","quality":"HIGH","price":0.22},
				{"resolution":"480","quality":"standard","price":0.08}
			]
		}
	}`
	if err := UpdateModelPriceVariantsByJSONString(configJSON); err != nil {
		t.Fatalf("UpdateModelPriceVariantsByJSONString() error = %v", err)
	}

	matched := MatchModelPriceVariant("custom-video", map[string]string{
		ModelPriceVariantResolution: "720p",
		ModelPriceVariantQuality:    "High",
	})
	if !matched.Configured || !matched.Matched || matched.Price != 0.22 {
		t.Fatalf("matched = %#v", matched)
	}

	unmatched := MatchModelPriceVariant("custom-video", map[string]string{
		ModelPriceVariantResolution: "1080p",
		ModelPriceVariantQuality:    "high",
	})
	if !unmatched.Configured || unmatched.Matched {
		t.Fatalf("unmatched = %#v", unmatched)
	}
}

func TestModelPriceVariantCanDisableBuiltInResolutionPricing(t *testing.T) {
	savedVariants := ModelPriceVariants2JSONString()
	t.Cleanup(func() { _ = UpdateModelPriceVariantsByJSONString(savedVariants) })

	if err := UpdateModelPriceVariantsByJSONString(`{
		"grok-imagine-video":{"resolution_enabled":false,"quality_enabled":false}
	}`); err != nil {
		t.Fatalf("UpdateModelPriceVariantsByJSONString() error = %v", err)
	}
	config, ok := GetModelPriceVariantConfig("grok-imagine-video")
	if !ok || config.ResolutionEnabled || config.QualityEnabled || len(config.Rules) != 0 {
		t.Fatalf("disabled config = %#v, ok = %v", config, ok)
	}
	match := MatchModelPriceVariant("grok-imagine-video", map[string]string{ModelPriceVariantResolution: "720p"})
	if !match.Configured || match.Matched {
		t.Fatalf("disabled match = %#v", match)
	}
}

func TestModelPriceVariantValidationRejectsIncompleteOrDuplicateRules(t *testing.T) {
	tests := []string{
		`{"video":{"resolution_enabled":true,"rules":[]}}`,
		`{"video":{"resolution_enabled":true,"rules":[{"price":0.1}]}}`,
		`{"video":{"resolution_enabled":true,"rules":[{"resolution":"720p"}]}}`,
		`{"video":{"quality_enabled":true,"rules":[{"quality":"high","price":-1}]}}`,
		`{"video":{"resolution_enabled":true,"rules":[{"resolution":"720p","price":0.1},{"resolution":"HD","price":0.2}]}}`,
	}
	for _, body := range tests {
		if err := CheckModelPriceVariantsJSONString(body); err == nil {
			t.Fatalf("CheckModelPriceVariantsJSONString(%s) error = nil", body)
		}
	}
}

func TestModelPriceVariantAllowsExplicitZeroPrice(t *testing.T) {
	savedVariants := ModelPriceVariants2JSONString()
	t.Cleanup(func() { _ = UpdateModelPriceVariantsByJSONString(savedVariants) })
	if err := UpdateModelPriceVariantsByJSONString(`{
		"free-preview":{"quality_enabled":true,"rules":[{"quality":"preview","price":0}]}
	}`); err != nil {
		t.Fatalf("explicit zero price error = %v", err)
	}
	match := MatchModelPriceVariant("free-preview", map[string]string{"quality": "preview"})
	if !match.Matched || match.Price != 0 {
		t.Fatalf("zero price match = %#v", match)
	}
}
