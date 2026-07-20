package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	autoConfig := setting.GetAutoGroupConfig()
	// auto 是虚拟令牌分组，不属于实体分组投影。运行时从独立配置合成，
	// 再应用用户分组的 +/- 规则，保留旧配置的覆盖语义。
	delete(groupsCopy, "auto")
	if autoConfig.UserSelectable {
		groupsCopy["auto"] = autoConfig.Description
	}
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	if description, ok := groupsCopy["auto"]; ok {
		description = strings.TrimSpace(description)
		if description == "" || strings.EqualFold(description, "auto") {
			description = autoConfig.Description
		}
		hasUsableTarget := false
		for _, group := range setting.GetAutoGroups() {
			group = strings.TrimSpace(group)
			if group == "" || strings.EqualFold(group, "auto") {
				continue
			}
			if _, usable := groupsCopy[group]; usable {
				hasUsableTarget = true
				break
			}
		}
		if hasUsableTarget {
			groupsCopy["auto"] = description
		} else {
			delete(groupsCopy, "auto")
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	if _, ok := groups["auto"]; !ok {
		return []string{}
	}
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if strings.EqualFold(strings.TrimSpace(group), "auto") {
			continue
		}
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}
