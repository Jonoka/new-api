package model

import (
	"math"
	"sync"
	"time"
)

// channelConcurrencyRampDownDuration 是渠道并发上限调低后的完整过渡时长。
// 过渡期间有效上限线性下降，避免大量流量在配置保存后瞬间切换到其他渠道。
const channelConcurrencyRampDownDuration = time.Minute

type channelConcurrencyLimitState struct {
	configuredLimit int
	effectiveLimit  int
	rampStartLimit  int
	rampTargetLimit int
	rampStartedAt   time.Time
}

var channelConcurrency = struct {
	sync.Mutex
	active map[int]int
	limits map[int]*channelConcurrencyLimitState
	now    func() time.Time
}{
	active: make(map[int]int),
	limits: make(map[int]*channelConcurrencyLimitState),
	now:    time.Now,
}

func IsChannelConcurrencyAvailable(channel *Channel) bool {
	// 不限并发的候选检查无需进入全局锁；真正准入时仍会记录在途请求，
	// 以便后续从“不限”调低为有限值时能够平滑收敛。
	if channel == nil || channel.GetConcurrencyLimit() <= 0 {
		return true
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()

	effectiveLimit := getEffectiveChannelConcurrencyLimitLocked(channel, channelConcurrency.now())
	if effectiveLimit <= 0 {
		return true
	}
	return channelConcurrency.active[channel.Id] < effectiveLimit
}

func TryAcquireChannelConcurrency(channel *Channel) bool {
	if channel == nil {
		return true
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()

	current := channelConcurrency.active[channel.Id]
	effectiveLimit := getEffectiveChannelConcurrencyLimitLocked(channel, channelConcurrency.now())
	if effectiveLimit > 0 && current >= effectiveLimit {
		return false
	}
	channelConcurrency.active[channel.Id] = current + 1
	return true
}

func getEffectiveChannelConcurrencyLimitLocked(channel *Channel, now time.Time) int {
	requestedLimit := channel.GetConcurrencyLimit()
	state, ok := channelConcurrency.limits[channel.Id]
	if !ok {
		state = &channelConcurrencyLimitState{
			configuredLimit: requestedLimit,
			effectiveLimit:  requestedLimit,
		}
		channelConcurrency.limits[channel.Id] = state
		return requestedLimit
	}

	advanceChannelConcurrencyRampLocked(state, now)
	if requestedLimit == state.configuredLimit {
		return state.effectiveLimit
	}

	previousConfiguredLimit := state.configuredLimit
	state.configuredLimit = requestedLimit
	if requestedLimit == 0 || (previousConfiguredLimit > 0 && requestedLimit > previousConfiguredLimit) {
		// 调高或改为不限制时立即生效，并取消尚未完成的调低过渡。
		state.effectiveLimit = requestedLimit
		clearChannelConcurrencyRamp(state)
		return state.effectiveLimit
	}

	// 以当前在途请求数为起点缓降，避免调低后重新把流量补到旧配置上限。
	rampStartLimit := channelConcurrency.active[channel.Id]
	if rampStartLimit < requestedLimit {
		rampStartLimit = requestedLimit
	}
	if rampStartLimit <= requestedLimit {
		state.effectiveLimit = requestedLimit
		clearChannelConcurrencyRamp(state)
		return state.effectiveLimit
	}

	state.effectiveLimit = rampStartLimit
	state.rampStartLimit = rampStartLimit
	state.rampTargetLimit = requestedLimit
	state.rampStartedAt = now
	return state.effectiveLimit
}

func advanceChannelConcurrencyRampLocked(state *channelConcurrencyLimitState, now time.Time) {
	if state.rampStartedAt.IsZero() || state.rampStartLimit <= state.rampTargetLimit {
		return
	}

	elapsed := now.Sub(state.rampStartedAt)
	if elapsed <= 0 {
		return
	}
	if elapsed >= channelConcurrencyRampDownDuration {
		state.effectiveLimit = state.rampTargetLimit
		clearChannelConcurrencyRamp(state)
		return
	}

	progress := float64(elapsed) / float64(channelConcurrencyRampDownDuration)
	delta := float64(state.rampStartLimit - state.rampTargetLimit)
	state.effectiveLimit = state.rampTargetLimit + int(math.Ceil(delta*(1-progress)))
}

func clearChannelConcurrencyRamp(state *channelConcurrencyLimitState) {
	state.rampStartLimit = 0
	state.rampTargetLimit = 0
	state.rampStartedAt = time.Time{}
}

func ReleaseChannelConcurrency(channelID int) {
	if channelID <= 0 {
		return
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()

	current := channelConcurrency.active[channelID]
	if current <= 1 {
		delete(channelConcurrency.active, channelID)
		// 渠道已无在途请求，不再需要保留过渡状态；后续请求直接使用当前配置。
		delete(channelConcurrency.limits, channelID)
		return
	}
	channelConcurrency.active[channelID] = current - 1
}

func resetChannelConcurrencyForTest() {
	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()

	channelConcurrency.active = make(map[int]int)
	channelConcurrency.limits = make(map[int]*channelConcurrencyLimitState)
	channelConcurrency.now = time.Now
}
