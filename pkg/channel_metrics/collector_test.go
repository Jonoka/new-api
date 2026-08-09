package channelmetrics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCollectorAggregatesUsageAndHistograms(t *testing.T) {
	now := time.Unix(1_700_000_123, 0)
	config := testConfig(now)
	collector := NewCollector(config, nil)
	sample := successfulAttempt(now, "model-a")
	sample.AttemptSeq = 2
	sample.RetryPlanned = true
	sample.PartialResponse = true
	sample.QualityEligible = true
	sample.LatencyPresent = true
	sample.LatencyMs = 620
	sample.TTFTPresent = true
	sample.TTFTMs = 90
	sample.UsagePresent = true
	sample.InputTokensTotal = 100
	sample.UncachedInputTokens = 70
	sample.OutputTokens = 20
	sample.CacheReadTokens = 30
	sample.CacheWriteTokens = 10
	sample.ChargedQuota = 500
	sample.ChargedMicroUSD = 42
	for index := 0; index < 2; index++ {
		if err := collector.Record(sample); err != nil {
			t.Fatalf("记录样本失败：%v", err)
		}
	}

	batch := collector.Drain()
	if batch.Empty() || len(batch.Buckets) != 1 {
		t.Fatalf("drain 批次异常：%+v", batch)
	}
	bucket := batch.Buckets[0]
	if bucket.BucketTs != floorBucket(now.Unix(), config.BucketSeconds) || bucket.BucketLevel != "5m" {
		t.Fatalf("时间桶异常：%+v", bucket)
	}
	counters := bucket.Counters
	if counters.EventCount != 2 || counters.SuccessCount != 2 || counters.NonFirstAttemptCount != 2 || counters.RetryPlannedCount != 2 {
		t.Fatalf("请求计数异常：%+v", counters)
	}
	if counters.QualityEligibleCount != 2 || counters.QualitySuccessCount != 2 || counters.PartialResponseCount != 2 {
		t.Fatalf("质量计数异常：%+v", counters)
	}
	if counters.UsageSampleCount != 2 || counters.CacheHitRequestCount != 2 || counters.InputTokensTotal != 200 || counters.CacheReadTokens != 60 || counters.ChargedMicroUSD != 84 {
		t.Fatalf("用量计数异常：%+v", counters)
	}
	if counters.LatencyCount != 2 || counters.LatencySumMs != 1240 || counters.LatencyHistogram.Total() != 2 {
		t.Fatalf("延迟聚合异常：%+v", counters)
	}
	if counters.TTFTCount != 2 || counters.TTFTSumMs != 180 || counters.TTFTHistogram.Total() != 2 {
		t.Fatalf("TTFT 聚合异常：%+v", counters)
	}
	if quality := collector.Quality(); quality.ReceivedMetricEventCount != 2 || quality.RecordedMetricEventCount != 2 || quality.HotBucketCount != 0 {
		t.Fatalf("采集器质量状态异常：%+v", quality)
	}
}

func TestCollectorKeepsThreeScopesSeparate(t *testing.T) {
	now := time.Unix(1_700_000_123, 0)
	collector := NewCollector(testConfig(now), nil)

	finalRequest := NewLiveSample(ScopeFinalRequest, OutcomeSuccess)
	finalRequest.OccurredAt = now
	finalRequest.ClientStatus = PresentStatus(200)
	attempt := successfulAttempt(now, "model-a")
	upstreamCall := NewLiveSample(ScopeUpstreamCall, OutcomeSuccess)
	upstreamCall.OccurredAt = now
	upstreamCall.AttemptSeq = 1
	upstreamCall.ChannelPresent = true
	upstreamCall.ChannelID = 7
	upstreamCall.UpstreamStatus = PresentStatus(200)
	for _, sample := range []Sample{finalRequest, attempt, upstreamCall} {
		if err := collector.Record(sample); err != nil {
			t.Fatalf("记录 %s 样本失败：%v", sample.Scope, err)
		}
	}
	batch := collector.Drain()
	if len(batch.Buckets) != 3 {
		t.Fatalf("三级 scope 桶数量 = %d，期望 3", len(batch.Buckets))
	}
	scopes := map[Scope]int64{}
	for _, bucket := range batch.Buckets {
		scopes[bucket.Dimension.Scope] += bucket.Counters.EventCount
	}
	for _, scope := range []Scope{ScopeFinalRequest, ScopeChannelAttempt, ScopeUpstreamCall} {
		if scopes[scope] != 1 {
			t.Fatalf("scope %s 事件数 = %d，期望 1", scope, scopes[scope])
		}
	}
}

