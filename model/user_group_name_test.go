package model

import "testing"

func TestApplyUserGroupNamesUsesCurrentDisplayName(t *testing.T) {
	users := []*User{
		{Group: "Codex-Team"},
		{Group: "unknown"},
	}

	applyUserGroupNames(users, map[string]string{
		"Codex-Team": "Codex-福利组",
	})

	if users[0].GroupName != "Codex-福利组" {
		t.Fatalf("用户分组未使用当前显示名: %q", users[0].GroupName)
	}
	if users[1].GroupName != "unknown" {
		t.Fatalf("未知用户分组未回退原值: %q", users[1].GroupName)
	}
}
