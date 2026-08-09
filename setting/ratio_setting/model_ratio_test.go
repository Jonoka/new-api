package ratio_setting

import "testing"

func TestGetCompletionRatioPrefersConfiguredRatioForGPT5(t *testing.T) {
	oldCompletionRatios := completionRatioMap.ReadAll()
	defer func() {
		completionRatioMap.Clear()
		completionRatioMap.AddAll(oldCompletionRatios)
	}()

	completionRatioMap.Clear()
	completionRatioMap.AddAll(map[string]float64{
		"gpt-5.6-sol": 6,
	})

	if got := GetCompletionRatio("gpt-5.6-sol"); got != 6 {
		t.Fatalf("GetCompletionRatio() = %v, want configured ratio 6", got)
	}

	info := GetCompletionRatioInfo("gpt-5.6-sol")
	if info.Locked {
		t.Fatalf("GetCompletionRatioInfo().Locked = true, want false")
	}
	if info.Ratio != 6 {
		t.Fatalf("GetCompletionRatioInfo().Ratio = %v, want 6", info.Ratio)
	}
}

func TestGetCompletionRatioFallsBackToGPT5Default(t *testing.T) {
	oldCompletionRatios := completionRatioMap.ReadAll()
	defer func() {
		completionRatioMap.Clear()
		completionRatioMap.AddAll(oldCompletionRatios)
	}()

	completionRatioMap.Clear()

	if got := GetCompletionRatio("gpt-5.6-sol"); got != 8 {
		t.Fatalf("GetCompletionRatio() = %v, want default ratio 8", got)
	}

	info := GetCompletionRatioInfo("gpt-5.6-sol")
	if info.Locked {
		t.Fatalf("GetCompletionRatioInfo().Locked = true, want false")
	}
	if info.Ratio != 8 {
		t.Fatalf("GetCompletionRatioInfo().Ratio = %v, want default ratio 8", info.Ratio)
	}
}

func TestDefaultXAIModelPricesArePerSecondBasePrices(t *testing.T) {
	prices := GetDefaultModelPriceMap()
	if got := prices["grok-imagine-video"]; got != 0.05 {
		t.Fatalf("grok-imagine-video price = %v, want 0.05", got)
	}
	if got := prices["grok-imagine-video-1.5"]; got != 0.08 {
		t.Fatalf("grok-imagine-video-1.5 price = %v, want 0.08", got)
	}
	if got := GetDefaultModelPriceUnitMap()["grok-imagine-video"]; got != "second" {
		t.Fatalf("grok-imagine-video price unit = %q, want second", got)
	}
	if got := GetDefaultModelPriceUnitMap()["grok-imagine-video-1.5"]; got != "second" {
		t.Fatalf("grok-imagine-video-1.5 price unit = %q, want second", got)
	}
}

func TestUpdateModelPriceMergesBuiltInDefaultsAndPreservesOverrides(t *testing.T) {
	original := modelPriceMap.ReadAll()
	defer func() {
		modelPriceMap.Clear()
		modelPriceMap.AddAll(original)
	}()

	if err := UpdateModelPriceByJSONString(`{"custom-video":0,"grok-imagine-video-1.5":0.09}`); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString() error = %v", err)
	}

	prices := GetModelPriceMap()
	if got := prices["custom-video"]; got != 0 {
		t.Fatalf("custom-video explicit zero price = %v, want 0", got)
	}
	if got := prices["grok-imagine-video-1.5"]; got != 0.09 {
		t.Fatalf("grok-imagine-video-1.5 override = %v, want 0.09", got)
	}
	if got := prices["grok-imagine-video"]; got != 0.05 {
		t.Fatalf("sparse update dropped built-in grok price: %v", got)
	}
}

func TestUpdateModelPriceInvalidJsonDoesNotChangeCurrentConfig(t *testing.T) {
	original := modelPriceMap.ReadAll()
	defer func() {
		modelPriceMap.Clear()
		modelPriceMap.AddAll(original)
	}()

	if err := UpdateModelPriceByJSONString(`{"custom-video":0.12}`); err != nil {
		t.Fatalf("UpdateModelPriceByJSONString() error = %v", err)
	}
	before := ModelPrice2JSONString()
	if err := UpdateModelPriceByJSONString(`{"custom-video":`); err == nil {
		t.Fatal("UpdateModelPriceByJSONString(invalid JSON) error = nil")
	}
	if after := ModelPrice2JSONString(); after != before {
		t.Fatalf("invalid update changed prices: before=%s after=%s", before, after)
	}
}

func TestUpdateModelPriceUnitRejectsInvalidUnitWithoutChangingCurrentConfig(t *testing.T) {
	original := modelPriceUnitMap.ReadAll()
	defer func() {
		modelPriceUnitMap.Clear()
		modelPriceUnitMap.AddAll(original)
	}()

	if err := UpdateModelPriceUnitByJSONString(`{"video-model":"second"}`); err != nil {
		t.Fatalf("UpdateModelPriceUnitByJSONString() error = %v", err)
	}
	if got := GetModelPriceUnit("video-model"); got != "second" {
		t.Fatalf("GetModelPriceUnit() = %q, want second", got)
	}
	if got := GetModelPriceUnit("grok-imagine-video"); got != "second" {
		t.Fatalf("sparse update dropped built-in grok unit: %q", got)
	}
	if got := GetModelPriceUnitCopy()["grok-imagine-video"]; got != "second" {
		t.Fatalf("effective unit map dropped built-in grok unit: %q", got)
	}

	if err := UpdateModelPriceUnitByJSONString(`{"video-model":"minute"}`); err == nil {
		t.Fatal("UpdateModelPriceUnitByJSONString() error = nil, want invalid unit error")
	}
	if err := UpdateModelPriceUnitByJSONString(`null`); err == nil {
		t.Fatal("UpdateModelPriceUnitByJSONString(null) error = nil, want object error")
	}
	if got := GetModelPriceUnit("video-model"); got != "second" {
		t.Fatalf("invalid update changed unit to %q", got)
	}
}

func TestGetModelPriceUnitFallsBackToBuiltInVideoUnit(t *testing.T) {
	original := modelPriceUnitMap.ReadAll()
	defer func() {
		modelPriceUnitMap.Clear()
		modelPriceUnitMap.AddAll(original)
	}()

	modelPriceUnitMap.Clear()
	modelPriceUnitMap.AddAll(map[string]string{
		"custom-video": "second",
	})

	if got := GetModelPriceUnit("grok-imagine-video"); got != "second" {
		t.Fatalf("GetModelPriceUnit(default video) = %q, want second", got)
	}
	if got := GetModelPriceUnit("custom-video"); got != "second" {
		t.Fatalf("GetModelPriceUnit(custom video) = %q, want second", got)
	}
	if got := GetModelPriceUnit("ordinary-fixed-price-model"); got != "request" {
		t.Fatalf("GetModelPriceUnit(ordinary model) = %q, want request", got)
	}
}
