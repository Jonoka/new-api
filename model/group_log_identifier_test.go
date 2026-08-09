package model

import (
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

func TestResolveGroupLogIdentifiersUsesStablePrecedence(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Group{}, &GroupAlias{}); err != nil {
		t.Fatalf("迁移日志分组标识测试表失败: %v", err)
	}

	current := &Group{Code: "group_2", Name: "图像组", Ratio: 1, Status: GroupStatusActive}
	disabled := &Group{Code: "disabled-code", Name: "停用分组", Ratio: 1, Status: GroupStatusDisabled}
	shadowOwner := &Group{Code: "shadow-code", Name: "代码优先分组", Ratio: 1, Status: GroupStatusActive}
	aliasOwner := &Group{Code: "alias-owner", Name: "别名所属分组", Ratio: 1, Status: GroupStatusActive}
	for _, group := range []*Group{current, disabled, shadowOwner, aliasOwner} {
		if err := db.Create(group).Error; err != nil {
			t.Fatalf("创建日志分组测试数据失败: %v", err)
		}
	}
	aliases := []*GroupAlias{
		{Alias: "old-group-2", GroupId: current.Id},
		{Alias: "disabled-old", GroupId: disabled.Id},
		{Alias: "shadow-code", GroupId: aliasOwner.Id},
		{Alias: "alias-old", GroupId: aliasOwner.Id},
	}
	if err := db.Create(&aliases).Error; err != nil {
		t.Fatalf("创建日志分组别名测试数据失败: %v", err)
	}

	assertIdentifiers := func(input string, want []string) {
		t.Helper()
		got, err := ResolveGroupLogIdentifiers(input)
		if err != nil {
			t.Fatalf("解析日志分组 %q 失败: %v", input, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("日志分组 %q 标识错误: got=%#v want=%#v", input, got, want)
		}
	}

	assertIdentifiers("group_2", []string{"group_2", "old-group-2"})
	assertIdentifiers("old-group-2", []string{"group_2", "old-group-2"})
	assertIdentifiers("图像组", []string{"group_2", "old-group-2"})
	assertIdentifiers("停用分组", []string{"disabled-code", "disabled-old"})

	// 当前 code 优先于指向另一分组的历史 alias。
	assertIdentifiers("shadow-code", []string{"shadow-code"})
	assertIdentifiers("alias-old", []string{"alias-owner", "alias-old"})

	if err := db.Model(&Group{}).Where("id = ?", current.Id).Update("name", "图像新名称").Error; err != nil {
		t.Fatalf("修改分组显示名称失败: %v", err)
	}
	assertIdentifiers("图像新名称", []string{"group_2", "old-group-2"})
	if _, err := ResolveGroupLogIdentifiers("图像组"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("旧显示名称应无法作为当前日志筛选标识: %v", err)
	}
	if _, err := ResolveGroupLogIdentifiers("不存在的分组"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("未知日志分组应返回记录不存在: %v", err)
	}
}
