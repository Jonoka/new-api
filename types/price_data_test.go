package types

import "testing"

func TestApplyTaskRatiosToFloatUsesSecondsForPerSecondPrice(t *testing.T) {
	price := PriceData{
		UsePrice:       true,
		ModelPriceUnit: ModelPriceUnitSecond,
		OtherRatios: map[string]float64{
			"seconds":    8,
			"resolution": 1.4,
		},
	}

	if got := price.ApplyTaskRatiosToFloat(100); got != 1120 {
		t.Fatalf("ApplyTaskRatiosToFloat() = %v, want 1120", got)
	}
}

func TestApplyTaskRatiosToFloatSkipsSecondsForPerRequestPrice(t *testing.T) {
	price := PriceData{
		UsePrice:       true,
		ModelPriceUnit: ModelPriceUnitRequest,
		OtherRatios: map[string]float64{
			"seconds":    8,
			"resolution": 1.4,
		},
	}

	if got := price.ApplyTaskRatiosToFloat(100); got != 140 {
		t.Fatalf("ApplyTaskRatiosToFloat() = %v, want 140", got)
	}
}

func TestApplyTaskRatiosToFloatKeepsLegacyRatioBilling(t *testing.T) {
	price := PriceData{
		UsePrice: false,
		OtherRatios: map[string]float64{
			"seconds": 8,
		},
	}

	if got := price.ApplyTaskRatiosToFloat(100); got != 800 {
		t.Fatalf("ApplyTaskRatiosToFloat() = %v, want 800", got)
	}
}
