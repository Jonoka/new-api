package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	"github.com/QuantumNous/new-api/setting/channel_metrics_setting"
	"gorm.io/gorm"
)

type channelMetricsRuntime struct {
	collector   *channelmetrics.Collector
	setting     channel_metrics_setting.ChannelMetricsSetting
	cancel      context.CancelFunc
	done        chan struct{}
	failureCh   chan []model.ChannelFailureEvent
	failureDone chan struct{}
	cleanupDone chan struct{}
}

const (
	channelMetricCleanupInitialDelay = 10 * time.Minute
	channelMetricCleanupInterval     = 6 * time.Hour
	channelMetricCleanupCatchUpDelay = time.Minute
	channelMetricCleanupMaxBatches   = 20
)

var (
	channelMetricsRuntimeMu sync.RWMutex
	channelMetricsCurrent   *channelMetricsRuntime
)

// InitChannelMetrics 在 LOG_DB 完成初始化后启动实时采集与异步刷新。
func InitChannelMetrics() error {
	channelMetricsRuntimeMu.Lock()
	defer channelMetricsRuntimeMu.Unlock()
	if channelMetricsCurrent != nil {
		return nil
	}
	if model.LOG_DB == nil {
		return fmt.Errorf("channel metrics: LOG_DB is nil")
	}

	settingSnapshot := channel_metrics_setting.GetSetting().Normalized()
	config := settingSnapshot.CollectorConfig()
	config.OnFlushError = func(err error) {
		common.SysError("channel metrics flush failed: " + err.Error())
	}
	collector := channelmetrics.NewCollector(config, channelmetrics.SinkFunc(flushChannelMetricBatch))
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &channelMetricsRuntime{
		collector:   collector,
		setting:     settingSnapshot,
		cancel:      cancel,
		done:        make(chan struct{}),
		failureCh:   make(chan []model.ChannelFailureEvent, 1024),
		failureDone: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}
	channelMetricsCurrent = runtime

	go func() {
		defer close(runtime.done)
		if err := collector.Run(ctx); err != nil {
			common.SysError("channel metrics flush loop stopped: " + err.Error())
		}
	}()
	go runChannelFailureWriter(ctx, runtime)
	go runChannelMetricCleanup(ctx, runtime)
	return nil
}

// ShutdownChannelMetrics 先完成有界最终刷新，再由主程序关闭数据库。
func ShutdownChannelMetrics() {
	channelMetricsRuntimeMu.Lock()
	runtime := channelMetricsCurrent
	channelMetricsCurrent = nil
	channelMetricsRuntimeMu.Unlock()
	if runtime == nil {
		return
	}
	runtime.cancel()

	timeout := channelMetricFinalFlushTimeout(runtime.setting)
	collectorStopped := false
	select {
	case <-runtime.done:
		collectorStopped = true
	case <-time.After(timeout):
		common.SysError("channel metrics final flush timed out")
	}
	failureWriterStopped := false
	select {
	case <-runtime.failureDone:
		failureWriterStopped = true
	case <-time.After(timeout):
		common.SysError("channel failure events final flush timed out")
	}
	// 失败明细写入器可能在 collector 首轮关停刷新之后才产生丢弃计数；
	// 两个后台任务都已停止时再完整刷新一次，避免与仍持锁的超时刷新发生竞争。
	if collectorStopped && failureWriterStopped {
		finalContext, cancel := context.WithTimeout(context.Background(), timeout)
		if err := runtime.collector.FlushAll(finalContext); err != nil {
			common.SysError("channel metrics post-failure final flush failed: " + err.Error())
		}
		cancel()
	}
	select {
	case <-runtime.cleanupDone:
	case <-time.After(timeout):
		common.SysError("channel metrics cleanup loop shutdown timed out")
	}
}

func channelMetricFinalFlushTimeout(setting channel_metrics_setting.ChannelMetricsSetting) time.Duration {
	timeout := time.Duration(setting.FinalFlushTimeoutSeconds+1) * time.Second
	if timeout <= time.Second {
		return 6 * time.Second
	}
	return timeout
}

