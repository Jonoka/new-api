package channel_metrics_setting

import (
	"time"

	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	maxModelSnapshotBytes   = 191
	maxChannelSnapshotBytes = 191
	maxGroupSnapshotBytes   = 64
	maxErrorStageBytes      = 32
)

// ChannelMetricsSetting 控制渠道可观测性采集，不影响现有 perf_metrics。
type ChannelMetricsSetting struct {
	Enabled                      bool   `json:"enabled"`
	BucketLevel                  string `json:"bucket_level"`
	BucketSeconds                int    `json:"bucket_seconds"`
	FlushIntervalSeconds         int    `json:"flush_interval_seconds"`
	FinalFlushTimeoutSeconds     int    `json:"final_flush_timeout_seconds"`
	RetentionDays                int    `json:"retention_days"`
	FailureRetentionDays         int    `json:"failure_retention_days"`
	BackfillEnabled              bool   `json:"backfill_enabled"`
	BackfillBatchSize            int    `json:"backfill_batch_size"`
	BackfillPauseMilliseconds    int    `json:"backfill_pause_milliseconds"`
	MaxActiveDimensionsPerBucket int    `json:"max_active_dimensions_per_bucket"`
	MaxHotBuckets                int    `json:"max_hot_buckets"`
	CollectorShards              int    `json:"collector_shards"`
	ModelSnapshotMaxBytes        int    `json:"model_snapshot_max_bytes"`
	ChannelSnapshotMaxBytes      int    `json:"channel_snapshot_max_bytes"`
	GroupSnapshotMaxBytes        int    `json:"group_snapshot_max_bytes"`
	ErrorStageMaxBytes           int    `json:"error_stage_max_bytes"`
}

var channelMetricsSetting = DefaultSetting()

func DefaultSetting() ChannelMetricsSetting {
	return ChannelMetricsSetting{
		Enabled:                      true,
		BucketLevel:                  "5m",
		BucketSeconds:                300,
		FlushIntervalSeconds:         20,
		FinalFlushTimeoutSeconds:     5,
		RetentionDays:                7,
		FailureRetentionDays:         14,
		BackfillEnabled:              true,
		BackfillBatchSize:            500,
		BackfillPauseMilliseconds:    25,
		MaxActiveDimensionsPerBucket: 10_000,
		MaxHotBuckets:                50_000,
		CollectorShards:              32,
		ModelSnapshotMaxBytes:        128,
		ChannelSnapshotMaxBytes:      128,
		GroupSnapshotMaxBytes:        64,
		ErrorStageMaxBytes:           32,
	}
}

func init() {
	config.GlobalConfig.Register("channel_metrics_setting", &channelMetricsSetting)
}

// GetSetting 返回副本，避免调用方直接修改全局配置。
func GetSetting() ChannelMetricsSetting {
	return channelMetricsSetting
}

// Normalized 返回可安全持久化的设置副本。
// 采集器在启动时固定该副本；修改采集粒度、快照或容量设置后需重启进程生效。
func (s ChannelMetricsSetting) Normalized() ChannelMetricsSetting {
	defaults := DefaultSetting()
	if s.BucketLevel == "" {
		s.BucketLevel = defaults.BucketLevel
	}
	if s.BucketSeconds <= 0 {
		s.BucketSeconds = defaults.BucketSeconds
	}
	if s.FlushIntervalSeconds <= 0 {
		s.FlushIntervalSeconds = defaults.FlushIntervalSeconds
	}
	if s.FinalFlushTimeoutSeconds <= 0 {
		s.FinalFlushTimeoutSeconds = defaults.FinalFlushTimeoutSeconds
	}
	if s.RetentionDays <= 0 {
		s.RetentionDays = defaults.RetentionDays
	}
	if s.FailureRetentionDays <= 0 {
		s.FailureRetentionDays = defaults.FailureRetentionDays
	}
	if s.BackfillBatchSize <= 0 {
		s.BackfillBatchSize = defaults.BackfillBatchSize
	}
	// 单批还会按 request_id / channel_id 构造 IN 查询；限制到 500，
	// 为 SQLite 的绑定变量上限保留过滤参数空间。
	if s.BackfillBatchSize > 500 {
		s.BackfillBatchSize = 500
	}
	if s.BackfillPauseMilliseconds < 0 {
		s.BackfillPauseMilliseconds = defaults.BackfillPauseMilliseconds
	}
	if s.MaxActiveDimensionsPerBucket <= 0 {
		s.MaxActiveDimensionsPerBucket = defaults.MaxActiveDimensionsPerBucket
	}
	if s.MaxHotBuckets <= 0 {
		s.MaxHotBuckets = defaults.MaxHotBuckets
	}
	if s.CollectorShards <= 0 {
		s.CollectorShards = defaults.CollectorShards
	}
	if s.CollectorShards > 256 {
		s.CollectorShards = 256
	}
	s.ModelSnapshotMaxBytes = normalizeSnapshotLimit(s.ModelSnapshotMaxBytes, defaults.ModelSnapshotMaxBytes, maxModelSnapshotBytes)
	s.ChannelSnapshotMaxBytes = normalizeSnapshotLimit(s.ChannelSnapshotMaxBytes, defaults.ChannelSnapshotMaxBytes, maxChannelSnapshotBytes)
	s.GroupSnapshotMaxBytes = normalizeSnapshotLimit(s.GroupSnapshotMaxBytes, defaults.GroupSnapshotMaxBytes, maxGroupSnapshotBytes)
	s.ErrorStageMaxBytes = normalizeSnapshotLimit(s.ErrorStageMaxBytes, defaults.ErrorStageMaxBytes, maxErrorStageBytes)
	return s
}

func normalizeSnapshotLimit(value int, fallback int, maximum int) int {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

// CollectorConfig 把可持久化设置转换为不含数据库依赖的采集器配置。
// NodeID、时钟、Sink 和错误回调由启动层注入。
func (s ChannelMetricsSetting) CollectorConfig() channelmetrics.Config {
	s = s.Normalized()
	return channelmetrics.Config{
		Enabled:                      s.Enabled,
		BucketLevel:                  s.BucketLevel,
		BucketSeconds:                int64(s.BucketSeconds),
		FlushInterval:                time.Duration(s.FlushIntervalSeconds) * time.Second,
		FinalFlushTimeout:            time.Duration(s.FinalFlushTimeoutSeconds) * time.Second,
		MaxActiveDimensionsPerBucket: s.MaxActiveDimensionsPerBucket,
		MaxHotBuckets:                s.MaxHotBuckets,
		ShardCount:                   s.CollectorShards,
		SnapshotLimits: channelmetrics.SnapshotLimits{
			ModelBytes:       s.ModelSnapshotMaxBytes,
			ChannelNameBytes: s.ChannelSnapshotMaxBytes,
			GroupBytes:       s.GroupSnapshotMaxBytes,
			ErrorStageBytes:  s.ErrorStageMaxBytes,
		},
	}
}

func GetCollectorConfig() channelmetrics.Config {
	return GetSetting().CollectorConfig()
}

func GetRetentionDays() int {
	if channelMetricsSetting.RetentionDays <= 0 {
		return DefaultSetting().RetentionDays
	}
	return channelMetricsSetting.RetentionDays
}

func GetFailureRetentionDays() int {
	if channelMetricsSetting.FailureRetentionDays <= 0 {
		return DefaultSetting().FailureRetentionDays
	}
	return channelMetricsSetting.FailureRetentionDays
}
