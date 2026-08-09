package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func setupGroupBindingsTest(t *testing.T) (*Group, *Group) {
	t.Helper()
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(
		&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{},
		&ChannelGroupBinding{}, &TokenGroupBinding{},
		&Channel{}, &Token{}, &User{}, &Ability{},
	); err != nil {
		t.Fatalf("迁移关联测试表失败: %v", err)
	}
	defaultGroup := &Group{Code: "default", Name: "默认分组", Ratio: 1, Status: GroupStatusActive}
	vipGroup := &Group{Code: "vip", Name: "VIP", Ratio: 0.5, Status: GroupStatusActive}
	if err := db.Create(defaultGroup).Error; err != nil {
		t.Fatalf("创建 default 分组失败: %v", err)
	}
	if err := db.Create(vipGroup).Error; err != nil {
		t.Fatalf("创建 vip 分组失败: %v", err)
	}
	return defaultGroup, vipGroup
}

func TestChannelGroupBindingsSurviveDisplayNameChange(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:     "test-channel",
		Key:      "secret",
		Models:   "gpt-test",
		GroupIds: []int{vipGroup.Id, defaultGroup.Id},
		Status:   1,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("创建渠道失败: %v", err)
	}
	if channel.Group != "vip,default" {
		t.Fatalf("旧 CSV 镜像错误: %q", channel.Group)
	}
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Update("name", "尊贵用户").Error; err != nil {
		t.Fatalf("修改显示名失败: %v", err)
	}
	reloaded, err := GetChannelById(channel.Id, false)
	if err != nil {
		t.Fatalf("读取渠道失败: %v", err)
	}
	if len(reloaded.GroupIds) != 2 || reloaded.GroupIds[0] != vipGroup.Id || reloaded.GroupIds[1] != defaultGroup.Id {
		t.Fatalf("渠道分组 ID 或顺序改变: %#v", reloaded.GroupIds)
	}
	if len(reloaded.GroupDetails) != 2 || reloaded.GroupDetails[0].Name != "尊贵用户" {
		t.Fatalf("渠道没有解析最新显示名: %#v", reloaded.GroupDetails)
	}
}

func TestSaveGroupConfigRefreshesBoundChannelDisplayName(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:     "save-config-channel",
		Key:      "save-config-key",
		Models:   "gpt-test",
		GroupIds: []int{vipGroup.Id},
		Status:   common.ChannelStatusEnabled,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("创建渠道失败: %v", err)
	}

	configs := []GroupConfig{
		{
			Id:             defaultGroup.Id,
			Code:           defaultGroup.Code,
			Name:           defaultGroup.Name,
			Ratio:          defaultGroup.Ratio,
			Status:         GroupStatusActive,
			UserSelectable: defaultGroup.UserSelectable,
		},
		{
			Id:             vipGroup.Id,
			Code:           vipGroup.Code,
			Name:           "ccmaxaa",
			Ratio:          vipGroup.Ratio,
			Status:         GroupStatusActive,
			UserSelectable: vipGroup.UserSelectable,
		},
	}
	if err := SaveGroupConfig(configs, nil); err != nil {
		t.Fatalf("通过分组配置改名失败: %v", err)
	}

	reloaded, err := GetChannelById(channel.Id, false)
	if err != nil {
		t.Fatalf("读取改名后的渠道失败: %v", err)
	}
	if reloaded.Group != vipGroup.Code {
		t.Fatalf("兼容分组标识被错误改写: %q", reloaded.Group)
	}
	if len(reloaded.GroupIds) != 1 || reloaded.GroupIds[0] != vipGroup.Id {
		t.Fatalf("渠道稳定绑定发生变化: %#v", reloaded.GroupIds)
	}
	if len(reloaded.GroupDetails) != 1 || reloaded.GroupDetails[0].Name != "ccmaxaa" {
		t.Fatalf("渠道未返回最新显示名称: %#v", reloaded.GroupDetails)
	}
}

func TestTokenGroupBindingsPreserveOrderAndModes(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:         1,
		Key:            "token-explicit",
		Name:           "explicit",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id, defaultGroup.Id},
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建显式分组令牌失败: %v", err)
	}
	reloaded, err := GetTokenById(token.Id)
	if err != nil {
		t.Fatalf("读取显式分组令牌失败: %v", err)
	}
	if reloaded.Group != "vip,default" || reloaded.GroupMode != TokenGroupModeExplicit {
		t.Fatalf("令牌兼容字段错误: group=%q mode=%q", reloaded.Group, reloaded.GroupMode)
	}
	if len(reloaded.GroupIds) != 2 || reloaded.GroupIds[0] != vipGroup.Id || reloaded.GroupIds[1] != defaultGroup.Id {
		t.Fatalf("令牌分组 ID 或顺序错误: %#v", reloaded.GroupIds)
	}

	autoToken := &Token{UserId: 1, Key: "token-auto", Name: "auto", GroupMode: TokenGroupModeAuto, UnlimitedQuota: true}
	if err := autoToken.Insert(); err != nil {
		t.Fatalf("创建 auto 令牌失败: %v", err)
	}
	if autoToken.Group != "auto" || len(autoToken.GroupIds) != 0 {
		t.Fatalf("auto 被错误绑定为实体分组: %#v", autoToken)
	}
	var autoBindingCount int64
	if err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", autoToken.Id).Count(&autoBindingCount).Error; err != nil {
		t.Fatalf("统计 auto 关联失败: %v", err)
	}
	if autoBindingCount != 0 {
		t.Fatalf("auto 令牌不应存在分组关联，实际 %d", autoBindingCount)
	}
}

func TestMigrateTokenGroupReplacesBindingAndRatioLimit(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:           101,
		Key:              "token-group-migration-single",
		Name:             "migration-single",
		GroupMode:        TokenGroupModeExplicit,
		GroupIds:         []int{vipGroup.Id},
		GroupRatioLimits: `{"vip":2.5}`,
		UnlimitedQuota:   true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建待迁移令牌失败: %v", err)
	}

	preview, err := PreviewTokenGroupMigration(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("预览令牌分组迁移失败: %v", err)
	}
	if preview.MigratedTokens != 1 || preview.SingleGroupTokens != 1 || preview.AffectedUsers != 1 {
		t.Fatalf("迁移预览统计错误: %#v", preview)
	}

	result, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("迁移令牌分组失败: %v", err)
	}
	if result.MigratedTokens != 1 || result.DeduplicatedTokens != 0 {
		t.Fatalf("迁移结果统计错误: %#v", result)
	}

	var stored Token
	if err := DB.Unscoped().First(&stored, token.Id).Error; err != nil {
		t.Fatalf("读取迁移后令牌失败: %v", err)
	}
	if stored.Group != defaultGroup.Code || stored.GroupMode != TokenGroupModeExplicit {
		t.Fatalf("令牌兼容字段未同步: group=%q mode=%q", stored.Group, stored.GroupMode)
	}
	limits := stored.GetGroupRatioLimitsMap()
	if len(limits) != 1 || limits[defaultGroup.Code] != 2.5 {
		t.Fatalf("倍率保护未迁移到目标分组: %#v", limits)
	}
	var bindings []TokenGroupBinding
	if err := DB.Where("token_id = ?", token.Id).Order("position ASC").Find(&bindings).Error; err != nil {
		t.Fatalf("读取迁移后绑定失败: %v", err)
	}
	if len(bindings) != 1 || bindings[0].GroupId != defaultGroup.Id || bindings[0].Position != 0 || bindings[0].RatioLimit == nil || *bindings[0].RatioLimit != 2.5 {
		t.Fatalf("稳定绑定未正确迁移: %#v", bindings)
	}

	repeated, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("重复迁移不应失败: %v", err)
	}
	if repeated.MigratedTokens != 0 {
		t.Fatalf("重复迁移应保持幂等，实际迁移 %d 个令牌", repeated.MigratedTokens)
	}
}

func TestMigrateTokenGroupDeduplicatesAndKeepsTargetOrderAndLimit(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	otherGroup := &Group{Code: "other", Name: "其他分组", Ratio: 1, Status: GroupStatusActive}
	if err := DB.Create(otherGroup).Error; err != nil {
		t.Fatalf("创建其他分组失败: %v", err)
	}
	token := &Token{
		UserId:           102,
		Key:              "token-group-migration-deduplicate",
		Name:             "migration-deduplicate",
		GroupMode:        TokenGroupModeExplicit,
		GroupIds:         []int{vipGroup.Id, otherGroup.Id, defaultGroup.Id},
		GroupRatioLimits: `{"vip":2,"default":1.25}`,
		UnlimitedQuota:   true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建多分组令牌失败: %v", err)
	}

	result, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("迁移多分组令牌失败: %v", err)
	}
	if result.MigratedTokens != 1 || result.DeduplicatedTokens != 1 || result.MultiGroupTokens != 1 {
		t.Fatalf("多分组迁移统计错误: %#v", result)
	}

	reloaded, err := GetTokenById(token.Id)
	if err != nil {
		t.Fatalf("读取多分组令牌失败: %v", err)
	}
	if reloaded.Group != "other,default" {
		t.Fatalf("去重后分组顺序错误: %q", reloaded.Group)
	}
	if len(reloaded.GroupIds) != 2 || reloaded.GroupIds[0] != otherGroup.Id || reloaded.GroupIds[1] != defaultGroup.Id {
		t.Fatalf("去重后稳定 ID 顺序错误: %#v", reloaded.GroupIds)
	}
	limits := reloaded.GetGroupRatioLimitsMap()
	if len(limits) != 1 || limits[defaultGroup.Code] != 1.25 {
		t.Fatalf("目标分组原倍率保护未保留: %#v", limits)
	}
	var bindings []TokenGroupBinding
	if err := DB.Where("token_id = ?", token.Id).Order("position ASC").Find(&bindings).Error; err != nil {
		t.Fatalf("读取去重后绑定失败: %v", err)
	}
	if len(bindings) != 2 || bindings[0].GroupId != otherGroup.Id || bindings[0].Position != 0 || bindings[1].GroupId != defaultGroup.Id || bindings[1].Position != 1 {
		t.Fatalf("去重后绑定顺序未压缩: %#v", bindings)
	}
	if bindings[1].RatioLimit == nil || *bindings[1].RatioLimit != 1.25 {
		t.Fatalf("目标分组绑定倍率保护未保留: %#v", bindings[1])
	}
}

