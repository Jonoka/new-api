package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelMonitorPolicyRequiresConsecutiveFailuresBeforeDisable(t *testing.T) {
	enabled := true
	settings := dto.ChannelOtherSettings{
		MonitorAutoDisableEnabled: &enabled,
	}
	policy := newChannelMonitorPolicy(&model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}, settings, &operation_setting.MonitorSetting{
		AutoDisableThreshold: 2,
		AutoEnableThreshold:  2,
	})

	first := policy.applyResult(&settings, channelMonitorTestOutcome{
		failed:             true,
		disableCandidate:   true,
		enableCandidate:    false,
		responseTimeMillis: 100,
		now:                1000,
	})
	require.False(t, first.shouldDisable)
	require.Equal(t, 1, settings.MonitorConsecutiveFailures)
	require.Equal(t, 0, settings.MonitorConsecutiveSuccesses)

	second := policy.applyResult(&settings, channelMonitorTestOutcome{
		failed:             true,
		disableCandidate:   true,
		enableCandidate:    false,
		responseTimeMillis: 100,
		now:                1060,
	})
	require.True(t, second.shouldDisable)
	require.Equal(t, 2, settings.MonitorConsecutiveFailures)
	require.Equal(t, int64(1060), settings.MonitorLastTestTime)
}

func TestChannelMonitorPolicyRequiresConsecutiveSuccessesBeforeEnable(t *testing.T) {
	enabled := true
	settings := dto.ChannelOtherSettings{
		MonitorAutoEnableEnabled: &enabled,
	}
	policy := newChannelMonitorPolicy(&model.Channel{
		Status: common.ChannelStatusAutoDisabled,
	}, settings, &operation_setting.MonitorSetting{
		AutoDisableThreshold: 2,
		AutoEnableThreshold:  2,
	})

	first := policy.applyResult(&settings, channelMonitorTestOutcome{
		failed:          false,
		enableCandidate: true,
		now:             1000,
	})
	require.False(t, first.shouldEnable)
	require.Equal(t, 0, settings.MonitorConsecutiveFailures)
	require.Equal(t, 1, settings.MonitorConsecutiveSuccesses)

	second := policy.applyResult(&settings, channelMonitorTestOutcome{
		failed:          false,
		enableCandidate: true,
		now:             1060,
	})
	require.True(t, second.shouldEnable)
	require.Equal(t, 0, settings.MonitorConsecutiveFailures)
	require.Equal(t, 2, settings.MonitorConsecutiveSuccesses)
}

func TestChannelMonitorPolicySkipsAutomaticMonitoring(t *testing.T) {
	disabledMonitor := false
	manualDisabled := newChannelMonitorPolicy(&model.Channel{
		Status: common.ChannelStatusManuallyDisabled,
	}, dto.ChannelOtherSettings{}, &operation_setting.MonitorSetting{})
	require.False(t, manualDisabled.shouldTest(true, 1000))

	channelMonitorOff := newChannelMonitorPolicy(&model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}, dto.ChannelOtherSettings{
		MonitorEnabled: &disabledMonitor,
	}, &operation_setting.MonitorSetting{})
	require.False(t, channelMonitorOff.shouldTest(true, 1000))
	require.True(t, channelMonitorOff.shouldTest(false, 1000))

	legacyAutoBanOff := newChannelMonitorPolicy(&model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(0),
	}, dto.ChannelOtherSettings{}, &operation_setting.MonitorSetting{})
	require.False(t, legacyAutoBanOff.shouldTest(true, 1000))
	require.True(t, legacyAutoBanOff.shouldTest(false, 1000))
}

func TestChannelMonitorPolicyMonitorSwitchOverridesGlobal(t *testing.T) {
	enabled := true
	disabled := false
	channel := &model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}

	inheritsDisabled := newChannelMonitorPolicy(channel, dto.ChannelOtherSettings{}, &operation_setting.MonitorSetting{
		AutoTestChannelEnabled: false,
	})
	require.False(t, inheritsDisabled.shouldTest(true, 1000))

	explicitlyEnabled := newChannelMonitorPolicy(channel, dto.ChannelOtherSettings{
		MonitorEnabled: &enabled,
	}, &operation_setting.MonitorSetting{
		AutoTestChannelEnabled: false,
	})
	require.True(t, explicitlyEnabled.shouldTest(true, 1000))

	explicitlyDisabled := newChannelMonitorPolicy(channel, dto.ChannelOtherSettings{
		MonitorEnabled: &disabled,
	}, &operation_setting.MonitorSetting{
		AutoTestChannelEnabled: true,
	})
	require.False(t, explicitlyDisabled.shouldTest(true, 1000))
}

