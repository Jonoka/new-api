package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ChannelMetricDimensionVersion = 1
	ChannelMetricMaxDeleteBatch   = 1000
	ChannelMetricHistogramBuckets = 13
	channelMetricVerifiedCacheMax = 100000
)

// ChannelMetricHistogramUpperBoundsMs 的最后一项表示 +Inf。
var ChannelMetricHistogramUpperBoundsMs = [ChannelMetricHistogramBuckets]int64{
	100, 250, 500, 1000, 2000, 4000, 8000, 15000, 30000, 60000, 120000, 300000, 9223372036854775807,
}

var (
	ErrChannelMetricHashCollision = errors.New("channel metric dimension hash collision")
	ErrChannelMetricInvalidBatch  = errors.New("invalid channel metric batch")

	channelMetricVerifiedDimensions sync.Map
	channelMetricDimensionVerifyMu  sync.Mutex
	channelMetricVerifiedCacheSize  int
)

// ChannelMetricBucket 保存渠道可观测性的聚合事实。
// 模型及渠道名仅是展示快照，维度身份以 dimension_hash 为准。
type ChannelMetricBucket struct {
	Id               int64  `json:"id" gorm:"primaryKey"`
	BucketLevel      string `json:"bucket_level" gorm:"size:16;not null;uniqueIndex:ux_cmb_identity,priority:1;index:idx_cmb_scope_time,priority:1;index:idx_cmb_channel_time,priority:1;index:idx_cmb_reqmodel_time,priority:1;index:idx_cmb_upmodel_time,priority:1;index:idx_cmb_group_time,priority:1;index:idx_cmb_upstatus_time,priority:1"`
	BucketTs         int64  `json:"bucket_ts" gorm:"not null;uniqueIndex:ux_cmb_identity,priority:2;index:idx_cmb_scope_time,priority:5;index:idx_cmb_channel_time,priority:6;index:idx_cmb_reqmodel_time,priority:6;index:idx_cmb_upmodel_time,priority:6;index:idx_cmb_group_time,priority:6;index:idx_cmb_upstatus_time,priority:7"`
	DimensionHash    string `json:"dimension_hash" gorm:"size:64;not null;uniqueIndex:ux_cmb_identity,priority:3"`
	DimensionVersion int    `json:"dimension_version" gorm:"not null;default:1"`

	MetricScope         string `json:"metric_scope" gorm:"size:24;not null;index:idx_cmb_scope_time,priority:2;index:idx_cmb_channel_time,priority:2;index:idx_cmb_reqmodel_time,priority:2;index:idx_cmb_upmodel_time,priority:2;index:idx_cmb_group_time,priority:2;index:idx_cmb_upstatus_time,priority:2"`
	ChannelPresent      bool   `json:"channel_present" gorm:"not null;default:false"`
	ChannelId           int    `json:"channel_id" gorm:"not null;default:0;index:idx_cmb_channel_time,priority:5"`
	ChannelNameSnapshot string `json:"channel_name_snapshot" gorm:"size:191;not null;default:''"`
	ChannelNameHash     string `json:"channel_name_hash" gorm:"size:64;not null;default:''"`
	ChannelType         int    `json:"channel_type" gorm:"not null;default:0"`

	RequestedModelPresent bool   `json:"requested_model_present" gorm:"not null;default:false"`
	RequestedModel        string `json:"requested_model" gorm:"size:191;not null;default:''"`
	RequestedModelHash    string `json:"requested_model_hash" gorm:"size:64;not null;default:'';index:idx_cmb_reqmodel_time,priority:5"`
	UpstreamModelPresent  bool   `json:"upstream_model_present" gorm:"not null;default:false"`
	UpstreamModel         string `json:"upstream_model" gorm:"size:191;not null;default:''"`
	UpstreamModelHash     string `json:"upstream_model_hash" gorm:"size:64;not null;default:'';index:idx_cmb_upmodel_time,priority:5"`
	Group                 string `json:"group" gorm:"column:group;size:64;not null;default:''"`
	GroupHash             string `json:"group_hash" gorm:"size:64;not null;default:'';index:idx_cmb_group_time,priority:5"`

	TrafficSource   string `json:"traffic_source" gorm:"size:16;not null;index:idx_cmb_scope_time,priority:3;index:idx_cmb_channel_time,priority:3;index:idx_cmb_reqmodel_time,priority:3;index:idx_cmb_upmodel_time,priority:3;index:idx_cmb_group_time,priority:3;index:idx_cmb_upstatus_time,priority:3"`
	DataOrigin      string `json:"data_origin" gorm:"size:16;not null;index:idx_cmb_scope_time,priority:4;index:idx_cmb_channel_time,priority:4;index:idx_cmb_reqmodel_time,priority:4;index:idx_cmb_upmodel_time,priority:4;index:idx_cmb_group_time,priority:4;index:idx_cmb_upstatus_time,priority:4"`
	Stream          bool   `json:"stream" gorm:"not null;default:false"`
	Outcome         string `json:"outcome" gorm:"size:24;not null;default:''"`
	ErrorStage      string `json:"error_stage" gorm:"size:32;not null;default:''"`
	FailureOwner    string `json:"failure_owner" gorm:"size:16;not null;default:''"`
	QualityEligible bool   `json:"quality_eligible" gorm:"not null;default:false"`
	PartialResponse bool   `json:"partial_response" gorm:"not null;default:false"`
	Overflowed      bool   `json:"overflowed" gorm:"not null;default:false"`

	ClientStatusPresent     bool `json:"client_status_present" gorm:"not null;default:false"`
	ClientStatusCode        int  `json:"client_status_code" gorm:"not null;default:0"`
	UpstreamStatusPresent   bool `json:"upstream_status_present" gorm:"not null;default:false;index:idx_cmb_upstatus_time,priority:5"`
	UpstreamStatusCode      int  `json:"upstream_status_code" gorm:"not null;default:0;index:idx_cmb_upstatus_time,priority:6"`
	NormalizedStatusPresent bool `json:"normalized_status_present" gorm:"not null;default:false"`
	NormalizedStatusCode    int  `json:"normalized_status_code" gorm:"not null;default:0"`

	EventCount               int64 `json:"event_count" gorm:"not null;default:0"`
	SuccessCount             int64 `json:"success_count" gorm:"not null;default:0"`
	NonFirstAttemptCount     int64 `json:"non_first_attempt_count" gorm:"not null;default:0"`
	RetryPlannedCount        int64 `json:"retry_planned_count" gorm:"not null;default:0"`
	QualityEligibleCount     int64 `json:"quality_eligible_count" gorm:"not null;default:0"`
	QualitySuccessCount      int64 `json:"quality_success_count" gorm:"not null;default:0"`
	PartialResponseCount     int64 `json:"partial_response_count" gorm:"not null;default:0"`
	UsageSampleCount         int64 `json:"usage_sample_count" gorm:"not null;default:0"`
	CacheHitRequestCount     int64 `json:"cache_hit_request_count" gorm:"not null;default:0"`
	InputTokensTotal         int64 `json:"input_tokens_total" gorm:"not null;default:0"`
	UncachedInputTokens      int64 `json:"uncached_input_tokens" gorm:"not null;default:0"`
	OutputTokens             int64 `json:"output_tokens" gorm:"not null;default:0"`
	CacheReadTokens          int64 `json:"cache_read_tokens" gorm:"not null;default:0"`
	CacheWriteTokens         int64 `json:"cache_write_tokens" gorm:"not null;default:0"`
	ChargedQuota             int64 `json:"charged_quota" gorm:"not null;default:0"`
	ChargedMicroUsd          int64 `json:"charged_micro_usd" gorm:"not null;default:0"`
	LatencySumMs             int64 `json:"latency_sum_ms" gorm:"not null;default:0"`
	LatencyCount             int64 `json:"latency_count" gorm:"not null;default:0"`
	TtftSumMs                int64 `json:"ttft_sum_ms" gorm:"not null;default:0"`
	TtftCount                int64 `json:"ttft_count" gorm:"not null;default:0"`
	DimensionOverflowCount   int64 `json:"dimension_overflow_count" gorm:"not null;default:0"`
	DroppedMetricEventCount  int64 `json:"dropped_metric_event_count" gorm:"not null;default:0"`
	DroppedFailureEventCount int64 `json:"dropped_failure_event_count" gorm:"not null;default:0"`

	LatencyBucket100Ms int64 `json:"latency_bucket_100_ms" gorm:"column:latency_bucket_100_ms;not null;default:0"`
	LatencyBucket250Ms int64 `json:"latency_bucket_250_ms" gorm:"column:latency_bucket_250_ms;not null;default:0"`
	LatencyBucket500Ms int64 `json:"latency_bucket_500_ms" gorm:"column:latency_bucket_500_ms;not null;default:0"`
	LatencyBucket1S    int64 `json:"latency_bucket_1s" gorm:"column:latency_bucket_1s;not null;default:0"`
	LatencyBucket2S    int64 `json:"latency_bucket_2s" gorm:"column:latency_bucket_2s;not null;default:0"`
	LatencyBucket4S    int64 `json:"latency_bucket_4s" gorm:"column:latency_bucket_4s;not null;default:0"`
	LatencyBucket8S    int64 `json:"latency_bucket_8s" gorm:"column:latency_bucket_8s;not null;default:0"`
	LatencyBucket15S   int64 `json:"latency_bucket_15s" gorm:"column:latency_bucket_15s;not null;default:0"`
	LatencyBucket30S   int64 `json:"latency_bucket_30s" gorm:"column:latency_bucket_30s;not null;default:0"`
	LatencyBucket60S   int64 `json:"latency_bucket_60s" gorm:"column:latency_bucket_60s;not null;default:0"`
	LatencyBucket120S  int64 `json:"latency_bucket_120s" gorm:"column:latency_bucket_120s;not null;default:0"`
	LatencyBucket300S  int64 `json:"latency_bucket_300s" gorm:"column:latency_bucket_300s;not null;default:0"`
	LatencyBucketInf   int64 `json:"latency_bucket_inf" gorm:"column:latency_bucket_inf;not null;default:0"`

	TtftBucket100Ms int64 `json:"ttft_bucket_100_ms" gorm:"column:ttft_bucket_100_ms;not null;default:0"`
	TtftBucket250Ms int64 `json:"ttft_bucket_250_ms" gorm:"column:ttft_bucket_250_ms;not null;default:0"`
	TtftBucket500Ms int64 `json:"ttft_bucket_500_ms" gorm:"column:ttft_bucket_500_ms;not null;default:0"`
	TtftBucket1S    int64 `json:"ttft_bucket_1s" gorm:"column:ttft_bucket_1s;not null;default:0"`
	TtftBucket2S    int64 `json:"ttft_bucket_2s" gorm:"column:ttft_bucket_2s;not null;default:0"`
	TtftBucket4S    int64 `json:"ttft_bucket_4s" gorm:"column:ttft_bucket_4s;not null;default:0"`
	TtftBucket8S    int64 `json:"ttft_bucket_8s" gorm:"column:ttft_bucket_8s;not null;default:0"`
	TtftBucket15S   int64 `json:"ttft_bucket_15s" gorm:"column:ttft_bucket_15s;not null;default:0"`
	TtftBucket30S   int64 `json:"ttft_bucket_30s" gorm:"column:ttft_bucket_30s;not null;default:0"`
	TtftBucket60S   int64 `json:"ttft_bucket_60s" gorm:"column:ttft_bucket_60s;not null;default:0"`
	TtftBucket120S  int64 `json:"ttft_bucket_120s" gorm:"column:ttft_bucket_120s;not null;default:0"`
	TtftBucket300S  int64 `json:"ttft_bucket_300s" gorm:"column:ttft_bucket_300s;not null;default:0"`
	TtftBucketInf   int64 `json:"ttft_bucket_inf" gorm:"column:ttft_bucket_inf;not null;default:0"`
}

