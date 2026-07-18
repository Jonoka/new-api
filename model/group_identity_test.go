package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
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
	if err := SaveGroupConfig([]GroupConfig{{Id: group.Id, Code: "renamed-code", Name: "其他", Ratio: 0.5, Status: GroupStatusActive}}, nil); err == nil {
		t.Fatal("修改 code 应该被拒绝")
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