func TestMigrateTokenGroupBackfillsLegacyTokenAndLeavesAutoUntouched(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	legacyToken := &Token{
		UserId:           103,
		Key:              "token-group-migration-legacy",
		Name:             "migration-legacy",
		Group:            vipGroup.Code,
		GroupMode:        TokenGroupModeExplicit,
		GroupRatioLimits: `{"vip":1.8}`,
		UnlimitedQuota:   true,
	}
	if err := DB.Create(legacyToken).Error; err != nil {
		t.Fatalf("创建历史令牌失败: %v", err)
	}
	autoToken := &Token{
		UserId:         103,
		Key:            "token-group-migration-auto",
		Name:           "migration-auto",
		Group:          "auto",
		GroupMode:      TokenGroupModeAuto,
		UnlimitedQuota: true,
	}
	if err := DB.Create(autoToken).Error; err != nil {
		t.Fatalf("创建 auto 令牌失败: %v", err)
	}

	result, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("迁移历史令牌失败: %v", err)
	}
	if result.MigratedTokens != 1 {
		t.Fatalf("历史令牌迁移数量错误: %#v", result)
	}

	var migrated Token
	if err := DB.First(&migrated, legacyToken.Id).Error; err != nil {
		t.Fatalf("读取迁移后的历史令牌失败: %v", err)
	}
	if migrated.Group != defaultGroup.Code || migrated.GetGroupRatioLimitsMap()[defaultGroup.Code] != 1.8 {
		t.Fatalf("历史令牌兼容字段未同步: %#v", migrated)
	}
	var binding TokenGroupBinding
	if err := DB.First(&binding, "token_id = ?", legacyToken.Id).Error; err != nil {
		t.Fatalf("历史令牌未回填稳定绑定: %v", err)
	}
	if binding.GroupId != defaultGroup.Id || binding.Position != 0 {
		t.Fatalf("历史令牌稳定绑定错误: %#v", binding)
	}
	var storedAuto Token
	if err := DB.First(&storedAuto, autoToken.Id).Error; err != nil {
		t.Fatalf("读取 auto 令牌失败: %v", err)
	}
	if storedAuto.Group != "auto" || storedAuto.GroupMode != TokenGroupModeAuto {
		t.Fatalf("auto 令牌被错误修改: %#v", storedAuto)
	}
}

func TestMigrateTokenGroupRejectsInvalidTargetAndRollsBackInvalidRatioJSON(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	if _, err := MigrateTokenGroup(vipGroup.Id, vipGroup.Id); err == nil {
		t.Fatal("源分组与目标分组相同时应拒绝迁移")
	}
	if err := DB.Model(&Group{}).Where("id = ?", defaultGroup.Id).Update("status", GroupStatusDisabled).Error; err != nil {
		t.Fatalf("禁用目标分组失败: %v", err)
	}
	if _, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id); err == nil {
		t.Fatal("目标分组禁用时应拒绝迁移")
	}
	if err := DB.Model(&Group{}).Where("id = ?", defaultGroup.Id).Update("status", GroupStatusActive).Error; err != nil {
		t.Fatalf("重新启用目标分组失败: %v", err)
	}

	validToken := &Token{
		UserId:         104,
		Key:            "token-group-migration-valid-before-rollback",
		Name:           "valid-before-rollback",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := validToken.Insert(); err != nil {
		t.Fatalf("创建正常令牌失败: %v", err)
	}
	invalidToken := &Token{
		UserId:           104,
		Key:              "token-group-migration-invalid-json",
		Name:             "invalid-json",
		Group:            vipGroup.Code,
		GroupMode:        TokenGroupModeExplicit,
		GroupRatioLimits: "{invalid",
		UnlimitedQuota:   true,
	}
	if err := DB.Create(invalidToken).Error; err != nil {
		t.Fatalf("创建倍率 JSON 异常令牌失败: %v", err)
	}
	if err := DB.Create(&TokenGroupBinding{TokenId: invalidToken.Id, GroupId: vipGroup.Id, Position: 0}).Error; err != nil {
		t.Fatalf("创建倍率 JSON 异常令牌绑定失败: %v", err)
	}

	if _, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id); err == nil {
		t.Fatal("倍率保护 JSON 非法时应回滚整笔迁移")
	}
	var storedValid Token
	if err := DB.First(&storedValid, validToken.Id).Error; err != nil {
		t.Fatalf("读取回滚后的正常令牌失败: %v", err)
	}
	if storedValid.Group != vipGroup.Code {
		t.Fatalf("迁移失败后正常令牌仍被修改: %q", storedValid.Group)
	}
	var sourceBindingCount int64
	if err := DB.Model(&TokenGroupBinding{}).Where("group_id = ?", vipGroup.Id).Count(&sourceBindingCount).Error; err != nil {
		t.Fatalf("统计回滚后的源绑定失败: %v", err)
	}
	if sourceBindingCount != 2 {
		t.Fatalf("迁移失败后源绑定数量改变: %d", sourceBindingCount)
	}
}

func TestMigrateTokenGroupUsesExactStableIdentityForLegacyCandidates(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	upperVIP := &Group{Code: "VIP", Name: "大写 VIP", Ratio: 1, Status: GroupStatusActive}
	if err := DB.Create(upperVIP).Error; err != nil {
		t.Fatalf("创建大小写不同的分组失败: %v", err)
	}
	spacedToken := &Token{
		UserId:         105,
		Key:            "token-group-migration-spaced-legacy",
		Name:           "spaced-legacy",
		Group:          "default, vip",
		GroupMode:      TokenGroupModeExplicit,
		UnlimitedQuota: true,
	}
	if err := DB.Create(spacedToken).Error; err != nil {
		t.Fatalf("创建带空格历史令牌失败: %v", err)
	}
	upperToken := &Token{
		UserId:           106,
		Key:              "token-group-migration-uppercase",
		Name:             "uppercase",
		Group:            upperVIP.Code,
		GroupMode:        TokenGroupModeExplicit,
		GroupRatioLimits: "{invalid",
		UnlimitedQuota:   true,
	}
	if err := DB.Create(upperToken).Error; err != nil {
		t.Fatalf("创建大写分组令牌失败: %v", err)
	}

	result, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("迁移带空格历史令牌失败: %v", err)
	}
	if result.MigratedTokens != 1 || result.DeduplicatedTokens != 1 {
		t.Fatalf("精确候选统计错误: %#v", result)
	}
	var migrated Token
	if err := DB.First(&migrated, spacedToken.Id).Error; err != nil {
		t.Fatalf("读取带空格历史令牌失败: %v", err)
	}
	if migrated.Group != defaultGroup.Code {
		t.Fatalf("带空格历史令牌未迁移: %q", migrated.Group)
	}
	var untouched Token
	if err := DB.First(&untouched, upperToken.Id).Error; err != nil {
		t.Fatalf("读取大写分组令牌失败: %v", err)
	}
	if untouched.Group != upperVIP.Code {
		t.Fatalf("大小写不同的分组被误迁移: %q", untouched.Group)
	}
}

func TestMigrateTokenGroupResolvesSourceAliases(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	alias := &GroupAlias{Alias: "legacy-vip", GroupId: vipGroup.Id, CreatedAt: 1}
	if err := DB.Create(alias).Error; err != nil {
		t.Fatalf("创建源分组别名失败: %v", err)
	}
	legacyOnly := &Token{
		UserId:           112,
		Key:              "token-group-migration-alias-legacy",
		Name:             "alias-legacy",
		Group:            alias.Alias,
		GroupMode:        TokenGroupModeExplicit,
		GroupRatioLimits: `{"legacy-vip":1.7}`,
		UnlimitedQuota:   true,
	}
	if err := DB.Create(legacyOnly).Error; err != nil {
		t.Fatalf("创建仅含别名的历史令牌失败: %v", err)
	}
	stable := &Token{
		UserId:         112,
		Key:            "token-group-migration-alias-stable",
		Name:           "alias-stable",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := stable.Insert(); err != nil {
		t.Fatalf("创建稳定绑定别名令牌失败: %v", err)
	}
	if err := DB.Model(&Token{}).Where("id = ?", stable.Id).Update("group", alias.Alias).Error; err != nil {
		t.Fatalf("把稳定令牌镜像改为别名失败: %v", err)
	}

	result, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("通过源分组别名迁移失败: %v", err)
	}
	if result.MigratedTokens != 2 {
		t.Fatalf("别名迁移数量错误: %#v", result)
	}
	var tokens []Token
	if err := DB.Where("id IN ?", []int{legacyOnly.Id, stable.Id}).Order("id ASC").Find(&tokens).Error; err != nil {
		t.Fatalf("读取别名迁移后的令牌失败: %v", err)
	}
	for _, token := range tokens {
		if token.Group != defaultGroup.Code {
			t.Fatalf("别名迁移后未写入目标 canonical code: token=%d group=%q", token.Id, token.Group)
		}
	}
	if tokens[0].GetGroupRatioLimitsMap()[defaultGroup.Code] != 1.7 {
		t.Fatalf("别名倍率保护未迁移到目标 canonical code: %q", tokens[0].GroupRatioLimits)
	}
	if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err != nil {
		t.Fatalf("迁移别名令牌后删除源分组失败: %v", err)
	}
	var aliasCount int64
	if err := DB.Model(&GroupAlias{}).Where("group_id = ?", vipGroup.Id).Count(&aliasCount).Error; err != nil {
		t.Fatalf("统计删除后的分组别名失败: %v", err)
	}
	if aliasCount != 0 {
		t.Fatalf("删除源分组后仍残留 %d 个孤立别名", aliasCount)
	}
}

