package controller

import (
	"reflect"
	"testing"
)

func TestBuildPricingGroupNamesUsesDisplayNameAndFallback(t *testing.T) {
	groupRatio := map[string]float64{
		"vip":      0.5,
		"fallback": 1,
	}
	activeGroupNames := map[string]string{
		"vip":     "尊贵用户",
		"ignored": "不可用分组",
	}

	got := buildPricingGroupNames(groupRatio, activeGroupNames)
	want := map[string]string{
		"vip":      "尊贵用户",
		"fallback": "fallback",
		"ignored":  "不可用分组",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("模型广场分组显示名称错误：got %#v, want %#v", got, want)
	}
}