func channelMetricCollector() *channelmetrics.Collector {
	channelMetricsRuntimeMu.RLock()
	defer channelMetricsRuntimeMu.RUnlock()
	if channelMetricsCurrent == nil {
		return nil
	}
	return channelMetricsCurrent.collector
}

// channelMetricsEffectiveSetting 返回与当前采集器一致的启动快照。
// 采集粒度和维度快照不能在进程内混用，因此配置变更需重启后生效。
func channelMetricsEffectiveSetting() channel_metrics_setting.ChannelMetricsSetting {
	channelMetricsRuntimeMu.RLock()
	defer channelMetricsRuntimeMu.RUnlock()
	if channelMetricsCurrent != nil {
		return channelMetricsCurrent.setting
	}
	return channel_metrics_setting.GetSetting().Normalized()
}

func recordChannelMetric(sample channelmetrics.Sample) {
	collector := channelMetricCollector()
	if collector == nil {
		return
	}
	if err := collector.Record(sample); err != nil {
		common.SysError("record channel metric failed: " + err.Error())
	}
}

func GetChannelMetricsQuality() channelmetrics.QualitySnapshot {
	collector := channelMetricCollector()
	if collector == nil {
		return channelmetrics.QualitySnapshot{}
	}
	return collector.Quality()
}

func enqueueChannelFailureEvents(events []model.ChannelFailureEvent) {
	if len(events) == 0 {
		return
	}
	channelMetricsRuntimeMu.RLock()
	runtime := channelMetricsCurrent
	if runtime == nil || !runtime.setting.Enabled {
		channelMetricsRuntimeMu.RUnlock()
		return
	}
	copyEvents := append([]model.ChannelFailureEvent(nil), events...)
	select {
	case runtime.failureCh <- copyEvents:
		channelMetricsRuntimeMu.RUnlock()
	default:
		collector := runtime.collector
		channelMetricsRuntimeMu.RUnlock()
		collector.RecordDroppedFailureEvents(int64(len(events)))
	}
}

func runChannelFailureWriter(ctx context.Context, runtime *channelMetricsRuntime) {
	defer close(runtime.failureDone)
	recordDropped := func(events []model.ChannelFailureEvent, err error) {
		runtime.collector.RecordDroppedFailureEvents(int64(len(events)))
		common.SysError("persist channel failure events failed: " + err.Error())
	}
	drain := func(first []model.ChannelFailureEvent) {
		finalCtx, cancel := context.WithTimeout(context.Background(), channelMetricFinalFlushTimeout(runtime.setting))
		defer cancel()

		write := func(events []model.ChannelFailureEvent) {
			if err := persistChannelFailureEvents(finalCtx, model.LOG_DB, events); err != nil {
				recordDropped(events, err)
			}
		}
		if len(first) > 0 {
			write(first)
		}
		for {
			select {
			case events := <-runtime.failureCh:
				if finalCtx.Err() != nil {
					dropped := int64(len(events))
					for {
						select {
						case remaining := <-runtime.failureCh:
							dropped += int64(len(remaining))
						default:
							runtime.collector.RecordDroppedFailureEvents(dropped)
							common.SysError("persist channel failure events stopped: " + finalCtx.Err().Error())
							return
						}
					}
				}
				write(events)
			default:
				return
			}
		}
	}

	for {
		select {
		case events := <-runtime.failureCh:
			err := persistChannelFailureEvents(ctx, model.LOG_DB, events)
			if err == nil {
				continue
			}
			if ctx.Err() != nil {
				// 正在执行的写入会被运行时上下文取消，再使用独立的有界上下文重试该批次并排空队列。
				drain(events)
				return
			}
			recordDropped(events, err)
		case <-ctx.Done():
			drain(nil)
			return
		}
	}
}