func (ChannelMetricBucket) TableName() string { return "channel_metric_buckets" }

// ChannelMetricHistogramBucketIndex 返回非累计固定分箱下标，负值表示样本不适用。
func ChannelMetricHistogramBucketIndex(valueMs int64) int {
	if valueMs < 0 {
		return -1
	}
	for i, upperBound := range ChannelMetricHistogramUpperBoundsMs {
		if valueMs <= upperBound {
			return i
		}
	}
	return ChannelMetricHistogramBuckets - 1
}

func (bucket *ChannelMetricBucket) LatencyHistogram() [ChannelMetricHistogramBuckets]int64 {
	if bucket == nil {
		return [ChannelMetricHistogramBuckets]int64{}
	}
	return [ChannelMetricHistogramBuckets]int64{
		bucket.LatencyBucket100Ms, bucket.LatencyBucket250Ms, bucket.LatencyBucket500Ms,
		bucket.LatencyBucket1S, bucket.LatencyBucket2S, bucket.LatencyBucket4S, bucket.LatencyBucket8S,
		bucket.LatencyBucket15S, bucket.LatencyBucket30S, bucket.LatencyBucket60S,
		bucket.LatencyBucket120S, bucket.LatencyBucket300S, bucket.LatencyBucketInf,
	}
}

