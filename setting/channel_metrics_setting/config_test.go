package channel_metrics_setting

import (
	"testing"
	"time"
)

func TestDefaultSettingBuildsCollectorConfig(t *testing.T) {
	setting := DefaultSetting()
	config := setting.CollectorConfig()
	if !config.Enabled || config.BucketLevel != "5m" || config.BucketSeconds != 300 {
		t.Fatalf("默认采集粒度异常：%+v", config)
	}
	if config.FlushInterval != 20*time.Second || config.FinalFlushTimeout != 5*time.Second {
		t.Fatalf("默认刷新间隔异常：%+v", config)
	}
	if config.MaxActiveDimensionsPerBucket != 10_000 || config.MaxHotBuckets != 50_000 || config.ShardCount != 32 {
		t.Fatalf("默认内存边界异常：%+v", config)
	}
	if config.SnapshotLimits.ModelBytes != 128 || config.SnapshotLimits.GroupBytes != 64 {
		t.Fatalf("默认展示快照限制异常：%+v", config.SnapshotLimits)
	}
	if !setting.BackfillEnabled || setting.BackfillBatchSize != 500 || setting.BackfillPauseMilliseconds != 25 {
		t.Fatalf("默认历史回填边界异常：%+v", setting)
	}
}

func TestCollectorConfigPreservesDisabledAndNormalizesBounds(t *testing.T) {
	config := (ChannelMetricsSetting{}).CollectorConfig()
	if config.Enabled {
		t.Fatal("显式关闭配置不应被默认值重新启用")
	}
	if config.BucketSeconds != 300 || config.FlushInterval != 20*time.Second || config.ShardCount != 32 {
		t.Fatalf("零值边界没有回退到安全默认值：%+v", config)
	}
}

func TestCollectorConfigClampsSnapshotsToDatabaseColumns(t *testing.T) {
	setting := DefaultSetting()
	setting.ModelSnapshotMaxBytes = 10_000
	setting.ChannelSnapshotMaxBytes = 10_000
	setting.GroupSnapshotMaxBytes = 10_000
	setting.ErrorStageMaxBytes = 10_000
	setting.CollectorShards = 10_000
	setting.BackfillBatchSize = 10_000

	config := setting.CollectorConfig()
	if config.SnapshotLimits.ModelBytes != 191 || config.SnapshotLimits.ChannelNameBytes != 191 {
		t.Fatalf("模型或渠道快照超过数据库列宽：%+v", config.SnapshotLimits)
	}
	if config.SnapshotLimits.GroupBytes != 64 || config.SnapshotLimits.ErrorStageBytes != 32 {
		t.Fatalf("分组或错误阶段快照超过数据库列宽：%+v", config.SnapshotLimits)
	}
	if config.ShardCount != 256 {
		t.Fatalf("分片数未限制到采集器支持的上限：%d", config.ShardCount)
	}
	if normalized := setting.Normalized(); normalized.BackfillBatchSize != 500 {
		t.Fatalf("回填批量没有限制 SQLite 绑定变量：%d", normalized.BackfillBatchSize)
	}
}
