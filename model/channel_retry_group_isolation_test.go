package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestChannelRetryGroupIsolation(t *testing.T) {
	t.Run("memory cache", func(t *testing.T) {
		oldMemoryCacheEnabled := common.MemoryCacheEnabled
		common.MemoryCacheEnabled = true

		priority := int64(10)
		weight := uint(100)
		shared := &Channel{Id: 9101, Name: "shared", Priority: &priority, Weight: &weight}
		regularOnly := &Channel{Id: 9102, Name: "regular-only", Priority: &priority, Weight: &weight}

		channelSyncLock.Lock()
		oldGroups := group2model2channels
		oldChannels := channelsIDM
		group2model2channels = map[string]map[string][]int{
			"special": {"gpt-test": {shared.Id}},
			"regular": {"gpt-test": {shared.Id, regularOnly.Id}},
		}
		channelsIDM = map[int]*Channel{shared.Id: shared, regularOnly.Id: regularOnly}
		channelSyncLock.Unlock()

		t.Cleanup(func() {
			channelSyncLock.Lock()
			group2model2channels = oldGroups
			channelsIDM = oldChannels
			channelSyncLock.Unlock()
			common.MemoryCacheEnabled = oldMemoryCacheEnabled
			resetChannelConcurrencyForTest()
		})

		excluded := map[int]struct{}{shared.Id: {}}
		channel, err := GetRandomSatisfiedChannelWithExclusions("special", "gpt-test", 1, excluded)
		if err != nil {
			t.Fatalf("特价分组重试失败: %v", err)
		}
		if channel != nil {
			t.Fatalf("特价分组候选耗尽后错误选择渠道 #%d", channel.Id)
		}

		channel, err = GetRandomSatisfiedChannelWithExclusions("regular", "gpt-test", 1, excluded)
		if err != nil {
			t.Fatalf("正价分组选渠失败: %v", err)
		}
		if channel == nil || channel.Id != regularOnly.Id {
			t.Fatalf("正价分组应能选择仅属于正价的渠道，实际 %#v", channel)
		}
	})

	t.Run("database", func(t *testing.T) {
		db := openGroupIdentityTestDB(t)
		if err := db.AutoMigrate(&Channel{}, &Ability{}); err != nil {
			t.Fatalf("迁移选渠测试表失败: %v", err)
		}
		oldMemoryCacheEnabled := common.MemoryCacheEnabled
		common.MemoryCacheEnabled = false
		t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCacheEnabled })

		priority := int64(10)
		weight := uint(100)
		shared := &Channel{Id: 9201, Name: "shared", Key: "shared-key", Priority: &priority, Weight: &weight}
		regularOnly := &Channel{Id: 9202, Name: "regular-only", Key: "regular-key", Priority: &priority, Weight: &weight}
		if err := db.Create([]*Channel{shared, regularOnly}).Error; err != nil {
			t.Fatalf("创建测试渠道失败: %v", err)
		}
		abilities := []Ability{
			{Group: "special", Model: "gpt-test", ChannelId: shared.Id, Enabled: true, Priority: &priority, Weight: weight},
			{Group: "regular", Model: "gpt-test", ChannelId: shared.Id, Enabled: true, Priority: &priority, Weight: weight},
			{Group: "regular", Model: "gpt-test", ChannelId: regularOnly.Id, Enabled: true, Priority: &priority, Weight: weight},
		}
		if err := db.Create(&abilities).Error; err != nil {
			t.Fatalf("创建测试能力失败: %v", err)
		}
		excluded := map[int]struct{}{shared.Id: {}}
		channel, err := GetChannelWithExclusions("special", "gpt-test", 1, excluded)
		if err != nil {
			t.Fatalf("特价分组数据库重试失败: %v", err)
		}
		if channel != nil {
			t.Fatalf("特价分组候选耗尽后错误选择数据库渠道 #%d", channel.Id)
		}

		channel, err = GetChannelWithExclusions("regular", "gpt-test", 1, excluded)
		if err != nil {
			t.Fatalf("正价分组数据库选渠失败: %v", err)
		}
		if channel == nil || channel.Id != regularOnly.Id {
			t.Fatalf("正价分组应能选择仅属于正价的数据库渠道，实际 %#v", channel)
		}
	})
}