func (bucket *ChannelMetricBucket) SetLatencyHistogram(values [ChannelMetricHistogramBuckets]int64) {
	if bucket == nil {
		return
	}
	bucket.LatencyBucket100Ms, bucket.LatencyBucket250Ms, bucket.LatencyBucket500Ms = values[0], values[1], values[2]
	bucket.LatencyBucket1S, bucket.LatencyBucket2S, bucket.LatencyBucket4S, bucket.LatencyBucket8S = values[3], values[4], values[5], values[6]
	bucket.LatencyBucket15S, bucket.LatencyBucket30S, bucket.LatencyBucket60S = values[7], values[8], values[9]
	bucket.LatencyBucket120S, bucket.LatencyBucket300S, bucket.LatencyBucketInf = values[10], values[11], values[12]
}

func (bucket *ChannelMetricBucket) TtftHistogram() [ChannelMetricHistogramBuckets]int64 {
	if bucket == nil {
		return [ChannelMetricHistogramBuckets]int64{}
	}
	return [ChannelMetricHistogramBuckets]int64{
		bucket.TtftBucket100Ms, bucket.TtftBucket250Ms, bucket.TtftBucket500Ms,
		bucket.TtftBucket1S, bucket.TtftBucket2S, bucket.TtftBucket4S, bucket.TtftBucket8S,
		bucket.TtftBucket15S, bucket.TtftBucket30S, bucket.TtftBucket60S,
		bucket.TtftBucket120S, bucket.TtftBucket300S, bucket.TtftBucketInf,
	}
}

