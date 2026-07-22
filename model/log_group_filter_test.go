package model

import (
	"testing"
	"time"
)

func TestLogGroupFilterAcceptsCurrentDisplayName(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Group{}, &GroupAlias{}, &Log{}); err != nil {
		t.Fatalf("迁移日志分组筛选测试表失败: %v", err)
	}

	group := &Group{Code: "group_2", Name: "图片分组", Ratio: 1, Status: GroupStatusActive}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("创建日志分组失败: %v", err)
	}
	if err := db.Create(&GroupAlias{Alias: "legacy-image", GroupId: group.Id}).Error; err != nil {
		t.Fatalf("创建日志分组别名失败: %v", err)
	}

	now := time.Now().Unix()
	logs := []*Log{
		{UserId: 7, CreatedAt: now, Type: LogTypeConsume, Group: "group_2", Quota: 10, PromptTokens: 1, CompletionTokens: 2},
		{UserId: 7, CreatedAt: now, Type: LogTypeConsume, Group: "legacy-image", Quota: 20, PromptTokens: 3, CompletionTokens: 4},
		{UserId: 7, CreatedAt: now, Type: LogTypeConsume, Group: "orphan", Quota: 40},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("创建日志分组筛选数据失败: %v", err)
	}

	adminLogs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "图片分组", "", "")
	if err != nil {
		t.Fatalf("按显示名称查询管理员日志失败: %v", err)
	}
	if total != 2 || len(adminLogs) != 2 {
		t.Fatalf("管理员日志匹配数量错误: total=%d len=%d", total, len(adminLogs))
	}
	for _, log := range adminLogs {
		if log.GroupName != "图片分组" {
			t.Fatalf("管理员日志未返回当前显示名称: %#v", log)
		}
	}

	userLogs, total, err := GetUserLogs(7, LogTypeUnknown, 0, 0, "", "", 0, 100, "图片分组", "", "")
	if err != nil {
		t.Fatalf("按显示名称查询用户日志失败: %v", err)
	}
	if total != 2 || len(userLogs) != 2 {
		t.Fatalf("用户日志匹配数量错误: total=%d len=%d", total, len(userLogs))
	}

	stat, err := SumUsedQuota(LogTypeUnknown, 0, 0, "", "", "", 0, "图片分组")
	if err != nil {
		t.Fatalf("按显示名称统计日志失败: %v", err)
	}
	if stat.Quota != 30 || stat.Rpm != 2 {
		t.Fatalf("显示名称统计结果错误: %#v", stat)
	}

	if err := db.Model(&Group{}).Where("id = ?", group.Id).Update("name", "图像新名称").Error; err != nil {
		t.Fatalf("修改分组显示名称失败: %v", err)
	}
	adminLogs, total, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "图像新名称", "", "")
	if err != nil || total != 2 || len(adminLogs) != 2 {
		t.Fatalf("改名后查询历史日志失败: total=%d len=%d err=%v", total, len(adminLogs), err)
	}
	for _, log := range adminLogs {
		if log.GroupName != "图像新名称" {
			t.Fatalf("改名后日志未返回最新显示名称: %#v", log)
		}
	}

	_, total, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "legacy-image", "", "")
	if err != nil || total != 2 {
		t.Fatalf("按历史别名查询日志失败: total=%d err=%v", total, err)
	}
	_, total, err = GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 100, 0, "orphan", "", "")
	if err != nil || total != 1 {
		t.Fatalf("未知历史值未回退精确查询: total=%d err=%v", total, err)
	}
}
