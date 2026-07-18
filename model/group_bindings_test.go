package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