func (bucket *ChannelMetricBucket) SetTtftHistogram(values [ChannelMetricHistogramBuckets]int64) {
	if bucket == nil {
		return
	}
	bucket.TtftBucket100Ms, bucket.TtftBucket250Ms, bucket.TtftBucket500Ms = values[0], values[1], values[2]
	bucket.TtftBucket1S, bucket.TtftBucket2S, bucket.TtftBucket4S, bucket.TtftBucket8S = values[3], values[4], values[5], values[6]
	bucket.TtftBucket15S, bucket.TtftBucket30S, bucket.TtftBucket60S = values[7], values[8], values[9]
	bucket.TtftBucket120S, bucket.TtftBucket300S, bucket.TtftBucketInf = values[10], values[11], values[12]
}

// ChannelFailureEvent 是与完整业务日志解耦的最小化失败明细。
type ChannelFailureEvent struct {
	EventId                 string `json:"event_id" gorm:"column:event_id;size:64;primaryKey"`
	CreatedAt               int64  `json:"created_at" gorm:"not null;index:idx_cfe_created;index:idx_cfe_channel_time,priority:2;index:idx_cfe_outcome_time,priority:2;index:idx_cfe_upstatus_time,priority:3;index:idx_cfe_reqmodel_time,priority:2;index:idx_cfe_upmodel_time,priority:2;index:idx_cfe_origin_time,priority:2"`
	RequestId               string `json:"request_id" gorm:"size:128;not null;default:''"`
	AttemptSeq              int    `json:"attempt_seq" gorm:"not null;default:0"`
	RetryPlanned            bool   `json:"retry_planned" gorm:"not null;default:false"`
	IsLastStartedAttempt    bool   `json:"is_last_started_attempt" gorm:"not null;default:false"`
	CausalCallPresent       bool   `json:"causal_call_present" gorm:"not null;default:false"`
	CausalCallIndex         int    `json:"causal_call_index" gorm:"not null;default:0"`
	ChannelId               int    `json:"channel_id" gorm:"not null;default:0;index:idx_cfe_channel_time,priority:1"`
	ChannelNameSnapshot     string `json:"channel_name_snapshot" gorm:"size:191;not null;default:''"`
	ChannelType             int    `json:"channel_type" gorm:"not null;default:0"`
	RequestedModel          string `json:"requested_model" gorm:"size:191;not null;default:''"`
	RequestedModelHash      string `json:"requested_model_hash" gorm:"size:64;not null;default:'';index:idx_cfe_reqmodel_time,priority:1"`
	UpstreamModel           string `json:"upstream_model" gorm:"size:191;not null;default:''"`
	UpstreamModelHash       string `json:"upstream_model_hash" gorm:"size:64;not null;default:'';index:idx_cfe_upmodel_time,priority:1"`
	Group                   string `json:"group" gorm:"column:group;size:64;not null;default:''"`
	TrafficSource           string `json:"traffic_source" gorm:"size:16;not null;default:''"`
	DataOrigin              string `json:"data_origin" gorm:"size:16;not null;default:'live';index:idx_cfe_origin_time,priority:1"`
	Outcome                 string `json:"outcome" gorm:"size:24;not null;default:'';index:idx_cfe_outcome_time,priority:1"`
	FailureOwner            string `json:"failure_owner" gorm:"size:16;not null;default:''"`
	QualityEligible         bool   `json:"quality_eligible" gorm:"not null;default:false"`
	PartialResponse         bool   `json:"partial_response" gorm:"not null;default:false"`
	ErrorStage              string `json:"error_stage" gorm:"size:32;not null;default:''"`
	StreamEndReason         string `json:"stream_end_reason" gorm:"size:64;not null;default:''"`
	UpstreamStatusPresent   bool   `json:"upstream_status_present" gorm:"not null;default:false;index:idx_cfe_upstatus_time,priority:1"`
	UpstreamStatusCode      int    `json:"upstream_status_code" gorm:"not null;default:0;index:idx_cfe_upstatus_time,priority:2"`
	NormalizedStatusPresent bool   `json:"normalized_status_present" gorm:"not null;default:false"`
	NormalizedStatusCode    int    `json:"normalized_status_code" gorm:"not null;default:0"`
	ClientStatusPresent     bool   `json:"client_status_present" gorm:"not null;default:false"`
	ClientStatusCode        int    `json:"client_status_code" gorm:"not null;default:0"`
	LatencyMs               int64  `json:"latency_ms" gorm:"not null;default:0"`
	TtftPresent             bool   `json:"ttft_present" gorm:"not null;default:false"`
	TtftMs                  int64  `json:"ttft_ms" gorm:"not null;default:0"`
	RetryReason             string `json:"retry_reason" gorm:"size:128;not null;default:''"`
	MaskedErrorSummary      string `json:"masked_error_summary" gorm:"size:512;not null;default:''"`
}

func (ChannelFailureEvent) TableName() string { return "channel_failure_events" }

// InsertChannelFailureEvents 使用稳定 event_id 去重，可供有界异步失败队列独立重试。
func InsertChannelFailureEvents(db *gorm.DB, events []ChannelFailureEvent) error {
	if db == nil {
		return ErrChannelMetricInvalidBatch
	}
	if len(events) == 0 {
		return nil
	}
	for i := range events {
		if strings.TrimSpace(events[i].EventId) == "" || events[i].CreatedAt <= 0 {
			return fmt.Errorf("%w: invalid failure event at index %d", ErrChannelMetricInvalidBatch, i)
		}
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_id"}},
		DoNothing: true,
	}).CreateInBatches(&events, 100).Error
}

