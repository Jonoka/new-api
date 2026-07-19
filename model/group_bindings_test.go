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
