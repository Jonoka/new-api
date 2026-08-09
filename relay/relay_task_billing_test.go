package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

func TestRecalcQuotaFromRatiosSkipsSecondsForPerRequestPrice(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota:          560,
			UsePrice:       true,
			ModelPriceUnit: types.ModelPriceUnitRequest,
			OtherRatios: map[string]float64{
				"seconds":    8,
				"resolution": 1.4,
			},
		},
	}

	got := recalcQuotaFromRatios(info, map[string]float64{
		"seconds":    10,
		"resolution": 1.4,
	})
	if got != 560 {
		t.Fatalf("recalcQuotaFromRatios() = %d, want 560", got)
	}
}

func TestApplyTaskVariantPriceUsesAbsoluteResolutionPrice(t *testing.T) {
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	t.Cleanup(func() { _ = ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants) })
	if err := ratio_setting.UpdateModelPriceVariantsByJSONString(`{}`); err != nil {
		t.Fatalf("UpdateModelPriceVariantsByJSONString() error = %v", err)
	}

	baseQuota, _ := common.QuotaFromFloatStrict(0.08 * common.QuotaPerUnit)
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video-1.5",
		PriceData: types.PriceData{
			ModelPrice:     0.08,
			ModelPriceUnit: types.ModelPriceUnitSecond,
			UsePrice:       true,
			Quota:          baseQuota,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			OtherRatios: map[string]float64{
				"seconds":    10,
				"resolution": 1.75,
			},
		},
	}
	if err := applyTaskVariantPrice(info, channel.TaskBillingSpec{
		Dimensions:      map[string]string{"resolution": "720p"},
		LegacyRatioKeys: []string{"resolution"},
	}); err != nil {
		t.Fatalf("applyTaskVariantPrice() error = %v", err)
	}
	if info.PriceData.ModelPrice != 0.14 {
		t.Fatalf("ModelPrice = %v, want 0.14", info.PriceData.ModelPrice)
	}
	if _, exists := info.PriceData.OtherRatios["resolution"]; exists {
		t.Fatal("legacy resolution ratio was not removed")
	}
	if info.PriceData.OtherRatios["seconds"] != 10 {
		t.Fatalf("seconds ratio = %v, want 10", info.PriceData.OtherRatios["seconds"])
	}
	wantBaseQuota, _ := common.QuotaFromFloatStrict(0.14 * common.QuotaPerUnit)
	if info.PriceData.Quota != wantBaseQuota {
		t.Fatalf("base quota = %d, want %d", info.PriceData.Quota, wantBaseQuota)
	}
	if got := info.PriceData.BillingMeta["variant_price_status"]; got != "matched" {
		t.Fatalf("variant status = %q, want matched", got)
	}
	withSeconds := info.PriceData.ApplyTaskRatiosToFloat(float64(info.PriceData.Quota))
	if withSeconds != float64(wantBaseQuota)*10 {
		t.Fatalf("quota with seconds = %v, want %v", withSeconds, float64(wantBaseQuota)*10)
	}
}

func TestApplyTaskVariantPriceCanDisableResolutionAndUsePerRequestPrice(t *testing.T) {
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	t.Cleanup(func() { _ = ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants) })
	if err := ratio_setting.UpdateModelPriceVariantsByJSONString(`{
		"grok-imagine-video":{"resolution_enabled":false,"quality_enabled":false}
	}`); err != nil {
		t.Fatalf("UpdateModelPriceVariantsByJSONString() error = %v", err)
	}

	baseQuota, _ := common.QuotaFromFloatStrict(0.5 * common.QuotaPerUnit)
	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video",
		PriceData: types.PriceData{
			ModelPrice:     0.5,
			ModelPriceUnit: types.ModelPriceUnitRequest,
			UsePrice:       true,
			Quota:          baseQuota,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			OtherRatios: map[string]float64{
				"seconds":    10,
				"resolution": 1.4,
			},
		},
	}
	if err := applyTaskVariantPrice(info, channel.TaskBillingSpec{
		Dimensions:      map[string]string{"resolution": "720p"},
		LegacyRatioKeys: []string{"resolution"},
	}); err != nil {
		t.Fatalf("applyTaskVariantPrice() error = %v", err)
	}
	if info.PriceData.ModelPrice != 0.5 || info.PriceData.Quota != baseQuota {
		t.Fatalf("price/quota = %v/%d, want 0.5/%d", info.PriceData.ModelPrice, info.PriceData.Quota, baseQuota)
	}
	if _, exists := info.PriceData.OtherRatios["resolution"]; exists {
		t.Fatal("disabled resolution ratio was not removed")
	}
	if got := info.PriceData.ApplyTaskRatiosToFloat(float64(info.PriceData.Quota)); got != float64(baseQuota) {
		t.Fatalf("per-request quota = %v, want %v", got, float64(baseQuota))
	}
	if got := info.PriceData.BillingMeta["variant_price_status"]; got != "disabled" {
		t.Fatalf("variant status = %q, want disabled", got)
	}
}