// ChannelMetricFlush 用于在重试后识别已提交的增量批次。
type ChannelMetricFlush struct {
	FlushId                     string `json:"flush_id" gorm:"column:flush_id;size:64;primaryKey"`
	InstanceId                  string `json:"instance_id" gorm:"size:64;not null"`
	BatchCreatedAt              int64  `json:"batch_created_at" gorm:"not null"`
	CommittedAt                 int64  `json:"committed_at" gorm:"not null;index:idx_cmf_committed"`
	InvalidSampleCount          int64  `json:"invalid_sample_count" gorm:"not null;default:0"`
	DimensionOverflowCount      int64  `json:"dimension_overflow_count" gorm:"not null;default:0"`
	DroppedMetricEventCount     int64  `json:"dropped_metric_event_count" gorm:"not null;default:0"`
	DroppedFailureEventCount    int64  `json:"dropped_failure_event_count" gorm:"not null;default:0"`
	DimensionHashCollisionCount int64  `json:"dimension_hash_collision_count" gorm:"not null;default:0"`
	ClaimToken                  string `json:"-" gorm:"size:32;not null;default:''"`
}

func (ChannelMetricFlush) TableName() string { return "channel_metric_flushes" }

// ChannelMetricBackfillJob 保存历史回填的可恢复检查点。
type ChannelMetricBackfillJob struct {
	JobId             string `json:"job_id" gorm:"column:job_id;size:64;primaryKey"`
	Status            string `json:"status" gorm:"size:24;not null;index:idx_cmbj_status_updated,priority:1"`
	BackfillStartTs   int64  `json:"backfill_start_ts" gorm:"not null;default:0"`
	LiveCutoverTs     int64  `json:"live_cutover_ts" gorm:"not null;default:0"`
	MaxLogId          int64  `json:"max_log_id" gorm:"not null;default:0"`
	TotalRows         int64  `json:"total_rows" gorm:"not null;default:0"`
	CurrentCursor     int64  `json:"current_cursor" gorm:"not null;default:0"`
	ScannedRows       int64  `json:"scanned_rows" gorm:"not null;default:0"`
	ConvertedRows     int64  `json:"converted_rows" gorm:"not null;default:0"`
	SkippedRows       int64  `json:"skipped_rows" gorm:"not null;default:0"`
	MetricBucketCount int64  `json:"metric_bucket_count" gorm:"not null;default:0"`
	FailureEventCount int64  `json:"failure_event_count" gorm:"not null;default:0"`
	LastError         string `json:"last_error" gorm:"type:text"`
	CreatedAt         int64  `json:"created_at" gorm:"not null"`
	UpdatedAt         int64  `json:"updated_at" gorm:"not null;index:idx_cmbj_status_updated,priority:2"`
	CompletedAt       int64  `json:"completed_at" gorm:"not null;default:0"`
}

func (ChannelMetricBackfillJob) TableName() string { return "channel_metric_backfill_jobs" }

// MigrateChannelAnalyticsLogDB 只在调用方传入的日志库上创建事实表。
func MigrateChannelAnalyticsLogDB(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("%w: log database is nil", ErrChannelMetricInvalidBatch)
	}
	return db.AutoMigrate(
		&ChannelMetricBucket{},
		&ChannelFailureEvent{},
		&ChannelMetricFlush{},
		&ChannelMetricBackfillJob{},
	)
}

// FlushChannelMetrics 在同一日志库事务中完成批次去重、增量累加和失败明细去重。
// applied=false 表示该 flush_id 已成功提交，调用方应直接丢弃重试批次。
func FlushChannelMetrics(db *gorm.DB, flush *ChannelMetricFlush, buckets []ChannelMetricBucket, failures []ChannelFailureEvent) (applied bool, err error) {
	if db == nil || flush == nil || strings.TrimSpace(flush.FlushId) == "" || strings.TrimSpace(flush.InstanceId) == "" || flush.BatchCreatedAt <= 0 {
		return false, ErrChannelMetricInvalidBatch
	}
	for i := range failures {
		if strings.TrimSpace(failures[i].EventId) == "" || failures[i].CreatedAt <= 0 {
			return false, fmt.Errorf("%w: invalid failure event at index %d", ErrChannelMetricInvalidBatch, i)
		}
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		flushRecord := *flush
		if flushRecord.CommittedAt <= 0 {
			flushRecord.CommittedAt = time.Now().Unix()
		}
		flushRecord.ClaimToken = common.GetUUID()
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "flush_id"}},
			DoNothing: true,
		}).Create(&flushRecord)
		if result.Error != nil {
			return result.Error
		}
		// MySQL 对 ON DUPLICATE KEY 的 RowsAffected 语义受 clientFoundRows 影响，
		// 因此用本次事务独有的 claim token 判断是否真正插入。
		var existing ChannelMetricFlush
		if err := tx.Where("flush_id = ?", flushRecord.FlushId).Take(&existing).Error; err != nil {
			return err
		}
		if existing.InstanceId != flushRecord.InstanceId || existing.BatchCreatedAt != flushRecord.BatchCreatedAt {
			return fmt.Errorf("%w: flush id %q belongs to another batch", ErrChannelMetricInvalidBatch, flushRecord.FlushId)
		}
		if existing.ClaimToken != flushRecord.ClaimToken {
			applied = false
			return nil
		}

		for i := range buckets {
			bucket := buckets[i]
			if err := normalizeAndVerifyChannelMetricBucket(tx, &bucket); err != nil {
				return fmt.Errorf("channel metric bucket %d: %w", i, err)
			}
			if err := upsertChannelMetricBucketIncrement(tx, &bucket); err != nil {
				return err
			}
		}
		if err := InsertChannelFailureEvents(tx, failures); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

