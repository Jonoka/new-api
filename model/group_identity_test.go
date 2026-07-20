package model

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func openGroupIdentityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldLogDB := DB, LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	initCol()
	db, err := gorm.Open(sqlite.Open("file:group_identity_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	DB, LOG_DB = db, db
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestMigrateGroupIdentityIsIdempotent(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}, &Channel{}, &Token{}, &User{}, &Ability{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	options := []Option{
		{Key: "GroupRatio", Value: `{"default":1,"vip":0.5}`},
		{Key: "UserUsableGroups", Value: `{"default":"默认","vip":"VIP"}`},
		{Key: "AutoGroups", Value: `["vip","default"]`},
	}
	if err := db.Create(&options).Error; err != nil {
		t.Fatalf("写入旧配置失败: %v", err)
	}
	if err := db.Create(&Channel{Group: "default,vip"}).Error; err != nil {
		t.Fatalf("写入旧渠道失败: %v", err)
	}
	if err := MigrateGroupIdentity(); err != nil {
		t.Fatalf("首次回填失败: %v", err)
	}
	var groups []Group
	if err := db.Order("code ASC").Find(&groups).Error; err != nil {
		t.Fatalf("读取分组失败: %v", err)
	}
	if len(groups) != 2 || groups[0].Code != "default" || groups[1].Code != "vip" {
		t.Fatalf("回填分组不符合预期: %#v", groups)
	}
	firstIDs := map[string]int{"default": groups[0].Id, "vip": groups[1].Id}
	if err := MigrateGroupIdentity(); err != nil {
		t.Fatalf("重复回填失败: %v", err)
	}
	for _, group := range groups {
		var current Group
		if err := db.Where("code = ?", group.Code).First(&current).Error; err != nil {
			t.Fatalf("读取重复回填结果失败: %v", err)
		}
		if current.Id != firstIDs[group.Code] {
			t.Fatalf("分组 %s 的 ID 被改变: %d -> %d", group.Code, firstIDs[group.Code], current.Id)
		}
	}
	var members []AutoGroupMember
	if err := db.Order("position ASC").Find(&members).Error; err != nil {
		t.Fatalf("读取自动分组失败: %v", err)
	}
	if len(members) != 2 || members[0].Position != 0 || members[1].Position != 1 {
		t.Fatalf("自动分组顺序错误: %#v", members)
	}
}

func TestSaveGroupConfigChangesDisplayNameOnly(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}, &Channel{}, &Token{}, &User{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	group := &Group{Code: "vip", Name: "VIP", Ratio: 0.5, Status: GroupStatusActive, CreatedTime: 1, UpdatedTime: 1}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("创建分组失败: %v", err)
	}
	if err := SaveGroupConfig([]GroupConfig{{Id: group.Id, Code: "vip", Name: "尊贵用户", Ratio: 0.5, Status: GroupStatusActive}}, nil); err != nil {
		t.Fatalf("保存显示名称失败: %v", err)
	}
	var updated Group
	if err := db.First(&updated, group.Id).Error; err != nil {
		t.Fatalf("读取分组失败: %v", err)
	}
	if updated.Id != group.Id || updated.Code != "vip" || updated.Name != "尊贵用户" {
		t.Fatalf("显示名称更新错误: %#v", updated)
	}
	names, err := GetActiveGroupNameMap()
	if err != nil {
		t.Fatalf("读取模型广场分组名称失败: %v", err)
	}
	if names["vip"] != "尊贵用户" {
		t.Fatalf("模型广场未读取到修改后的分组名称: %#v", names)
	}
	if err := SaveGroupConfig([]GroupConfig{{Id: group.Id, Code: "renamed-code", Name: "其他", Ratio: 0.5, Status: GroupStatusActive}}, nil); err == nil {
		t.Fatal("修改 code 应该被拒绝")
	}
}

func TestSaveGroupConfigSupportsAtomicDisplayNameReassignment(t *testing.T) {
	tests := []struct {
		name        string
		initial     []string
		replacement []string
	}{
		{name: "两组交换", initial: []string{"A", "B"}, replacement: []string{"B", "A"}},
		{name: "三组循环", initial: []string{"A", "B", "C"}, replacement: []string{"B", "C", "A"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openGroupIdentityTestDB(t)
			if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}); err != nil {
				t.Fatalf("迁移测试表失败: %v", err)
			}
			groups := make([]Group, len(test.initial))
			for index, name := range test.initial {
				groups[index] = Group{
					Code:   "group-" + strings.ToLower(name),
					Name:   name,
					Ratio:  1,
					Status: GroupStatusActive,
				}
			}
			if err := db.Create(&groups).Error; err != nil {
				t.Fatalf("创建分组失败: %v", err)
			}

			configs := make([]GroupConfig, len(groups))
			for index := range groups {
				configs[index] = GroupConfig{
					Id:     groups[index].Id,
					Code:   groups[index].Code,
					Name:   test.replacement[index],
					Ratio:  1,
					Status: GroupStatusActive,
				}
			}
			if err := SaveGroupConfig(configs, nil); err != nil {
				t.Fatalf("原子修改分组名称失败: %v", err)
			}

			for index := range groups {
				var stored Group
				if err := db.First(&stored, groups[index].Id).Error; err != nil {
					t.Fatalf("读取分组失败: %v", err)
				}
				if stored.Code != groups[index].Code || stored.Name != test.replacement[index] {
					t.Fatalf("分组名称修改错误: %#v", stored)
				}
			}
		})
	}
}

