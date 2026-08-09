package controller

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type channelMonitorPolicy struct {
	channel                     *model.Channel
	monitorEnabled              bool
	autoDisableEnabled          bool
	autoEnableEnabled           bool
	disableThreshold            int
	enableThreshold             int
	responseTimeThresholdMillis int64
	channelTestIntervalSeconds  int64
	lastTestTime                int64
}

func monitorIntervalSeconds(minutes float64) int64 {
	if minutes <= 0 {
		return 0
	}
	seconds := int64(math.Round(minutes * float64(time.Minute/time.Second)))
	if seconds < 1 {
		return 1
	}
	return seconds
}

type channelMonitorTestOutcome struct {
	failed             bool
	disableCandidate   bool
	enableCandidate    bool
	responseTimeMillis int64
	now                int64
}

type channelMonitorDecision struct {
	shouldDisable bool
	shouldEnable  bool
}

func newChannelMonitorPolicy(channel *model.Channel, settings dto.ChannelOtherSettings, monitorSetting *operation_setting.MonitorSetting) channelMonitorPolicy {
	policy := channelMonitorPolicy{
		channel:                     channel,
		monitorEnabled:              false,
		autoDisableEnabled:          common.AutomaticDisableChannelEnabled,
		autoEnableEnabled:           common.AutomaticEnableChannelEnabled,
		disableThreshold:            1,
		enableThreshold:             1,
		responseTimeThresholdMillis: int64(common.ChannelDisableThreshold * 1000),
		lastTestTime:                settings.MonitorLastTestTime,
	}
	if monitorSetting != nil {
		policy.monitorEnabled = monitorSetting.AutoTestChannelEnabled
		policy.disableThreshold = monitorSetting.AutoDisableThreshold
		policy.enableThreshold = monitorSetting.AutoEnableThreshold
		policy.channelTestIntervalSeconds = monitorIntervalSeconds(monitorSetting.AutoTestChannelMinutes)
	}

	if settings.MonitorEnabled != nil {
		policy.monitorEnabled = *settings.MonitorEnabled
	}
	if settings.MonitorAutoDisableEnabled != nil {
		policy.autoDisableEnabled = *settings.MonitorAutoDisableEnabled
	}
	if settings.MonitorAutoEnableEnabled != nil {
		policy.autoEnableEnabled = *settings.MonitorAutoEnableEnabled
	}
	if settings.MonitorDisableThreshold != nil {
		policy.disableThreshold = *settings.MonitorDisableThreshold
	}
	if settings.MonitorEnableThreshold != nil {
		policy.enableThreshold = *settings.MonitorEnableThreshold
	}
	if settings.MonitorResponseTimeThresholdSeconds != nil {
		policy.responseTimeThresholdMillis = int64(*settings.MonitorResponseTimeThresholdSeconds * 1000)
	}
	if settings.MonitorTestIntervalMinutes != nil {
		policy.channelTestIntervalSeconds = monitorIntervalSeconds(*settings.MonitorTestIntervalMinutes)
	}
	if policy.disableThreshold <= 0 {
		policy.disableThreshold = 1
	}
	if policy.enableThreshold <= 0 {
		policy.enableThreshold = 1
	}
	if policy.responseTimeThresholdMillis == 0 {
		policy.responseTimeThresholdMillis = 10000000
	}
	return policy
}

func (policy channelMonitorPolicy) shouldTest(automatic bool, now int64) bool {
	if policy.channel == nil {
		return false
	}
	if policy.channel.Status == common.ChannelStatusManuallyDisabled {
		return false
	}
	if !automatic {
		return true
	}
	if !policy.monitorEnabled {
		return false
	}
	if !policy.channel.GetAutoBan() {
		return false
	}
	if policy.channelTestIntervalSeconds > 0 && policy.lastTestTime > 0 && now-policy.lastTestTime < policy.channelTestIntervalSeconds {
		return false
	}
	return true
}

func (policy channelMonitorPolicy) applyResult(settings *dto.ChannelOtherSettings, outcome channelMonitorTestOutcome) channelMonitorDecision {
	if settings == nil {
		return channelMonitorDecision{}
	}
	settings.MonitorLastTestTime = outcome.now
	if outcome.failed {
		settings.MonitorConsecutiveFailures++
		settings.MonitorConsecutiveSuccesses = 0
	} else {
		settings.MonitorConsecutiveSuccesses++
		settings.MonitorConsecutiveFailures = 0
	}

	return channelMonitorDecision{
		shouldDisable: policy.autoDisableEnabled &&
			outcome.disableCandidate &&
			settings.MonitorConsecutiveFailures >= policy.disableThreshold,
		shouldEnable: policy.autoEnableEnabled &&
			outcome.enableCandidate &&
			settings.MonitorConsecutiveSuccesses >= policy.enableThreshold,
	}
}

func saveChannelMonitorSettings(channel *model.Channel, settings dto.ChannelOtherSettings) error {
	if channel == nil || channel.Id == 0 {
		return errors.New("channel is required")
	}

	// 监控任务只拥有运行态字段。配置字段可能在测试请求执行期间被管理员修改，
	// 因此必须基于数据库中的最新配置合并，不能把任务启动时的旧 JSON 整段写回。
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var current struct {
			OtherSettings string `gorm:"column:settings"`
		}
		if err := model.DB.Model(&model.Channel{}).
			Select("settings").
			Where("id = ?", channel.Id).
			Take(&current).Error; err != nil {
			return err
		}

		latest := dto.ChannelOtherSettings{}
		if current.OtherSettings != "" {
			if err := common.UnmarshalJsonStr(current.OtherSettings, &latest); err != nil {
				return fmt.Errorf("failed to parse channel monitor settings: %w", err)
			}
		}
		latest.MonitorLastTestTime = settings.MonitorLastTestTime
		latest.MonitorConsecutiveFailures = settings.MonitorConsecutiveFailures
		latest.MonitorConsecutiveSuccesses = settings.MonitorConsecutiveSuccesses

		updated := &model.Channel{Id: channel.Id}
		updated.SetOtherSettings(latest)
		query := model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id)
		if current.OtherSettings == "" {
			query = query.Where("(settings = ? OR settings IS NULL)", "")
		} else {
			query = query.Where("settings = ?", current.OtherSettings)
		}
		result := query.Update("settings", updated.OtherSettings)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			channel.OtherSettings = updated.OtherSettings
			return nil
		}
	}

	return errors.New("channel monitor settings changed too frequently")
}
