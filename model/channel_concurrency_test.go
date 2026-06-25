package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestChannelConcurrencyUnlimited(t *testing.T) {
	resetChannelConcurrencyForTest()

	limit := 0
	channel := &Channel{Id: 1, ConcurrencyLimit: &limit}
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("expected unlimited channel to be acquired")
	}
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("expected unlimited channel to allow multiple acquires")
	}
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
}

func TestChannelConcurrencyLimit(t *testing.T) {
	resetChannelConcurrencyForTest()

	limit := 1
	channel := &Channel{Id: 2, ConcurrencyLimit: &limit}
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("expected first acquire to succeed")
	}
	if TryAcquireChannelConcurrency(channel) {
		t.Fatal("expected second acquire to be blocked")
	}
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("expected channel to be unavailable while at limit")
	}

	ReleaseChannelConcurrency(channel.Id)
	if !IsChannelConcurrencyAvailable(channel) {
		t.Fatal("expected channel to be available after release")
	}
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("expected acquire to succeed after release")
	}
}

func TestChannelConcurrencyReleaseDoesNotGoNegative(t *testing.T) {
	resetChannelConcurrencyForTest()

	limit := 1
	channel := &Channel{Id: 3, ConcurrencyLimit: &limit}
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)

	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("expected release without acquire not to poison state")
	}
	ReleaseChannelConcurrency(channel.Id)
}

func TestGetRandomSatisfiedChannelReturnsNilWhenAllCachedChannelsAtConcurrencyLimit(t *testing.T) {
	resetChannelConcurrencyForTest()

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousGroup2model2channels := group2model2channels
	previousChannelsIDM := channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		group2model2channels = previousGroup2model2channels
		channelsIDM = previousChannelsIDM
		resetChannelConcurrencyForTest()
	})

	limit := 1
	common.MemoryCacheEnabled = true
	group2model2channels = map[string]map[string][]int{
		"default": {
			"gpt-test": {10, 11},
		},
	}
	channelsIDM = map[int]*Channel{
		10: {Id: 10, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit},
		11: {Id: 11, Priority: common.GetPointer[int64](0), Weight: common.GetPointer[uint](0), ConcurrencyLimit: &limit},
	}
	if !TryAcquireChannelConcurrency(channelsIDM[10]) {
		t.Fatal("expected first channel acquire to succeed")
	}
	if !TryAcquireChannelConcurrency(channelsIDM[11]) {
		t.Fatal("expected second channel acquire to succeed")
	}

	channel, err := GetRandomSatisfiedChannel("default", "gpt-test", 0)

	if err != nil {
		t.Fatalf("expected no error when all channels are saturated, got %v", err)
	}
	if channel != nil {
		t.Fatalf("expected no available channel, got #%d", channel.Id)
	}
}

func TestGetRandomSatisfiedChannelWithExclusionsPrefersUnusedChannel(t *testing.T) {
	resetChannelConcurrencyForTest()

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousGroup2model2channels := group2model2channels
	previousChannelsIDM := channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		group2model2channels = previousGroup2model2channels
		channelsIDM = previousChannelsIDM
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	group2model2channels = map[string]map[string][]int{
		"default": {
			"gpt-test": {20, 21},
		},
	}
	channelsIDM = map[int]*Channel{
		20: {Id: 20, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0)},
		21: {Id: 21, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0)},
	}

	channel, err := GetRandomSatisfiedChannelWithExclusions("default", "gpt-test", 0, map[int]struct{}{20: {}})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if channel == nil {
		t.Fatal("expected a channel")
	}
	if channel.Id != 21 {
		t.Fatalf("expected unused channel #21, got #%d", channel.Id)
	}
}

func TestGetRandomSatisfiedChannelWithExclusionsFallsBackToLowerPriorityUnusedChannel(t *testing.T) {
	resetChannelConcurrencyForTest()

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousGroup2model2channels := group2model2channels
	previousChannelsIDM := channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		group2model2channels = previousGroup2model2channels
		channelsIDM = previousChannelsIDM
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	group2model2channels = map[string]map[string][]int{
		"default": {
			"gpt-test": {30, 31},
		},
	}
	channelsIDM = map[int]*Channel{
		30: {Id: 30, Priority: common.GetPointer[int64](10), Weight: common.GetPointer[uint](0)},
		31: {Id: 31, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0)},
	}

	channel, err := GetRandomSatisfiedChannelWithExclusions("default", "gpt-test", 0, map[int]struct{}{30: {}})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if channel == nil {
		t.Fatal("expected a channel")
	}
	if channel.Id != 31 {
		t.Fatalf("expected lower priority unused channel #31, got #%d", channel.Id)
	}
}