func TestSaveGroupConfigRejectsDuplicateFinalDisplayNames(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	groups := []Group{
		{Code: "group-a", Name: "A", Ratio: 1, Status: GroupStatusActive},
		{Code: "group-b", Name: "B", Ratio: 1, Status: GroupStatusActive},
	}
	if err := db.Create(&groups).Error; err != nil {
		t.Fatalf("创建分组失败: %v", err)
	}

	err := SaveGroupConfig([]GroupConfig{
		{Id: groups[0].Id, Code: groups[0].Code, Name: "共享", Ratio: 1, Status: GroupStatusActive},
		{Id: groups[1].Id, Code: groups[1].Code, Name: " 共享 ", Ratio: 1, Status: GroupStatusActive},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "分组名称重复: 共享") {
		t.Fatalf("重复名称应返回明确错误，实际为: %v", err)
	}

	for index, expectedName := range []string{"A", "B"} {
		var stored Group
		if err := db.First(&stored, groups[index].Id).Error; err != nil {
			t.Fatalf("读取分组失败: %v", err)
		}
		if stored.Name != expectedName {
			t.Fatalf("重复名称校验失败后数据未回滚: %#v", stored)
		}
	}
}

func TestSaveGroupConfigRejectsNameHeldOutsidePayload(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	groups := []Group{
		{Code: "group-a", Name: "A", Ratio: 1, Status: GroupStatusActive},
		{Code: "group-b", Name: "B", Ratio: 1, Status: GroupStatusActive},
	}
	if err := db.Create(&groups).Error; err != nil {
		t.Fatalf("创建分组失败: %v", err)
	}

	err := SaveGroupConfig([]GroupConfig{
		{Id: groups[0].Id, Code: groups[0].Code, Name: "B", Ratio: 1, Status: GroupStatusActive},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "分组名称重复: B") {
		t.Fatalf("payload 外名称冲突应返回明确错误，实际为: %v", err)
	}

	for index, expectedName := range []string{"A", "B"} {
		var stored Group
		if err := db.First(&stored, groups[index].Id).Error; err != nil {
			t.Fatalf("读取分组失败: %v", err)
		}
		if stored.Name != expectedName {
			t.Fatalf("名称冲突后数据未回滚: %#v", stored)
		}
	}
}

func TestSaveGroupConfigCanReuseDeletedGroupName(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(
		&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{},
		&Channel{}, &ChannelGroupBinding{}, &Token{}, &TokenGroupBinding{}, &User{}, &Ability{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	groups := []Group{
		{Code: "keeper", Name: "保留名称", Ratio: 1, Status: GroupStatusActive},
		{Code: "removed", Name: "复用名称", Ratio: 1, Status: GroupStatusActive},
	}
	if err := db.Create(&groups).Error; err != nil {
		t.Fatalf("创建分组失败: %v", err)
	}

	if err := SaveGroupConfig([]GroupConfig{
		{Id: groups[0].Id, Code: groups[0].Code, Name: "复用名称", Ratio: 1, Status: GroupStatusActive},
	}, []int{groups[1].Id}); err != nil {
		t.Fatalf("删除旧分组后复用名称失败: %v", err)
	}

	var keeper Group
	if err := db.First(&keeper, groups[0].Id).Error; err != nil {
		t.Fatalf("读取保留分组失败: %v", err)
	}
	if keeper.Name != "复用名称" {
		t.Fatalf("未复用已删除分组名称: %#v", keeper)
	}
	var removedCount int64
	if err := db.Model(&Group{}).Where("id = ?", groups[1].Id).Count(&removedCount).Error; err != nil {
		t.Fatalf("统计已删除分组失败: %v", err)
	}
	if removedCount != 0 {
		t.Fatalf("旧分组未删除: %d", removedCount)
	}
}

func TestGetActiveGroupNameMapUsesLatestDisplayName(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Group{}, &AutoGroupMember{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	groups := []Group{
		{Code: "vip", Name: "尊贵用户", Ratio: 0.5, Status: GroupStatusActive},
		{Code: "hidden", Name: "已停用", Ratio: 1, Status: GroupStatusDisabled},
		{Code: "fallback", Name: "", Ratio: 1, Status: GroupStatusActive},
	}
	if err := db.Create(&groups).Error; err != nil {
		t.Fatalf("创建分组失败: %v", err)
	}
	if err := db.Model(&Group{}).Where("code = ?", "hidden").Update("status", GroupStatusDisabled).Error; err != nil {
		t.Fatalf("停用测试分组失败: %v", err)
	}

	names, err := GetActiveGroupNameMap()
	if err != nil {
		t.Fatalf("读取分组显示名称失败: %v", err)
	}
	if names["vip"] != "尊贵用户" {
		t.Fatalf("未返回最新显示名称: %#v", names)
	}
	if names["fallback"] != "fallback" {
		t.Fatalf("空显示名称未回退到内部标识: %#v", names)
	}
	if _, ok := names["hidden"]; ok {
		t.Fatalf("停用分组不应出现在显示名称映射中: %#v", names)
	}
}

func TestNormalizeGroupCodeRejectsSelectorValues(t *testing.T) {
	for _, code := range []string{"", "auto", "all", "null", "a,b"} {
		if _, err := NormalizeGroupCode(code); err == nil {
			t.Fatalf("保留或非法 code 未被拒绝: %q", code)
		}
	}
	if normalized, err := NormalizeGroupCode(" vip "); err != nil || normalized != "vip" {
		t.Fatalf("合法 code 规范化错误: %q, %v", normalized, err)
	}
	if _, err := NormalizeGroupCode(strings.Repeat("组", 64)); err != nil {
		t.Fatalf("64 个中文字符的 code 应合法: %v", err)
	}
	if _, err := NormalizeGroupCode(strings.Repeat("组", 65)); err == nil {
		t.Fatal("65 个中文字符的 code 应被拒绝")
	}
	if _, err := normalizeGroupName(strings.Repeat("名", 128), ""); err != nil {
		t.Fatalf("128 个中文字符的名称应合法: %v", err)
	}
	if _, err := normalizeGroupName(strings.Repeat("名", 129), ""); err == nil {
		t.Fatal("129 个中文字符的名称应被拒绝")
	}
}

func TestMigrateGroupIdentityToleratesMissingLegacyColumns(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}); err != nil {
		t.Fatalf("迁移分组基础表失败: %v", err)
	}
	if err := db.Exec("CREATE TABLE channels (id integer primary key, `group` text)").Error; err != nil {
		t.Fatalf("创建旧渠道表失败: %v", err)
	}
	if err := db.Exec("CREATE TABLE users (id integer primary key, `group` text)").Error; err != nil {
		t.Fatalf("创建缺少 group_id 的旧用户表失败: %v", err)
	}
	if err := db.Exec("INSERT INTO channels (id, `group`) VALUES (1, 'legacy-channel')").Error; err != nil {
		t.Fatalf("写入旧渠道数据失败: %v", err)
	}
	if err := db.Exec("INSERT INTO users (id, `group`) VALUES (1, 'legacy-user')").Error; err != nil {
		t.Fatalf("写入旧用户数据失败: %v", err)
	}

	if err := MigrateGroupIdentity(); err != nil {
		t.Fatalf("旧库缺列时迁移不应失败: %v", err)
	}
	for _, code := range []string{"default", "legacy-channel", "legacy-user"} {
		var count int64
		if err := db.Model(&Group{}).Where("code = ?", code).Count(&count).Error; err != nil {
			t.Fatalf("统计迁移分组 %s 失败: %v", code, err)
		}
		if count != 1 {
			t.Fatalf("旧库分组 %s 未被迁移", code)
		}
	}
}

func TestMigrateGroupIdentitySkipsNullLegacyGroupValues(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}); err != nil {
		t.Fatalf("迁移分组基础表失败: %v", err)
	}
	if err := db.Exec("CREATE TABLE channels (id integer primary key, `group` text)").Error; err != nil {
		t.Fatalf("创建旧渠道表失败: %v", err)
	}
	if err := db.Exec("INSERT INTO channels (id, `group`) VALUES (1, NULL), (2, 'legacy-channel')").Error; err != nil {
		t.Fatalf("写入含 NULL 的旧渠道数据失败: %v", err)
	}

	if err := MigrateGroupIdentity(); err != nil {
		t.Fatalf("NULL 历史分组不应阻塞迁移: %v", err)
	}
	for _, code := range []string{"default", "legacy-channel"} {
		var count int64
		if err := db.Model(&Group{}).Where("code = ?", code).Count(&count).Error; err != nil {
			t.Fatalf("统计迁移分组 %s 失败: %v", code, err)
		}
		if count != 1 {
			t.Fatalf("迁移分组 %s 数量错误: %d", code, count)
		}
	}
}

func TestMigrateGroupIdentityHandlesLegacyCodeNameConflict(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}, &Token{}); err != nil {
		t.Fatalf("迁移分组冲突测试表失败: %v", err)
	}
	occupied := &Group{Code: "vip", Name: "legacy", Ratio: 1, Status: GroupStatusActive}
	if err := db.Create(occupied).Error; err != nil {
		t.Fatalf("创建占用显示名的分组失败: %v", err)
	}
	if err := db.Create(&Token{UserId: 1, Key: "legacy-name-conflict", Name: "legacy-name-conflict", Group: "legacy"}).Error; err != nil {
		t.Fatalf("写入旧令牌失败: %v", err)
	}

	var firstID int
	for run := 1; run <= 2; run++ {
		if err := MigrateGroupIdentity(); err != nil {
			t.Fatalf("第 %d 次迁移不应被显示名冲突阻塞: %v", run, err)
		}
		var migrated Group
		if err := db.Where("code = ?", "legacy").First(&migrated).Error; err != nil {
			t.Fatalf("读取迁移分组失败: %v", err)
		}
		if migrated.Name != "legacy (legacy)" {
			t.Fatalf("迁移分组未使用冲突回退名称: %q", migrated.Name)
		}
		if run == 1 {
			firstID = migrated.Id
		} else if migrated.Id != firstID {
			t.Fatalf("重复迁移改变了分组 ID: %d -> %d", firstID, migrated.Id)
		}
	}

	resolved, err := createLegacyGroup(db, Group{Code: "legacy", Ratio: 1, Status: GroupStatusActive})
	if err != nil {
		t.Fatalf("同 code 冲突重查失败: %v", err)
	}
	if resolved.Id != firstID {
		t.Fatalf("同 code 冲突未复用既有分组: %d != %d", resolved.Id, firstID)
	}
	var occupiedAfter Group
	if err := db.First(&occupiedAfter, occupied.Id).Error; err != nil {
		t.Fatalf("读取原分组失败: %v", err)
	}
	if occupiedAfter.Name != "legacy" {
		t.Fatalf("原分组显示名被迁移改写: %q", occupiedAfter.Name)
	}
}