func TestMigrateTokenGroupDoesNotInheritSourceLimitWhenTargetAlreadyExists(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:           110,
		Key:              "token-group-migration-target-without-limit",
		Name:             "target-without-limit",
		GroupMode:        TokenGroupModeExplicit,
		GroupIds:         []int{defaultGroup.Id, vipGroup.Id},
		GroupRatioLimits: `{"vip":2.2}`,
		UnlimitedQuota:   true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建目标无倍率保护令牌失败: %v", err)
	}
	if _, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id); err != nil {
		t.Fatalf("迁移目标无倍率保护令牌失败: %v", err)
	}
	var stored Token
	if err := DB.First(&stored, token.Id).Error; err != nil {
		t.Fatalf("读取目标无倍率保护令牌失败: %v", err)
	}
	if stored.Group != defaultGroup.Code || strings.TrimSpace(stored.GroupRatioLimits) != "" {
		t.Fatalf("源倍率保护被错误继承到已有目标: %#v", stored)
	}
	var binding TokenGroupBinding
	if err := DB.First(&binding, "token_id = ?", token.Id).Error; err != nil {
		t.Fatalf("读取目标无倍率保护绑定失败: %v", err)
	}
	if binding.GroupId != defaultGroup.Id || binding.RatioLimit != nil {
		t.Fatalf("已有目标错误继承源绑定倍率: %#v", binding)
	}
}

func TestMigrateTokenGroupCleansDeletedTokenWithoutRebindingTarget(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:           111,
		Key:              "token-group-migration-deleted",
		Name:             "deleted",
		GroupMode:        TokenGroupModeExplicit,
		GroupIds:         []int{vipGroup.Id},
		GroupRatioLimits: `{"vip":1.6}`,
		UnlimitedQuota:   true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建待删除令牌失败: %v", err)
	}
	if err := token.Delete(); err != nil {
		t.Fatalf("软删除令牌失败: %v", err)
	}

	result, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("清理软删除令牌分组失败: %v", err)
	}
	if result.MigratedTokens != 0 || result.CleanedDeletedTokens != 1 {
		t.Fatalf("软删除令牌统计错误: %#v", result)
	}
	var stored Token
	if err := DB.Unscoped().First(&stored, token.Id).Error; err != nil {
		t.Fatalf("读取清理后的软删除令牌失败: %v", err)
	}
	if stored.Group != "" || stored.GroupMode != TokenGroupModeInherit || stored.GroupRatioLimits != "" {
		t.Fatalf("软删除令牌历史分组未清空: %#v", stored)
	}
	var bindingCount int64
	if err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", token.Id).Count(&bindingCount).Error; err != nil {
		t.Fatalf("统计软删除令牌绑定失败: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("软删除令牌不应重新绑定目标分组，实际 %d 条", bindingCount)
	}
}

func TestBatchDeleteTokensRemovesOnlySelectedTokenBindings(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	selected := &Token{
		UserId: 201, Key: "batch-delete-selected", Name: "batch-delete-selected",
		GroupMode: TokenGroupModeExplicit, GroupIds: []int{vipGroup.Id}, UnlimitedQuota: true,
	}
	otherUser := &Token{
		UserId: 202, Key: "batch-delete-other-user", Name: "batch-delete-other-user",
		GroupMode: TokenGroupModeExplicit, GroupIds: []int{vipGroup.Id}, UnlimitedQuota: true,
	}
	if err := selected.Insert(); err != nil {
		t.Fatalf("创建待批量删除令牌失败: %v", err)
	}
	if err := otherUser.Insert(); err != nil {
		t.Fatalf("创建其他用户令牌失败: %v", err)
	}

	deleted, err := BatchDeleteTokens([]int{otherUser.Id, selected.Id}, selected.UserId)
	if err != nil {
		t.Fatalf("批量删除令牌失败: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("批量删除数量错误: %d", deleted)
	}
	var selectedBindings, otherBindings int64
	if err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", selected.Id).Count(&selectedBindings).Error; err != nil {
		t.Fatalf("统计已删除令牌绑定失败: %v", err)
	}
	if err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", otherUser.Id).Count(&otherBindings).Error; err != nil {
		t.Fatalf("统计其他用户令牌绑定失败: %v", err)
	}
	if selectedBindings != 0 || otherBindings != 1 {
		t.Fatalf("批量删除影响范围错误: selected=%d other=%d", selectedBindings, otherBindings)
	}
	if err := DB.First(&Token{}, otherUser.Id).Error; err != nil {
		t.Fatalf("其他用户令牌被误删: %v", err)
	}
}

func TestMigrateTokenGroupRejectsInconsistentBindingMirrors(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	partialToken := &Token{
		UserId:         107,
		Key:            "token-group-migration-partial-binding",
		Name:           "partial-binding",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := partialToken.Insert(); err != nil {
		t.Fatalf("创建部分绑定测试令牌失败: %v", err)
	}
	if err := DB.Model(&Token{}).Where("id = ?", partialToken.Id).Update("group", "vip,default").Error; err != nil {
		t.Fatalf("制造分组镜像不一致失败: %v", err)
	}
	if _, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id); err == nil {
		t.Fatal("稳定绑定与 CSV 不一致时应拒绝迁移")
	}

	if err := DB.Model(&Token{}).Where("id = ?", partialToken.Id).Update("group", vipGroup.Code).Error; err != nil {
		t.Fatalf("恢复分组 CSV 失败: %v", err)
	}
	if err := DB.Model(&Token{}).Where("id = ?", partialToken.Id).Update("group_ratio_limits", `{"vip":2}`).Error; err != nil {
		t.Fatalf("制造倍率镜像不一致失败: %v", err)
	}
	if _, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id); err == nil {
		t.Fatal("稳定倍率保护与 JSON 镜像不一致时应拒绝迁移")
	}

	var stored Token
	if err := DB.First(&stored, partialToken.Id).Error; err != nil {
		t.Fatalf("读取拒绝迁移后的令牌失败: %v", err)
	}
	if stored.Group != vipGroup.Code {
		t.Fatalf("镜像不一致时仍修改了令牌: %q", stored.Group)
	}
}

func TestMigrateTokenGroupAllowsDisabledSourceAndRejectsMissingGroups(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:         108,
		Key:            "token-group-migration-disabled-source",
		Name:           "disabled-source",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建禁用源分组测试令牌失败: %v", err)
	}
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Update("status", GroupStatusDisabled).Error; err != nil {
		t.Fatalf("禁用源分组失败: %v", err)
	}
	if _, err := MigrateTokenGroup(999999, defaultGroup.Id); err == nil {
		t.Fatal("源分组不存在时应拒绝迁移")
	}
	if _, err := MigrateTokenGroup(vipGroup.Id, 999999); err == nil {
		t.Fatal("目标分组不存在时应拒绝迁移")
	}
	if _, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id); err != nil {
		t.Fatalf("禁用的源分组应允许迁移: %v", err)
	}
}

