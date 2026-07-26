package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestValidateTokenExclusiveGroupBinding(t *testing.T) {
	defaultGroup, exclusiveGroup := setupGroupBindingsTest(t)
	if err := DB.Model(&Group{}).Where("id = ?", exclusiveGroup.Id).Update("exclusive", true).Error; err != nil {
		t.Fatalf("设置独立分组失败: %v", err)
	}
	single := &Token{GroupMode: TokenGroupModeExplicit, GroupIds: []int{exclusiveGroup.Id}}
	if err := ValidateTokenExclusiveGroupBinding(DB, single); err != nil {
		t.Fatalf("独立分组单独绑定应该成功: %v", err)
	}

	conflict := &Token{GroupMode: TokenGroupModeExplicit, GroupIds: []int{exclusiveGroup.Id, defaultGroup.Id}}
	if err := ValidateTokenExclusiveGroupBinding(DB, conflict); !errors.Is(err, ErrTokenGroupBindingConflict) {
		t.Fatalf("独立分组与普通分组应该冲突，实际: %v", err)
	}

	if err := DB.Model(&Group{}).Where("id = ?", exclusiveGroup.Id).Update("exclusive", false).Error; err != nil {
		t.Fatalf("取消独立分组失败: %v", err)
	}
	if err := ValidateTokenExclusiveGroupBinding(DB, conflict); err != nil {
		t.Fatalf("普通多分组绑定应该保持可用: %v", err)
	}
}

func TestValidateTokenExclusiveGroupBindingCachedRefreshesAfterInvalidation(t *testing.T) {
	defaultGroup, exclusiveGroup := setupGroupBindingsTest(t)
	if err := DB.Model(&Group{}).Where("id = ?", exclusiveGroup.Id).Update("exclusive", true).Error; err != nil {
		t.Fatalf("设置独立分组失败: %v", err)
	}
	InvalidateExclusiveGroupSnapshot()
	token := &Token{GroupMode: TokenGroupModeExplicit, GroupIds: []int{exclusiveGroup.Id, defaultGroup.Id}}
	if err := ValidateTokenExclusiveGroupBindingCached(token); !errors.Is(err, ErrTokenGroupBindingConflict) {
		t.Fatalf("快照应识别独立分组冲突，实际: %v", err)
	}

	if err := DB.Model(&Group{}).Where("id = ?", exclusiveGroup.Id).Update("exclusive", false).Error; err != nil {
		t.Fatalf("取消独立分组失败: %v", err)
	}
	if err := ValidateTokenExclusiveGroupBindingCached(token); !errors.Is(err, ErrTokenGroupBindingConflict) {
		t.Fatalf("未失效的快照应避免重复查库，实际: %v", err)
	}
	InvalidateExclusiveGroupSnapshot()
	if err := ValidateTokenExclusiveGroupBindingCached(token); err != nil {
		t.Fatalf("快照失效后应读取最新独立属性: %v", err)
	}
}

