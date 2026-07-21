package model

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelMetricBucketFilter 统一表达指标桶的半开时间区间和维度过滤。
type ChannelMetricBucketFilter struct {
	StartTs int64
	EndTs   int64

	BucketLevel           string
	MetricScopes          []string
	TrafficSources        []string
	DataOrigins           []string
	DimensionHashes       []string
	ChannelIds            []int
	ChannelTypes          []int
	Groups                []string
	GroupHashes           []string
	RequestedModels       []string
	RequestedModelHashes  []string
	UpstreamModels        []string
	UpstreamModelHashes   []string
	Outcomes              []string
	ErrorStages           []string
	FailureOwners         []string
	ClientStatusCodes     []int
	UpstreamStatusCodes   []int
	NormalizedStatusCodes []int

	ChannelPresent          *bool
	RequestedModelPresent   *bool
	UpstreamModelPresent    *bool
	Stream                  *bool
	QualityEligible         *bool
	PartialResponse         *bool
	ClientStatusPresent     *bool
	UpstreamStatusPresent   *bool
	NormalizedStatusPresent *bool

	Limit  int
	Offset int
}

// QueryChannelMetricBuckets 只查询传入日志库，不会隐式回退到主库。
func QueryChannelMetricBuckets(db *gorm.DB, filter ChannelMetricBucketFilter) ([]ChannelMetricBucket, error) {
	if db == nil {
		return nil, ErrChannelMetricInvalidBatch
	}
	query, err := applyChannelMetricBucketFilter(db.Model(&ChannelMetricBucket{}), filter)
	if err != nil {
		return nil, err
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	var rows []ChannelMetricBucket
	err = query.Order("bucket_ts ASC").Order("id ASC").Find(&rows).Error
	return rows, err
}

func applyChannelMetricBucketFilter(query *gorm.DB, filter ChannelMetricBucketFilter) (*gorm.DB, error) {
	if filter.StartTs > 0 {
		query = query.Where("bucket_ts >= ?", filter.StartTs)
	}
	if filter.EndTs > 0 {
		if filter.StartTs > 0 && filter.EndTs <= filter.StartTs {
			return nil, fmt.Errorf("%w: end timestamp must be greater than start timestamp", ErrChannelMetricInvalidBatch)
		}
		query = query.Where("bucket_ts < ?", filter.EndTs)
	}
	if filter.BucketLevel != "" {
		query = query.Where(columnEquals("bucket_level", filter.BucketLevel))
	}
	query = whereStringsIn(query, "metric_scope", filter.MetricScopes)
	query = whereStringsIn(query, "traffic_source", filter.TrafficSources)
	query = whereStringsIn(query, "data_origin", filter.DataOrigins)
	query = whereStringsIn(query, "dimension_hash", filter.DimensionHashes)
	query = whereIntsIn(query, "channel_id", filter.ChannelIds)
	query = whereIntsIn(query, "channel_type", filter.ChannelTypes)
	query = whereStringsIn(query, "group", filter.Groups)
	query = whereStringsIn(query, "group_hash", filter.GroupHashes)
	query = whereStringsIn(query, "requested_model", filter.RequestedModels)
	query = whereStringsIn(query, "requested_model_hash", filter.RequestedModelHashes)
	query = whereStringsIn(query, "upstream_model", filter.UpstreamModels)
	query = whereStringsIn(query, "upstream_model_hash", filter.UpstreamModelHashes)
	query = whereStringsIn(query, "outcome", filter.Outcomes)
	query = whereStringsIn(query, "error_stage", filter.ErrorStages)
	query = whereStringsIn(query, "failure_owner", filter.FailureOwners)
	query = whereIntsIn(query, "client_status_code", filter.ClientStatusCodes)
	query = whereIntsIn(query, "upstream_status_code", filter.UpstreamStatusCodes)
	query = whereIntsIn(query, "normalized_status_code", filter.NormalizedStatusCodes)

	query = whereOptionalBool(query, "channel_present", filter.ChannelPresent)
	query = whereOptionalBool(query, "requested_model_present", filter.RequestedModelPresent)
	query = whereOptionalBool(query, "upstream_model_present", filter.UpstreamModelPresent)
	query = whereOptionalBool(query, "stream", filter.Stream)
	query = whereOptionalBool(query, "quality_eligible", filter.QualityEligible)
	query = whereOptionalBool(query, "partial_response", filter.PartialResponse)
	query = whereOptionalBool(query, "client_status_present", filter.ClientStatusPresent)
	query = whereOptionalBool(query, "upstream_status_present", filter.UpstreamStatusPresent)
	query = whereOptionalBool(query, "normalized_status_present", filter.NormalizedStatusPresent)

	// 查具体状态码时必须排除“不适用但数值默认为 0”的样本。
	if len(filter.ClientStatusCodes) > 0 {
		query = query.Where(columnEquals("client_status_present", true))
	}
	if len(filter.UpstreamStatusCodes) > 0 {
		query = query.Where(columnEquals("upstream_status_present", true))
	}
	if len(filter.NormalizedStatusCodes) > 0 {
		query = query.Where(columnEquals("normalized_status_present", true))
	}
	return query, nil
}

// ChannelFailureEventFilter 保持失败明细的查询字段有界且可索引。
type ChannelFailureEventFilter struct {
	StartTs int64
	EndTs   int64

	ChannelIds            []int
	ChannelTypes          []int
	TrafficSources        []string
	DataOrigins           []string
	Groups                []string
	RequestedModels       []string
	RequestedModelHashes  []string
	UpstreamModels        []string
	UpstreamModelHashes   []string
	Outcomes              []string
	FailureOwners         []string
	ErrorStages           []string
	UpstreamStatusCodes   []int
	NormalizedStatusCodes []int
	ClientStatusCodes     []int

	RetryPlanned            *bool
	IsLastStartedAttempt    *bool
	QualityEligible         *bool
	PartialResponse         *bool
	UpstreamStatusPresent   *bool
	NormalizedStatusPresent *bool
	ClientStatusPresent     *bool

	Limit  int
	Offset int
}

// QueryChannelFailureEvents 返回脱敏失败明细和应用分页前的总数。
func QueryChannelFailureEvents(db *gorm.DB, filter ChannelFailureEventFilter) ([]ChannelFailureEvent, int64, error) {
	if db == nil {
		return nil, 0, ErrChannelMetricInvalidBatch
	}
	query, err := applyChannelFailureEventFilter(db.Model(&ChannelFailureEvent{}), filter)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var rows []ChannelFailureEvent
	err = query.Limit(limit).Order("created_at DESC").Order("event_id DESC").Find(&rows).Error
	return rows, total, err
}

func applyChannelFailureEventFilter(query *gorm.DB, filter ChannelFailureEventFilter) (*gorm.DB, error) {
	if filter.StartTs > 0 {
		query = query.Where("created_at >= ?", filter.StartTs)
	}
	if filter.EndTs > 0 {
		if filter.StartTs > 0 && filter.EndTs <= filter.StartTs {
			return nil, fmt.Errorf("%w: end timestamp must be greater than start timestamp", ErrChannelMetricInvalidBatch)
		}
		query = query.Where("created_at < ?", filter.EndTs)
	}
	query = whereIntsIn(query, "channel_id", filter.ChannelIds)
	query = whereIntsIn(query, "channel_type", filter.ChannelTypes)
	query = whereStringsIn(query, "traffic_source", filter.TrafficSources)
	query = whereStringsIn(query, "data_origin", filter.DataOrigins)
	query = whereStringsIn(query, "group", filter.Groups)
	query = whereStringsIn(query, "requested_model", filter.RequestedModels)
	query = whereStringsIn(query, "requested_model_hash", filter.RequestedModelHashes)
	query = whereStringsIn(query, "upstream_model", filter.UpstreamModels)
	query = whereStringsIn(query, "upstream_model_hash", filter.UpstreamModelHashes)
	query = whereStringsIn(query, "outcome", filter.Outcomes)
	query = whereStringsIn(query, "failure_owner", filter.FailureOwners)
	query = whereStringsIn(query, "error_stage", filter.ErrorStages)
	query = whereIntsIn(query, "upstream_status_code", filter.UpstreamStatusCodes)
	query = whereIntsIn(query, "normalized_status_code", filter.NormalizedStatusCodes)
	query = whereIntsIn(query, "client_status_code", filter.ClientStatusCodes)

	query = whereOptionalBool(query, "retry_planned", filter.RetryPlanned)
	query = whereOptionalBool(query, "is_last_started_attempt", filter.IsLastStartedAttempt)
	query = whereOptionalBool(query, "quality_eligible", filter.QualityEligible)
	query = whereOptionalBool(query, "partial_response", filter.PartialResponse)
	query = whereOptionalBool(query, "upstream_status_present", filter.UpstreamStatusPresent)
	query = whereOptionalBool(query, "normalized_status_present", filter.NormalizedStatusPresent)
	query = whereOptionalBool(query, "client_status_present", filter.ClientStatusPresent)

	if len(filter.UpstreamStatusCodes) > 0 {
		query = query.Where(columnEquals("upstream_status_present", true))
	}
	if len(filter.NormalizedStatusCodes) > 0 {
		query = query.Where(columnEquals("normalized_status_present", true))
	}
	if len(filter.ClientStatusCodes) > 0 {
		query = query.Where(columnEquals("client_status_present", true))
	}
	return query, nil
}

// DeleteChannelMetricBucketsBefore 每次只删除一个有界批次，调用方根据返回数决定是否继续。
// 清理覆盖所有粒度，避免修改 bucket_level 后旧粒度数据永久残留。
func DeleteChannelMetricBucketsBefore(db *gorm.DB, cutoffTs int64, batchSize int) (int64, error) {
	if db == nil || cutoffTs <= 0 {
		return 0, ErrChannelMetricInvalidBatch
	}
	batchSize = normalizedDeleteBatchSize(batchSize)
	var ids []int64
	if err := db.Model(&ChannelMetricBucket{}).
		Select("id").
		Where("bucket_ts < ?", cutoffTs).
		Order("bucket_ts ASC").
		Limit(batchSize).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := db.Where("id IN ?", ids).Delete(&ChannelMetricBucket{})
	return result.RowsAffected, result.Error
}

func DeleteChannelFailureEventsBefore(db *gorm.DB, cutoffTs int64, batchSize int) (int64, error) {
	if db == nil || cutoffTs <= 0 {
		return 0, ErrChannelMetricInvalidBatch
	}
	batchSize = normalizedDeleteBatchSize(batchSize)
	var ids []string
	if err := db.Model(&ChannelFailureEvent{}).
		Where("created_at < ?", cutoffTs).
		Order("created_at ASC").
		Limit(batchSize).
		Pluck("event_id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := db.Where("event_id IN ?", ids).Delete(&ChannelFailureEvent{})
	return result.RowsAffected, result.Error
}

func DeleteChannelMetricFlushesBefore(db *gorm.DB, cutoffTs int64, batchSize int) (int64, error) {
	if db == nil || cutoffTs <= 0 {
		return 0, ErrChannelMetricInvalidBatch
	}
	batchSize = normalizedDeleteBatchSize(batchSize)
	var ids []string
	if err := db.Model(&ChannelMetricFlush{}).
		Where("committed_at < ?", cutoffTs).
		Order("committed_at ASC").
		Limit(batchSize).
		Pluck("flush_id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := db.Where("flush_id IN ?", ids).Delete(&ChannelMetricFlush{})
	return result.RowsAffected, result.Error
}

func GetLastChannelMetricFlushAt(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, ErrChannelMetricInvalidBatch
	}
	var committedAt int64
	err := db.Model(&ChannelMetricFlush{}).Select("COALESCE(MAX(committed_at), 0)").Scan(&committedAt).Error
	return committedAt, err
}

type ChannelMetricDataQuality struct {
	InvalidSampleCount          int64 `json:"invalid_sample_count"`
	DimensionOverflowCount      int64 `json:"dimension_overflow_count"`
	DroppedMetricEventCount     int64 `json:"dropped_metric_event_count"`
	DroppedFailureEventCount    int64 `json:"dropped_failure_event_count"`
	DimensionHashCollisionCount int64 `json:"dimension_hash_collision_count"`
	LastFlushedAt               int64 `json:"last_flushed_at"`
}

// GetChannelMetricDataQuality 从幂等 flush 记录汇总多节点数据质量，时间区间同样为 [start, end)。
func GetChannelMetricDataQuality(db *gorm.DB, startTs int64, endTs int64) (ChannelMetricDataQuality, error) {
	if db == nil || (startTs > 0 && endTs > 0 && endTs <= startTs) {
		return ChannelMetricDataQuality{}, ErrChannelMetricInvalidBatch
	}
	query := db.Model(&ChannelMetricFlush{})
	if startTs > 0 {
		query = query.Where("batch_created_at >= ?", startTs)
	}
	if endTs > 0 {
		query = query.Where("batch_created_at < ?", endTs)
	}
	var quality ChannelMetricDataQuality
	err := query.Select(`
		COALESCE(SUM(invalid_sample_count), 0) AS invalid_sample_count,
		COALESCE(SUM(dimension_overflow_count), 0) AS dimension_overflow_count,
		COALESCE(SUM(dropped_metric_event_count), 0) AS dropped_metric_event_count,
		COALESCE(SUM(dropped_failure_event_count), 0) AS dropped_failure_event_count,
		COALESCE(SUM(dimension_hash_collision_count), 0) AS dimension_hash_collision_count,
		COALESCE(MAX(committed_at), 0) AS last_flushed_at`).
		Scan(&quality).Error
	return quality, err
}

func normalizedDeleteBatchSize(size int) int {
	if size <= 0 {
		return 100
	}
	if size > ChannelMetricMaxDeleteBatch {
		return ChannelMetricMaxDeleteBatch
	}
	return size
}

func columnEquals(name string, value interface{}) clause.Eq {
	return clause.Eq{Column: clause.Column{Name: name}, Value: value}
}

func whereOptionalBool(query *gorm.DB, column string, value *bool) *gorm.DB {
	if value == nil {
		return query
	}
	return query.Where(columnEquals(column, *value))
}

func whereStringsIn(query *gorm.DB, column string, values []string) *gorm.DB {
	if len(values) == 0 {
		return query
	}
	items := make([]interface{}, len(values))
	for i := range values {
		items[i] = values[i]
	}
	return query.Where(clause.IN{Column: clause.Column{Name: column}, Values: items})
}

func whereIntsIn(query *gorm.DB, column string, values []int) *gorm.DB {
	if len(values) == 0 {
		return query
	}
	items := make([]interface{}, len(values))
	for i := range values {
		items[i] = values[i]
	}
	return query.Where(clause.IN{Column: clause.Column{Name: column}, Values: items})
}