func TestMigrateTokenGroupRollsBackWritesWhenLaterBindingFails(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	first := &Token{
		UserId:         109,
		Key:            "token-group-migration-transaction-first",
		Name:           "transaction-first",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	second := &Token{
		UserId:         109,
		Key:            "token-group-migration-transaction-second",
		Name:           "transaction-second",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := first.Insert(); err != nil {
		t.Fatalf("创建第一个事务测试令牌失败: %v", err)
	}
	if err := second.Insert(); err != nil {
		t.Fatalf("创建第二个事务测试令牌失败: %v", err)
	}
	triggerSQL := fmt.Sprintf(`
		CREATE TRIGGER fail_second_group_migration
		BEFORE INSERT ON token_groups
		WHEN NEW.token_id = %d AND NEW.group_id = %d
		BEGIN
			SELECT RAISE(ABORT, 'forced migration failure');
		END`, second.Id, defaultGroup.Id)
	if err := DB.Exec(triggerSQL).Error; err != nil {
		t.Fatalf("创建事务失败注入触发器失败: %v", err)
	}

	if _, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id); err == nil {
		t.Fatal("第二个令牌写入失败时整笔迁移应失败")
	}
	var tokens []Token
	if err := DB.Where("id IN ?", []int{first.Id, second.Id}).Order("id ASC").Find(&tokens).Error; err != nil {
		t.Fatalf("读取事务回滚后的令牌失败: %v", err)
	}
	if len(tokens) != 2 || tokens[0].Group != vipGroup.Code || tokens[1].Group != vipGroup.Code {
		t.Fatalf("事务失败后令牌镜像未完整回滚: %#v", tokens)
	}
	var sourceBindings int64
	if err := DB.Model(&TokenGroupBinding{}).
		Where("token_id IN ? AND group_id = ?", []int{first.Id, second.Id}, vipGroup.Id).
		Count(&sourceBindings).Error; err != nil {
		t.Fatalf("统计事务回滚后的稳定绑定失败: %v", err)
	}
	if sourceBindings != 2 {
		t.Fatalf("事务失败后稳定绑定未完整回滚: %d", sourceBindings)
	}
}

func TestMigrateTokenGroupBatchesMoreThanSQLiteParameterLimit(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	const tokenCount = 501
	tokens := make([]Token, tokenCount)
	for index := range tokens {
		tokens[index] = Token{
			UserId:         113,
			Key:            fmt.Sprintf("token-group-migration-batch-%d", index),
			Name:           fmt.Sprintf("migration-batch-%d", index),
			Group:          vipGroup.Code,
			GroupMode:      TokenGroupModeExplicit,
			UnlimitedQuota: true,
		}
	}
	if err := DB.CreateInBatches(&tokens, 100).Error; err != nil {
		t.Fatalf("批量创建待迁移令牌失败: %v", err)
	}

	result, err := MigrateTokenGroup(vipGroup.Id, defaultGroup.Id)
	if err != nil {
		t.Fatalf("分批迁移大量令牌失败: %v", err)
	}
	if result.MigratedTokens != tokenCount {
		t.Fatalf("大量令牌迁移数量错误: %d", result.MigratedTokens)
	}
	var targetBindings int64
	if err := DB.Model(&TokenGroupBinding{}).Where("group_id = ?", defaultGroup.Id).Count(&targetBindings).Error; err != nil {
		t.Fatalf("统计大量迁移后的目标绑定失败: %v", err)
	}
	if targetBindings != tokenCount {
		t.Fatalf("大量迁移后的目标绑定数量错误: %d", targetBindings)
	}
}

func TestBatchInsertChannelsWritesIDsAndBindings(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	channels := make([]Channel, 51)
	for i := range channels {
		channels[i] = Channel{
			Name:     fmt.Sprintf("batch-channel-%d", i),
			Key:      fmt.Sprintf("batch-key-%d", i),
			Models:   "gpt-test",
			GroupIds: []int{vipGroup.Id},
			Status:   common.ChannelStatusEnabled,
		}
	}
	if err := BatchInsertChannels(channels); err != nil {
		t.Fatalf("批量创建渠道失败: %v", err)
	}
	for i := range channels {
		if channels[i].Id <= 0 {
			t.Fatalf("第 %d 个渠道未写回 ID", i)
		}
	}
	var bindingCount int64
	if err := DB.Model(&ChannelGroupBinding{}).Count(&bindingCount).Error; err != nil {
		t.Fatalf("统计批量渠道绑定失败: %v", err)
	}
	if bindingCount != int64(len(channels)) {
		t.Fatalf("批量渠道绑定数量错误: got %d want %d", bindingCount, len(channels))
	}
}

func TestDeleteDisabledChannelCleansGroupBindings(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:     "disabled-channel",
		Key:      "disabled-key",
		Models:   "gpt-test",
		GroupIds: []int{vipGroup.Id},
		Status:   common.ChannelStatusManuallyDisabled,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("创建禁用渠道失败: %v", err)
	}
	rows, err := DeleteDisabledChannel()
	if err != nil {
		t.Fatalf("删除禁用渠道失败: %v", err)
	}
	if rows != 1 {
		t.Fatalf("删除禁用渠道数量错误: %d", rows)
	}
	var bindingCount int64
	if err := DB.Model(&ChannelGroupBinding{}).Where("channel_id = ?", channel.Id).Count(&bindingCount).Error; err != nil {
		t.Fatalf("检查渠道绑定清理失败: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("禁用渠道绑定未清理: %d", bindingCount)
	}
}

func TestLegacyFallbackAllowsExistingDisabledBindings(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:     "legacy-disabled-channel",
		Key:      "legacy-disabled-key",
		Models:   "gpt-test",
		GroupIds: []int{vipGroup.Id},
		Status:   common.ChannelStatusEnabled,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("创建渠道失败: %v", err)
	}
	token := &Token{
		UserId:         1,
		Key:            "legacy-disabled-token",
		Name:           "legacy-disabled-token",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}
	if err := DB.Where("channel_id = ?", channel.Id).Delete(&ChannelGroupBinding{}).Error; err != nil {
		t.Fatalf("清空渠道关系表失败: %v", err)
	}
	if err := DB.Where("token_id = ?", token.Id).Delete(&TokenGroupBinding{}).Error; err != nil {
		t.Fatalf("清空令牌关系表失败: %v", err)
	}
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Update("status", GroupStatusDisabled).Error; err != nil {
		t.Fatalf("禁用分组失败: %v", err)
	}

	reloadedChannel, err := GetChannelById(channel.Id, true)
	if err != nil {
		t.Fatalf("从旧 CSV 回退读取渠道失败: %v", err)
	}
	if len(reloadedChannel.GroupIds) != 1 || reloadedChannel.GroupIds[0] != vipGroup.Id {
		t.Fatalf("渠道旧 CSV 未回退为稳定 ID: %#v", reloadedChannel.GroupIds)
	}
	reloadedChannel.Name = "legacy-disabled-channel-updated"
	if err := reloadedChannel.Update(); err != nil {
		t.Fatalf("编辑已有禁用分组渠道不应失败: %v", err)
	}

	reloadedToken, err := GetTokenById(token.Id)
	if err != nil {
		t.Fatalf("从旧 CSV 回退读取令牌失败: %v", err)
	}
	if len(reloadedToken.GroupIds) != 1 || reloadedToken.GroupIds[0] != vipGroup.Id {
		t.Fatalf("令牌旧 CSV 未回退为稳定 ID: %#v", reloadedToken.GroupIds)
	}
	reloadedToken.Name = "legacy-disabled-token-updated"
	if err := reloadedToken.Update(); err != nil {
		t.Fatalf("编辑已有禁用分组令牌不应失败: %v", err)
	}

	newChannel := &Channel{
		Name:     "new-channel",
		Key:      "new-key",
		Models:   "gpt-test",
		GroupIds: []int{defaultGroup.Id},
		Status:   common.ChannelStatusEnabled,
	}
	if err := newChannel.Insert(); err != nil {
		t.Fatalf("创建默认分组渠道失败: %v", err)
	}
	newChannel.GroupIds = []int{vipGroup.Id}
	if err := newChannel.Update(); err == nil {
		t.Fatal("新增选择已禁用分组应被拒绝")
	}
	newToken := &Token{
		UserId:         1,
		Key:            "new-token",
		Name:           "new-token",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{defaultGroup.Id},
		UnlimitedQuota: true,
	}
	if err := newToken.Insert(); err != nil {
		t.Fatalf("创建默认分组令牌失败: %v", err)
	}
	newToken.GroupIds = []int{vipGroup.Id}
	if err := newToken.Update(); err == nil {
		t.Fatal("令牌新增选择已禁用分组应被拒绝")
	}
}

func TestTokenUpdateExplicitEmptyDoesNotReuseOldBindings(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:         1,
		Key:            "explicit-empty-token",
		Name:           "explicit-empty-token",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}
	token.Group = ""
	token.GroupMode = ""
	token.GroupIds = []int{}
	if err := token.Update(); err != nil {
		t.Fatalf("显式空分组转 inherit 失败: %v", err)
	}
	if token.GroupMode != TokenGroupModeInherit || token.Group != "" {
		t.Fatalf("显式空分组语义错误: mode=%q group=%q", token.GroupMode, token.Group)
	}
	var bindingCount int64
	if err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", token.Id).Count(&bindingCount).Error; err != nil {
		t.Fatalf("检查令牌绑定失败: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("显式空分组沿用了旧绑定: %d", bindingCount)
	}
}