func TestMigrateGroupIdentityHandlesExistingEmptyNameConflict(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}); err != nil {
		t.Fatalf("迁移分组冲突测试表失败: %v", err)
	}
	occupied := &Group{Code: "occupied", Name: "legacy", Ratio: 1, Status: GroupStatusActive}
	legacy := &Group{Code: "legacy", Name: "", Ratio: 1, Status: GroupStatusActive}
	if err := db.Create(occupied).Error; err != nil {
		t.Fatalf("创建占用显示名的分组失败: %v", err)
	}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("创建空显示名分组失败: %v", err)
	}
	if err := db.Create(&Option{Key: "GroupRatio", Value: `{"legacy":1}`}).Error; err != nil {
		t.Fatalf("写入旧配置失败: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := MigrateGroupIdentity(); err != nil {
			t.Fatalf("第 %d 次迁移不应被空显示名冲突阻塞: %v", run, err)
		}
		var migrated Group
		if err := db.First(&migrated, legacy.Id).Error; err != nil {
			t.Fatalf("读取空显示名分组失败: %v", err)
		}
		if migrated.Name != "legacy (legacy)" {
			t.Fatalf("空显示名分组未使用冲突回退名称: %q", migrated.Name)
		}
	}
	var occupiedAfter Group
	if err := db.First(&occupiedAfter, occupied.Id).Error; err != nil {
		t.Fatalf("读取原分组失败: %v", err)
	}
	if occupiedAfter.Name != "legacy" {
		t.Fatalf("原分组显示名被迁移改写: %q", occupiedAfter.Name)
	}
}

func TestMigrateGroupIdentityPreservesCaseSensitiveCodes(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{}); err != nil {
		t.Fatalf("迁移大小写测试表失败: %v", err)
	}
	if err := db.Create(&Option{Key: "GroupRatio", Value: `{"VIP":1,"vip":1}`}).Error; err != nil {
		t.Fatalf("写入大小写旧配置失败: %v", err)
	}
	if err := MigrateGroupIdentity(); err != nil {
		t.Fatalf("迁移大小写分组失败: %v", err)
	}
	upper, err := GetGroupByCodeOrAliasWithDB(db, "VIP")
	if err != nil {
		t.Fatalf("读取大写分组失败: %v", err)
	}
	lower, err := GetGroupByCodeOrAliasWithDB(db, "vip")
	if err != nil {
		t.Fatalf("读取小写分组失败: %v", err)
	}
	if upper.Id == lower.Id || upper.Code != "VIP" || lower.Code != "vip" {
		t.Fatalf("大小写分组身份被合并: upper=%#v lower=%#v", upper, lower)
	}
}

