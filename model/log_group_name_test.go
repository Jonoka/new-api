package model

import "testing"

func TestApplyLogGroupNamesUsesCurrentDisplayName(t *testing.T) {
	logs := []*Log{
		{Group: "Codex-Team"},
		{Other: `{"group":"Codex-Team"}`},
		{Group: "unknown"},
	}

	applyLogGroupNames(logs, map[string]string{
		"Codex-Team": "Codex-福利组",
	})

	if logs[0].GroupName != "Codex-福利组" {
		t.Fatalf("直接分组未使用当前显示名: %q", logs[0].GroupName)
	}
	if logs[1].GroupName != "Codex-福利组" {
		t.Fatalf("Other 中的分组未使用当前显示名: %q", logs[1].GroupName)
	}
	if logs[2].GroupName != "unknown" {
		t.Fatalf("未知分组未回退原值: %q", logs[2].GroupName)
	}
}

func TestGetGroupDisplayNameMapIncludesDisabledGroupsAndAliases(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Group{}, &GroupAlias{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	group := &Group{
		Code:   "Codex-Team",
		Name:   "Codex-福利组",
		Ratio:  0.05,
		Status: GroupStatusDisabled,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("创建测试分组失败: %v", err)
	}
	if err := db.Create(&GroupAlias{Alias: "legacy-team", GroupId: group.Id}).Error; err != nil {
		t.Fatalf("创建测试别名失败: %v", err)
	}

	names, err := GetGroupDisplayNameMap()
	if err != nil {
		t.Fatalf("读取日志分组名称映射失败: %v", err)
	}
	for _, key := range []string{"Codex-Team", "legacy-team", "Codex-福利组"} {
		if names[key] != "Codex-福利组" {
			t.Fatalf("分组键 %q 未映射到当前名称: %#v", key, names)
		}
	}
}