// UpsertChannelMetricBucketIncrement 只能用于实时 drain，所有数值字段均以增量累加。
func UpsertChannelMetricBucketIncrement(db *gorm.DB, bucket *ChannelMetricBucket) error {
	if db == nil || bucket == nil {
		return ErrChannelMetricInvalidBatch
	}
	copyBucket := *bucket
	if err := normalizeAndVerifyChannelMetricBucket(db, &copyBucket); err != nil {
		return err
	}
	return upsertChannelMetricBucketIncrement(db, &copyBucket)
}

func upsertChannelMetricBucketIncrement(db *gorm.DB, bucket *ChannelMetricBucket) error {
	bucket.Id = 0
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bucket_level"}, {Name: "bucket_ts"}, {Name: "dimension_hash"}},
		DoUpdates: clause.Assignments(channelMetricIncrementAssignments(bucket)),
	}).Create(bucket).Error
}

// UpsertChannelMetricBucketOverwrite 仅用于降采样或指定时间窗重算，不会与实时增量语义混用。
func UpsertChannelMetricBucketOverwrite(db *gorm.DB, bucket *ChannelMetricBucket) error {
	if db == nil || bucket == nil {
		return ErrChannelMetricInvalidBatch
	}
	copyBucket := *bucket
	if err := normalizeAndVerifyChannelMetricBucket(db, &copyBucket); err != nil {
		return err
	}
	copyBucket.Id = 0
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bucket_level"}, {Name: "bucket_ts"}, {Name: "dimension_hash"}},
		DoUpdates: clause.AssignmentColumns(channelMetricOverwriteColumns),
	}).Create(&copyBucket).Error
}

func normalizeAndVerifyChannelMetricBucket(db *gorm.DB, bucket *ChannelMetricBucket) error {
	if bucket.DimensionVersion == 0 {
		bucket.DimensionVersion = ChannelMetricDimensionVersion
	}
	if bucket.BucketLevel == "" || bucket.BucketTs <= 0 || bucket.MetricScope == "" || bucket.TrafficSource == "" || bucket.DataOrigin == "" {
		return ErrChannelMetricInvalidBatch
	}
	if len(bucket.DimensionHash) != 64 || strings.ToLower(bucket.DimensionHash) != bucket.DimensionHash {
		return fmt.Errorf("%w: dimension hash must be a 64-character lowercase SHA-256", ErrChannelMetricInvalidBatch)
	}
	for _, r := range bucket.DimensionHash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return fmt.Errorf("%w: dimension hash is not hexadecimal", ErrChannelMetricInvalidBatch)
		}
	}
	if bucket.DimensionVersion != ChannelMetricDimensionVersion {
		return fmt.Errorf("%w: unsupported dimension version %d", ErrChannelMetricInvalidBatch, bucket.DimensionVersion)
	}

	dimension := channelMetricDimensionOf(bucket)
	cacheKey := channelMetricDimensionCacheKeyFor(db, bucket.DimensionHash)
	if cached, ok := channelMetricVerifiedDimensions.Load(cacheKey); ok {
		if cached != dimension {
			return fmt.Errorf("%w: %s", ErrChannelMetricHashCollision, bucket.DimensionHash)
		}
		return nil
	}
	channelMetricDimensionVerifyMu.Lock()
	defer channelMetricDimensionVerifyMu.Unlock()
	if cached, ok := channelMetricVerifiedDimensions.Load(cacheKey); ok {
		if cached != dimension {
			return fmt.Errorf("%w: %s", ErrChannelMetricHashCollision, bucket.DimensionHash)
		}
		return nil
	}

	var existing ChannelMetricBucket
	err := db.Select(channelMetricDimensionSelectColumns).
		Where("dimension_hash = ?", bucket.DimensionHash).
		Limit(1).
		Take(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && channelMetricDimensionOf(&existing) != dimension {
		return fmt.Errorf("%w: %s", ErrChannelMetricHashCollision, bucket.DimensionHash)
	}
	// 高基数模型不应让进程级验证缓存无界增长。
	if channelMetricVerifiedCacheSize >= channelMetricVerifiedCacheMax {
		channelMetricVerifiedDimensions.Clear()
		channelMetricVerifiedCacheSize = 0
	}
	channelMetricVerifiedDimensions.Store(cacheKey, dimension)
	channelMetricVerifiedCacheSize++
	return nil
}