func TestTokenInsertRejectsExclusiveGroupConflict(t *testing.T) {
	defaultGroup, exclusiveGroup := setupGroupBindingsTest(t)
	if err := DB.Model(&Group{}).Where("id = ?", exclusiveGroup.Id).Update("exclusive", true).Error; err != nil {
		t.Fatalf("设置独立分组失败: %v", err)
	}
	token := &Token{
		UserId:         1,
		Key:            "exclusive-conflict-insert",
		Name:           "exclusive-conflict",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{exclusiveGroup.Id, defaultGroup.Id},
	}
	if err := token.Insert(); !errors.Is(err, ErrTokenGroupBindingConflict) {
		t.Fatalf("新建冲突令牌应该失败，实际: %v", err)
	}
	var count int64
	if err := DB.Model(&Token{}).Where("key = ?", token.Key).Count(&count).Error; err != nil {
		t.Fatalf("查询令牌失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("冲突令牌不应写入数据库: %d", count)
	}
}

func TestTokenUpdateRejectsExclusiveGroupConflict(t *testing.T) {
	defaultGroup, exclusiveGroup := setupGroupBindingsTest(t)
	if err := DB.Model(&Group{}).Where("id = ?", exclusiveGroup.Id).Update("exclusive", true).Error; err != nil {
		t.Fatalf("设置独立分组失败: %v", err)
	}
	token := &Token{
		UserId: 1, Key: "exclusive-update", Name: "exclusive-update",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
		GroupMode: TokenGroupModeExplicit, GroupIds: []int{exclusiveGroup.Id},
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建独立单分组令牌失败: %v", err)
	}
	token.GroupIds = []int{exclusiveGroup.Id, defaultGroup.Id}
	token.GroupDetails = nil
	if err := token.Update(); !errors.Is(err, ErrTokenGroupBindingConflict) {
		t.Fatalf("更新令牌为冲突绑定应该失败，实际: %v", err)
	}
	reloaded, err := GetTokenById(token.Id)
	if err != nil {
		t.Fatalf("重读令牌失败: %v", err)
	}
	if len(reloaded.GroupIds) != 1 || reloaded.GroupIds[0] != exclusiveGroup.Id {
		t.Fatalf("更新失败后应保留原独立绑定: %#v", reloaded.GroupIds)
	}
}

func TestSaveGroupConfigPersistsExclusiveAndRejectsAutoMembership(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	configs := []GroupConfig{
		{Id: defaultGroup.Id, Code: defaultGroup.Code, Name: defaultGroup.Name, Ratio: 1, Status: GroupStatusActive},
		{Id: vipGroup.Id, Code: vipGroup.Code, Name: vipGroup.Name, Ratio: 0.5, Exclusive: true, Status: GroupStatusActive},
	}
	if err := SaveGroupConfig(configs, nil); err != nil {
		t.Fatalf("保存独立分组失败: %v", err)
	}
	var saved Group
	if err := DB.First(&saved, "id = ?", vipGroup.Id).Error; err != nil {
		t.Fatalf("重读独立分组失败: %v", err)
	}
	if !saved.Exclusive || !saved.ToConfig(nil).Exclusive {
		t.Fatal("独立分组字段未持久化或未投影到 API 配置")
	}

	configs[1].AutoEnabled = true
	if err := SaveGroupConfig(configs, nil); err == nil {
		t.Fatal("独立分组不应允许加入自动分组")
	}
}

func TestSaveGroupConfigPreservesOmittedExclusiveAndAllowsExplicitFalse(t *testing.T) {
	defaultGroup, exclusiveGroup := setupGroupBindingsTest(t)
	if err := DB.Model(&Group{}).Where("id = ?", exclusiveGroup.Id).Update("exclusive", true).Error; err != nil {
		t.Fatalf("设置独立分组失败: %v", err)
	}
	conflictToken := &Token{GroupMode: TokenGroupModeExplicit, GroupIds: []int{exclusiveGroup.Id, defaultGroup.Id}}
	InvalidateExclusiveGroupSnapshot()
	if err := ValidateTokenExclusiveGroupBindingCached(conflictToken); !errors.Is(err, ErrTokenGroupBindingConflict) {
		t.Fatalf("保存前应识别独立分组冲突，实际: %v", err)
	}
	configs := []GroupConfig{
		{Id: defaultGroup.Id, Code: defaultGroup.Code, Name: defaultGroup.Name, Ratio: 1, Status: GroupStatusActive, ExclusiveOmitted: true},
		{Id: exclusiveGroup.Id, Code: exclusiveGroup.Code, Name: exclusiveGroup.Name, Ratio: 1, Status: GroupStatusActive, ExclusiveOmitted: true},
	}
	if err := SaveGroupConfig(configs, nil); err != nil {
		t.Fatalf("保存旧客户端分组请求失败: %v", err)
	}
	var saved Group
	if err := DB.First(&saved, "id = ?", exclusiveGroup.Id).Error; err != nil {
		t.Fatalf("重读独立分组失败: %v", err)
	}
	if !saved.Exclusive {
		t.Fatal("旧客户端缺失 exclusive 时不应取消独立属性")
	}
	if err := ValidateTokenExclusiveGroupBindingCached(conflictToken); !errors.Is(err, ErrTokenGroupBindingConflict) {
		t.Fatalf("保留独立属性后快照仍应拦截冲突，实际: %v", err)
	}

	configs[1].ExclusiveOmitted = false
	configs[1].Exclusive = false
	if err := SaveGroupConfig(configs, nil); err != nil {
		t.Fatalf("明确取消独立属性失败: %v", err)
	}
	if err := DB.First(&saved, "id = ?", exclusiveGroup.Id).Error; err != nil {
		t.Fatalf("重读取消独立后的分组失败: %v", err)
	}
	if saved.Exclusive {
		t.Fatal("明确传入 false 时应取消独立属性")
	}
	if err := ValidateTokenExclusiveGroupBindingCached(conflictToken); err != nil {
		t.Fatalf("取消独立属性后快照应立即刷新: %v", err)
	}
}

func TestUpdateAutoGroupsRejectsExclusiveGroup(t *testing.T) {
	_, exclusiveGroup := setupGroupBindingsTest(t)
	if err := DB.Model(&Group{}).Where("id = ?", exclusiveGroup.Id).Update("exclusive", true).Error; err != nil {
		t.Fatalf("设置独立分组失败: %v", err)
	}
	value := `["` + exclusiveGroup.Code + `"]`
	if err := UpdateOption("AutoGroups", value); err == nil {
		t.Fatal("通用选项更新不应允许独立分组加入 AutoGroups")
	}
	if err := UpdateOptionsBulk(map[string]string{"AutoGroups": value}); err == nil {
		t.Fatal("批量选项更新不应允许独立分组加入 AutoGroups")
	}
}

func TestTokenGroupMigrationRejectsExclusiveTargetConflict(t *testing.T) {
	defaultGroup, sourceGroup := setupGroupBindingsTest(t)
	exclusiveTarget := &Group{Code: "hack-exclusive", Name: "Hack", Ratio: 1, Exclusive: true, Status: GroupStatusActive}
	if err := DB.Create(exclusiveTarget).Error; err != nil {
		t.Fatalf("创建独立目标分组失败: %v", err)
	}
	token := &Token{
		UserId: 1, Key: "exclusive-migration", Name: "exclusive-migration",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
		GroupMode: TokenGroupModeExplicit, GroupIds: []int{sourceGroup.Id, defaultGroup.Id},
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建待迁移令牌失败: %v", err)
	}
	if _, err := MigrateTokenGroup(sourceGroup.Id, exclusiveTarget.Id); !errors.Is(err, ErrTokenGroupBindingConflict) {
		t.Fatalf("迁移到独立分组后形成多组冲突应该失败，实际: %v", err)
	}
	reloaded, err := GetTokenById(token.Id)
	if err != nil {
		t.Fatalf("重读迁移失败后的令牌失败: %v", err)
	}
	if reloaded.Group != sourceGroup.Code+","+defaultGroup.Code {
		t.Fatalf("迁移失败应回滚原绑定，实际: %q", reloaded.Group)
	}
}