func TestCollectorConcurrentRecordDrainAndRestore(t *testing.T) {
	now := time.Unix(1_700_000_123, 0)
	collector := NewCollector(testConfig(now), nil)
	sample := successfulAttempt(now, "model-a")
	const workers = 16
	const recordsPerWorker = 500
	errorsChannel := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for index := 0; index < recordsPerWorker; index++ {
				if err := collector.Record(sample); err != nil {
					errorsChannel <- err
					return
				}
			}
		}()
	}
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("并发记录失败：%v", err)
	}

	first := collector.Drain()
	if len(first.Buckets) != 1 || first.Buckets[0].Counters.EventCount != workers*recordsPerWorker {
		t.Fatalf("并发事件未守恒：%+v", first.Buckets)
	}
	for index := 0; index < 3; index++ {
		if err := collector.Record(sample); err != nil {
			t.Fatalf("drain 后记录失败：%v", err)
		}
	}
	if err := collector.Restore(first); err != nil {
		t.Fatalf("恢复批次失败：%v", err)
	}
	merged := collector.Drain()
	if len(merged.Buckets) != 1 || merged.Buckets[0].Counters.EventCount != workers*recordsPerWorker+3 {
		t.Fatalf("恢复批次未与新流量合并：%+v", merged.Buckets)
	}
}

func TestCollectorOverflowAndCapacityQuality(t *testing.T) {
	now := time.Unix(1_700_000_123, 0)
	config := testConfig(now)
	config.MaxActiveDimensionsPerBucket = 1
	config.MaxHotBuckets = 2
	collector := NewCollector(config, nil)
	for _, model := range []string{"model-a", "model-b", "model-c"} {
		if err := collector.Record(successfulAttempt(now, model)); err != nil {
			t.Fatalf("记录溢出维度失败：%v", err)
		}
	}
	batch := collector.Drain()
	if len(batch.Buckets) != 2 {
		t.Fatalf("正常桶加溢出桶数量 = %d，期望 2", len(batch.Buckets))
	}
	var overflowCount int64
	for _, bucket := range batch.Buckets {
		if bucket.Dimension.Overflowed {
			if bucket.Dimension.RequestedModel != "__other__" {
				t.Fatalf("溢出维度未明确标记：%+v", bucket.Dimension)
			}
			overflowCount += bucket.Counters.DimensionOverflowCount
		}
	}
	if overflowCount != 2 || batch.Quality.DimensionOverflowCount != 2 {
		t.Fatalf("维度溢出计数不守恒：桶=%d，批次=%+v", overflowCount, batch.Quality)
	}
	if quality := collector.Quality(); quality.DimensionOverflowCount != 2 || !quality.Partial {
		t.Fatalf("全局溢出质量状态异常：%+v", quality)
	}

	capacityConfig := testConfig(now)
	capacityConfig.MaxActiveDimensionsPerBucket = 1
	capacityConfig.MaxHotBuckets = 1
	capacityCollector := NewCollector(capacityConfig, nil)
	if err := capacityCollector.Record(successfulAttempt(now, "model-a")); err != nil {
		t.Fatalf("记录首个容量样本失败：%v", err)
	}
	if err := capacityCollector.Record(successfulAttempt(now, "model-b")); !errors.Is(err, ErrCollectorCapacity) {
		t.Fatalf("容量耗尽应返回 ErrCollectorCapacity，实际：%v", err)
	}
	if quality := capacityCollector.Quality(); quality.DroppedMetricEventCount != 1 || !quality.Partial {
		t.Fatalf("容量丢弃质量状态异常：%+v", quality)
	}
}