func TestEditChannelByTagUsesStableGroupIDs(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	tag := "bulk-tag"
	for i := 0; i < 2; i++ {
		channel := &Channel{
			Name:     fmt.Sprintf("bulk-channel-%d", i),
			Key:      fmt.Sprintf("bulk-key-%d", i),
			Models:   "gpt-test",
			GroupIds: []int{defaultGroup.Id},
			Status:   common.ChannelStatusEnabled,
			Tag:      &tag,
		}
		if err := channel.Insert(); err != nil {
			t.Fatalf("创建标签渠道失败: %v", err)
		}
	}

	groupIDs := []int{vipGroup.Id}
	if err := EditChannelByTag(
		tag,
		nil,
		nil,
		nil,
		nil,
		&groupIDs,
		nil,
		nil,
		nil,
		nil,
		nil,
	); err != nil {
		t.Fatalf("按标签绑定稳定分组失败: %v", err)
	}

	var channels []Channel
	if err := DB.Where("tag = ?", tag).Find(&channels).Error; err != nil {
		t.Fatalf("读取标签渠道失败: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("标签渠道数量错误: %d", len(channels))
	}
	for _, channel := range channels {
		if channel.Group != "vip" {
			t.Fatalf("兼容分组字段未规范化: %q", channel.Group)
		}
		var binding ChannelGroupBinding
		if err := DB.Where("channel_id = ?", channel.Id).First(&binding).Error; err != nil {
			t.Fatalf("读取渠道稳定绑定失败: %v", err)
		}
		if binding.GroupId != vipGroup.Id {
			t.Fatalf("渠道稳定绑定错误: %#v", binding)
		}
		var ability Ability
		if err := DB.Where("channel_id = ? AND model = ?", channel.Id, "gpt-test").First(&ability).Error; err != nil {
			t.Fatalf("读取渠道能力失败: %v", err)
		}
		if ability.GroupId != vipGroup.Id {
			t.Fatalf("能力稳定分组 ID 未同步: %#v", ability)
		}
	}
}

func TestHydrateIgnoresMissingRelationshipTables(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	channel := &Channel{Id: 1, Group: "legacy-channel"}
	token := &Token{Id: 1, Group: "legacy-token", GroupMode: TokenGroupModeExplicit}
	if err := HydrateChannelGroupBindings(db, []*Channel{channel}); err != nil {
		t.Fatalf("关系表不存在时渠道 Hydrate 不应报错: %v", err)
	}
	if err := HydrateTokenGroupBindings(db, []*Token{token}); err != nil {
		t.Fatalf("关系表不存在时令牌 Hydrate 不应报错: %v", err)
	}
	if channel.Group != "legacy-channel" || token.Group != "legacy-token" {
		t.Fatalf("关系表不存在时旧 CSV 被改写: channel=%q token=%q", channel.Group, token.Group)
	}
}

func TestChannelInfoScanHandlesDatabaseValues(t *testing.T) {
	validJSON := `{"is_multi_key":true,"multi_key_size":2,"multi_key_polling_index":1}`
	for _, value := range []interface{}{validJSON, []byte(validJSON)} {
		var info ChannelInfo
		if err := info.Scan(value); err != nil {
			t.Fatalf("合法渠道信息扫描失败（%T）: %v", value, err)
		}
		if !info.IsMultiKey || info.MultiKeySize != 2 || info.MultiKeyPollingIndex != 1 {
			t.Fatalf("合法渠道信息扫描结果错误（%T）: %#v", value, info)
		}
	}

	for _, value := range []interface{}{nil, "", " \t\r\n ", []byte(nil), []byte(" \n ")} {
		info := ChannelInfo{IsMultiKey: true, MultiKeySize: 9, MultiKeyPollingIndex: 8}
		if err := info.Scan(value); err != nil {
			t.Fatalf("空渠道信息应被视为零值（%T）: %v", value, err)
		}
		if info.IsMultiKey || info.MultiKeySize != 0 || info.MultiKeyPollingIndex != 0 {
			t.Fatalf("空渠道信息未重置为零值（%T）: %#v", value, info)
		}
	}

	info := ChannelInfo{MultiKeySize: 7}
	if err := info.Scan([]byte(`{"is_multi_key":`)); err == nil {
		t.Fatal("非空坏 JSON 不应被静默接受")
	}
	if info.MultiKeySize != 7 {
		t.Fatalf("坏 JSON 不应部分改写原值: %#v", info)
	}
	if err := info.Scan(123); err == nil {
		t.Fatal("未知数据库类型不应被静默接受")
	}
}

func TestBackfillGroupBindingsDoesNotScanUnrelatedChannelFields(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:   "legacy-broken-info-channel",
		Key:    "legacy-broken-info-channel-key",
		Models: "gpt-test",
		Group:  vipGroup.Code,
		Status: common.ChannelStatusEnabled,
	}
	token := &Token{
		UserId:         1,
		Key:            "legacy-idempotent-token",
		Name:           "legacy-idempotent-token",
		Group:          vipGroup.Code,
		GroupMode:      TokenGroupModeInherit,
		UnlimitedQuota: true,
	}
	if err := DB.Create(channel).Error; err != nil {
		t.Fatalf("创建渠道失败: %v", err)
	}
	if err := DB.Create(token).Error; err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}
	if err := DB.Exec("UPDATE channels SET channel_info = ? WHERE id = ?", "{broken", channel.Id).Error; err != nil {
		t.Fatalf("写入损坏渠道信息失败: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := BackfillGroupBindings(); err != nil {
			t.Fatalf("第 %d 次回填不应扫描无关渠道字段: %v", run, err)
		}
	}

	channelIDs, err := loadChannelBindingIDs(DB, channel.Id)
	if err != nil {
		t.Fatalf("读取渠道关联失败: %v", err)
	}
	tokenIDs, err := loadTokenBindingIDs(DB, token.Id)
	if err != nil {
		t.Fatalf("读取令牌关联失败: %v", err)
	}
	if len(channelIDs) != 1 || channelIDs[0] != vipGroup.Id {
		t.Fatalf("渠道关联回填错误: %#v", channelIDs)
	}
	if len(tokenIDs) != 1 || tokenIDs[0] != vipGroup.Id {
		t.Fatalf("令牌关联回填错误: %#v", tokenIDs)
	}
	var storedToken Token
	if err := DB.Select("id", "group_mode").First(&storedToken, token.Id).Error; err != nil {
		t.Fatalf("读取令牌模式失败: %v", err)
	}
	if storedToken.GroupMode != TokenGroupModeExplicit {
		t.Fatalf("令牌模式未幂等回填: %q", storedToken.GroupMode)
	}
}

func TestBackfillGroupBindingsReturnsMissingTableErrors(t *testing.T) {
	setupGroupBindingsTest(t)
	if err := DB.Migrator().DropTable(&TokenGroupBinding{}); err != nil {
		t.Fatalf("删除令牌关系表失败: %v", err)
	}
	err := BackfillGroupBindings()
	if err == nil {
		t.Fatal("启动回填不应静默忽略缺失关系表")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "token_groups") {
		t.Fatalf("缺表错误未保留表名: %v", err)
	}
}

func TestHydrateGroupBindingsOnlyIgnoresInvalidLegacyValues(t *testing.T) {
	setupGroupBindingsTest(t)
	invalidGroup := "1' AND SUBSTRING((SELECT username FROM users WHERE role=1 LIMIT 1)"
	channel := &Channel{Id: 9001, Group: "null"}
	token := &Token{Id: 9001, Group: invalidGroup, GroupMode: TokenGroupModeExplicit}
	if err := HydrateChannelGroupBindings(DB, []*Channel{channel}); err != nil {
		t.Fatalf("渠道历史非法值应被忽略: %v", err)
	}
	if err := HydrateTokenGroupBindings(DB, []*Token{token}); err != nil {
		t.Fatalf("令牌历史非法值应被忽略: %v", err)
	}

	if err := DB.Migrator().DropTable(&GroupAlias{}); err != nil {
		t.Fatalf("删除分组别名表失败: %v", err)
	}
	if err := DB.Exec("CREATE TABLE group_aliases (id integer primary key)").Error; err != nil {
		t.Fatalf("创建畸形分组别名表失败: %v", err)
	}
	channel.Group = "hydrate-missing-group"
	token.Group = "hydrate-missing-group"
	for entity, err := range map[string]error{
		"渠道": HydrateChannelGroupBindings(DB, []*Channel{channel}),
		"令牌": HydrateTokenGroupBindings(DB, []*Token{token}),
	} {
		if err == nil {
			t.Fatalf("%s Hydrate 不应吞掉真实数据库错误", entity)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("%s 数据库错误被错误分类为历史缺失值: %v", entity, err)
		}
		if !strings.Contains(strings.ToLower(err.Error()), "alias") {
			t.Fatalf("%s 数据库错误链未保留底层列信息: %v", entity, err)
		}
	}
}

func TestBackfillBindingInsertIgnoresConcurrentDuplicates(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{Id: 8101, GroupIds: []int{vipGroup.Id, defaultGroup.Id}}
	token := &Token{
		Id:           8101,
		GroupMode:    TokenGroupModeExplicit,
		GroupIds:     []int{vipGroup.Id, defaultGroup.Id},
		GroupDetails: []GroupReference{newGroupReference(vipGroup), newGroupReference(defaultGroup)},
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := insertChannelGroupBindingsForBackfill(DB, channel); err != nil {
			t.Fatalf("第 %d 次渠道并发式插入失败: %v", attempt, err)
		}
		if err := insertTokenGroupBindingsForBackfill(DB, token); err != nil {
			t.Fatalf("第 %d 次令牌并发式插入失败: %v", attempt, err)
		}
	}
	var channelCount, tokenCount int64
	if err := DB.Model(&ChannelGroupBinding{}).Where("channel_id = ?", channel.Id).Count(&channelCount).Error; err != nil {
		t.Fatalf("统计渠道关联失败: %v", err)
	}
	if err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", token.Id).Count(&tokenCount).Error; err != nil {
		t.Fatalf("统计令牌关联失败: %v", err)
	}
	if channelCount != 2 || tokenCount != 2 {
		t.Fatalf("迁移冲突插入产生重复或丢失: channel=%d token=%d", channelCount, tokenCount)
	}
}