type channelMetricDimensionCacheKey struct {
	Database interface{}
	Hash     string
}

func channelMetricDimensionCacheKeyFor(db *gorm.DB, hash string) channelMetricDimensionCacheKey {
	database := interface{}(db)
	if sqlDB, err := db.DB(); err == nil {
		database = sqlDB
	}
	return channelMetricDimensionCacheKey{Database: database, Hash: hash}
}

type channelMetricDimension struct {
	Version                 int
	MetricScope             string
	ChannelPresent          bool
	ChannelId               int
	ChannelNameHash         string
	ChannelType             int
	RequestedModelPresent   bool
	RequestedModelHash      string
	UpstreamModelPresent    bool
	UpstreamModelHash       string
	GroupHash               string
	TrafficSource           string
	DataOrigin              string
	Stream                  bool
	Outcome                 string
	ErrorStage              string
	FailureOwner            string
	QualityEligible         bool
	PartialResponse         bool
	Overflowed              bool
	ClientStatusPresent     bool
	ClientStatusCode        int
	UpstreamStatusPresent   bool
	UpstreamStatusCode      int
	NormalizedStatusPresent bool
	NormalizedStatusCode    int
}

func channelMetricDimensionOf(bucket *ChannelMetricBucket) channelMetricDimension {
	return channelMetricDimension{
		Version: bucket.DimensionVersion, MetricScope: bucket.MetricScope,
		ChannelPresent: bucket.ChannelPresent, ChannelId: bucket.ChannelId,
		ChannelNameHash: bucket.ChannelNameHash, ChannelType: bucket.ChannelType,
		RequestedModelPresent: bucket.RequestedModelPresent,
		RequestedModelHash:    bucket.RequestedModelHash, UpstreamModelPresent: bucket.UpstreamModelPresent,
		UpstreamModelHash: bucket.UpstreamModelHash, GroupHash: bucket.GroupHash,
		TrafficSource: bucket.TrafficSource, DataOrigin: bucket.DataOrigin, Stream: bucket.Stream,
		Outcome: bucket.Outcome, ErrorStage: bucket.ErrorStage, FailureOwner: bucket.FailureOwner,
		QualityEligible: bucket.QualityEligible, PartialResponse: bucket.PartialResponse, Overflowed: bucket.Overflowed,
		ClientStatusPresent: bucket.ClientStatusPresent, ClientStatusCode: bucket.ClientStatusCode,
		UpstreamStatusPresent: bucket.UpstreamStatusPresent, UpstreamStatusCode: bucket.UpstreamStatusCode,
		NormalizedStatusPresent: bucket.NormalizedStatusPresent, NormalizedStatusCode: bucket.NormalizedStatusCode,
	}
}

func channelMetricIncrementAssignments(bucket *ChannelMetricBucket) map[string]interface{} {
	values := map[string]int64{
		"event_count": bucket.EventCount, "success_count": bucket.SuccessCount,
		"non_first_attempt_count": bucket.NonFirstAttemptCount, "retry_planned_count": bucket.RetryPlannedCount,
		"quality_eligible_count": bucket.QualityEligibleCount, "quality_success_count": bucket.QualitySuccessCount,
		"partial_response_count": bucket.PartialResponseCount, "usage_sample_count": bucket.UsageSampleCount,
		"cache_hit_request_count": bucket.CacheHitRequestCount, "input_tokens_total": bucket.InputTokensTotal,
		"uncached_input_tokens": bucket.UncachedInputTokens, "output_tokens": bucket.OutputTokens,
		"cache_read_tokens": bucket.CacheReadTokens, "cache_write_tokens": bucket.CacheWriteTokens,
		"charged_quota": bucket.ChargedQuota, "charged_micro_usd": bucket.ChargedMicroUsd,
		"latency_sum_ms": bucket.LatencySumMs, "latency_count": bucket.LatencyCount,
		"ttft_sum_ms": bucket.TtftSumMs, "ttft_count": bucket.TtftCount,
		"dimension_overflow_count":    bucket.DimensionOverflowCount,
		"dropped_metric_event_count":  bucket.DroppedMetricEventCount,
		"dropped_failure_event_count": bucket.DroppedFailureEventCount,
		"latency_bucket_100_ms":       bucket.LatencyBucket100Ms, "latency_bucket_250_ms": bucket.LatencyBucket250Ms,
		"latency_bucket_500_ms": bucket.LatencyBucket500Ms, "latency_bucket_1s": bucket.LatencyBucket1S,
		"latency_bucket_2s": bucket.LatencyBucket2S, "latency_bucket_4s": bucket.LatencyBucket4S,
		"latency_bucket_8s": bucket.LatencyBucket8S, "latency_bucket_15s": bucket.LatencyBucket15S,
		"latency_bucket_30s": bucket.LatencyBucket30S, "latency_bucket_60s": bucket.LatencyBucket60S,
		"latency_bucket_120s": bucket.LatencyBucket120S, "latency_bucket_300s": bucket.LatencyBucket300S,
		"latency_bucket_inf": bucket.LatencyBucketInf,
		"ttft_bucket_100_ms": bucket.TtftBucket100Ms, "ttft_bucket_250_ms": bucket.TtftBucket250Ms,
		"ttft_bucket_500_ms": bucket.TtftBucket500Ms, "ttft_bucket_1s": bucket.TtftBucket1S,
		"ttft_bucket_2s": bucket.TtftBucket2S, "ttft_bucket_4s": bucket.TtftBucket4S,
		"ttft_bucket_8s": bucket.TtftBucket8S, "ttft_bucket_15s": bucket.TtftBucket15S,
		"ttft_bucket_30s": bucket.TtftBucket30S, "ttft_bucket_60s": bucket.TtftBucket60S,
		"ttft_bucket_120s": bucket.TtftBucket120S, "ttft_bucket_300s": bucket.TtftBucket300S,
		"ttft_bucket_inf": bucket.TtftBucketInf,
	}
	assignments := make(map[string]interface{}, len(values))
	for column, value := range values {
		assignments[column] = gorm.Expr("channel_metric_buckets."+column+" + ?", value)
	}
	return assignments
}

