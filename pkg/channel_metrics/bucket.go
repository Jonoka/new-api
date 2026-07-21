package channelmetrics

// MetricCounters 是一个维度桶内的可累加事实数据。
type MetricCounters struct {
	EventCount               int64            `json:"event_count"`
	SuccessCount             int64            `json:"success_count"`
	NonFirstAttemptCount     int64            `json:"non_first_attempt_count"`
	RetryPlannedCount        int64            `json:"retry_planned_count"`
	QualityEligibleCount     int64            `json:"quality_eligible_count"`
	QualitySuccessCount      int64            `json:"quality_success_count"`
	PartialResponseCount     int64            `json:"partial_response_count"`
	UsageSampleCount         int64            `json:"usage_sample_count"`
	CacheHitRequestCount     int64            `json:"cache_hit_request_count"`
	InputTokensTotal         int64            `json:"input_tokens_total"`
	UncachedInputTokens      int64            `json:"uncached_input_tokens"`
	OutputTokens             int64            `json:"output_tokens"`
	CacheReadTokens          int64            `json:"cache_read_tokens"`
	CacheWriteTokens         int64            `json:"cache_write_tokens"`
	ChargedQuota             int64            `json:"charged_quota"`
	ChargedMicroUSD          int64            `json:"charged_micro_usd"`
	LatencySumMs             int64            `json:"latency_sum_ms"`
	LatencyCount             int64            `json:"latency_count"`
	TTFTSumMs                int64            `json:"ttft_sum_ms"`
	TTFTCount                int64            `json:"ttft_count"`
	LatencyHistogram         LatencyHistogram `json:"latency_histogram"`
	TTFTHistogram            LatencyHistogram `json:"ttft_histogram"`
	DimensionOverflowCount   int64            `json:"dimension_overflow_count"`
	DroppedMetricEventCount  int64            `json:"dropped_metric_event_count"`
	DroppedFailureEventCount int64            `json:"dropped_failure_event_count"`
}

func (c *MetricCounters) addSample(sample Sample, overflowed bool) {
	c.EventCount++
	if sample.Outcome == OutcomeSuccess {
		c.SuccessCount++
	}
	if sample.AttemptSeq > 1 && sample.Scope != ScopeFinalRequest {
		c.NonFirstAttemptCount++
	}
	if sample.RetryPlanned {
		c.RetryPlannedCount++
	}
	if sample.QualityEligible {
		c.QualityEligibleCount++
		if sample.Outcome == OutcomeSuccess {
			c.QualitySuccessCount++
		}
	}
	if sample.PartialResponse {
		c.PartialResponseCount++
	}
	if sample.UsagePresent {
		c.UsageSampleCount++
		if sample.CacheReadTokens > 0 {
			c.CacheHitRequestCount++
		}
		c.InputTokensTotal += sample.InputTokensTotal
		c.UncachedInputTokens += sample.UncachedInputTokens
		c.OutputTokens += sample.OutputTokens
		c.CacheReadTokens += sample.CacheReadTokens
		c.CacheWriteTokens += sample.CacheWriteTokens
		c.ChargedQuota += sample.ChargedQuota
		c.ChargedMicroUSD += sample.ChargedMicroUSD
	}
	if sample.LatencyPresent {
		c.LatencySumMs += sample.LatencyMs
		c.LatencyCount++
		c.LatencyHistogram.Observe(sample.LatencyMs)
	}
	if sample.TTFTPresent {
		c.TTFTSumMs += sample.TTFTMs
		c.TTFTCount++
		c.TTFTHistogram.Observe(sample.TTFTMs)
	}
	if overflowed {
		c.DimensionOverflowCount++
	}
}

// Merge 原位合并另一个桶，供恢复和后续降采样复用。
func (c *MetricCounters) Merge(other MetricCounters) {
	c.EventCount += other.EventCount
	c.SuccessCount += other.SuccessCount
	c.NonFirstAttemptCount += other.NonFirstAttemptCount
	c.RetryPlannedCount += other.RetryPlannedCount
	c.QualityEligibleCount += other.QualityEligibleCount
	c.QualitySuccessCount += other.QualitySuccessCount
	c.PartialResponseCount += other.PartialResponseCount
	c.UsageSampleCount += other.UsageSampleCount
	c.CacheHitRequestCount += other.CacheHitRequestCount
	c.InputTokensTotal += other.InputTokensTotal
	c.UncachedInputTokens += other.UncachedInputTokens
	c.OutputTokens += other.OutputTokens
	c.CacheReadTokens += other.CacheReadTokens
	c.CacheWriteTokens += other.CacheWriteTokens
	c.ChargedQuota += other.ChargedQuota
	c.ChargedMicroUSD += other.ChargedMicroUSD
	c.LatencySumMs += other.LatencySumMs
	c.LatencyCount += other.LatencyCount
	c.TTFTSumMs += other.TTFTSumMs
	c.TTFTCount += other.TTFTCount
	c.LatencyHistogram.Merge(other.LatencyHistogram)
	c.TTFTHistogram.Merge(other.TTFTHistogram)
	c.DimensionOverflowCount += other.DimensionOverflowCount
	c.DroppedMetricEventCount += other.DroppedMetricEventCount
	c.DroppedFailureEventCount += other.DroppedFailureEventCount
}

func (c MetricCounters) Empty() bool {
	return c.EventCount == 0 &&
		c.DimensionOverflowCount == 0 &&
		c.DroppedMetricEventCount == 0 &&
		c.DroppedFailureEventCount == 0
}

// Bucket 是一次 drain 后的不可变热桶快照。
type Bucket struct {
	BucketLevel   string         `json:"bucket_level"`
	BucketTs      int64          `json:"bucket_ts"`
	DimensionHash string         `json:"dimension_hash"`
	Dimension     Dimension      `json:"dimension"`
	Counters      MetricCounters `json:"counters"`
}

// DataQualityDelta 保存不能自然归属到普通指标桶的数据质量增量。
type DataQualityDelta struct {
	InvalidSampleCount          int64 `json:"invalid_sample_count"`
	DimensionOverflowCount      int64 `json:"dimension_overflow_count"`
	DroppedMetricEventCount     int64 `json:"dropped_metric_event_count"`
	DroppedFailureEventCount    int64 `json:"dropped_failure_event_count"`
	DimensionHashCollisionCount int64 `json:"dimension_hash_collision_count"`
}

func (d DataQualityDelta) Empty() bool {
	return d.InvalidSampleCount == 0 &&
		d.DimensionOverflowCount == 0 &&
		d.DroppedMetricEventCount == 0 &&
		d.DroppedFailureEventCount == 0 &&
		d.DimensionHashCollisionCount == 0
}

// MetricBatch 的 ID 在失败重试时保持不变，持久化 Sink 必须据此实现幂等。
type MetricBatch struct {
	ID              string           `json:"flush_id"`
	NodeID          string           `json:"node_id"`
	CreatedAtUnixMs int64            `json:"created_at_unix_ms"`
	Buckets         []Bucket         `json:"buckets"`
	Quality         DataQualityDelta `json:"quality"`
}

func (b MetricBatch) Empty() bool {
	return len(b.Buckets) == 0 && b.Quality.Empty()
}

// Clone 防止 Sink 意外修改待重试批次。
func (b MetricBatch) Clone() MetricBatch {
	clone := b
	if b.Buckets != nil {
		clone.Buckets = make([]Bucket, len(b.Buckets))
		copy(clone.Buckets, b.Buckets)
	}
	return clone
}