func TestBackfillGroupBindingsSkipsInvalidLegacyRowsAndContinues(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	invalidTokenGroup := "1' AND SUBSTRING((SELECT username FROM users WHERE role=1 LIMIT 1)"
	if _, err := NormalizeGroupCode(invalidTokenGroup); err == nil {
		t.Fatal("生产故障形态的超长分组应被识别为非法")
	}

	validChannel := &Channel{
		Name:   "legacy-valid-channel",
		Key:    "legacy-valid-channel-key",
		Models: "gpt-test",
		Group:  "vip,default",
		Status: common.ChannelStatusEnabled,
	}
	invalidChannel := &Channel{
		Name:   "legacy-invalid-channel",
		Key:    "legacy-invalid-channel-key",
		Models: "gpt-test",
		Group:  "null",
		Status: common.ChannelStatusEnabled,
	}
	validToken := &Token{
		UserId:         1,
		Key:            "legacy-valid-token",
		Name:           "legacy-valid-token",
		Group:          "vip",
		GroupMode:      TokenGroupModeInherit,
		UnlimitedQuota: true,
	}
	invalidToken := &Token{
		UserId:         1,
		Key:            "legacy-invalid-token",
		Name:           "legacy-invalid-token",
		Group:          invalidTokenGroup,
		GroupMode:      TokenGroupModeInherit,
		UnlimitedQuota: true,
	}
	invalidEmptyToken := &Token{
		UserId:         1,
		Key:            "legacy-empty-token",
		Name:           "legacy-empty-token",
		Group:          " , ",
		GroupMode:      TokenGroupModeExplicit,
		UnlimitedQuota: true,
	}
	for _, value := range []interface{}{validChannel, invalidChannel, validToken, invalidToken, invalidEmptyToken} {
		if err := DB.Create(value).Error; err != nil {
			t.Fatalf("写入历史分组数据失败: %v", err)
		}
	}

	if err := MigrateGroupIdentity(); err != nil {
		t.Fatalf("迁移分组身份失败: %v", err)
	}
	lateMissingTokens := []*Token{
		{
			UserId:         1,
			Key:            "legacy-late-missing-token",
			Name:           "legacy-late-missing-token",
			Group:          "late-group",
			GroupMode:      TokenGroupModeInherit,
			UnlimitedQuota: true,
		},
		{
			UserId:         1,
			Key:            "legacy-mixed-missing-token",
			Name:           "legacy-mixed-missing-token",
			Group:          "vip,late-group",
			GroupMode:      TokenGroupModeInherit,
			UnlimitedQuota: true,
		},
	}
	for _, token := range lateMissingTokens {
		// 模拟身份迁移扫描完成后，旧实例并发写入尚不存在的合法分组。
		if err := DB.Create(token).Error; err != nil {
			t.Fatalf("写入迁移窗口内的历史令牌失败: %v", err)
		}
	}
	if err := BackfillGroupBindings(); err != nil {
		t.Fatalf("历史非法分组不应阻塞关联回填: %v", err)
	}

	channelIDs, err := loadChannelBindingIDs(DB, validChannel.Id)
	if err != nil {
		t.Fatalf("读取合法渠道关联失败: %v", err)
	}
	if len(channelIDs) != 2 || channelIDs[0] != vipGroup.Id || channelIDs[1] != defaultGroup.Id {
		t.Fatalf("合法渠道关联回填错误: %#v", channelIDs)
	}
	tokenIDs, err := loadTokenBindingIDs(DB, validToken.Id)
	if err != nil {
		t.Fatalf("读取合法令牌关联失败: %v", err)
	}
	if len(tokenIDs) != 1 || tokenIDs[0] != vipGroup.Id {
		t.Fatalf("合法令牌关联回填错误: %#v", tokenIDs)
	}

	var storedValidToken Token
	if err := DB.First(&storedValidToken, validToken.Id).Error; err != nil {
		t.Fatalf("读取合法令牌失败: %v", err)
	}
	if storedValidToken.GroupMode != TokenGroupModeExplicit {
		t.Fatalf("合法令牌模式未回填为 explicit: %q", storedValidToken.GroupMode)
	}
	var storedInvalidChannel Channel
	if err := DB.First(&storedInvalidChannel, invalidChannel.Id).Error; err != nil {
		t.Fatalf("读取非法渠道失败: %v", err)
	}
	if storedInvalidChannel.Group != "null" {
		t.Fatalf("非法渠道旧分组被改写: %q", storedInvalidChannel.Group)
	}
	var storedInvalidToken Token
	if err := DB.First(&storedInvalidToken, invalidToken.Id).Error; err != nil {
		t.Fatalf("读取非法令牌失败: %v", err)
	}
	if storedInvalidToken.Group != invalidTokenGroup || storedInvalidToken.GroupMode != TokenGroupModeInherit {
		t.Fatalf("非法令牌旧值被改写: group=%q mode=%q", storedInvalidToken.Group, storedInvalidToken.GroupMode)
	}
	var storedInvalidEmptyToken Token
	if err := DB.First(&storedInvalidEmptyToken, invalidEmptyToken.Id).Error; err != nil {
		t.Fatalf("读取空分组令牌失败: %v", err)
	}
	if storedInvalidEmptyToken.Group != " , " || storedInvalidEmptyToken.GroupMode != TokenGroupModeExplicit {
		t.Fatalf("空分组令牌旧值被改写: group=%q mode=%q", storedInvalidEmptyToken.Group, storedInvalidEmptyToken.GroupMode)
	}
	invalidChannelIDs, err := loadChannelBindingIDs(DB, invalidChannel.Id)
	if err != nil {
		t.Fatalf("读取非法渠道关联失败: %v", err)
	}
	invalidTokenIDs, err := loadTokenBindingIDs(DB, invalidToken.Id)
	if err != nil {
		t.Fatalf("读取非法令牌关联失败: %v", err)
	}
	if len(invalidChannelIDs) != 0 || len(invalidTokenIDs) != 0 {
		t.Fatalf("非法历史值不应产生关联: channel=%#v token=%#v", invalidChannelIDs, invalidTokenIDs)
	}
	invalidEmptyTokenIDs, err := loadTokenBindingIDs(DB, invalidEmptyToken.Id)
	if err != nil {
		t.Fatalf("读取空分组令牌关联失败: %v", err)
	}
	if len(invalidEmptyTokenIDs) != 0 {
		t.Fatalf("空分组令牌不应产生关联: %#v", invalidEmptyTokenIDs)
	}
	for _, token := range lateMissingTokens {
		var stored Token
		if err := DB.First(&stored, token.Id).Error; err != nil {
			t.Fatalf("读取迁移窗口内的历史令牌失败: %v", err)
		}
		if stored.Group != token.Group || stored.GroupMode != TokenGroupModeInherit {
			t.Fatalf("缺失分组令牌旧值被改写: group=%q mode=%q", stored.Group, stored.GroupMode)
		}
		ids, err := loadTokenBindingIDs(DB, token.Id)
		if err != nil {
			t.Fatalf("读取缺失分组令牌关联失败: %v", err)
		}
		if len(ids) != 0 {
			t.Fatalf("缺失或混合缺失分组不应产生部分关联: %#v", ids)
		}
	}
	var invalidGroupCount int64
	if err := DB.Model(&Group{}).Where("code IN ?", []string{"null", invalidTokenGroup, "late-group"}).Count(&invalidGroupCount).Error; err != nil {
		t.Fatalf("检查非法分组实体失败: %v", err)
	}
	if invalidGroupCount != 0 {
		t.Fatalf("非法历史值不应被创建为分组实体: %d", invalidGroupCount)
	}

	for restart := 1; restart <= 2; restart++ {
		if err := MigrateGroupIdentity(); err != nil {
			t.Fatalf("第 %d 次完整重启的分组身份迁移失败: %v", restart, err)
		}
		if err := BackfillGroupBindings(); err != nil {
			t.Fatalf("第 %d 次完整重启的关联回填失败: %v", restart, err)
		}
	}
	var channelBindingCount, tokenBindingCount int64
	if err := DB.Model(&ChannelGroupBinding{}).Count(&channelBindingCount).Error; err != nil {
		t.Fatalf("统计渠道关联失败: %v", err)
	}
	if err := DB.Model(&TokenGroupBinding{}).Count(&tokenBindingCount).Error; err != nil {
		t.Fatalf("统计令牌关联失败: %v", err)
	}
	if channelBindingCount != 2 || tokenBindingCount != 4 {
		t.Fatalf("完整重启后的关联数量错误或存在重复: channel=%d token=%d", channelBindingCount, tokenBindingCount)
	}
}