func persistChannelFailureEvents(ctx context.Context, db *gorm.DB, events []model.ChannelFailureEvent) error {
	if db == nil {
		return model.ErrChannelMetricInvalidBatch
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		err = model.InsertChannelFailureEvents(db.WithContext(ctx), events)
		if err == nil {
			return nil
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		}
	}
	return err
}

func runChannelMetricCleanup(ctx context.Context, runtime *channelMetricsRuntime) {
	defer close(runtime.cleanupDone)
	// 延迟首次清理，避免迁移和启动阶段立刻叠加数据库压力。
	initial := time.NewTimer(channelMetricCleanupInitialDelay)
	defer initial.Stop()
	select {
	case <-initial.C:
		// 继续进入动态调度循环。
	case <-ctx.Done():
		return
	}

	needsCatchUp := cleanupChannelMetricRetention(ctx)
	for {
		delay := channelMetricCleanupInterval
		if needsCatchUp {
			delay = channelMetricCleanupCatchUpDelay
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			needsCatchUp = cleanupChannelMetricRetention(ctx)
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

func cleanupChannelMetricRetention(ctx context.Context) bool {
	if model.LOG_DB == nil {
		return false
	}
	db := model.LOG_DB.WithContext(ctx)
	settings := channelMetricsEffectiveSetting()
	now := time.Now()
	metricCutoff := now.Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour).Unix()
	failureCutoff := now.Add(-time.Duration(settings.FailureRetentionDays) * 24 * time.Hour).Unix()
	flushRetentionDays := settings.RetentionDays
	if settings.FailureRetentionDays > flushRetentionDays {
		flushRetentionDays = settings.FailureRetentionDays
	}
	flushCutoff := now.Add(-time.Duration(flushRetentionDays) * 24 * time.Hour).Unix()

	needsCatchUp := deleteChannelMetricBatches("channel metric buckets", func() (int64, error) {
		return model.DeleteChannelMetricBucketsBefore(db, metricCutoff, model.ChannelMetricMaxDeleteBatch)
	})
	if deleteChannelMetricBatches("channel failure events", func() (int64, error) {
		return model.DeleteChannelFailureEventsBefore(db, failureCutoff, model.ChannelMetricMaxDeleteBatch)
	}) {
		needsCatchUp = true
	}
	if deleteChannelMetricBatches("channel metric flushes", func() (int64, error) {
		return model.DeleteChannelMetricFlushesBefore(db, flushCutoff, model.ChannelMetricMaxDeleteBatch)
	}) {
		needsCatchUp = true
	}
	return needsCatchUp
}

func deleteChannelMetricBatches(name string, remove func() (int64, error)) bool {
	// 单轮有界执行；若仍有积压，运行时会短间隔续跑，避免清理速度受六小时间隔硬限制。
	for batch := 0; batch < channelMetricCleanupMaxBatches; batch++ {
		removed, err := remove()
		if err != nil {
			common.SysError("cleanup " + name + " failed: " + err.Error())
			return true
		}
		if removed < model.ChannelMetricMaxDeleteBatch {
			return false
		}
	}
	return true
}

func flushChannelMetricBatch(ctx context.Context, batch channelmetrics.MetricBatch) error {
	if batch.Empty() {
		return nil
	}
	buckets := make([]model.ChannelMetricBucket, 0, len(batch.Buckets))
	for _, bucket := range batch.Buckets {
		buckets = append(buckets, channelMetricBucketToModel(bucket))
	}
	flush := &model.ChannelMetricFlush{
		FlushId:                     batch.ID,
		InstanceId:                  batch.NodeID,
		BatchCreatedAt:              batch.CreatedAtUnixMs / 1000,
		CommittedAt:                 time.Now().Unix(),
		InvalidSampleCount:          batch.Quality.InvalidSampleCount,
		DimensionOverflowCount:      batch.Quality.DimensionOverflowCount,
		DroppedMetricEventCount:     batch.Quality.DroppedMetricEventCount,
		DroppedFailureEventCount:    batch.Quality.DroppedFailureEventCount,
		DimensionHashCollisionCount: batch.Quality.DimensionHashCollisionCount,
	}
	_, err := model.FlushChannelMetrics(model.LOG_DB.WithContext(ctx), flush, buckets, nil)
	return err
}

func channelMetricBucketToModel(bucket channelmetrics.Bucket) model.ChannelMetricBucket {
	dimension := bucket.Dimension
	counters := bucket.Counters
	result := model.ChannelMetricBucket{
		BucketLevel:              bucket.BucketLevel,
		BucketTs:                 bucket.BucketTs,
		DimensionHash:            bucket.DimensionHash,
		DimensionVersion:         dimension.Version,
		MetricScope:              string(dimension.Scope),
		ChannelPresent:           dimension.ChannelPresent,
		ChannelId:                dimension.ChannelID,
		ChannelNameSnapshot:      dimension.ChannelNameSnapshot,
		ChannelNameHash:          dimension.ChannelNameHash,
		ChannelType:              dimension.ChannelType,
		RequestedModelPresent:    dimension.RequestedModelPresent,
		RequestedModel:           dimension.RequestedModel,
		RequestedModelHash:       dimension.RequestedModelHash,
		UpstreamModelPresent:     dimension.UpstreamModelPresent,
		UpstreamModel:            dimension.UpstreamModel,
		UpstreamModelHash:        dimension.UpstreamModelHash,
		Group:                    dimension.Group,
		GroupHash:                dimension.GroupHash,
		TrafficSource:            string(dimension.TrafficSource),
		DataOrigin:               string(dimension.DataOrigin),
		Stream:                   dimension.Stream,
		Outcome:                  string(dimension.Outcome),
		ErrorStage:               string(dimension.ErrorStage),
		FailureOwner:             string(dimension.FailureOwner),
		QualityEligible:          dimension.QualityEligible,
		PartialResponse:          dimension.PartialResponse,
		Overflowed:               dimension.Overflowed,
		ClientStatusPresent:      dimension.ClientStatus.Present,
		ClientStatusCode:         dimension.ClientStatus.Code,
		UpstreamStatusPresent:    dimension.UpstreamStatus.Present,
		UpstreamStatusCode:       dimension.UpstreamStatus.Code,
		NormalizedStatusPresent:  dimension.NormalizedStatus.Present,
		NormalizedStatusCode:     dimension.NormalizedStatus.Code,
		EventCount:               counters.EventCount,
		SuccessCount:             counters.SuccessCount,
		NonFirstAttemptCount:     counters.NonFirstAttemptCount,
		RetryPlannedCount:        counters.RetryPlannedCount,
		QualityEligibleCount:     counters.QualityEligibleCount,
		QualitySuccessCount:      counters.QualitySuccessCount,
		PartialResponseCount:     counters.PartialResponseCount,
		UsageSampleCount:         counters.UsageSampleCount,
		CacheHitRequestCount:     counters.CacheHitRequestCount,
		InputTokensTotal:         counters.InputTokensTotal,
		UncachedInputTokens:      counters.UncachedInputTokens,
		OutputTokens:             counters.OutputTokens,
		CacheReadTokens:          counters.CacheReadTokens,
		CacheWriteTokens:         counters.CacheWriteTokens,
		ChargedQuota:             counters.ChargedQuota,
		ChargedMicroUsd:          counters.ChargedMicroUSD,
		LatencySumMs:             counters.LatencySumMs,
		LatencyCount:             counters.LatencyCount,
		TtftSumMs:                counters.TTFTSumMs,
		TtftCount:                counters.TTFTCount,
		DimensionOverflowCount:   counters.DimensionOverflowCount,
		DroppedMetricEventCount:  counters.DroppedMetricEventCount,
		DroppedFailureEventCount: counters.DroppedFailureEventCount,
	}
	result.SetLatencyHistogram(counters.LatencyHistogram.Counts)
	result.SetTtftHistogram(counters.TTFTHistogram.Counts)
	return result
}