func TestEnsureMySQLGroupIdentityCaseSensitivity(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("设置 TEST_MYSQL_DSN 后运行 MySQL 分组排序规则兼容测试")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开 MySQL 测试数据库失败: %v", err)
	}
	sqlDB, sqlErr := db.DB()
	if sqlErr != nil {
		t.Fatalf("读取 MySQL 连接失败: %v", sqlErr)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if db.Migrator().HasTable(&Group{}) || db.Migrator().HasTable(&GroupAlias{}) {
		t.Skip("拒绝在已有分组表的外部数据库上运行兼容测试")
	}
	oldDB, oldLogDB := DB, LOG_DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	DB, LOG_DB = db, db
	common.UsingSQLite = false
	common.UsingMySQL = true
	common.UsingPostgreSQL = false
	t.Cleanup(func() {
		_ = db.Migrator().DropTable(&GroupAlias{})
		_ = db.Migrator().DropTable(&Group{})
		DB, LOG_DB = oldDB, oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgres
	})
	if err := db.AutoMigrate(&Group{}, &GroupAlias{}); err != nil {
		t.Fatalf("创建 MySQL 分组测试表失败: %v", err)
	}
	for run := 1; run <= 2; run++ {
		if err := ensureMySQLGroupIdentityCaseSensitivity(db); err != nil {
			t.Fatalf("第 %d 次迁移 MySQL 排序规则失败: %v", run, err)
		}
	}
	for _, column := range []struct {
		table  string
		column string
	}{
		{table: "groups", column: "code"},
		{table: "group_aliases", column: "alias"},
	} {
		var collation string
		if err := db.Raw(`SELECT COLLATION_NAME FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			column.table, column.column).Scan(&collation).Error; err != nil {
			t.Fatalf("读取 MySQL %s.%s 排序规则失败: %v", column.table, column.column, err)
		}
		if !strings.EqualFold(collation, "utf8mb4_bin") {
			t.Fatalf("MySQL %s.%s 排序规则未迁移: %q", column.table, column.column, collation)
		}
	}
	groups := []Group{
		{Code: "VIP", Name: "VIP upper", Ratio: 1, Status: GroupStatusActive},
		{Code: "vip", Name: "VIP lower", Ratio: 1, Status: GroupStatusActive},
	}
	if err := db.Create(&groups).Error; err != nil {
		t.Fatalf("MySQL 大小写分组未能共存: %v", err)
	}
	upper, err := GetGroupByCodeOrAliasWithDB(db, "VIP")
	if err != nil {
		t.Fatalf("读取 MySQL 大写分组失败: %v", err)
	}
	lower, err := GetGroupByCodeOrAliasWithDB(db, "vip")
	if err != nil {
		t.Fatalf("读取 MySQL 小写分组失败: %v", err)
	}
	if upper.Id == lower.Id {
		t.Fatalf("MySQL 大小写分组身份被合并: %d", upper.Id)
	}
}

func TestCreateLegacyGroupReturnsDatabaseErrors(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.Exec("CREATE TABLE groups (id integer primary key)").Error; err != nil {
		t.Fatalf("创建畸形分组表失败: %v", err)
	}

	_, err := createLegacyGroup(db, Group{Code: "legacy", Ratio: 1, Status: GroupStatusActive})
	if err == nil {
		t.Fatal("真实数据库错误不应被唯一键容错吞掉")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "code") {
		t.Fatalf("数据库错误未保留缺失列信息: %v", err)
	}
}

func TestSaveGroupConfigProtectsStableBindingReferences(t *testing.T) {
	tests := []struct {
		name   string
		create func(groupID int) error
	}{
		{
			name: "channel_groups",
			create: func(groupID int) error {
				return DB.Create(&ChannelGroupBinding{ChannelId: 9001, GroupId: groupID, Position: 0}).Error
			},
		},
		{
			name: "token_groups",
			create: func(groupID int) error {
				return DB.Create(&TokenGroupBinding{TokenId: 9001, GroupId: groupID, Position: 0}).Error
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, vipGroup := setupGroupBindingsTest(t)
			if err := test.create(vipGroup.Id); err != nil {
				t.Fatalf("创建稳定绑定失败: %v", err)
			}
			if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err == nil {
				t.Fatalf("存在 %s 引用时删除分组应被拒绝", test.name)
			}
			var count int64
			if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&count).Error; err != nil {
				t.Fatalf("检查分组是否保留失败: %v", err)
			}
			if count != 1 {
				t.Fatal("引用保护失败，分组被删除")
			}
		})
	}
}

func TestSaveGroupConfigProtectsLegacyAliasReferences(t *testing.T) {
	tests := []struct {
		name   string
		create func(alias string) error
	}{
		{
			name: "channel",
			create: func(alias string) error {
				return DB.Create(&Channel{
					Name: "legacy-alias-channel", Key: "legacy-alias-channel-key",
					Models: "gpt-test", Group: "default, " + alias,
				}).Error
			},
		},
		{
			name: "user",
			create: func(alias string) error {
				return DB.Create(&User{
					Username: "legacy-alias-user", Password: "legacy-alias-password", Group: alias,
				}).Error
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, vipGroup := setupGroupBindingsTest(t)
			alias := "legacy%_!vip"
			if err := DB.Create(&GroupAlias{Alias: alias, GroupId: vipGroup.Id}).Error; err != nil {
				t.Fatalf("创建分组兼容别名失败: %v", err)
			}
			if err := test.create(alias); err != nil {
				t.Fatalf("创建仅引用兼容别名的 %s 失败: %v", test.name, err)
			}
			if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err == nil {
				t.Fatalf("存在 %s 兼容别名引用时删除分组应被拒绝", test.name)
			}

			var groupCount, aliasCount int64
			if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&groupCount).Error; err != nil {
				t.Fatalf("检查分组是否保留失败: %v", err)
			}
			if err := DB.Model(&GroupAlias{}).Where("group_id = ?", vipGroup.Id).Count(&aliasCount).Error; err != nil {
				t.Fatalf("检查分组兼容别名是否保留失败: %v", err)
			}
			if groupCount != 1 || aliasCount != 1 {
				t.Fatalf("引用保护失败: group=%d alias=%d", groupCount, aliasCount)
			}
		})
	}
}

func TestSaveGroupConfigIgnoresPartialLegacyAliasMatches(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	alias := "legacy-vip"
	if err := DB.Create(&GroupAlias{Alias: alias, GroupId: vipGroup.Id}).Error; err != nil {
		t.Fatalf("创建分组兼容别名失败: %v", err)
	}
	if err := DB.Create(&Token{
		UserId: 1, Key: "legacy-alias-partial-token", Name: "legacy-alias-partial-token",
		Group: alias + "-extra", GroupMode: TokenGroupModeExplicit,
	}).Error; err != nil {
		t.Fatalf("创建部分匹配兼容别名的令牌失败: %v", err)
	}

	if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err != nil {
		t.Fatalf("部分匹配不应阻止删除分组: %v", err)
	}
	var groupCount, aliasCount int64
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&groupCount).Error; err != nil {
		t.Fatalf("检查分组删除结果失败: %v", err)
	}
	if err := DB.Model(&GroupAlias{}).Where("group_id = ?", vipGroup.Id).Count(&aliasCount).Error; err != nil {
		t.Fatalf("检查兼容别名删除结果失败: %v", err)
	}
	if groupCount != 0 || aliasCount != 0 {
		t.Fatalf("部分匹配被误判为真实引用: group=%d alias=%d", groupCount, aliasCount)
	}
}

func TestSaveGroupConfigPrunesDeletedGroupOptionsAndPreventsRecreation(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	otherGroup := &Group{Code: "other-option", Name: "其他选项分组", Ratio: 1, Status: GroupStatusActive}
	if err := DB.Create(otherGroup).Error; err != nil {
		t.Fatalf("创建保留分组失败: %v", err)
	}
	alias := "vip-option-alias"
	if err := DB.Create(&GroupAlias{Alias: alias, GroupId: vipGroup.Id}).Error; err != nil {
		t.Fatalf("创建待清理别名失败: %v", err)
	}
	options := []Option{
		{
			Key:   "GroupGroupRatio",
			Value: `{"vip":{"default":0.8},"default":{"vip":0.9,"vip-option-alias":0.7,"other-option":1.1}}`,
		},
		{
			Key:   "TopupGroupRatio",
			Value: `{"vip":2,"vip-option-alias":3,"default":1,"other-option":1.2}`,
		},
		{
			Key:   "group_ratio_setting.group_special_usable_group",
			Value: `{"vip":{"default":"owner"},"default":{"vip":"plain","+:vip-option-alias":"append","-:vip":"remove","other-option":"keep"}}`,
		},
		{
			Key:   "ModelRequestRateLimitGroup",
			Value: `{"vip":[10,10],"vip-option-alias":[20,20],"other-option":[30,30]}`,
		},
		{
			Key:   "ModelRequestRateLimitUserGroup",
			Value: `{"vip":{"global":[1,1]},"default":{"global":[2,2],"groups":{"vip":[3,3],"vip-option-alias":[4,4],"other-option":[5,5]}}}`,
		},
	}
	if err := DB.Create(&options).Error; err != nil {
		t.Fatalf("创建待清理高级分组选项失败: %v", err)
	}

	if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err != nil {
		t.Fatalf("删除分组并清理高级选项失败: %v", err)
	}

	var storedOptions []Option
	if err := DB.Where("key IN ?", groupReferenceOptionKeys).Find(&storedOptions).Error; err != nil {
		t.Fatalf("读取清理后的高级选项失败: %v", err)
	}
	byKey := make(map[string]string, len(storedOptions))
	for _, option := range storedOptions {
		byKey[option.Key] = option.Value
	}
	var groupRatios map[string]map[string]float64
	if err := common.UnmarshalJsonStr(byKey["GroupGroupRatio"], &groupRatios); err != nil {
		t.Fatalf("解析清理后的组间倍率失败: %v", err)
	}
	if _, exists := groupRatios[vipGroup.Code]; exists {
		t.Fatal("待删分组仍作为组间倍率 owner")
	}
	if _, exists := groupRatios[defaultGroup.Code][vipGroup.Code]; exists {
		t.Fatal("待删分组仍作为组间倍率 target")
	}
	if _, exists := groupRatios[defaultGroup.Code][alias]; exists {
		t.Fatal("待删分组别名仍作为组间倍率 target")
	}
	if groupRatios[defaultGroup.Code][otherGroup.Code] != 1.1 {
		t.Fatalf("保留组间倍率被误删: %#v", groupRatios)
	}

	var topupRatios map[string]float64
	if err := common.UnmarshalJsonStr(byKey["TopupGroupRatio"], &topupRatios); err != nil {
		t.Fatalf("解析清理后的充值倍率失败: %v", err)
	}
	if _, exists := topupRatios[vipGroup.Code]; exists {
		t.Fatal("充值倍率仍包含待删分组")
	}
	if _, exists := topupRatios[alias]; exists {
		t.Fatal("充值倍率仍包含待删分组别名")
	}
	if topupRatios[otherGroup.Code] != 1.2 {
		t.Fatalf("保留充值倍率被误删: %#v", topupRatios)
	}

	var specialRules map[string]map[string]string
	if err := common.UnmarshalJsonStr(
		byKey["group_ratio_setting.group_special_usable_group"],
		&specialRules,
	); err != nil {
		t.Fatalf("解析清理后的特殊可用分组失败: %v", err)
	}
	if _, exists := specialRules[vipGroup.Code]; exists {
		t.Fatal("特殊可用分组仍包含待删 owner")
	}
	for _, target := range []string{vipGroup.Code, "+:" + alias, "-:" + vipGroup.Code} {
		if _, exists := specialRules[defaultGroup.Code][target]; exists {
			t.Fatalf("特殊可用分组仍包含待删 target %q", target)
		}
	}
	if specialRules[defaultGroup.Code][otherGroup.Code] != "keep" {
		t.Fatalf("保留特殊可用规则被误删: %#v", specialRules)
	}

	var groupLimits map[string]setting.RateLimitCounts
	if err := common.UnmarshalJsonStr(byKey["ModelRequestRateLimitGroup"], &groupLimits); err != nil {
		t.Fatalf("解析清理后的分组限流失败: %v", err)
	}
	if _, exists := groupLimits[vipGroup.Code]; exists {
		t.Fatal("分组限流仍包含待删分组")
	}
	if _, exists := groupLimits[alias]; exists {
		t.Fatal("分组限流仍包含待删分组别名")
	}
	if groupLimits[otherGroup.Code] != (setting.RateLimitCounts{30, 30}) {
		t.Fatalf("保留分组限流被误删: %#v", groupLimits)
	}

	var userGroupLimits map[string]setting.UserGroupRateLimit
	if err := common.UnmarshalJsonStr(byKey["ModelRequestRateLimitUserGroup"], &userGroupLimits); err != nil {
		t.Fatalf("解析清理后的用户组限流失败: %v", err)
	}
	if _, exists := userGroupLimits[vipGroup.Code]; exists {
		t.Fatal("用户组限流仍包含待删 owner")
	}
	if _, exists := userGroupLimits[defaultGroup.Code].Groups[vipGroup.Code]; exists {
		t.Fatal("用户组限流仍包含待删 target")
	}
	if _, exists := userGroupLimits[defaultGroup.Code].Groups[alias]; exists {
		t.Fatal("用户组限流仍包含待删分组别名 target")
	}
	if userGroupLimits[defaultGroup.Code].Groups[otherGroup.Code] != (setting.RateLimitCounts{5, 5}) {
		t.Fatalf("保留用户组限流被误删: %#v", userGroupLimits)
	}

	if err := MigrateGroupIdentity(); err != nil {
		t.Fatalf("第一次重新执行分组身份迁移失败: %v", err)
	}
	if err := MigrateGroupIdentity(); err != nil {
		t.Fatalf("第二次重新执行分组身份迁移失败: %v", err)
	}
	var recreated int64
	if err := DB.Model(&Group{}).Where("code IN ?", []string{vipGroup.Code, alias}).Count(&recreated).Error; err != nil {
		t.Fatalf("统计重新创建的分组失败: %v", err)
	}
	if recreated != 0 {
		t.Fatalf("已删除分组被高级选项重新创建: %d", recreated)
	}
}

func TestSaveGroupConfigRejectsInvalidGroupOptionJSONAndRollsBack(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	option := Option{Key: "GroupGroupRatio", Value: "{broken-json"}
	if err := DB.Create(&option).Error; err != nil {
		t.Fatalf("创建损坏分组选项失败: %v", err)
	}

	err := SaveGroupConfig(nil, []int{vipGroup.Id})
	if err == nil || !strings.Contains(err.Error(), "解析分组选项") {
		t.Fatalf("损坏分组选项应阻止删除，实际错误: %v", err)
	}
	var groupCount int64
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&groupCount).Error; err != nil {
		t.Fatalf("统计回滚后的分组失败: %v", err)
	}
	var stored Option
	if err := DB.First(&stored, "key = ?", option.Key).Error; err != nil {
		t.Fatalf("读取回滚后的损坏选项失败: %v", err)
	}
	if groupCount != 1 || stored.Value != option.Value {
		t.Fatalf("损坏选项删除失败后未完整回滚: group=%d option=%q", groupCount, stored.Value)
	}
}

func TestSaveGroupConfigDeleteRetryIsIdempotent(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err != nil {
		t.Fatalf("首次删除分组失败: %v", err)
	}
	if err := DB.Create(&AutoGroupMember{GroupId: defaultGroup.Id, Position: 0}).Error; err != nil {
		t.Fatalf("创建保留的自动分组成员失败: %v", err)
	}

	if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err != nil {
		t.Fatalf("重复删除已不存在的分组应幂等成功: %v", err)
	}
	var groupCount, memberCount int64
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&groupCount).Error; err != nil {
		t.Fatalf("统计重复删除结果失败: %v", err)
	}
	if err := DB.Model(&AutoGroupMember{}).Where("group_id = ?", defaultGroup.Id).Count(&memberCount).Error; err != nil {
		t.Fatalf("统计保留的自动分组成员失败: %v", err)
	}
	if groupCount != 0 || memberCount != 1 {
		t.Fatalf("重复删除产生了额外修改: group=%d auto_member=%d", groupCount, memberCount)
	}
}

func TestSaveGroupConfigNewGroupRetryIsIdempotent(t *testing.T) {
	setupGroupBindingsTest(t)
	config := GroupConfig{
		Code:   "retry-new-group",
		Name:   "可重试新分组",
		Ratio:  0.8,
		Status: GroupStatusActive,
	}
	if err := SaveGroupConfig([]GroupConfig{config}, nil); err != nil {
		t.Fatalf("首次创建分组失败: %v", err)
	}
	if err := SaveGroupConfig([]GroupConfig{config}, nil); err != nil {
		t.Fatalf("缺少返回 ID 后按相同 code 重试应幂等成功: %v", err)
	}
	var groups []Group
	if err := DB.Where("code = ?", config.Code).Find(&groups).Error; err != nil {
		t.Fatalf("读取重试创建结果失败: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != config.Name || groups[0].Ratio != config.Ratio {
		t.Fatalf("重试创建产生重复或错误数据: %#v", groups)
	}

	conflicting := config
	conflicting.Name = "冲突名称"
	if err := SaveGroupConfig([]GroupConfig{conflicting}, nil); err == nil {
		t.Fatal("相同 code 但不同名称不应被当作幂等重试")
	}
}

func TestUpdateOptionRejectsReferencesToDeletedGroups(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	alias := "deleted-option-alias"
	if err := DB.Create(&GroupAlias{Alias: alias, GroupId: vipGroup.Id}).Error; err != nil {
		t.Fatalf("创建待删除分组别名失败: %v", err)
	}
	if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err != nil {
		t.Fatalf("删除分组失败: %v", err)
	}

	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	tests := []struct {
		key   string
		value string
	}{
		{key: "GroupRatio", value: `{"default":1,"vip":0.8}`},
		{key: "group_ratio_setting.group_ratio", value: `{"deleted-option-alias":0.8}`},
		{key: "UserUsableGroups", value: `{"default":"默认","vip":"VIP"}`},
		{key: "AutoGroups", value: `["default","deleted-option-alias"]`},
		{key: "GroupGroupRatio", value: `{"default":{"vip":0.8}}`},
		{key: "TopupGroupRatio", value: `{"deleted-option-alias":2}`},
		{key: "group_ratio_setting.group_special_usable_group", value: `{"default":{"+:vip":"legacy"}}`},
		{key: "ModelRequestRateLimitGroup", value: `{"vip":[10,10]}`},
		{key: "ModelRequestRateLimitUserGroup", value: `{"default":{"groups":{"deleted-option-alias":[10,10]}}}`},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			var before Option
			beforeErr := DB.First(&before, "key = ?", test.key).Error
			if beforeErr != nil && !errors.Is(beforeErr, gorm.ErrRecordNotFound) {
				t.Fatalf("读取更新前选项失败: %v", beforeErr)
			}
			if err := UpdateOption(test.key, test.value); err == nil {
				t.Fatalf("选项 %s 不应接受已删除分组引用", test.key)
			}
			var after Option
			afterErr := DB.First(&after, "key = ?", test.key).Error
			if errors.Is(beforeErr, gorm.ErrRecordNotFound) {
				if !errors.Is(afterErr, gorm.ErrRecordNotFound) {
					t.Fatalf("无效选项 %s 被新建", test.key)
				}
			} else if afterErr != nil || after.Value != before.Value {
				t.Fatalf("无效选项 %s 覆盖了原值: before=%q after=%q err=%v", test.key, before.Value, after.Value, afterErr)
			}
		})
	}

	validValue := fmt.Sprintf(`{"%s":1}`, defaultGroup.Code)
	if err := UpdateOption("TopupGroupRatio", validValue); err != nil {
		t.Fatalf("有效分组选项保存失败: %v", err)
	}
}

func TestSaveGroupConfigProtectsLegacyAbilityAndSubscriptionReferences(t *testing.T) {
	tests := []struct {
		name   string
		create func(groupCode string) error
	}{
		{
			name: "ability_group",
			create: func(groupCode string) error {
				return DB.Create(&Ability{Group: groupCode, Model: "gpt-test", ChannelId: 801}).Error
			},
		},
		{
			name: "subscription_plan_upgrade_group",
			create: func(groupCode string) error {
				return DB.Create(&SubscriptionPlan{Title: "test", UpgradeGroup: groupCode}).Error
			},
		},
		{
			name: "user_subscription_upgrade_group",
			create: func(groupCode string) error {
				return DB.Create(&UserSubscription{UserId: 1, UpgradeGroup: groupCode}).Error
			},
		},
		{
			name: "user_subscription_previous_group",
			create: func(groupCode string) error {
				return DB.Create(&UserSubscription{UserId: 1, PrevUserGroup: groupCode}).Error
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, vipGroup := setupGroupBindingsTest(t)
			if err := DB.AutoMigrate(&SubscriptionPlan{}, &UserSubscription{}); err != nil {
				t.Fatalf("迁移订阅引用测试表失败: %v", err)
			}
			if err := test.create(vipGroup.Code); err != nil {
				t.Fatalf("创建非令牌历史引用失败: %v", err)
			}
			if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err == nil {
				t.Fatal("非令牌历史引用应阻止删除分组")
			}
			var groupCount int64
			if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&groupCount).Error; err != nil {
				t.Fatalf("统计删除失败后的分组失败: %v", err)
			}
			if groupCount != 1 {
				t.Fatal("非令牌历史引用保护失败，分组被删除")
			}
		})
	}
}

func TestGroupIdentifierResolutionAndDisplayMapUseStablePrecedence(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Group{}, &GroupAlias{}); err != nil {
		t.Fatalf("迁移分组标识优先级测试表失败: %v", err)
	}

	codeOwner := &Group{Code: "shared-code", Name: "代码分组", Ratio: 1, Status: GroupStatusActive}
	aliasOwner := &Group{Code: "alias-owner", Name: "别名分组", Ratio: 1, Status: GroupStatusActive}
	nameOwner := &Group{Code: "name-owner", Name: "shared-name", Ratio: 1, Status: GroupStatusActive}
	for _, group := range []*Group{codeOwner, aliasOwner, nameOwner} {
		if err := db.Create(group).Error; err != nil {
			t.Fatalf("创建分组标识优先级测试数据失败: %v", err)
		}
	}
	aliases := []*GroupAlias{
		{Alias: codeOwner.Code, GroupId: aliasOwner.Id},
		{Alias: nameOwner.Name, GroupId: aliasOwner.Id},
	}
	if err := db.Create(&aliases).Error; err != nil {
		t.Fatalf("创建冲突分组别名失败: %v", err)
	}

	resolved, err := GetGroupByCodeOrAlias(codeOwner.Code)
	if err != nil {
		t.Fatalf("解析与别名冲突的当前 code 失败: %v", err)
	}
	if resolved.Id != codeOwner.Id {
		t.Fatalf("当前 code 未优先于历史别名: got=%d want=%d", resolved.Id, codeOwner.Id)
	}

	for attempt := 0; attempt < 3; attempt++ {
		names, err := GetGroupDisplayNameMap()
		if err != nil {
			t.Fatalf("读取分组显示名称映射失败: %v", err)
		}
		if names[codeOwner.Code] != codeOwner.Name {
			t.Fatalf("显示映射中当前 code 未优先于别名: %#v", names)
		}
		if names[nameOwner.Name] != aliasOwner.Name {
			t.Fatalf("显示映射中别名未优先于同名显示名称: %#v", names)
		}
	}
}

func TestDeletingGroupIgnoresAliasShadowedByAnotherCurrentCode(t *testing.T) {
	_, sourceGroup := setupGroupBindingsTest(t)
	codeOwner := &Group{
		Code:   "legacy-vip-shadow",
		Name:   "保留分组",
		Ratio:  1,
		Status: GroupStatusActive,
	}
	if err := DB.Create(codeOwner).Error; err != nil {
		t.Fatalf("创建别名冲突的 code 分组失败: %v", err)
	}
	if err := DB.Create(&GroupAlias{Alias: codeOwner.Code, GroupId: sourceGroup.Id}).Error; err != nil {
		t.Fatalf("创建与当前 code 冲突的历史别名失败: %v", err)
	}

	token := &Token{
		UserId:         401,
		Key:            "token-group-shadowed-alias",
		Name:           "shadowed-alias",
		Group:          codeOwner.Code,
		GroupMode:      TokenGroupModeExplicit,
		UnlimitedQuota: true,
	}
	if err := DB.Create(token).Error; err != nil {
		t.Fatalf("创建使用当前 code 的令牌失败: %v", err)
	}
	option := &Option{Key: "TopupGroupRatio", Value: fmt.Sprintf(`{"%s":2}`, codeOwner.Code)}
	if err := DB.Create(option).Error; err != nil {
		t.Fatalf("创建使用当前 code 的分组选项失败: %v", err)
	}

	preview, err := PreviewTokenGroupMigrationToAuto(sourceGroup.Id)
	if err != nil {
		t.Fatalf("预览删除源分组的令牌迁移失败: %v", err)
	}
	if preview.MigratedTokens != 0 {
		t.Fatalf("与其他组当前 code 冲突的 alias 被错误视为源引用: %#v", preview)
	}

	result, err := SaveGroupConfigWithResult(nil, []int{sourceGroup.Id})
	if err != nil {
		t.Fatalf("删除具有阴影别名的源分组失败: %v", err)
	}
	if result.MigratedTokens != 0 {
		t.Fatalf("删除分组时错误迁移了当前 code 所属令牌: %#v", result)
	}

	var storedToken Token
	if err := DB.First(&storedToken, token.Id).Error; err != nil {
		t.Fatalf("读取删除分组后的令牌失败: %v", err)
	}
	if storedToken.Group != codeOwner.Code || storedToken.GroupMode != TokenGroupModeExplicit {
		t.Fatalf("当前 code 所属令牌被错误修改: %#v", storedToken)
	}
	var storedOption Option
	if err := DB.First(&storedOption, "key = ?", option.Key).Error; err != nil {
		t.Fatalf("读取删除分组后的选项失败: %v", err)
	}
	if storedOption.Value != option.Value {
		t.Fatalf("当前 code 所属选项被错误清理: got=%q want=%q", storedOption.Value, option.Value)
	}
}

func TestSaveGroupConfigWithOptionsCommitsGroupsAndOptionsAtomically(t *testing.T) {
	setupGroupBindingsTest(t)

	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	oldDefaultUseAutoGroup := setting.DefaultUseAutoGroup
	oldTopupGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		setting.DefaultUseAutoGroup = oldDefaultUseAutoGroup
		_ = common.UpdateTopupGroupRatioByJSONString(oldTopupGroupRatio)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	const newGroupCode = "atomic-option-new"
	topupValue := fmt.Sprintf(`{"%s":2}`, newGroupCode)
	_, err := SaveGroupConfigWithOptionsAndResult(
		[]GroupConfig{{
			Code:   newGroupCode,
			Name:   "原子选项新分组",
			Ratio:  0.8,
			Status: GroupStatusActive,
		}},
		nil,
		map[string]string{
			"DefaultUseAutoGroup": "true",
			"TopupGroupRatio":     topupValue,
		},
	)
	if err != nil {
		t.Fatalf("原子保存分组与选项失败: %v", err)
	}

	var group Group
	if err := DB.First(&group, "code = ?", newGroupCode).Error; err != nil {
		t.Fatalf("读取原子创建的分组失败: %v", err)
	}
	var storedOptions []Option
	if err := DB.Where("key IN ?", []string{"DefaultUseAutoGroup", "TopupGroupRatio"}).Find(&storedOptions).Error; err != nil {
		t.Fatalf("读取原子保存的选项失败: %v", err)
	}
	optionByKey := make(map[string]string, len(storedOptions))
	for _, option := range storedOptions {
		optionByKey[option.Key] = option.Value
	}
	if optionByKey["DefaultUseAutoGroup"] != "true" || optionByKey["TopupGroupRatio"] != topupValue {
		t.Fatalf("原子保存的选项错误: %#v", optionByKey)
	}
	common.OptionMapRWMutex.RLock()
	runtimeDefault := common.OptionMap["DefaultUseAutoGroup"]
	runtimeTopup := common.OptionMap["TopupGroupRatio"]
	common.OptionMapRWMutex.RUnlock()
	if runtimeDefault != "true" || runtimeTopup != topupValue || !setting.DefaultUseAutoGroup {
		t.Fatalf("事务提交后运行时选项未同步: default=%q topup=%q enabled=%v", runtimeDefault, runtimeTopup, setting.DefaultUseAutoGroup)
	}
}

func TestSaveGroupConfigWithOptionsNormalizesBlankJSONWithoutMutatingInput(t *testing.T) {
	setupGroupBindingsTest(t)

	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	oldDefaultUseAutoGroup := setting.DefaultUseAutoGroup
	oldTopupGroupRatio := common.TopupGroupRatio2JSONString()
	oldGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	specialUsableGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	oldSpecialUsableGroups := specialUsableGroups.MarshalJSONString()
	t.Cleanup(func() {
		setting.DefaultUseAutoGroup = oldDefaultUseAutoGroup
		_ = common.UpdateTopupGroupRatioByJSONString(oldTopupGroupRatio)
		_ = ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatio)
		_ = specialUsableGroups.UnmarshalJSON([]byte(oldSpecialUsableGroups))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	updates := map[string]string{
		"DefaultUseAutoGroup": "false",
		"GroupGroupRatio":     " ",
		"TopupGroupRatio":     "\t",
		"group_ratio_setting.group_special_usable_group": "\r\n",
	}
	_, err := SaveGroupConfigWithOptionsAndResult(nil, nil, updates)
	if err != nil {
		t.Fatalf("空白 JSON 选项应被原子规范化并保存: %v", err)
	}

	if updates["GroupGroupRatio"] != " " ||
		updates["TopupGroupRatio"] != "\t" ||
		updates["group_ratio_setting.group_special_usable_group"] != "\r\n" ||
		updates["DefaultUseAutoGroup"] != "false" {
		t.Fatalf("保存过程修改了调用方选项 map: %#v", updates)
	}

	jsonKeys := []string{
		"GroupGroupRatio",
		"TopupGroupRatio",
		"group_ratio_setting.group_special_usable_group",
	}
	var storedOptions []Option
	if err := DB.Where(commonKeyCol+" IN ?", jsonKeys).Find(&storedOptions).Error; err != nil {
		t.Fatalf("读取规范化后的选项失败: %v", err)
	}
	if len(storedOptions) != len(jsonKeys) {
		t.Fatalf("规范化后的选项数量错误: %#v", storedOptions)
	}
	for _, option := range storedOptions {
		if option.Value != "{}" {
			t.Fatalf("选项 %s 未规范化为空 JSON 对象: %q", option.Key, option.Value)
		}
	}

	common.OptionMapRWMutex.RLock()
	runtimeJSONOptions := make(map[string]string, len(jsonKeys))
	for _, key := range jsonKeys {
		runtimeJSONOptions[key] = common.OptionMap[key]
	}
	runtimeDefault := common.OptionMap["DefaultUseAutoGroup"]
	common.OptionMapRWMutex.RUnlock()
	for _, key := range jsonKeys {
		if runtimeJSONOptions[key] != "{}" {
			t.Fatalf("运行时选项 %s 未同步为空 JSON 对象: %q", key, runtimeJSONOptions[key])
		}
	}
	if runtimeDefault != "false" || setting.DefaultUseAutoGroup {
		t.Fatalf("布尔选项被错误规范化: value=%q enabled=%v", runtimeDefault, setting.DefaultUseAutoGroup)
	}
	if common.TopupGroupRatio2JSONString() != "{}" ||
		ratio_setting.GroupGroupRatio2JSONString() != "{}" ||
		specialUsableGroups.Len() != 0 {
		t.Fatal("空白 JSON 选项没有同步到运行时配置")
	}
}

func TestGroupConfigWriteLockIDsAreSortedAndUnique(t *testing.T) {
	configs := []GroupConfig{{Id: 7}, {Id: 2}, {Id: 7}, {Id: 0}}
	ids := groupConfigWriteLockIDs(configs, []int{5, 2, -1, 3})
	want := []int{2, 3, 5, 7}
	if len(ids) != len(want) {
		t.Fatalf("分组写锁 ID 数量错误: got=%v want=%v", ids, want)
	}
	for index := range want {
		if ids[index] != want[index] {
			t.Fatalf("分组写锁未按统一顺序去重: got=%v want=%v", ids, want)
		}
	}
}

func TestSaveGroupConfigWithOptionsPrunesDeletedAliasFromSameRequest(t *testing.T) {
	defaultGroup, deletedGroup := setupGroupBindingsTest(t)
	const alias = "atomic-deleted-alias"
	if err := DB.Create(&GroupAlias{Alias: alias, GroupId: deletedGroup.Id}).Error; err != nil {
		t.Fatalf("创建待删除分组别名失败: %v", err)
	}

	value := fmt.Sprintf(`{"%s":3,"%s":1}`, alias, defaultGroup.Code)
	_, err := SaveGroupConfigWithOptionsAndResult(
		nil,
		[]int{deletedGroup.Id},
		map[string]string{"TopupGroupRatio": value},
	)
	if err != nil {
		t.Fatalf("同请求保存选项并删除分组失败: %v", err)
	}

	var option Option
	if err := DB.First(&option, "key = ?", "TopupGroupRatio").Error; err != nil {
		t.Fatalf("读取清理后的充值倍率失败: %v", err)
	}
	var ratios map[string]float64
	if err := common.UnmarshalJsonStr(option.Value, &ratios); err != nil {
		t.Fatalf("解析清理后的充值倍率失败: %v", err)
	}
	if _, exists := ratios[alias]; exists || ratios[defaultGroup.Code] != 1 {
		t.Fatalf("同请求提交的待删别名没有被正确清理: %#v", ratios)
	}
}

func TestSaveGroupConfigWithOptionsRollsBackAllChangesOnInvalidReference(t *testing.T) {
	defaultGroup, _ := setupGroupBindingsTest(t)
	originalOption := &Option{Key: "TopupGroupRatio", Value: `{"default":1}`}
	if err := DB.Create(originalOption).Error; err != nil {
		t.Fatalf("创建原始充值倍率失败: %v", err)
	}

	_, err := SaveGroupConfigWithOptionsAndResult(
		[]GroupConfig{
			{
				Id:     defaultGroup.Id,
				Code:   defaultGroup.Code,
				Name:   "不应提交的新名称",
				Ratio:  defaultGroup.Ratio,
				Status: GroupStatusActive,
			},
			{
				Code:   "atomic-rollback-new",
				Name:   "不应提交的新分组",
				Ratio:  1,
				Status: GroupStatusActive,
			},
		},
		nil,
		map[string]string{"TopupGroupRatio": `{"missing-group":2}`},
	)
	if err == nil || !strings.Contains(err.Error(), "不存在的分组") {
		t.Fatalf("无效分组引用应使整个事务失败，实际错误: %v", err)
	}

	var storedDefault Group
	if err := DB.First(&storedDefault, defaultGroup.Id).Error; err != nil {
		t.Fatalf("读取回滚后的默认分组失败: %v", err)
	}
	var createdCount int64
	if err := DB.Model(&Group{}).Where("code = ?", "atomic-rollback-new").Count(&createdCount).Error; err != nil {
		t.Fatalf("统计回滚后的新分组失败: %v", err)
	}
	var storedOption Option
	if err := DB.First(&storedOption, "key = ?", originalOption.Key).Error; err != nil {
		t.Fatalf("读取回滚后的充值倍率失败: %v", err)
	}
	if storedDefault.Name != defaultGroup.Name || createdCount != 0 || storedOption.Value != originalOption.Value {
		t.Fatalf(
			"选项失败后没有完整回滚: name=%q new_count=%d option=%q",
			storedDefault.Name,
			createdCount,
			storedOption.Value,
		)
	}
}

func TestValidateGroupConfigOptionUpdatesRejectsProjectionAndUnknownKeys(t *testing.T) {
	for _, values := range []map[string]string{
		{"GroupRatio": `{"default":1}`},
		{"UnrelatedOption": "value"},
	} {
		if err := validateGroupConfigOptionUpdates(values); err == nil {
			t.Fatalf("分组配置不应接受选项: %#v", values)
		}
	}
}