func TestCollectorFlushRetriesStableImmutableBatch(t *testing.T) {
	now := time.Unix(1_700_000_123, 0)
	sink := &recordingSink{failuresRemaining: 1, mutateReceivedBatch: true}
	collector := NewCollector(testConfig(now), sink)
	if err := collector.Record(successfulAttempt(now, "model-a")); err != nil {
		t.Fatalf("记录首个刷新样本失败：%v", err)
	}
	if err := collector.Flush(context.Background()); err == nil {
		t.Fatal("首次刷新应模拟失败")
	}
	if quality := collector.Quality(); quality.PendingBatchCount != 1 || quality.FlushFailureCount != 1 {
		t.Fatalf("失败刷新质量状态异常：%+v", quality)
	}
	if err := collector.Record(successfulAttempt(now, "model-b")); err != nil {
		t.Fatalf("失败期间记录新样本失败：%v", err)
	}
	if err := collector.Flush(context.Background()); err != nil {
		t.Fatalf("重试刷新失败：%v", err)
	}
	if err := collector.Flush(context.Background()); err != nil {
		t.Fatalf("刷新失败期间的新热桶失败：%v", err)
	}

	calls := sink.snapshot()
	if len(calls) != 3 {
		t.Fatalf("Sink 调用次数 = %d，期望 3", len(calls))
	}
	if calls[0].ID == "" || calls[0].ID != calls[1].ID {
		t.Fatalf("失败重试没有复用稳定 flush_id：%q / %q", calls[0].ID, calls[1].ID)
	}
	if calls[1].Buckets[0].Counters.EventCount != 1 {
		t.Fatalf("Sink 对首个批次的修改污染了重试批次：%+v", calls[1].Buckets)
	}
	if calls[2].ID == calls[1].ID || calls[2].Buckets[0].Counters.EventCount != 1 {
		t.Fatalf("新热桶没有形成独立批次：%+v", calls[2])
	}
	quality := collector.Quality()
	if quality.PendingBatchCount != 0 || quality.FlushAttemptCount != 3 || quality.FlushSuccessCount != 2 {
		t.Fatalf("最终刷新质量状态异常：%+v", quality)
	}
}

func TestCollectorFlushAllCommitsPendingBatchAndNewHotBuckets(t *testing.T) {
	now := time.Unix(1_700_000_123, 0)
	sink := &recordingSink{failuresRemaining: 1}
	collector := NewCollector(testConfig(now), sink)
	if err := collector.Record(successfulAttempt(now, "model-a")); err != nil {
		t.Fatalf("记录首个刷新样本失败：%v", err)
	}
	if err := collector.Flush(context.Background()); err == nil {
		t.Fatal("首次刷新应模拟失败")
	}
	if err := collector.Record(successfulAttempt(now, "model-b")); err != nil {
		t.Fatalf("记录失败期间的新样本失败：%v", err)
	}

	if err := collector.FlushAll(context.Background()); err != nil {
		t.Fatalf("完整刷新失败：%v", err)
	}

	calls := sink.snapshot()
	if len(calls) != 3 {
		t.Fatalf("Sink 调用次数 = %d，期望失败批次、稳定重试和新热桶共 3 次", len(calls))
	}
	if calls[0].ID != calls[1].ID {
		t.Fatalf("失败批次重试没有复用 flush_id：%q / %q", calls[0].ID, calls[1].ID)
	}
	if calls[2].ID == calls[1].ID || len(calls[2].Buckets) != 1 || calls[2].Buckets[0].Dimension.RequestedModel != "model-b" {
		t.Fatalf("失败期间的新热桶没有作为独立批次提交：%+v", calls[2])
	}
	if quality := collector.Quality(); quality.PendingBatchCount != 0 || quality.HotBucketCount != 0 || quality.FlushSuccessCount != 2 {
		t.Fatalf("完整刷新后的质量状态异常：%+v", quality)
	}
}