func TestApplyTaskVariantPriceKeepsLegacyRatioWhenTierIsMissing(t *testing.T) {
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	t.Cleanup(func() { _ = ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants) })
	if err := ratio_setting.UpdateModelPriceVariantsByJSONString(`{
		"custom-video":{"resolution_enabled":true,"rules":[{"resolution":"480p","price":0.08}]}
	}`); err != nil {
		t.Fatalf("UpdateModelPriceVariantsByJSONString() error = %v", err)
	}
	baseQuota, _ := common.QuotaFromFloatStrict(0.08 * common.QuotaPerUnit)
	info := &relaycommon.RelayInfo{
		OriginModelName: "custom-video",
		PriceData: types.PriceData{
			ModelPrice:     0.08,
			ModelPriceUnit: types.ModelPriceUnitSecond,
			UsePrice:       true,
			Quota:          baseQuota,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			OtherRatios: map[string]float64{
				"seconds":    10,
				"resolution": 3.125,
			},
		},
	}
	if err := applyTaskVariantPrice(info, channel.TaskBillingSpec{
		Dimensions:      map[string]string{"resolution": "1080p"},
		LegacyRatioKeys: []string{"resolution"},
	}); err != nil {
		t.Fatalf("applyTaskVariantPrice() error = %v", err)
	}
	if got := info.PriceData.OtherRatios["resolution"]; got != 3.125 {
		t.Fatalf("legacy resolution ratio = %v, want 3.125", got)
	}
	if got := info.PriceData.BillingMeta["variant_price_status"]; got != "legacy" {
		t.Fatalf("variant status = %q, want legacy", got)
	}
}

func TestApplyTaskVariantPriceReplacesSoraSizeRatio(t *testing.T) {
	savedVariants := ratio_setting.ModelPriceVariants2JSONString()
	t.Cleanup(func() { _ = ratio_setting.UpdateModelPriceVariantsByJSONString(savedVariants) })
	if err := ratio_setting.UpdateModelPriceVariantsByJSONString(`{
		"sora-custom":{"resolution_enabled":true,"rules":[{"resolution":"1792x1024","price":0.5}]}
	}`); err != nil {
		t.Fatalf("UpdateModelPriceVariantsByJSONString() error = %v", err)
	}
	baseQuota, _ := common.QuotaFromFloatStrict(0.3 * common.QuotaPerUnit)
	info := &relaycommon.RelayInfo{
		OriginModelName: "sora-custom",
		PriceData: types.PriceData{
			ModelPrice:     0.3,
			ModelPriceUnit: types.ModelPriceUnitSecond,
			UsePrice:       true,
			Quota:          baseQuota,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			OtherRatios: map[string]float64{
				"seconds": 4,
				"size":    1.666667,
			},
		},
	}
	if err := applyTaskVariantPrice(info, channel.TaskBillingSpec{
		Dimensions:      map[string]string{"resolution": "1792x1024"},
		LegacyRatioKeys: []string{"size"},
	}); err != nil {
		t.Fatalf("applyTaskVariantPrice() error = %v", err)
	}
	if info.PriceData.ModelPrice != 0.5 {
		t.Fatalf("ModelPrice = %v, want 0.5", info.PriceData.ModelPrice)
	}
	if _, exists := info.PriceData.OtherRatios["size"]; exists {
		t.Fatal("legacy Sora size ratio was not removed")
	}
	if info.PriceData.OtherRatios["seconds"] != 4 {
		t.Fatalf("seconds = %v, want 4", info.PriceData.OtherRatios["seconds"])
	}
}

func TestRecalcQuotaFromRatiosUsesSecondsForPerSecondPrice(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota:          4480,
			UsePrice:       true,
			ModelPriceUnit: types.ModelPriceUnitSecond,
			OtherRatios: map[string]float64{
				"seconds":    8,
				"resolution": 1.4,
			},
		},
	}

	got := recalcQuotaFromRatios(info, map[string]float64{
		"seconds":    10,
		"resolution": 1.4,
	})
	if got != 5600 {
		t.Fatalf("recalcQuotaFromRatios() = %d, want 5600", got)
	}
}