func TestChannelMonitorPolicyHonorsChannelInterval(t *testing.T) {
	minutes := 10.0
	policy := newChannelMonitorPolicy(&model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}, dto.ChannelOtherSettings{
		MonitorTestIntervalMinutes: &minutes,
		MonitorLastTestTime:        1000,
	}, &operation_setting.MonitorSetting{AutoTestChannelEnabled: true})

	require.False(t, policy.shouldTest(true, 1200))
	require.True(t, policy.shouldTest(true, 1600))
	require.True(t, policy.shouldTest(false, 1200))
}

func TestChannelMonitorPolicyHonorsGlobalIntervalWhenChannelInherits(t *testing.T) {
	policy := newChannelMonitorPolicy(&model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}, dto.ChannelOtherSettings{
		MonitorLastTestTime: 1000,
	}, &operation_setting.MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 10,
	})

	require.False(t, policy.shouldTest(true, 1200))
	require.True(t, policy.shouldTest(true, 1600))
	require.True(t, policy.shouldTest(false, 1200))
}

func TestAutomaticChannelTestPollIntervalAllowsShorterChannelOverrides(t *testing.T) {
	channel := &model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}
	interval, enabled := automaticChannelTestPollInterval(&operation_setting.MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 10,
	}, []*model.Channel{channel})
	require.True(t, enabled)
	require.Equal(t, time.Minute, interval)

	channelOverrideEnabled := true
	channelOverrideMinutes := 0.5
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		MonitorEnabled:             &channelOverrideEnabled,
		MonitorTestIntervalMinutes: &channelOverrideMinutes,
	})
	interval, enabled = automaticChannelTestPollInterval(&operation_setting.MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 10,
	}, []*model.Channel{channel})
	require.True(t, enabled)
	require.Equal(t, 30*time.Second, interval)
}

func TestAutomaticChannelTestPollIntervalSkipsDisabledChannels(t *testing.T) {
	disabled := false
	channel := &model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{MonitorEnabled: &disabled})

	interval, enabled := automaticChannelTestPollInterval(&operation_setting.MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 0.5,
	}, []*model.Channel{channel})

	require.False(t, enabled)
	require.Equal(t, time.Minute, interval)
}

func TestSaveChannelMonitorSettingsOnlyUpdatesSettings(t *testing.T) {
	oldDB := model.DB
	t.Cleanup(func() {
		model.DB = oldDB
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db

	channel := model.Channel{
		Key:           "sk-test",
		Name:          "test",
		Status:        common.ChannelStatusEnabled,
		AutoBan:       common.GetPointer(1),
		OtherSettings: "{}",
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).
		Where("id = ?", channel.Id).
		Update("status", common.ChannelStatusAutoDisabled).Error)

	settings := channel.GetOtherSettings()
	settings.MonitorConsecutiveFailures = 1
	require.NoError(t, saveChannelMonitorSettings(&channel, settings))

	var reloaded model.Channel
	require.NoError(t, model.DB.First(&reloaded, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	require.Equal(t, 1, reloaded.GetOtherSettings().MonitorConsecutiveFailures)
}

func TestSaveChannelMonitorSettingsPreservesConcurrentConfigChanges(t *testing.T) {
	oldDB := model.DB
	t.Cleanup(func() {
		model.DB = oldDB
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db

	monitorEnabled := true
	channel := model.Channel{
		Key:     "sk-test",
		Name:    "test",
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		MonitorEnabled: &monitorEnabled,
	})
	require.NoError(t, model.DB.Create(&channel).Error)

	// 模拟监控任务持有旧快照后，管理员关闭了该渠道监控并修改间隔。
	staleSettings := channel.GetOtherSettings()
	staleSettings.MonitorLastTestTime = 1234
	staleSettings.MonitorConsecutiveFailures = 2
	monitorEnabled = false
	intervalMinutes := 30.0
	latestSettings := dto.ChannelOtherSettings{
		MonitorEnabled:             &monitorEnabled,
		MonitorTestIntervalMinutes: &intervalMinutes,
	}
	latestChannel := model.Channel{Id: channel.Id}
	latestChannel.SetOtherSettings(latestSettings)
	require.NoError(t, model.DB.Model(&model.Channel{}).
		Where("id = ?", channel.Id).
		Update("settings", latestChannel.OtherSettings).Error)

	require.NoError(t, saveChannelMonitorSettings(&channel, staleSettings))

	var reloaded model.Channel
	require.NoError(t, model.DB.First(&reloaded, channel.Id).Error)
	saved := reloaded.GetOtherSettings()
	require.NotNil(t, saved.MonitorEnabled)
	require.False(t, *saved.MonitorEnabled)
	require.NotNil(t, saved.MonitorTestIntervalMinutes)
	require.Equal(t, 30.0, *saved.MonitorTestIntervalMinutes)
	require.Equal(t, int64(1234), saved.MonitorLastTestTime)
	require.Equal(t, 2, saved.MonitorConsecutiveFailures)
}