func TestCollectorFlushIDFitsCrossDatabaseColumn(t *testing.T) {
	now := time.Unix(1_700_000_123, 0)
	config := testConfig(now)
	config.NodeID = stringsOfLength(64, 'n')
	collector := NewCollector(config, nil)
	if err := collector.Record(successfulAttempt(now, "model-a")); err != nil {
		t.Fatalf("记录 flush_id 测试样本失败：%v", err)
	}

	batch := collector.Drain()
	if len(batch.ID) != 64 {
		t.Fatalf("flush_id 长度 = %d，期望与三库字段一致的 64 字符：%q", len(batch.ID), batch.ID)
	}
	if batch.ID != strings.ToLower(batch.ID) {
		t.Fatalf("flush_id 必须是规范化小写十六进制：%q", batch.ID)
	}
}

func TestCollectorRestoresQualityDeltaAndRejectsTamperedHash(t *testing.T) {
	now := time.Unix(1_700_000_123, 0)
	collector := NewCollector(testConfig(now), nil)
	invalid := NewLiveSample(ScopeChannelAttempt, OutcomeSuccess)
	if err := collector.Record(invalid); !errors.Is(err, ErrInvalidSample) {
		t.Fatalf("缺失渠道维度应为无效样本：%v", err)
	}
	qualityOnly := collector.Drain()
	if qualityOnly.Quality.InvalidSampleCount != 1 || len(qualityOnly.Buckets) != 0 {
		t.Fatalf("质量增量批次异常：%+v", qualityOnly)
	}
	if err := collector.Restore(qualityOnly); err != nil {
		t.Fatalf("恢复质量增量失败：%v", err)
	}
	if restored := collector.Drain(); restored.Quality.InvalidSampleCount != 1 {
		t.Fatalf("恢复后的质量增量丢失：%+v", restored.Quality)
	}

	if err := collector.Record(successfulAttempt(now, "model-a")); err != nil {
		t.Fatalf("记录哈希测试样本失败：%v", err)
	}
	tampered := collector.Drain()
	tampered.Buckets[0].DimensionHash = stringsOfLength(64, '0')
	if err := collector.Restore(tampered); !errors.Is(err, ErrDimensionHashCollision) {
		t.Fatalf("篡改维度哈希应被拒绝：%v", err)
	}
}

func testConfig(now time.Time) Config {
	config := DefaultConfig()
	config.NodeID = "test-node"
	config.Now = func() time.Time { return now }
	return config
}

func successfulAttempt(now time.Time, model string) Sample {
	sample := NewLiveSample(ScopeChannelAttempt, OutcomeSuccess)
	sample.OccurredAt = now
	sample.AttemptSeq = 1
	sample.ChannelPresent = true
	sample.ChannelID = 7
	sample.ChannelNameSnapshot = "渠道 A"
	sample.ChannelType = 1
	sample.RequestedModelPresent = true
	sample.RequestedModel = model
	sample.UpstreamModelPresent = true
	sample.UpstreamModel = model
	sample.Group = "default"
	return sample
}

type recordingSink struct {
	mu                  sync.Mutex
	calls               []MetricBatch
	failuresRemaining   int
	mutateReceivedBatch bool
}

func (s *recordingSink) Flush(_ context.Context, batch MetricBatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, batch.Clone())
	if s.mutateReceivedBatch && len(batch.Buckets) > 0 {
		batch.Buckets[0].Counters.EventCount = 999
	}
	if s.failuresRemaining > 0 {
		s.failuresRemaining--
		return fmt.Errorf("模拟持久化失败")
	}
	return nil
}

func (s *recordingSink) snapshot() []MetricBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]MetricBatch, len(s.calls))
	for index := range s.calls {
		result[index] = s.calls[index].Clone()
	}
	return result
}

func stringsOfLength(length int, character byte) string {
	buffer := make([]byte, length)
	for index := range buffer {
		buffer[index] = character
	}
	return string(buffer)
}
