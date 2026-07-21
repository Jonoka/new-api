package model

import (
	"strings"

	"gorm.io/gorm"
)

// ChannelMetricAggregateRow 是渠道指标桶经数据库侧聚合后的通用结果。
// 只使用 SUM、MIN、MAX 和 GROUP BY，保证 SQLite、MySQL 和 PostgreSQL 行为一致。
type ChannelMetricAggregateRow struct {
	BucketTs int64 `json:"bucket_ts"`

	ChannelId           int    `json:"channel_id"`
	ChannelNameSnapshot string `json:"channel_name_snapshot"`
	ChannelType         int    `json:"channel_type"`
	RequestedModel      string `json:"requested_model"`
	RequestedModelHash  string `json:"requested_model_hash"`
	UpstreamModel       string `json:"upstream_model"`
	UpstreamModelHash   string `json:"upstream_model_hash"`

	StatusPresent bool   `json:"status_present"`
	StatusCode    int    `json:"status_code"`
	ErrorStage    string `json:"error_stage"`

	EventCount               int64 `json:"event_count"`
	SuccessCount             int64 `json:"success_count"`
	NonFirstAttemptCount     int64 `json:"non_first_attempt_count"`
	RetryPlannedCount        int64 `json:"retry_planned_count"`
	QualityEligibleCount     int64 `json:"quality_eligible_count"`
	QualitySuccessCount      int64 `json:"quality_success_count"`
	PartialResponseCount     int64 `json:"partial_response_count"`
	UsageSampleCount         int64 `json:"usage_sample_count"`
	CacheHitRequestCount     int64 `json:"cache_hit_request_count"`
	InputTokensTotal         int64 `json:"input_tokens_total"`
	UncachedInputTokens      int64 `json:"uncached_input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadTokens          int64 `json:"cache_read_tokens"`
	CacheWriteTokens         int64 `json:"cache_write_tokens"`
	ChargedQuota             int64 `json:"charged_quota"`
	ChargedMicroUsd          int64 `json:"charged_micro_usd"`
	LatencySumMs             int64 `json:"latency_sum_ms"`
	LatencyCount             int64 `json:"latency_count"`
	TtftSumMs                int64 `json:"ttft_sum_ms"`
	TtftCount                int64 `json:"ttft_count"`
	DimensionOverflowCount   int64 `json:"dimension_overflow_count"`
	DroppedMetricEventCount  int64 `json:"dropped_metric_event_count"`
	DroppedFailureEventCount int64 `json:"dropped_failure_event_count"`

	LatencyBucket100Ms int64 `json:"latency_bucket_100_ms" gorm:"column:latency_bucket_100_ms"`
	LatencyBucket250Ms int64 `json:"latency_bucket_250_ms" gorm:"column:latency_bucket_250_ms"`
	LatencyBucket500Ms int64 `json:"latency_bucket_500_ms" gorm:"column:latency_bucket_500_ms"`
	LatencyBucket1S    int64 `json:"latency_bucket_1s" gorm:"column:latency_bucket_1s"`
	LatencyBucket2S    int64 `json:"latency_bucket_2s" gorm:"column:latency_bucket_2s"`
	LatencyBucket4S    int64 `json:"latency_bucket_4s" gorm:"column:latency_bucket_4s"`
	LatencyBucket8S    int64 `json:"latency_bucket_8s" gorm:"column:latency_bucket_8s"`
	LatencyBucket15S   int64 `json:"latency_bucket_15s" gorm:"column:latency_bucket_15s"`
	LatencyBucket30S   int64 `json:"latency_bucket_30s" gorm:"column:latency_bucket_30s"`
	LatencyBucket60S   int64 `json:"latency_bucket_60s" gorm:"column:latency_bucket_60s"`
	LatencyBucket120S  int64 `json:"latency_bucket_120s" gorm:"column:latency_bucket_120s"`
	LatencyBucket300S  int64 `json:"latency_bucket_300s" gorm:"column:latency_bucket_300s"`
	LatencyBucketInf   int64 `json:"latency_bucket_inf" gorm:"column:latency_bucket_inf"`
	TtftBucket100Ms    int64 `json:"ttft_bucket_100_ms" gorm:"column:ttft_bucket_100_ms"`
	TtftBucket250Ms    int64 `json:"ttft_bucket_250_ms" gorm:"column:ttft_bucket_250_ms"`
	TtftBucket500Ms    int64 `json:"ttft_bucket_500_ms" gorm:"column:ttft_bucket_500_ms"`
	TtftBucket1S       int64 `json:"ttft_bucket_1s" gorm:"column:ttft_bucket_1s"`
	TtftBucket2S       int64 `json:"ttft_bucket_2s" gorm:"column:ttft_bucket_2s"`
	TtftBucket4S       int64 `json:"ttft_bucket_4s" gorm:"column:ttft_bucket_4s"`
	TtftBucket8S       int64 `json:"ttft_bucket_8s" gorm:"column:ttft_bucket_8s"`
	TtftBucket15S      int64 `json:"ttft_bucket_15s" gorm:"column:ttft_bucket_15s"`
	TtftBucket30S      int64 `json:"ttft_bucket_30s" gorm:"column:ttft_bucket_30s"`
	TtftBucket60S      int64 `json:"ttft_bucket_60s" gorm:"column:ttft_bucket_60s"`
	TtftBucket120S     int64 `json:"ttft_bucket_120s" gorm:"column:ttft_bucket_120s"`
	TtftBucket300S     int64 `json:"ttft_bucket_300s" gorm:"column:ttft_bucket_300s"`
	TtftBucketInf      int64 `json:"ttft_bucket_inf" gorm:"column:ttft_bucket_inf"`
}

func (row ChannelMetricAggregateRow) LatencyHistogram() [ChannelMetricHistogramBuckets]int64 {
	return [ChannelMetricHistogramBuckets]int64{
		row.LatencyBucket100Ms, row.LatencyBucket250Ms, row.LatencyBucket500Ms,
		row.LatencyBucket1S, row.LatencyBucket2S, row.LatencyBucket4S, row.LatencyBucket8S,
		row.LatencyBucket15S, row.LatencyBucket30S, row.LatencyBucket60S,
		row.LatencyBucket120S, row.LatencyBucket300S, row.LatencyBucketInf,
	}
}

func (row ChannelMetricAggregateRow) TtftHistogram() [ChannelMetricHistogramBuckets]int64 {
	return [ChannelMetricHistogramBuckets]int64{
		row.TtftBucket100Ms, row.TtftBucket250Ms, row.TtftBucket500Ms,
		row.TtftBucket1S, row.TtftBucket2S, row.TtftBucket4S, row.TtftBucket8S,
		row.TtftBucket15S, row.TtftBucket30S, row.TtftBucket60S,
		row.TtftBucket120S, row.TtftBucket300S, row.TtftBucketInf,
	}
}

var channelMetricCounterAggregateColumns = []string{
	"COALESCE(SUM(event_count), 0) AS event_count",
	"COALESCE(SUM(success_count), 0) AS success_count",
	"COALESCE(SUM(non_first_attempt_count), 0) AS non_first_attempt_count",
	"COALESCE(SUM(retry_planned_count), 0) AS retry_planned_count",
	"COALESCE(SUM(quality_eligible_count), 0) AS quality_eligible_count",
	"COALESCE(SUM(quality_success_count), 0) AS quality_success_count",
	"COALESCE(SUM(partial_response_count), 0) AS partial_response_count",
	"COALESCE(SUM(usage_sample_count), 0) AS usage_sample_count",
	"COALESCE(SUM(cache_hit_request_count), 0) AS cache_hit_request_count",
	"COALESCE(SUM(input_tokens_total), 0) AS input_tokens_total",
	"COALESCE(SUM(uncached_input_tokens), 0) AS uncached_input_tokens",
	"COALESCE(SUM(output_tokens), 0) AS output_tokens",
	"COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens",
	"COALESCE(SUM(cache_write_tokens), 0) AS cache_write_tokens",
	"COALESCE(SUM(charged_quota), 0) AS charged_quota",
	"COALESCE(SUM(charged_micro_usd), 0) AS charged_micro_usd",
	"COALESCE(SUM(latency_sum_ms), 0) AS latency_sum_ms",
	"COALESCE(SUM(latency_count), 0) AS latency_count",
	"COALESCE(SUM(ttft_sum_ms), 0) AS ttft_sum_ms",
	"COALESCE(SUM(ttft_count), 0) AS ttft_count",
	"COALESCE(SUM(dimension_overflow_count), 0) AS dimension_overflow_count",
	"COALESCE(SUM(dropped_metric_event_count), 0) AS dropped_metric_event_count",
	"COALESCE(SUM(dropped_failure_event_count), 0) AS dropped_failure_event_count",
	"COALESCE(SUM(latency_bucket_100_ms), 0) AS latency_bucket_100_ms",
	"COALESCE(SUM(latency_bucket_250_ms), 0) AS latency_bucket_250_ms",
	"COALESCE(SUM(latency_bucket_500_ms), 0) AS latency_bucket_500_ms",
	"COALESCE(SUM(latency_bucket_1s), 0) AS latency_bucket_1s",
	"COALESCE(SUM(latency_bucket_2s), 0) AS latency_bucket_2s",
	"COALESCE(SUM(latency_bucket_4s), 0) AS latency_bucket_4s",
	"COALESCE(SUM(latency_bucket_8s), 0) AS latency_bucket_8s",
	"COALESCE(SUM(latency_bucket_15s), 0) AS latency_bucket_15s",
	"COALESCE(SUM(latency_bucket_30s), 0) AS latency_bucket_30s",
	"COALESCE(SUM(latency_bucket_60s), 0) AS latency_bucket_60s",
	"COALESCE(SUM(latency_bucket_120s), 0) AS latency_bucket_120s",
	"COALESCE(SUM(latency_bucket_300s), 0) AS latency_bucket_300s",
	"COALESCE(SUM(latency_bucket_inf), 0) AS latency_bucket_inf",
	"COALESCE(SUM(ttft_bucket_100_ms), 0) AS ttft_bucket_100_ms",
	"COALESCE(SUM(ttft_bucket_250_ms), 0) AS ttft_bucket_250_ms",
	"COALESCE(SUM(ttft_bucket_500_ms), 0) AS ttft_bucket_500_ms",
	"COALESCE(SUM(ttft_bucket_1s), 0) AS ttft_bucket_1s",
	"COALESCE(SUM(ttft_bucket_2s), 0) AS ttft_bucket_2s",
	"COALESCE(SUM(ttft_bucket_4s), 0) AS ttft_bucket_4s",
	"COALESCE(SUM(ttft_bucket_8s), 0) AS ttft_bucket_8s",
	"COALESCE(SUM(ttft_bucket_15s), 0) AS ttft_bucket_15s",
	"COALESCE(SUM(ttft_bucket_30s), 0) AS ttft_bucket_30s",
	"COALESCE(SUM(ttft_bucket_60s), 0) AS ttft_bucket_60s",
	"COALESCE(SUM(ttft_bucket_120s), 0) AS ttft_bucket_120s",
	"COALESCE(SUM(ttft_bucket_300s), 0) AS ttft_bucket_300s",
	"COALESCE(SUM(ttft_bucket_inf), 0) AS ttft_bucket_inf",
}

func queryChannelMetricAggregate(db *gorm.DB, filter ChannelMetricBucketFilter, dimensions []string) ([]ChannelMetricAggregateRow, error) {
	if db == nil {
		return nil, ErrChannelMetricInvalidBatch
	}
	query, err := applyChannelMetricBucketFilter(db.Model(&ChannelMetricBucket{}), filter)
	if err != nil {
		return nil, err
	}
	selectColumns := append(append([]string(nil), dimensions...), channelMetricCounterAggregateColumns...)
	query = query.Select(strings.Join(selectColumns, ", "))
	for _, dimension := range dimensions {
		groupColumn := dimension
		if aliasAt := strings.Index(strings.ToUpper(groupColumn), " AS "); aliasAt >= 0 {
			groupColumn = groupColumn[:aliasAt]
		}
		query = query.Group(groupColumn)
	}
	var rows []ChannelMetricAggregateRow
	if len(dimensions) == 0 {
		rows = make([]ChannelMetricAggregateRow, 1)
		if err := query.Scan(&rows[0]).Error; err != nil {
			return nil, err
		}
		return rows, nil
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func AggregateChannelMetricTotals(db *gorm.DB, filter ChannelMetricBucketFilter) (ChannelMetricAggregateRow, error) {
	rows, err := queryChannelMetricAggregate(db, filter, nil)
	if err != nil || len(rows) == 0 {
		return ChannelMetricAggregateRow{}, err
	}
	return rows[0], nil
}

func AggregateChannelMetricTrend(db *gorm.DB, filter ChannelMetricBucketFilter) ([]ChannelMetricAggregateRow, error) {
	rows, err := queryChannelMetricAggregate(db, filter, []string{"bucket_ts"})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func AggregateChannelMetricsByChannel(db *gorm.DB, filter ChannelMetricBucketFilter) ([]ChannelMetricAggregateRow, error) {
	return queryChannelMetricAggregate(db, filter, []string{"channel_id", "channel_name_snapshot", "channel_type"})
}

func AggregateChannelMetricsByModel(db *gorm.DB, filter ChannelMetricBucketFilter, upstream bool) ([]ChannelMetricAggregateRow, error) {
	if upstream {
		return queryChannelMetricAggregate(db, filter, []string{"channel_name_snapshot", "channel_type", "upstream_model_hash", "upstream_model"})
	}
	return queryChannelMetricAggregate(db, filter, []string{"channel_name_snapshot", "channel_type", "requested_model_hash", "requested_model"})
}

func AggregateChannelMetricStatusCodes(db *gorm.DB, filter ChannelMetricBucketFilter, client bool) ([]ChannelMetricAggregateRow, error) {
	presentColumn, codeColumn := "upstream_status_present", "upstream_status_code"
	if client {
		presentColumn, codeColumn = "client_status_present", "client_status_code"
	}
	return aggregateChannelMetricStatusCodes(db, filter, nil, presentColumn, codeColumn)
}

func aggregateChannelMetricStatusCodes(db *gorm.DB, filter ChannelMetricBucketFilter, dimensions []string, presentColumn string, codeColumn string) ([]ChannelMetricAggregateRow, error) {
	if db == nil {
		return nil, ErrChannelMetricInvalidBatch
	}
	query, err := applyChannelMetricBucketFilter(db.Model(&ChannelMetricBucket{}), filter)
	if err != nil {
		return nil, err
	}
	selectColumns := append([]string(nil), dimensions...)
	selectColumns = append(selectColumns,
		presentColumn+" AS status_present",
		codeColumn+" AS status_code",
	)
	selectColumns = append(selectColumns, channelMetricCounterAggregateColumns...)
	query = query.Select(strings.Join(selectColumns, ", "))
	for _, dimension := range dimensions {
		query = query.Group(dimension)
	}
	query = query.Group(presentColumn).Group(codeColumn)
	var rows []ChannelMetricAggregateRow
	err = query.Scan(&rows).Error
	return rows, err
}

func AggregateChannelMetricStatusCodesByChannel(db *gorm.DB, filter ChannelMetricBucketFilter) ([]ChannelMetricAggregateRow, error) {
	return aggregateChannelMetricStatusCodes(db, filter, []string{"channel_id"}, "upstream_status_present", "upstream_status_code")
}

func AggregateChannelMetricStatusCodesByModel(db *gorm.DB, filter ChannelMetricBucketFilter, upstream bool) ([]ChannelMetricAggregateRow, error) {
	modelHashColumn := "requested_model_hash"
	if upstream {
		modelHashColumn = "upstream_model_hash"
	}
	return aggregateChannelMetricStatusCodes(db, filter, []string{modelHashColumn}, "upstream_status_present", "upstream_status_code")
}

func AggregateChannelMetricErrorStages(db *gorm.DB, filter ChannelMetricBucketFilter) ([]ChannelMetricAggregateRow, error) {
	return queryChannelMetricAggregate(db, filter, []string{"error_stage"})
}

type ChannelMetricBounds struct {
	DataStartTs int64 `json:"data_start_ts"`
	DataEndTs   int64 `json:"data_end_ts"`
}

func GetChannelMetricBounds(db *gorm.DB, filter ChannelMetricBucketFilter) (ChannelMetricBounds, error) {
	if db == nil {
		return ChannelMetricBounds{}, ErrChannelMetricInvalidBatch
	}
	query, err := applyChannelMetricBucketFilter(db.Model(&ChannelMetricBucket{}), filter)
	if err != nil {
		return ChannelMetricBounds{}, err
	}
	var bounds ChannelMetricBounds
	err = query.Select("COALESCE(MIN(bucket_ts), 0) AS data_start_ts, COALESCE(MAX(bucket_ts), 0) AS data_end_ts").Scan(&bounds).Error
	return bounds, err
}

type ChannelMetricModelOption struct {
	Model     string `json:"model"`
	ModelHash string `json:"model_hash"`
}

func GetChannelMetricModelOptions(db *gorm.DB, bucketLevel string, upstream bool, limit int) ([]ChannelMetricModelOption, error) {
	if db == nil {
		return nil, ErrChannelMetricInvalidBatch
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	presentColumn, modelColumn, hashColumn := "requested_model_present", "requested_model", "requested_model_hash"
	if upstream {
		presentColumn, modelColumn, hashColumn = "upstream_model_present", "upstream_model", "upstream_model_hash"
	}
	var rows []ChannelMetricModelOption
	err := db.Model(&ChannelMetricBucket{}).
		Select(modelColumn+" AS model, "+hashColumn+" AS model_hash").
		Where("bucket_level = ?", bucketLevel).
		Where(presentColumn+" = ?", true).
		Where(modelColumn+" <> ?", "").
		Group(hashColumn).
		Group(modelColumn).
		Order(modelColumn + " ASC").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}

type ChannelFailureLatestRow struct {
	ChannelId          int    `json:"channel_id"`
	RequestedModel     string `json:"requested_model"`
	RequestedModelHash string `json:"requested_model_hash"`
	UpstreamModel      string `json:"upstream_model"`
	UpstreamModelHash  string `json:"upstream_model_hash"`
	CreatedAt          int64  `json:"created_at"`
}

func GetLatestChannelFailureTimes(db *gorm.DB, filter ChannelFailureEventFilter, byModel bool) ([]ChannelFailureLatestRow, error) {
	return getLatestChannelFailureTimes(db, filter, byModel, false)
}

// GetLatestChannelFailureTimesByModel 支持按请求模型或上游模型下钻最近失败时间。
func GetLatestChannelFailureTimesByModel(db *gorm.DB, filter ChannelFailureEventFilter, upstream bool) ([]ChannelFailureLatestRow, error) {
	return getLatestChannelFailureTimes(db, filter, true, upstream)
}

func getLatestChannelFailureTimes(db *gorm.DB, filter ChannelFailureEventFilter, byModel bool, upstream bool) ([]ChannelFailureLatestRow, error) {
	if db == nil {
		return nil, ErrChannelMetricInvalidBatch
	}
	query, err := applyChannelFailureEventFilter(db.Model(&ChannelFailureEvent{}), filter)
	if err != nil {
		return nil, err
	}
	dimensions := []string{"channel_id"}
	if byModel {
		if upstream {
			dimensions = append(dimensions, "upstream_model_hash", "upstream_model")
		} else {
			dimensions = append(dimensions, "requested_model_hash", "requested_model")
		}
	}
	query = query.Select(strings.Join(append(dimensions, "MAX(created_at) AS created_at"), ", "))
	for _, dimension := range dimensions {
		query = query.Group(dimension)
	}
	var rows []ChannelFailureLatestRow
	err = query.Scan(&rows).Error
	return rows, err
}
