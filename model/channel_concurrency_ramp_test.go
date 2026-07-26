package model

import (
	"testing"
	"time"
)

func TestChannelConcurrencyLimitDecreaseRampsDown(t *testing.T) {
	resetChannelConcurrencyForTest()
	t.Cleanup(resetChannelConcurrencyForTest)

	now := time.Date(2026, time.July, 26, 0, 0, 0, 0, time.UTC)
	channelConcurrency.now = func() time.Time { return now }

	limit := 10
	channel := &Channel{Id: 1001, ConcurrencyLimit: &limit}
	for i := 0; i < 8; i++ {
		if !TryAcquireChannelConcurrency(channel) {
			t.Fatalf("旧上限下第 %d 个并发请求不应被拒绝", i+1)
		}
	}

	limit = 4
	if TryAcquireChannelConcurrency(channel) {
		t.Fatal("调低配置刚生效时不应把并发从当前在途数重新抬高")
	}
	ReleaseChannelConcurrency(channel.Id)
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("缓降起始时应允许请求补回当前有效上限")
	}

	now = now.Add(channelConcurrencyRampDownDuration / 2)
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("过渡一半后有效上限应已降至 6，当前 8 个请求不应继续准入")
	}
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("当前 6 个请求等于过渡中点有效上限，不应继续准入")
	}
	ReleaseChannelConcurrency(channel.Id)
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("过渡一半后应允许并发数从 5 补到有效上限 6")
	}

	now = now.Add(channelConcurrencyRampDownDuration / 2)
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("过渡完成后应严格按新上限 4 准入")
	}
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("在途请求等于新上限 4 时不应继续准入")
	}
	ReleaseChannelConcurrency(channel.Id)
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("在途请求降至 3 后应可按新上限准入")
	}
}

func TestChannelConcurrencyLimitIncreaseRemainsImmediate(t *testing.T) {
	resetChannelConcurrencyForTest()
	t.Cleanup(resetChannelConcurrencyForTest)

	now := time.Date(2026, time.July, 26, 0, 0, 0, 0, time.UTC)
	channelConcurrency.now = func() time.Time { return now }

	limit := 2
	channel := &Channel{Id: 1002, ConcurrencyLimit: &limit}
	if !TryAcquireChannelConcurrency(channel) || !TryAcquireChannelConcurrency(channel) {
		t.Fatal("应允许占满初始并发上限")
	}
	if TryAcquireChannelConcurrency(channel) {
		t.Fatal("达到初始并发上限后不应继续准入")
	}

	limit = 4
	if !TryAcquireChannelConcurrency(channel) || !TryAcquireChannelConcurrency(channel) {
		t.Fatal("调高并发上限后新增容量应立即可用")
	}
	if TryAcquireChannelConcurrency(channel) {
		t.Fatal("达到调高后的并发上限后不应继续准入")
	}
}

func TestChannelConcurrencyUnlimitedToFiniteRampsFromActiveRequests(t *testing.T) {
	resetChannelConcurrencyForTest()
	t.Cleanup(resetChannelConcurrencyForTest)

	now := time.Date(2026, time.July, 26, 0, 0, 0, 0, time.UTC)
	channelConcurrency.now = func() time.Time { return now }

	limit := 0
	channel := &Channel{Id: 1003, ConcurrencyLimit: &limit}
	for i := 0; i < 6; i++ {
		if !TryAcquireChannelConcurrency(channel) {
			t.Fatalf("不限并发时第 %d 个请求不应被拒绝", i+1)
		}
	}

	limit = 3
	if TryAcquireChannelConcurrency(channel) {
		t.Fatal("从不限并发调低后，不应继续突破当前 6 个在途请求")
	}
	ReleaseChannelConcurrency(channel.Id)
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("缓降起始时应允许请求补回当前有效上限")
	}

	now = now.Add(channelConcurrencyRampDownDuration)
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("缓降完成后，6 个在途请求应超过新上限 3")
	}
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("在途请求等于新上限 3 时不应继续准入")
	}
	ReleaseChannelConcurrency(channel.Id)
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("在途请求低于新上限后应恢复准入")
	}
}

func TestChannelConcurrencyIncreaseDuringRampAppliesImmediately(t *testing.T) {
	resetChannelConcurrencyForTest()
	t.Cleanup(resetChannelConcurrencyForTest)

	now := time.Date(2026, time.July, 26, 0, 0, 0, 0, time.UTC)
	channelConcurrency.now = func() time.Time { return now }

	limit := 10
	channel := &Channel{Id: 1004, ConcurrencyLimit: &limit}
	for i := 0; i < 8; i++ {
		if !TryAcquireChannelConcurrency(channel) {
			t.Fatalf("旧上限下第 %d 个并发请求不应被拒绝", i+1)
		}
	}

	limit = 2
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("调低刚开始时不应把并发从当前在途数重新抬高")
	}
	now = now.Add(channelConcurrencyRampDownDuration / 2)
	if IsChannelConcurrencyAvailable(channel) {
		t.Fatal("缓降中点后 8 个在途请求应超过有效上限")
	}

	limit = 4
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
	ReleaseChannelConcurrency(channel.Id)
	if !TryAcquireChannelConcurrency(channel) {
		t.Fatal("缓降途中调高到 4 后，应立即允许从 3 补到 4")
	}
	if TryAcquireChannelConcurrency(channel) {
		t.Fatal("调高后的上限 4 应立即生效，不应继续沿用旧缓降有效值")
	}
}
