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
