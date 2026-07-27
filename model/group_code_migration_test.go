package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func prepareGroupCodeMigrationTestDB(t *testing.T) {
	t.Helper()
	db := openGroupIdentityTestDB(t)
	oldOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() { common.OptionMap = oldOptionMap })
	if err := db.AutoMigrate(
		&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{},
		&Channel{}, &ChannelGroupBinding{}, &Token{}, &TokenGroupBinding{},
		&User{}, &Ability{}, &SubscriptionPlan{}, &UserSubscription{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
}

func TestMigrateLegacyGroupCodesToIDsRebuildsAllCurrentReferences(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	defaultGroup := Group{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive}
	legacyGroup := Group{Code: "group_2", Name: "特价", Ratio: 0.8, Status: GroupStatusActive, UserSelectable: true}
	if err := DB.Create(&defaultGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	targetCode := strconv.Itoa(legacyGroup.Id)

	channel := Channel{Name: "迁移渠道", Group: legacyGroup.Code + ",default", Models: "gpt-test", Status: common.ChannelStatusEnabled}
	if err := DB.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create([]ChannelGroupBinding{{ChannelId: channel.Id, GroupId: legacyGroup.Id, Position: 0}, {ChannelId: channel.Id, GroupId: defaultGroup.Id, Position: 1}}).Error; err != nil {
		t.Fatal(err)
	}

	limit := 1.6
	token := Token{UserId: 9, Key: "group-code-migration-token", Name: "迁移令牌", Group: legacyGroup.Code + ",default", GroupMode: TokenGroupModeExplicit, GroupRatioLimits: `{"group_2":1.6}`}
	if err := DB.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create([]TokenGroupBinding{{TokenId: token.Id, GroupId: legacyGroup.Id, Position: 0, RatioLimit: &limit}, {TokenId: token.Id, GroupId: defaultGroup.Id, Position: 1}}).Error; err != nil {
		t.Fatal(err)
	}

	user := User{Username: "migration-user", Password: "password", Group: legacyGroup.Code, GroupId: legacyGroup.Id}
	if err := DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	ability := Ability{Group: legacyGroup.Code, GroupId: legacyGroup.Id, Model: "gpt-test", ChannelId: channel.Id, Enabled: true}
	if err := DB.Create(&ability).Error; err != nil {
		t.Fatal(err)
	}
	plan := SubscriptionPlan{Title: "迁移套餐", UpgradeGroup: legacyGroup.Code}
	if err := DB.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	subscription := UserSubscription{UserId: user.Id, UpgradeGroup: legacyGroup.Code, PrevUserGroup: legacyGroup.Code}
	if err := DB.Create(&subscription).Error; err != nil {
		t.Fatal(err)
	}
	options := []Option{
		{Key: "GroupRatio", Value: `{"default":1,"group_2":0.8}`},
		{Key: "UserUsableGroups", Value: `{"default":"默认","group_2":"特价"}`},
		{Key: "GroupGroupRatio", Value: `{"default":{"group_2":0.9}}`},
		{Key: "TopupGroupRatio", Value: `{"group_2":2}`},
		{Key: "ModelRequestRateLimitGroup", Value: `{"group_2":[10,20]}`},
	}
	if err := DB.Create(&options).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatalf("预检失败: %v", err)
	}
	if !preview.CanExecute || len(preview.Groups) != 1 || preview.Groups[0].TargetCode != targetCode {
		t.Fatalf("预检结果异常: %#v", preview)
	}
	if preview.AffectedSubscriptions != 1 {
		t.Fatalf("同一订阅的两个分组字段应按订阅去重统计，实际为 %d", preview.AffectedSubscriptions)
	}
	result, err := MigrateLegacyGroupCodesToIDs()
	if err != nil {
		t.Fatalf("执行迁移失败: %v", err)
	}
	if !result.Executed || len(result.Groups) != 1 {
		t.Fatalf("执行结果异常: %#v", result)
	}
	if result.Warning != "" {
		t.Fatalf("成功执行后不应保留仅用于预检的部署警告: %q", result.Warning)
	}

	var migrated Group
	if err := DB.First(&migrated, legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.Code != targetCode {
		t.Fatalf("分组 code 未跟随 ID: %q", migrated.Code)
	}
	var alias GroupAlias
	if err := DB.First(&alias, "alias = ?", legacyGroup.Code).Error; err != nil {
		t.Fatal(err)
	}
	if alias.GroupId != legacyGroup.Id {
		t.Fatalf("历史别名归属错误: %#v", alias)
	}
	if err := DB.First(&channel, channel.Id).Error; err != nil {
		t.Fatal(err)
	}
	if channel.Group != targetCode+",default" {
		t.Fatalf("渠道镜像错误: %q", channel.Group)
	}
	if err := DB.Unscoped().First(&token, token.Id).Error; err != nil {
		t.Fatal(err)
	}
	if token.Group != targetCode+",default" || token.GetGroupRatioLimitsMap()[targetCode] != limit {
		t.Fatalf("令牌镜像或倍率保护错误: group=%q limits=%q", token.Group, token.GroupRatioLimits)
	}
	if err := DB.First(&user, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if user.Group != targetCode {
		t.Fatalf("用户分组镜像错误: %q", user.Group)
	}
	ability = Ability{}
	if err := DB.First(&ability, "group_id = ?", legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if ability.Group != targetCode {
		t.Fatalf("能力分组镜像错误: %q", ability.Group)
	}
	if err := DB.First(&plan, plan.Id).Error; err != nil {
		t.Fatal(err)
	}
	if plan.UpgradeGroup != targetCode {
		t.Fatalf("套餐升级分组错误: %q", plan.UpgradeGroup)
	}
	if err := DB.First(&subscription, subscription.Id).Error; err != nil {
		t.Fatal(err)
	}
	if subscription.UpgradeGroup != targetCode || subscription.PrevUserGroup != targetCode {
		t.Fatalf("订阅分组快照错误: %#v", subscription)
	}
	var topup Option
	if err := DB.First(&topup, commonKeyCol+" = ?", "TopupGroupRatio").Error; err != nil {
		t.Fatal(err)
	}
	if topup.Value != `{"`+targetCode+`":2}` {
		t.Fatalf("充值倍率选项错误: %s", topup.Value)
	}

	repeated, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if !repeated.CanExecute || len(repeated.Groups) != 0 {
		t.Fatalf("迁移应幂等: %#v", repeated)
	}
}

func TestPreviewGroupCodeMigrationBlocksAbilityTargetCollision(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	defaultGroup := Group{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive}
	legacyGroup := Group{Code: "group_2", Name: "特价", Ratio: 0.8, Status: GroupStatusActive}
	if err := DB.Create(&defaultGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	targetCode := strconv.Itoa(legacyGroup.Id)
	abilities := []Ability{
		{Group: legacyGroup.Code, GroupId: legacyGroup.Id, Model: "gpt-test", ChannelId: 9, Enabled: true},
		{Group: targetCode, GroupId: 999, Model: "gpt-test", ChannelId: 9, Enabled: true},
	}
	if err := DB.Create(&abilities).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanExecute || len(preview.Blockers) == 0 {
		t.Fatalf("能力目标主键冲突未阻止迁移: %#v", preview)
	}
	if _, err := MigrateLegacyGroupCodesToIDs(); err == nil {
		t.Fatal("执行阶段必须重新预检并拒绝能力目标主键冲突")
	}
	var stored Group
	if err := DB.First(&stored, legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Code != legacyGroup.Code {
		t.Fatalf("失败迁移不应修改分组标识: %q", stored.Code)
	}
}

func TestPreviewGroupCodeMigrationKeepsVirtualAutoCode(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	groups := []Group{
		{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive},
		{Code: TokenGroupModeAuto, Name: "历史自动分组", Ratio: 1, Status: GroupStatusActive},
	}
	if err := DB.Create(&groups).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanExecute || len(preview.Groups) != 0 {
		t.Fatalf("虚拟 auto 不应进入实体分组 code 迁移: %#v", preview)
	}
}

func TestPreviewGroupCodeMigrationBlocksTargetAliasCollision(t *testing.T) {
	prepareGroupCodeMigrationTestDB(t)
	defaultGroup := Group{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive}
	legacyGroup := Group{Code: "group_2", Name: "特价", Ratio: 0.8, Status: GroupStatusActive}
	if err := DB.Create(&defaultGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&legacyGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&GroupAlias{Alias: strconv.Itoa(legacyGroup.Id), GroupId: defaultGroup.Id}).Error; err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewGroupCodeMigration()
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanExecute || len(preview.Blockers) == 0 {
		t.Fatalf("目标别名冲突未阻止迁移: %#v", preview)
	}
	if _, err := MigrateLegacyGroupCodesToIDs(); err == nil {
		t.Fatal("执行阶段必须重新预检并拒绝冲突")
	}
	var stored Group
	if err := DB.First(&stored, legacyGroup.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Code != legacyGroup.Code {
		t.Fatalf("失败迁移不应修改数据: %q", stored.Code)
	}
}
