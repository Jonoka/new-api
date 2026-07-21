package channelmetrics

import (
	"sync"
	"sync/atomic"
)

// QualitySnapshot 是进程启动以来的累计数据质量状态。
type QualitySnapshot struct {
	ReceivedMetricEventCount    int64  `json:"received_metric_event_count"`
	RecordedMetricEventCount    int64  `json:"recorded_metric_event_count"`
	InvalidSampleCount          int64  `json:"invalid_sample_count"`
	DimensionOverflowCount      int64  `json:"dimension_overflow_count"`
	DroppedMetricEventCount     int64  `json:"dropped_metric_event_count"`
	DroppedFailureEventCount    int64  `json:"dropped_failure_event_count"`
	DimensionHashCollisionCount int64  `json:"dimension_hash_collision_count"`
	FlushAttemptCount           int64  `json:"flush_attempt_count"`
	FlushSuccessCount           int64  `json:"flush_success_count"`
	FlushFailureCount           int64  `json:"flush_failure_count"`
	RestoredBatchCount          int64  `json:"restored_batch_count"`
	HotBucketCount              int64  `json:"hot_bucket_count"`
	PendingBatchCount           int64  `json:"pending_batch_count"`
	LastFlushedAtUnix           int64  `json:"last_flushed_at"`
	LastFlushErrorAtUnix        int64  `json:"last_flush_error_at"`
	LastFlushError              string `json:"last_flush_error"`
	Partial                     bool   `json:"partial"`
}

type deltaCounter struct {
	total   atomic.Int64
	pending atomic.Int64
}

func (c *deltaCounter) add(count int64) {
	if count <= 0 {
		return
	}
	c.total.Add(count)
	c.pending.Add(count)
}

func (c *deltaCounter) drain() int64 {
	return c.pending.Swap(0)
}

func (c *deltaCounter) restore(count int64) {
	if count > 0 {
		c.pending.Add(count)
	}
}

type qualityState struct {
	receivedMetricEvents atomic.Int64
	recordedMetricEvents atomic.Int64
	invalidSamples       deltaCounter
	dimensionOverflows   deltaCounter
	droppedMetricEvents  deltaCounter
	droppedFailureEvents deltaCounter
	hashCollisions       deltaCounter
	flushAttempts        atomic.Int64
	flushSuccesses       atomic.Int64
	flushFailures        atomic.Int64
	restoredBatches      atomic.Int64
	hotBuckets           atomic.Int64
	pendingBatches       atomic.Int64
	lastFlushedAt        atomic.Int64
	lastFlushErrorAt     atomic.Int64
	errorMu              sync.RWMutex
	lastFlushError       string
}

func (q *qualityState) drainDelta() DataQualityDelta {
	return DataQualityDelta{
		InvalidSampleCount:          q.invalidSamples.drain(),
		DimensionOverflowCount:      q.dimensionOverflows.drain(),
		DroppedMetricEventCount:     q.droppedMetricEvents.drain(),
		DroppedFailureEventCount:    q.droppedFailureEvents.drain(),
		DimensionHashCollisionCount: q.hashCollisions.drain(),
	}
}

func (q *qualityState) restoreDelta(delta DataQualityDelta) {
	q.invalidSamples.restore(delta.InvalidSampleCount)
	q.dimensionOverflows.restore(delta.DimensionOverflowCount)
	q.droppedMetricEvents.restore(delta.DroppedMetricEventCount)
	q.droppedFailureEvents.restore(delta.DroppedFailureEventCount)
	q.hashCollisions.restore(delta.DimensionHashCollisionCount)
}

func (q *qualityState) setFlushError(unixSeconds int64, err error) {
	q.lastFlushErrorAt.Store(unixSeconds)
	q.errorMu.Lock()
	q.lastFlushError = err.Error()
	q.errorMu.Unlock()
}

func (q *qualityState) snapshot() QualitySnapshot {
	q.errorMu.RLock()
	lastError := q.lastFlushError
	q.errorMu.RUnlock()

	snapshot := QualitySnapshot{
		ReceivedMetricEventCount:    q.receivedMetricEvents.Load(),
		RecordedMetricEventCount:    q.recordedMetricEvents.Load(),
		InvalidSampleCount:          q.invalidSamples.total.Load(),
		DimensionOverflowCount:      q.dimensionOverflows.total.Load(),
		DroppedMetricEventCount:     q.droppedMetricEvents.total.Load(),
		DroppedFailureEventCount:    q.droppedFailureEvents.total.Load(),
		DimensionHashCollisionCount: q.hashCollisions.total.Load(),
		FlushAttemptCount:           q.flushAttempts.Load(),
		FlushSuccessCount:           q.flushSuccesses.Load(),
		FlushFailureCount:           q.flushFailures.Load(),
		RestoredBatchCount:          q.restoredBatches.Load(),
		HotBucketCount:              q.hotBuckets.Load(),
		PendingBatchCount:           q.pendingBatches.Load(),
		LastFlushedAtUnix:           q.lastFlushedAt.Load(),
		LastFlushErrorAtUnix:        q.lastFlushErrorAt.Load(),
		LastFlushError:              lastError,
	}
	snapshot.Partial = snapshot.InvalidSampleCount > 0 ||
		snapshot.DimensionOverflowCount > 0 ||
		snapshot.DroppedMetricEventCount > 0 ||
		snapshot.DroppedFailureEventCount > 0 ||
		snapshot.DimensionHashCollisionCount > 0
	return snapshot
}
