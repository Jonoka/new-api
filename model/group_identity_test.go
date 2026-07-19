package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