var channelMetricOverwriteColumns = []string{
	"dimension_version", "metric_scope", "channel_present", "channel_id", "channel_name_snapshot", "channel_name_hash", "channel_type",
	"requested_model_present", "requested_model", "requested_model_hash", "upstream_model_present", "upstream_model", "upstream_model_hash", "group",
	"group_hash", "traffic_source", "data_origin", "stream", "outcome", "error_stage", "failure_owner", "quality_eligible", "partial_response", "overflowed",
	"client_status_present", "client_status_code", "upstream_status_present", "upstream_status_code", "normalized_status_present", "normalized_status_code",
	"event_count", "success_count", "non_first_attempt_count", "retry_planned_count", "quality_eligible_count", "quality_success_count",
	"partial_response_count", "usage_sample_count", "cache_hit_request_count", "input_tokens_total", "uncached_input_tokens", "output_tokens",
	"cache_read_tokens", "cache_write_tokens", "charged_quota", "charged_micro_usd", "latency_sum_ms", "latency_count", "ttft_sum_ms", "ttft_count",
	"dimension_overflow_count", "dropped_metric_event_count", "dropped_failure_event_count",
	"latency_bucket_100_ms", "latency_bucket_250_ms", "latency_bucket_500_ms", "latency_bucket_1s", "latency_bucket_2s", "latency_bucket_4s",
	"latency_bucket_8s", "latency_bucket_15s", "latency_bucket_30s", "latency_bucket_60s", "latency_bucket_120s", "latency_bucket_300s", "latency_bucket_inf",
	"ttft_bucket_100_ms", "ttft_bucket_250_ms", "ttft_bucket_500_ms", "ttft_bucket_1s", "ttft_bucket_2s", "ttft_bucket_4s",
	"ttft_bucket_8s", "ttft_bucket_15s", "ttft_bucket_30s", "ttft_bucket_60s", "ttft_bucket_120s", "ttft_bucket_300s", "ttft_bucket_inf",
}

var channelMetricDimensionSelectColumns = []string{
	"dimension_version", "metric_scope", "channel_present", "channel_id", "channel_name_hash", "channel_type",
	"requested_model_present", "requested_model_hash", "upstream_model_present", "upstream_model_hash",
	"group_hash", "traffic_source", "data_origin", "stream", "outcome", "error_stage", "failure_owner", "quality_eligible", "partial_response", "overflowed",
	"client_status_present", "client_status_code", "upstream_status_present", "upstream_status_code", "normalized_status_present", "normalized_status_code",
}

// SaveChannelMetricBackfillJob 可以接收事务句柄，便于检查点与本批聚合同事务提交。
func SaveChannelMetricBackfillJob(db *gorm.DB, job *ChannelMetricBackfillJob) error {
	if db == nil || job == nil || strings.TrimSpace(job.JobId) == "" {
		return ErrChannelMetricInvalidBatch
	}
	copyJob := *job
	now := time.Now().Unix()
	if copyJob.CreatedAt <= 0 {
		copyJob.CreatedAt = now
	}
	if copyJob.UpdatedAt <= 0 {
		copyJob.UpdatedAt = now
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "backfill_start_ts", "live_cutover_ts", "max_log_id", "total_rows", "current_cursor",
			"scanned_rows", "converted_rows", "skipped_rows", "metric_bucket_count", "failure_event_count",
			"last_error", "updated_at", "completed_at",
		}),
	}).Create(&copyJob).Error
}

func GetChannelMetricBackfillJob(db *gorm.DB, jobId string) (*ChannelMetricBackfillJob, error) {
	if db == nil || strings.TrimSpace(jobId) == "" {
		return nil, ErrChannelMetricInvalidBatch
	}
	var job ChannelMetricBackfillJob
	result := db.Where("job_id = ?", jobId).Limit(1).Find(&job)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &job, nil
}
