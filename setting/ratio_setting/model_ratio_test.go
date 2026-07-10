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
