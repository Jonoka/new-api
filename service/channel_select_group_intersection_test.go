package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectGroupTestCache(t *testing.T) map[string]*model.Channel {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	oldDB := model.DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = true

	priority := int64(10)
	fallbackPriority := int64(5)
	weight := uint(100)
	channels := map[string]*model.Channel{
		"shared":           {Id: 9301, Name: "shared", Key: "shared", Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "hack,special,regular", Priority: &priority, Weight: &weight},
		"hack-only":        {Id: 9302, Name: "hack-only", Key: "hack", Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "hack", Priority: &priority, Weight: &weight},
		"special-only":     {Id: 9303, Name: "special-only", Key: "special", Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "special", Priority: &priority, Weight: &weight},
		"regular-only":     {Id: 9304, Name: "regular-only", Key: "regular", Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "regular", Priority: &priority, Weight: &weight},
		"hack-fallback":    {Id: 9305, Name: "hack-fallback", Key: "hack-low", Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "hack", Priority: &fallbackPriority, Weight: &weight},
		"special-fallback": {Id: 9306, Name: "special-fallback", Key: "special-low", Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "special", Priority: &fallbackPriority, Weight: &weight},
		"regular-fallback": {Id: 9307, Name: "regular-fallback", Key: "regular-low", Status: common.ChannelStatusEnabled, Models: "gpt-test", Group: "regular", Priority: &fallbackPriority, Weight: &weight},
	}
	for _, channel := range channels {
		require.NoError(t, db.Create(channel).Error)
	}
	for index, group := range []string{"hack", "special", "regular"} {
		require.NoError(t, db.Create(&model.Ability{
			Group: group, Model: "gpt-test", ChannelId: 9900 + index, Enabled: true, Priority: &priority, Weight: weight,
		}).Error)
	}
	model.InitChannelCache()

	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		if oldMemoryCacheEnabled && oldDB != nil {
			model.InitChannelCache()
		}
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return channels
}

func TestMultiGroupRetryUsesCurrentGroupMembership(t *testing.T) {
	channels := setupChannelSelectGroupTestCache(t)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		groupIndex     int
		wantGroup      string
		wantChannelKey string
	}{
		{name: "Hack 阶段只使用 Hack 渠道", groupIndex: 0, wantGroup: "hack", wantChannelKey: "hack-only"},
		{name: "特价阶段只使用特价渠道", groupIndex: 1, wantGroup: "special", wantChannelKey: "special-only"},
		{name: "正价阶段可使用仅配置正价的渠道", groupIndex: 2, wantGroup: "regular", wantChannelKey: "regular-only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyAutoGroupIndex, tt.groupIndex)
			param := &RetryParam{
				Ctx:                ctx,
				TokenGroup:         "hack,special,regular",
				ModelName:          "gpt-test",
				Retry:              common.GetPointer(0),
				ExcludedChannelIDs: map[int]struct{}{channels["shared"].Id: {}},
			}
			channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.Equal(t, tt.wantGroup, selectedGroup)
			require.NotNil(t, channel)
			require.Equal(t, channels[tt.wantChannelKey].Id, channel.Id)
		})
	}
}

func TestCrossGroupRetryChangesSelectedGroupBeforeUsingNextGroupChannel(t *testing.T) {
	channels := setupChannelSelectGroupTestCache(t)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyAutoGroupIndex, 1)
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:                ctx,
		TokenGroup:         "hack,special,regular",
		ModelName:          "gpt-test",
		Retry:              common.GetPointer(0),
		ExcludedChannelIDs: map[int]struct{}{channels["shared"].Id: {}, channels["special-only"].Id: {}, channels["special-fallback"].Id: {}},
	})
	require.NoError(t, err)
	require.Equal(t, "regular", selectedGroup)
	require.NotNil(t, channel)
	require.Equal(t, channels["regular-only"].Id, channel.Id, "只有选中分组已切换为正价后才能使用正价渠道")
}

func TestMultiGroupRetryExhaustsPrioritiesBeforeAdvancing(t *testing.T) {
	channels := setupChannelSelectGroupTestCache(t)
	oldRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = oldRetryTimes })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{
		Ctx:                ctx,
		TokenGroup:         "hack,special,regular",
		ModelName:          "gpt-test",
		Retry:              common.GetPointer(0),
		ExcludedChannelIDs: map[int]struct{}{channels["shared"].Id: {}},
	}

	wants := []struct {
		group     string
		channelID int
	}{
		{group: "hack", channelID: channels["hack-only"].Id},
		{group: "hack", channelID: channels["hack-fallback"].Id},
		{group: "special", channelID: channels["special-only"].Id},
	}
	for _, want := range wants {
		channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		require.Equal(t, want.group, selectedGroup)
		require.Equal(t, want.channelID, channel.Id)
		param.ExcludedChannelIDs[channel.Id] = struct{}{}
		param.IncreaseRetry()
	}
}

func TestAutoCrossGroupRetryExhaustsPrioritiesBeforeAdvancing(t *testing.T) {
	channels := setupChannelSelectGroupTestCache(t)
	preserveAutoGroupSettings(t)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"hack":"Hack","special":"特价","regular":"正价"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["hack","special","regular"]`))
	require.NoError(t, setting.UpdateAutoGroupConfigByJsonString(`{"user_selectable":true,"description":"自动选择"}`))
	oldRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = oldRetryTimes })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	param := &RetryParam{
		Ctx:                ctx,
		TokenGroup:         "auto",
		ModelName:          "gpt-test",
		Retry:              common.GetPointer(0),
		ExcludedChannelIDs: map[int]struct{}{channels["shared"].Id: {}},
	}
	wants := []struct {
		group     string
		channelID int
	}{
		{group: "hack", channelID: channels["hack-only"].Id},
		{group: "hack", channelID: channels["hack-fallback"].Id},
		{group: "special", channelID: channels["special-only"].Id},
	}
	for _, want := range wants {
		channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		require.Equal(t, want.group, selectedGroup)
		require.Equal(t, want.channelID, channel.Id)
		param.ExcludedChannelIDs[channel.Id] = struct{}{}
		param.IncreaseRetry()
	}
}

func TestRelayMaxRetriesBudgetsEveryGroupPriority(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Equal(t, 5*(common.RetryTimes+1)-1, RelayMaxRetries(&RetryParam{Ctx: ctx, TokenGroup: "g1,g2,g3,g4,g5"}))
	require.Equal(t, common.RetryTimes, RelayMaxRetries(&RetryParam{Ctx: ctx, TokenGroup: "g1"}))
}
