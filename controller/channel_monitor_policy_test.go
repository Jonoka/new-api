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

func TestChannelMonitorPolicyHonorsChannelInterval(t *testing.T) {
	minutes := 10.0
	policy := newChannelMonitorPolicy(&model.Channel{
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
	}, dto.ChannelOtherSettings{
		MonitorTestIntervalMinutes: &minutes,
		MonitorLastTestTime:        1000,
	}, &operation_setting.MonitorSetting{})

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
		AutoTestChannelMinutes: 10,
	})

	require.False(t, policy.shouldTest(true, 1200))
	require.True(t, policy.shouldTest(true, 1600))
	require.True(t, policy.shouldTest(false, 1200))
}

func TestAutomaticChannelTestPollIntervalAllowsShorterChannelOverrides(t *testing.T) {
	require.Equal(t, time.Minute, automaticChannelTestPollInterval(&operation_setting.MonitorSetting{
		AutoTestChannelMinutes: 10,
	}))
	require.Equal(t, 30*time.Second, automaticChannelTestPollInterval(&operation_setting.MonitorSetting{
		AutoTestChannelMinutes: 0.5,
	}))
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
