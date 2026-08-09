package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func preserveAutoGroupSettings(t *testing.T) {
	t.Helper()
	previousAutoConfig := setting.AutoGroupConfig2JsonString()
	previousAutoGroups := setting.AutoGroups2JsonString()
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	previousSpecialGroups := specialGroups.ReadAll()
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupConfigByJsonString(previousAutoConfig)
		_ = setting.UpdateAutoGroupsByJsonString(previousAutoGroups)
		_ = setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups)
		specialGroups.Clear()
		specialGroups.AddAll(previousSpecialGroups)
	})
}

func TestGetUserUsableGroupsSynthesizesVirtualAuto(t *testing.T) {
	preserveAutoGroupSettings(t)
	if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`); err != nil {
		t.Fatalf("设置用户可用分组失败: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["default"]`); err != nil {
		t.Fatalf("设置自动分组失败: %v", err)
	}
	if err := setting.UpdateAutoGroupConfigByJsonString(`{"user_selectable":true,"description":"按健康度自动选择"}`); err != nil {
		t.Fatalf("设置 auto 配置失败: %v", err)
	}

	groups := GetUserUsableGroups("")
	if groups["auto"] != "按健康度自动选择" {
		t.Fatalf("未合成虚拟 auto 或描述错误: %#v", groups)
	}
	autoGroups := GetUserAutoGroup("")
	if len(autoGroups) != 1 || autoGroups[0] != "default" {
		t.Fatalf("自动分组目标错误: %#v", autoGroups)
	}
}

func TestGetUserUsableGroupsHonorsAutoAvailabilityAndOverrides(t *testing.T) {
	preserveAutoGroupSettings(t)
	if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`); err != nil {
		t.Fatalf("设置用户可用分组失败: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["default"]`); err != nil {
		t.Fatalf("设置自动分组失败: %v", err)
	}
	if err := setting.UpdateAutoGroupConfigByJsonString(`{"user_selectable":true,"description":"自动选择"}`); err != nil {
		t.Fatalf("设置 auto 配置失败: %v", err)
	}

	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	specialGroups.Set("vip", map[string]string{"-:auto": ""})
	if _, ok := GetUserUsableGroups("vip")["auto"]; ok {
		t.Fatal("用户分组 -:auto 规则未生效")
	}

	if err := setting.UpdateAutoGroupConfigByJsonString(`{"user_selectable":false,"description":"保留描述"}`); err != nil {
		t.Fatalf("关闭 auto 配置失败: %v", err)
	}
	if _, ok := GetUserUsableGroups("")["auto"]; ok {
		t.Fatal("关闭用户可选后仍返回 auto")
	}

	if err := setting.UpdateAutoGroupConfigByJsonString(`{"user_selectable":true,"description":"自动选择"}`); err != nil {
		t.Fatalf("重新启用 auto 配置失败: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`["missing"]`); err != nil {
		t.Fatalf("设置无效自动目标失败: %v", err)
	}
	if _, ok := GetUserUsableGroups("")["auto"]; ok {
		t.Fatal("没有可用自动目标时不应返回 auto")
	}
}