func TestBackfillGroupBindingsReturnsDatabaseErrors(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	validChannel := &Channel{
		Name:   "rollback-channel",
		Key:    "rollback-channel-key",
		Models: "gpt-test",
		Group:  vipGroup.Code,
		Status: common.ChannelStatusEnabled,
	}
	unknownToken := &Token{
		UserId:         1,
		Key:            "database-error-token",
		Name:           "database-error-token",
		Group:          "missing-group",
		GroupMode:      TokenGroupModeInherit,
		UnlimitedQuota: true,
	}
	if err := DB.Create(validChannel).Error; err != nil {
		t.Fatalf("写入合法渠道失败: %v", err)
	}
	if err := DB.Create(unknownToken).Error; err != nil {
		t.Fatalf("写入未知分组令牌失败: %v", err)
	}
	if err := DB.Migrator().DropTable(&GroupAlias{}); err != nil {
		t.Fatalf("删除分组别名表失败: %v", err)
	}
	if err := DB.Exec("CREATE TABLE group_aliases (id integer primary key)").Error; err != nil {
		t.Fatalf("创建畸形分组别名表失败: %v", err)
	}

	err := BackfillGroupBindings()
	if err == nil {
		t.Fatal("真实数据库错误不应被当作历史脏值跳过")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("真实数据库错误被错误改写为 record not found: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "alias") {
		t.Fatalf("数据库错误链未保留底层列信息: %v", err)
	}
	channelIDs, loadErr := loadChannelBindingIDs(DB, validChannel.Id)
	if loadErr != nil {
		t.Fatalf("读取回滚后的渠道关联失败: %v", loadErr)
	}
	if len(channelIDs) != 0 {
		t.Fatalf("数据库错误时关联回填事务未回滚: %#v", channelIDs)
	}
}

func TestPrepareGroupBindingsStillRejectsInvalidLegacyValues(t *testing.T) {
	setupGroupBindingsTest(t)
	invalidTokenGroup := "1' AND SUBSTRING((SELECT username FROM users WHERE role=1 LIMIT 1)"
	tests := []struct {
		name    string
		prepare func() error
	}{
		{
			name: "channel",
			prepare: func() error {
				return PrepareChannelGroupBindings(DB, &Channel{Group: "null"})
			},
		},
		{
			name: "token",
			prepare: func() error {
				return PrepareTokenGroupBindings(DB, &Token{Group: invalidTokenGroup, GroupMode: TokenGroupModeExplicit})
			},
		},
		{
			name: "missing_token_group",
			prepare: func() error {
				return PrepareTokenGroupBindings(DB, &Token{Group: "missing-group", GroupMode: TokenGroupModeExplicit})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.prepare()
			if err == nil {
				t.Fatal("正常写入不应接受非法历史分组值")
			}
			if !isInvalidLegacyGroupCodeError(err) {
				t.Fatalf("非法分组错误分类不正确: %v", err)
			}
		})
	}
}

func TestMigrateTokenGroupToAutoClearsAllGroupsAndRepairsDirtyMirrors(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	otherGroup := &Group{Code: "other-auto", Name: "其他自动分组", Ratio: 1, Status: GroupStatusActive}
	if err := DB.Create(otherGroup).Error; err != nil {
		t.Fatalf("创建其他分组失败: %v", err)
	}
	token := &Token{
		UserId:           301,
		Key:              "token-migrate-to-auto",
		Name:             "token-migrate-to-auto",
		GroupMode:        TokenGroupModeExplicit,
		GroupIds:         []int{vipGroup.Id, otherGroup.Id},
		GroupRatioLimits: `{"vip":2.5,"other-auto":1.5}`,
		UnlimitedQuota:   true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建多分组令牌失败: %v", err)
	}
	if err := DB.Model(&Token{}).Where("id = ?", token.Id).Updates(map[string]interface{}{
		"group":              vipGroup.Code,
		"group_ratio_limits": "{broken-json",
	}).Error; err != nil {
		t.Fatalf("构造不一致令牌镜像失败: %v", err)
	}
	deletedToken := &Token{
		UserId:         302,
		Key:            "deleted-token-migrate-to-auto",
		Name:           "deleted-token-migrate-to-auto",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := deletedToken.Insert(); err != nil {
		t.Fatalf("创建待清理软删除令牌失败: %v", err)
	}
	if err := deletedToken.Delete(); err != nil {
		t.Fatalf("软删除令牌失败: %v", err)
	}
	unrelated := &Token{
		UserId:         303,
		Key:            "unrelated-token-migrate-to-auto",
		Name:           "unrelated-token-migrate-to-auto",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{defaultGroup.Id},
		UnlimitedQuota: true,
	}
	if err := unrelated.Insert(); err != nil {
		t.Fatalf("创建无关令牌失败: %v", err)
	}

	preview, err := PreviewTokenGroupMigrationToAuto(vipGroup.Id)
	if err != nil {
		t.Fatalf("预览迁移到 auto 失败: %v", err)
	}
	if preview.TargetGroupMode != TokenGroupModeAuto || preview.TargetGroup.Id != 0 || preview.TargetGroup.Code != TokenGroupModeAuto {
		t.Fatalf("auto 目标信息错误: %#v", preview)
	}
	if preview.MigratedTokens != 1 || preview.MultiGroupTokens != 1 || preview.CleanedDeletedTokens != 1 {
		t.Fatalf("auto 迁移预览统计错误: %#v", preview)
	}
	var before Token
	if err := DB.First(&before, token.Id).Error; err != nil {
		t.Fatalf("读取预览后的令牌失败: %v", err)
	}
	if before.Group != vipGroup.Code || before.GroupRatioLimits != "{broken-json" {
		t.Fatalf("预览不应写入令牌: %#v", before)
	}

	result, err := MigrateTokenGroupToAuto(vipGroup.Id)
	if err != nil {
		t.Fatalf("迁移到 auto 失败: %v", err)
	}
	if result.MigratedTokens != 1 || result.MultiGroupTokens != 1 || result.CleanedDeletedTokens != 1 {
		t.Fatalf("auto 迁移结果统计错误: %#v", result)
	}
	var stored Token
	if err := DB.First(&stored, token.Id).Error; err != nil {
		t.Fatalf("读取迁移后的令牌失败: %v", err)
	}
	if stored.Group != TokenGroupModeAuto || stored.GroupMode != TokenGroupModeAuto || stored.GroupRatioLimits != "" {
		t.Fatalf("令牌未只保留 auto: %#v", stored)
	}
	var bindingCount int64
	if err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", token.Id).Count(&bindingCount).Error; err != nil {
		t.Fatalf("统计迁移后绑定失败: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("迁移到 auto 后仍有 %d 条显式分组绑定", bindingCount)
	}
	var storedDeleted Token
	if err := DB.Unscoped().First(&storedDeleted, deletedToken.Id).Error; err != nil {
		t.Fatalf("读取清理后的软删除令牌失败: %v", err)
	}
	if storedDeleted.Group != "" || storedDeleted.GroupMode != TokenGroupModeInherit || storedDeleted.GroupRatioLimits != "" {
		t.Fatalf("软删除令牌未恢复 inherit: %#v", storedDeleted)
	}
	var storedUnrelated Token
	if err := DB.First(&storedUnrelated, unrelated.Id).Error; err != nil {
		t.Fatalf("读取无关令牌失败: %v", err)
	}
	if storedUnrelated.Group != defaultGroup.Code || storedUnrelated.GroupMode != TokenGroupModeExplicit {
		t.Fatalf("无关令牌被错误修改: %#v", storedUnrelated)
	}

	repeated, err := MigrateTokenGroupToAuto(vipGroup.Id)
	if err != nil {
		t.Fatalf("重复迁移到 auto 不应失败: %v", err)
	}
	if repeated.MigratedTokens != 0 || repeated.CleanedDeletedTokens != 0 {
		t.Fatalf("重复迁移到 auto 应保持幂等: %#v", repeated)
	}
}

func TestSaveGroupConfigDeletesGroupAfterMigratingAliasTokenToAuto(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	alias := "legacy-delete-to-auto"
	if err := DB.Create(&GroupAlias{Alias: alias, GroupId: vipGroup.Id}).Error; err != nil {
		t.Fatalf("创建分组别名失败: %v", err)
	}
	token := &Token{
		UserId:           304,
		Key:              "legacy-delete-to-auto-token",
		Name:             "legacy-delete-to-auto-token",
		Group:            "default," + alias,
		GroupMode:        TokenGroupModeExplicit,
		GroupRatioLimits: "{broken-json",
		UnlimitedQuota:   true,
	}
	if err := DB.Create(token).Error; err != nil {
		t.Fatalf("创建别名历史令牌失败: %v", err)
	}

	if err := SaveGroupConfig(nil, []int{vipGroup.Id}); err != nil {
		t.Fatalf("删除仅被令牌引用的分组失败: %v", err)
	}
	var stored Token
	if err := DB.First(&stored, token.Id).Error; err != nil {
		t.Fatalf("读取自动迁移后的令牌失败: %v", err)
	}
	if stored.Group != TokenGroupModeAuto || stored.GroupMode != TokenGroupModeAuto || stored.GroupRatioLimits != "" {
		t.Fatalf("删除分组时令牌未切换到 auto: %#v", stored)
	}
	var groupCount, aliasCount, bindingCount int64
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&groupCount).Error; err != nil {
		t.Fatalf("统计删除后的分组失败: %v", err)
	}
	if err := DB.Model(&GroupAlias{}).Where("group_id = ?", vipGroup.Id).Count(&aliasCount).Error; err != nil {
		t.Fatalf("统计删除后的别名失败: %v", err)
	}
	if err := DB.Model(&TokenGroupBinding{}).Where("token_id = ?", token.Id).Count(&bindingCount).Error; err != nil {
		t.Fatalf("统计删除后的令牌绑定失败: %v", err)
	}
	if groupCount != 0 || aliasCount != 0 || bindingCount != 0 {
		t.Fatalf("删除后仍有残留: group=%d alias=%d binding=%d", groupCount, aliasCount, bindingCount)
	}
}

func TestSaveGroupConfigRollsBackAutoMigrationWhenNonTokenReferenceBlocksDelete(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:         305,
		Key:            "rollback-delete-to-auto-token",
		Name:           "rollback-delete-to-auto-token",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建待回滚令牌失败: %v", err)
	}
	if err := DB.Create(&ChannelGroupBinding{ChannelId: 9901, GroupId: vipGroup.Id, Position: 0}).Error; err != nil {
		t.Fatalf("创建非令牌阻塞引用失败: %v", err)
	}

	err := SaveGroupConfig(nil, []int{vipGroup.Id})
	if err == nil || !strings.Contains(err.Error(), "非令牌业务数据引用") {
		t.Fatalf("非令牌引用应阻止删除，实际错误: %v", err)
	}
	var stored Token
	if err := DB.First(&stored, token.Id).Error; err != nil {
		t.Fatalf("读取回滚后的令牌失败: %v", err)
	}
	if stored.Group != vipGroup.Code || stored.GroupMode != TokenGroupModeExplicit {
		t.Fatalf("删除失败后令牌迁移未回滚: %#v", stored)
	}
	var groupCount, bindingCount int64
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&groupCount).Error; err != nil {
		t.Fatalf("统计回滚后的分组失败: %v", err)
	}
	if err := DB.Model(&TokenGroupBinding{}).
		Where("token_id = ? AND group_id = ?", token.Id, vipGroup.Id).
		Count(&bindingCount).Error; err != nil {
		t.Fatalf("统计回滚后的令牌绑定失败: %v", err)
	}
	if groupCount != 1 || bindingCount != 1 {
		t.Fatalf("删除失败未完整回滚: group=%d binding=%d", groupCount, bindingCount)
	}
}

func TestSaveGroupConfigReportsTokenCacheInvalidationFailure(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	token := &Token{
		UserId:         306,
		Key:            "cache-warning-delete-to-auto-token",
		Name:           "cache-warning-delete-to-auto-token",
		GroupMode:      TokenGroupModeExplicit,
		GroupIds:       []int{vipGroup.Id},
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("创建缓存告警测试令牌失败: %v", err)
	}
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
	})

	result, err := SaveGroupConfigWithResult(nil, []int{vipGroup.Id})
	if err != nil {
		t.Fatalf("缓存清理失败不应回滚已提交的分组保存: %v", err)
	}
	if result.MigratedTokens != 1 || result.CacheInvalidated != 0 || result.CacheInvalidationFailed != 1 {
		t.Fatalf("缓存清理告警统计错误: %#v", result)
	}
	if !strings.Contains(result.Warning, "缓存清理失败") {
		t.Fatalf("缓存清理失败未返回可展示警告: %#v", result)
	}
	var stored Token
	if err := DB.First(&stored, token.Id).Error; err != nil {
		t.Fatalf("读取缓存失败后的令牌失败: %v", err)
	}
	if stored.Group != TokenGroupModeAuto || stored.GroupMode != TokenGroupModeAuto {
		t.Fatalf("缓存清理失败不应回滚数据库迁移: %#v", stored)
	}
}