func TestGetRandomSatisfiedChannelWithExclusionsFallsBackToUsedChannelWhenAllExcluded(t *testing.T) {
	resetChannelConcurrencyForTest()

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousGroup2model2channels := group2model2channels
	previousChannelsIDM := channelsIDM
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		group2model2channels = previousGroup2model2channels
		channelsIDM = previousChannelsIDM
		resetChannelConcurrencyForTest()
	})

	common.MemoryCacheEnabled = true
	group2model2channels = map[string]map[string][]int{
		"default": {
			"gpt-test": {32, 33},
		},
	}
	channelsIDM = map[int]*Channel{
		32: {Id: 32, Priority: common.GetPointer[int64](10), Weight: common.GetPointer[uint](0)},
		33: {Id: 33, Priority: common.GetPointer[int64](1), Weight: common.GetPointer[uint](0)},
	}

	channel, err := GetRandomSatisfiedChannelWithExclusions("default", "gpt-test", 0, map[int]struct{}{32: {}, 33: {}})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if channel == nil {
		t.Fatal("expected a channel")
	}
	if channel.Id != 32 {
		t.Fatalf("expected fallback to original priority channel #32, got #%d", channel.Id)
	}
}

func TestGetChannelWithExclusionsFallsBackToLowerPriorityUnusedChannel(t *testing.T) {
	truncateTables(t)
	resetChannelConcurrencyForTest()

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		resetChannelConcurrencyForTest()
	})
	common.MemoryCacheEnabled = false

	highPriority := int64(10)
	lowPriority := int64(1)
	zeroWeight := uint(0)
	channels := []Channel{
		{Id: 40, Name: "high-priority", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-test", Priority: &highPriority, Weight: &zeroWeight, Key: "sk-high"},
		{Id: 41, Name: "low-priority", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-test", Priority: &lowPriority, Weight: &zeroWeight, Key: "sk-low"},
	}
	if err := DB.Create(&channels).Error; err != nil {
		t.Fatalf("insert channels failed: %v", err)
	}
	abilities := []Ability{
		{Group: "default", Model: "gpt-test", ChannelId: 40, Enabled: true, Priority: &highPriority, Weight: zeroWeight},
		{Group: "default", Model: "gpt-test", ChannelId: 41, Enabled: true, Priority: &lowPriority, Weight: zeroWeight},
	}
	if err := DB.Create(&abilities).Error; err != nil {
		t.Fatalf("insert abilities failed: %v", err)
	}

	channel, err := GetChannelWithExclusions("default", "gpt-test", 0, map[int]struct{}{40: {}})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if channel == nil {
		t.Fatal("expected a channel")
	}
	if channel.Id != 41 {
		t.Fatalf("expected lower priority unused channel #41, got #%d", channel.Id)
	}
}

func TestGetChannelWithExclusionsFallsBackToUsedChannelWhenAllExcluded(t *testing.T) {
	truncateTables(t)
	resetChannelConcurrencyForTest()

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		resetChannelConcurrencyForTest()
	})
	common.MemoryCacheEnabled = false

	highPriority := int64(10)
	lowPriority := int64(1)
	zeroWeight := uint(0)
	channels := []Channel{
		{Id: 42, Name: "high-priority", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-test", Priority: &highPriority, Weight: &zeroWeight, Key: "sk-high"},
		{Id: 43, Name: "low-priority", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-test", Priority: &lowPriority, Weight: &zeroWeight, Key: "sk-low"},
	}
	if err := DB.Create(&channels).Error; err != nil {
		t.Fatalf("insert channels failed: %v", err)
	}
	abilities := []Ability{
		{Group: "default", Model: "gpt-test", ChannelId: 42, Enabled: true, Priority: &highPriority, Weight: zeroWeight},
		{Group: "default", Model: "gpt-test", ChannelId: 43, Enabled: true, Priority: &lowPriority, Weight: zeroWeight},
	}
	if err := DB.Create(&abilities).Error; err != nil {
		t.Fatalf("insert abilities failed: %v", err)
	}

	channel, err := GetChannelWithExclusions("default", "gpt-test", 0, map[int]struct{}{42: {}, 43: {}})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if channel == nil {
		t.Fatal("expected a channel")
	}
	if channel.Id != 42 {
		t.Fatalf("expected fallback to original priority channel #42, got #%d", channel.Id)
	}
}

func TestGetChannelReturnsNilWhenNoAbilityExists(t *testing.T) {
	truncateTables(t)
	resetChannelConcurrencyForTest()

	channel, err := GetChannel("missing-group", "missing-model", 0)

	if err != nil {
		t.Fatalf("expected no error when no ability exists, got %v", err)
	}
	if channel != nil {
		t.Fatalf("expected no channel, got #%d", channel.Id)
	}
}