func TestLockTokenGroupBindingGroupsRejectsDeletedTarget(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	selection := &Token{
		GroupMode: TokenGroupModeExplicit,
		GroupIds:  []int{vipGroup.Id},
	}
	if err := PrepareTokenGroupBindings(DB, selection); err != nil {
		t.Fatalf("准备令牌分组失败: %v", err)
	}
	if err := DB.Delete(vipGroup).Error; err != nil {
		t.Fatalf("构造并发删除后的分组状态失败: %v", err)
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		return lockTokenGroupBindingGroups(tx, selection)
	})
	if err == nil || !strings.Contains(err.Error(), "写入期间已被删除") {
		t.Fatalf("写入令牌绑定前必须重新确认并锁定分组，实际错误: %v", err)
	}
}

func TestLockChannelGroupBindingGroupsRejectsDeletedTarget(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	selection := &Channel{GroupIds: []int{vipGroup.Id}}
	if err := PrepareChannelGroupBindings(DB, selection); err != nil {
		t.Fatalf("准备渠道分组失败: %v", err)
	}
	if err := DB.Delete(vipGroup).Error; err != nil {
		t.Fatalf("构造并发删除后的分组状态失败: %v", err)
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		return lockChannelGroupBindingGroups(tx, selection)
	})
	if err == nil || !strings.Contains(err.Error(), "写入期间已被删除") {
		t.Fatalf("写入渠道绑定前必须重新确认并锁定分组，实际错误: %v", err)
	}
}

func TestWriteChannelGroupBindingsRollsBackEntityWhenTargetDeleted(t *testing.T) {
	_, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:     "entity-first-channel",
		Key:      "entity-first-key",
		Models:   "gpt-test",
		GroupIds: []int{vipGroup.Id},
		Status:   common.ChannelStatusEnabled,
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := PrepareChannelGroupBindings(tx, channel); err != nil {
			return err
		}
		if err := tx.Create(channel).Error; err != nil {
			return err
		}
		if err := tx.Delete(&Group{}, vipGroup.Id).Error; err != nil {
			return err
		}
		if err := writeChannelGroupBindings(tx, channel); err != nil {
			return err
		}
		return channel.AddAbilities(tx)
	})
	if err == nil || !strings.Contains(err.Error(), "写入期间已被删除") {
		t.Fatalf("实体先写时必须在绑定前复核目标分组，实际错误: %v", err)
	}
	var groupCount, channelCount, bindingCount, abilityCount int64
	if err := DB.Model(&Group{}).Where("id = ?", vipGroup.Id).Count(&groupCount).Error; err != nil {
		t.Fatalf("统计回滚后的分组失败: %v", err)
	}
	if err := DB.Model(&Channel{}).Where("name = ?", channel.Name).Count(&channelCount).Error; err != nil {
		t.Fatalf("统计回滚后的渠道失败: %v", err)
	}
	if err := DB.Model(&ChannelGroupBinding{}).Count(&bindingCount).Error; err != nil {
		t.Fatalf("统计回滚后的渠道绑定失败: %v", err)
	}
	if err := DB.Model(&Ability{}).Count(&abilityCount).Error; err != nil {
		t.Fatalf("统计回滚后的能力失败: %v", err)
	}
	if groupCount != 1 || channelCount != 0 || bindingCount != 0 || abilityCount != 0 {
		t.Fatalf(
			"目标分组失效后的事务未完整回滚: group=%d channel=%d binding=%d ability=%d",
			groupCount,
			channelCount,
			bindingCount,
			abilityCount,
		)
	}
}

func TestChannelUpdateRejectsDeletedTargetWithoutChangingBindingsOrAbilities(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:     "channel-update-lock",
		Key:      "channel-update-lock-key",
		Models:   "gpt-old",
		GroupIds: []int{vipGroup.Id},
		Status:   common.ChannelStatusEnabled,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("创建待更新渠道失败: %v", err)
	}
	if err := DB.Delete(defaultGroup).Error; err != nil {
		t.Fatalf("删除待选目标分组失败: %v", err)
	}
	update := &Channel{
		Id:       channel.Id,
		Name:     "channel-update-changed",
		Models:   "gpt-new",
		GroupIds: []int{defaultGroup.Id},
	}
	err := update.Update()
	if err == nil {
		t.Fatal("更新到已删除分组必须失败")
	}
	var stored Channel
	if err := DB.First(&stored, channel.Id).Error; err != nil {
		t.Fatalf("读取拒绝更新后的渠道失败: %v", err)
	}
	if stored.Name != channel.Name || stored.Models != channel.Models || stored.Group != vipGroup.Code {
		t.Fatalf("拒绝更新后渠道实体被修改: %#v", stored)
	}
	var bindings []ChannelGroupBinding
	if err := DB.Where("channel_id = ?", channel.Id).Find(&bindings).Error; err != nil {
		t.Fatalf("读取拒绝更新后的渠道绑定失败: %v", err)
	}
	var abilities []Ability
	if err := DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error; err != nil {
		t.Fatalf("读取拒绝更新后的能力失败: %v", err)
	}
	if len(bindings) != 1 || bindings[0].GroupId != vipGroup.Id {
		t.Fatalf("拒绝更新后渠道绑定被修改: %#v", bindings)
	}
	if len(abilities) != 1 || abilities[0].GroupId != vipGroup.Id || abilities[0].Model != channel.Models {
		t.Fatalf("拒绝更新后能力被修改: %#v", abilities)
	}
}

func TestBatchInsertChannelsRejectsDeletedTargetAtomically(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	if err := DB.Delete(vipGroup).Error; err != nil {
		t.Fatalf("删除批量导入目标分组失败: %v", err)
	}
	channels := []Channel{
		{Name: "batch-valid", Key: "batch-valid-key", Models: "gpt-test", GroupIds: []int{defaultGroup.Id}, Status: common.ChannelStatusEnabled},
		{Name: "batch-invalid", Key: "batch-invalid-key", Models: "gpt-test", GroupIds: []int{vipGroup.Id}, Status: common.ChannelStatusEnabled},
	}
	if err := BatchInsertChannels(channels); err == nil {
		t.Fatal("批量导入包含已删除目标分组时必须失败")
	}
	var channelCount, bindingCount, abilityCount int64
	if err := DB.Model(&Channel{}).Where("name IN ?", []string{"batch-valid", "batch-invalid"}).Count(&channelCount).Error; err != nil {
		t.Fatalf("统计批量回滚后的渠道失败: %v", err)
	}
	if err := DB.Model(&ChannelGroupBinding{}).Count(&bindingCount).Error; err != nil {
		t.Fatalf("统计批量回滚后的渠道绑定失败: %v", err)
	}
	if err := DB.Model(&Ability{}).Count(&abilityCount).Error; err != nil {
		t.Fatalf("统计批量回滚后的能力失败: %v", err)
	}
	if channelCount != 0 || bindingCount != 0 || abilityCount != 0 {
		t.Fatalf("批量导入失败未完整回滚: channel=%d binding=%d ability=%d", channelCount, bindingCount, abilityCount)
	}
}

func TestEditChannelByTagRejectsDeletedTargetWithoutChangingAbilities(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	tag := "group-lock-tag"
	channel := &Channel{
		Name:     "tag-edit-channel",
		Key:      "tag-edit-channel-key",
		Models:   "gpt-old",
		GroupIds: []int{vipGroup.Id},
		Status:   common.ChannelStatusEnabled,
		Tag:      &tag,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("创建标签渠道失败: %v", err)
	}
	if err := DB.Delete(defaultGroup).Error; err != nil {
		t.Fatalf("删除标签编辑目标分组失败: %v", err)
	}
	newModels := "gpt-new"
	targetGroupIDs := []int{defaultGroup.Id}
	err := EditChannelByTag(
		tag,
		nil,
		nil,
		&newModels,
		nil,
		&targetGroupIDs,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("标签渠道编辑到已删除分组必须失败")
	}
	var stored Channel
	if err := DB.First(&stored, channel.Id).Error; err != nil {
		t.Fatalf("读取拒绝标签编辑后的渠道失败: %v", err)
	}
	if stored.Models != channel.Models || stored.Group != vipGroup.Code {
		t.Fatalf("拒绝标签编辑后渠道实体被修改: %#v", stored)
	}
	var bindings []ChannelGroupBinding
	if err := DB.Where("channel_id = ?", channel.Id).Find(&bindings).Error; err != nil {
		t.Fatalf("读取拒绝标签编辑后的渠道绑定失败: %v", err)
	}
	var abilities []Ability
	if err := DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error; err != nil {
		t.Fatalf("读取拒绝标签编辑后的能力失败: %v", err)
	}
	if len(bindings) != 1 || bindings[0].GroupId != vipGroup.Id {
		t.Fatalf("拒绝标签编辑后渠道绑定被修改: %#v", bindings)
	}
	if len(abilities) != 1 || abilities[0].GroupId != vipGroup.Id || abilities[0].Model != channel.Models {
		t.Fatalf("拒绝标签编辑后能力被修改: %#v", abilities)
	}
}
