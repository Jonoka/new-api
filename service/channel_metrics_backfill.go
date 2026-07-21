package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	"github.com/QuantumNous/new-api/setting/channel_metrics_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

const channelMetricLegacyBackfillJobId = "legacy-relay-logs-v1"

type channelMetricLegacyLogContext struct {
	channelName   string
	channelType   int
	requestModel  string
	upstreamModel string
	group         string
	trafficSource channelmetrics.TrafficSource
	attemptSeq    int
	isLast        bool
	skip          bool
	other         map[string]interface{}
}

type channelMetricLegacyOutcome struct {
	outcome         channelmetrics.Outcome
	owner           channelmetrics.FailureOwner
	stage           channelmetrics.ErrorStage
	qualityEligible bool
	partial         bool
	upstreamStarted bool
	status          channelmetrics.StatusCode
}

func runChannelMetricBackfill(ctx context.Context, runtime *channelMetricsRuntime) {
	if runtime == nil {
		return
	}
	defer close(runtime.backfillDone)
	if !runtime.setting.Enabled || !runtime.setting.BackfillEnabled || model.LOG_DB == nil {
		return
	}

	job, err := prepareChannelMetricBackfillJob(ctx, runtime)
	if err != nil {
		common.SysError("prepare channel metric legacy backfill failed: " + err.Error())
		return
	}
	if job.Status == model.ChannelMetricBackfillStatusCompleted {
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}
		if job.CurrentCursor >= job.MaxLogId {
			if err := completeEmptyChannelMetricBackfill(ctx, job); err != nil {
				common.SysError("complete channel metric legacy backfill failed: " + err.Error())
			}
			return
		}

		applied, batchErr := runChannelMetricBackfillBatchWithRetry(ctx, runtime.setting, job)
		if batchErr != nil {
			if ctx.Err() != nil {
				return
			}
			_ = model.MarkChannelMetricBackfillFailed(model.LOG_DB.WithContext(ctx), job.JobId, job.CurrentCursor, batchErr.Error())
			common.SysError("channel metric legacy backfill stopped: " + batchErr.Error())
			return
		}
		if !applied {
			job, err = model.GetChannelMetricBackfillJob(model.LOG_DB.WithContext(ctx), job.JobId)
			if err != nil {
				common.SysError("reload channel metric legacy backfill job failed: " + err.Error())
				return
			}
			if job.Status == model.ChannelMetricBackfillStatusCompleted {
				return
			}
			continue
		}

		if job.Status == model.ChannelMetricBackfillStatusCompleted {
			common.SysLog(fmt.Sprintf("channel metric legacy backfill completed: scanned=%d converted=%d skipped=%d buckets=%d failures=%d",
				job.ScannedRows, job.ConvertedRows, job.SkippedRows, job.MetricBucketCount, job.FailureEventCount))
			return
		}
		if runtime.setting.BackfillPauseMilliseconds > 0 {
			timer := time.NewTimer(time.Duration(runtime.setting.BackfillPauseMilliseconds) * time.Millisecond)
			select {
			case <-timer.C:
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
}

func runChannelMetricBackfillBatchWithRetry(ctx context.Context, setting channel_metrics_setting.ChannelMetricsSetting, job *model.ChannelMetricBackfillJob) (bool, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		applied, err := runChannelMetricBackfillBatch(ctx, setting, job)
		if err == nil {
			return applied, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 200 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return false, ctx.Err()
		}
	}
	return false, lastErr
}

func prepareChannelMetricBackfillJob(ctx context.Context, runtime *channelMetricsRuntime) (*model.ChannelMetricBackfillJob, error) {
	db := model.LOG_DB.WithContext(ctx)
	existing, err := model.GetChannelMetricBackfillJob(db, channelMetricLegacyBackfillJobId)
	if err == nil {
		if existing.Status == model.ChannelMetricBackfillStatusFailed || existing.Status == model.ChannelMetricBackfillStatusPending {
			if _, resumeErr := model.ResumeChannelMetricBackfillJob(db, existing.JobId, existing.CurrentCursor); resumeErr != nil {
				return nil, resumeErr
			}
			return model.GetChannelMetricBackfillJob(db, existing.JobId)
		}
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := time.Now()
	startTs := now.Add(-time.Duration(runtime.setting.RetentionDays) * 24 * time.Hour).Unix()
	cutoverTs := runtime.startedAt
	if cutoverTs <= 0 {
		cutoverTs = now.Unix()
	}
	earliestLive, err := model.GetEarliestLiveChannelMetricBucketTs(db, runtime.setting.BucketLevel)
	if err != nil {
		return nil, err
	}
	if earliestLive > 0 && earliestLive < cutoverTs {
		cutoverTs = earliestLive
	}

	maxLogId := int64(0)
	totalRows := int64(0)
	if cutoverTs > startTs {
		maxLogId, err = model.GetChannelMetricBackfillMaxLogId(db, startTs, cutoverTs)
		if err != nil {
			return nil, err
		}
		totalRows, err = model.CountChannelMetricBackfillLogs(db, startTs, cutoverTs, maxLogId)
		if err != nil {
			return nil, err
		}
	}
	status := model.ChannelMetricBackfillStatusRunning
	completedAt := int64(0)
	if maxLogId == 0 {
		status = model.ChannelMetricBackfillStatusCompleted
		completedAt = now.Unix()
	}
	job := &model.ChannelMetricBackfillJob{
		JobId: channelMetricLegacyBackfillJobId, Status: status,
		BackfillStartTs: startTs, LiveCutoverTs: cutoverTs,
		MaxLogId: maxLogId, TotalRows: totalRows, CurrentCursor: 0,
		CreatedAt: now.Unix(), UpdatedAt: now.Unix(), CompletedAt: completedAt,
	}
	if err := model.EnsureChannelMetricBackfillJob(db, job); err != nil {
		return nil, err
	}
	return model.GetChannelMetricBackfillJob(db, job.JobId)
}

func completeEmptyChannelMetricBackfill(ctx context.Context, job *model.ChannelMetricBackfillJob) error {
	if job.Status == model.ChannelMetricBackfillStatusCompleted {
		return nil
	}
	// max_log_id=0 的任务在创建时已经完成；这里只处理日志被并发清理形成的空洞。
	if job.MaxLogId <= job.CurrentCursor {
		job.Status = model.ChannelMetricBackfillStatusCompleted
		job.CompletedAt = time.Now().Unix()
		job.UpdatedAt = job.CompletedAt
		return model.SaveChannelMetricBackfillJob(model.LOG_DB.WithContext(ctx), job)
	}
	return nil
}

func runChannelMetricBackfillBatch(ctx context.Context, setting channel_metrics_setting.ChannelMetricsSetting, job *model.ChannelMetricBackfillJob) (bool, error) {
	db := model.LOG_DB.WithContext(ctx)
	logs, err := model.ListChannelMetricBackfillLogs(db, job.BackfillStartTs, job.LiveCutoverTs, job.CurrentCursor, job.MaxLogId, setting.BackfillBatchSize)
	if err != nil {
		return false, err
	}
	expectedCursor := job.CurrentCursor
	if len(logs) == 0 {
		nextJob := *job
		nextJob.CurrentCursor = nextJob.MaxLogId
		nextJob.Status = model.ChannelMetricBackfillStatusCompleted
		nextJob.CompletedAt = time.Now().Unix()
		nextJob.UpdatedAt = nextJob.CompletedAt
		accountForMissingChannelMetricBackfillRows(&nextJob)
		applied, applyErr := model.ApplyChannelMetricBackfillBatch(db, expectedCursor, &nextJob, nil, nil)
		if applied {
			*job = nextJob
		}
		return applied, applyErr
	}

	requestIds := make([]string, 0, len(logs))
	channelIds := make([]int, 0, len(logs))
	seenChannel := make(map[int]struct{}, len(logs))
	for i := range logs {
		if logs[i].RequestId != "" {
			requestIds = append(requestIds, logs[i].RequestId)
		}
		if _, exists := seenChannel[logs[i].ChannelId]; !exists {
			seenChannel[logs[i].ChannelId] = struct{}{}
			channelIds = append(channelIds, logs[i].ChannelId)
		}
	}
	lastLogIds, err := model.GetChannelMetricBackfillLastLogIds(db, requestIds, job.MaxLogId, job.LiveCutoverTs)
	if err != nil {
		return false, err
	}
	channelSnapshots, err := legacyChannelMetricSnapshots(channelIds)
	if err != nil {
		return false, err
	}

	collectorConfig := setting.CollectorConfig()
	collectorConfig.Enabled = true
	collectorConfig.NodeID = "legacy-log-backfill"
	// 回填已经由 batch_size 严格限制内存，不能继续沿用实时采集器的容量降级。
	// 否则较小的实时容量配置会把历史分组/渠道/模型永久合并到 __other__。
	batchFactCapacity := len(logs) * 3
	if collectorConfig.MaxActiveDimensionsPerBucket < batchFactCapacity {
		collectorConfig.MaxActiveDimensionsPerBucket = batchFactCapacity
	}
	if collectorConfig.MaxHotBuckets < batchFactCapacity {
		collectorConfig.MaxHotBuckets = batchFactCapacity
	}
	collector := channelmetrics.NewCollector(collectorConfig, nil)
	failures := make([]model.ChannelFailureEvent, 0)
	convertedRows := int64(0)
	skippedRows := int64(0)
	for i := range logs {
		logRow := &logs[i]
		context := buildChannelMetricLegacyLogContext(logRow, channelSnapshots[logRow.ChannelId], lastLogIds)
		if context.skip {
			skippedRows++
			continue
		}
		samples, failure := channelMetricSamplesFromLegacyLog(logRow, context)
		recorded := false
		for _, sample := range samples {
			if err := collector.Record(sample); err != nil {
				common.SysError(fmt.Sprintf("skip invalid legacy channel metric log %d: %s", logRow.Id, err.Error()))
				continue
			}
			recorded = true
		}
		if recorded {
			convertedRows++
			if failure != nil {
				failures = append(failures, *failure)
			}
		} else {
			skippedRows++
		}
	}
	metricBatch := collector.Drain()
	buckets := make([]model.ChannelMetricBucket, 0, len(metricBatch.Buckets))
	for _, bucket := range metricBatch.Buckets {
		buckets = append(buckets, channelMetricBucketToModel(bucket))
	}

	nextJob := *job
	nextJob.CurrentCursor = int64(logs[len(logs)-1].Id)
	nextJob.ScannedRows += int64(len(logs))
	nextJob.ConvertedRows += convertedRows
	nextJob.SkippedRows += skippedRows
	nextJob.MetricBucketCount += int64(len(buckets))
	nextJob.FailureEventCount += int64(len(failures))
	nextJob.Status = model.ChannelMetricBackfillStatusRunning
	nextJob.LastError = ""
	nextJob.UpdatedAt = time.Now().Unix()
	if nextJob.CurrentCursor >= nextJob.MaxLogId {
		nextJob.Status = model.ChannelMetricBackfillStatusCompleted
		nextJob.CompletedAt = nextJob.UpdatedAt
		accountForMissingChannelMetricBackfillRows(&nextJob)
	}
	applied, applyErr := model.ApplyChannelMetricBackfillBatch(db, expectedCursor, &nextJob, buckets, failures)
	if applied {
		*job = nextJob
	}
	return applied, applyErr
}

func accountForMissingChannelMetricBackfillRows(job *model.ChannelMetricBackfillJob) {
	if job != nil && job.TotalRows > job.ScannedRows {
		job.SkippedRows += job.TotalRows - job.ScannedRows
	}
}

func legacyChannelMetricSnapshots(ids []int) (map[int]*model.Channel, error) {
	result := make(map[int]*model.Channel, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var channels []*model.Channel
	if err := model.DB.Select("id", "name", "type").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		if channel != nil {
			result[channel.Id] = channel
		}
	}
	return result, nil
}

func buildChannelMetricLegacyLogContext(logRow *model.Log, channel *model.Channel, lastLogIds map[string]int64) channelMetricLegacyLogContext {
	other, err := common.StrToMap(logRow.Other)
	if err != nil || other == nil {
		other = map[string]interface{}{}
	}
	channelName := legacyMetricString(other["channel_name"])
	channelType := legacyMetricInt(other["channel_type"])
	if channel != nil {
		if channelName == "" {
			channelName = channel.Name
		}
		if channelType == 0 {
			channelType = channel.Type
		}
	}
	if channelName == "" {
		channelName = fmt.Sprintf("渠道 #%d", logRow.ChannelId)
	}
	requestModel := strings.TrimSpace(logRow.ModelName)
	upstreamModel := strings.TrimSpace(legacyMetricString(other["upstream_model_name"]))
	if upstreamModel == "" {
		upstreamModel = requestModel
	}
	group := strings.TrimSpace(logRow.Group)
	if group == "" {
		group = strings.TrimSpace(legacyMetricString(other["group"]))
	}
	requestId := strings.TrimSpace(logRow.RequestId)
	isLast := requestId == "" || lastLogIds[requestId] == int64(logRow.Id)
	isBillingAdjustment := legacyMetricString(other["task_id"]) != "" || legacyMetricBool(other["violation_fee"])
	return channelMetricLegacyLogContext{
		channelName: channelName, channelType: channelType,
		requestModel: requestModel, upstreamModel: upstreamModel, group: group,
		trafficSource: legacyMetricTrafficSource(logRow, other), attemptSeq: legacyMetricAttemptSeq(other),
		isLast: isLast, skip: isBillingAdjustment, other: other,
	}
}

func channelMetricSamplesFromLegacyLog(logRow *model.Log, legacy channelMetricLegacyLogContext) ([]channelmetrics.Sample, *model.ChannelFailureEvent) {
	classification := classifyChannelMetricLegacyLog(logRow, legacy.other)
	occurredAt := time.Unix(logRow.CreatedAt, 0)
	latencyMs, latencyPresent := legacyMetricLatency(logRow, legacy.other)
	ttftMs, ttftPresent := legacyMetricNonNegativeInt64(legacy.other["frt"])
	if ttftMs <= 0 {
		ttftPresent = false
	}
	retryPlanned := logRow.Type == model.LogTypeError && !legacy.isLast

	attempt := channelmetrics.NewLiveSample(channelmetrics.ScopeChannelAttempt, classification.outcome)
	attempt.DataOrigin = channelmetrics.DataOriginLegacy
	attempt.OccurredAt = occurredAt
	attempt.RequestID = strings.TrimSpace(logRow.RequestId)
	attempt.AttemptSeq = legacy.attemptSeq
	attempt.RetryPlanned = retryPlanned
	applyLegacyChannelMetricDimensions(&attempt, logRow, legacy)
	attempt.FailureOwner = classification.owner
	attempt.QualityEligible = classification.qualityEligible
	attempt.PartialResponse = classification.partial
	attempt.UpstreamStarted = classification.upstreamStarted
	attempt.ErrorStage = classification.stage
	attempt.LatencyPresent, attempt.LatencyMs = latencyPresent, latencyMs
	attempt.TTFTPresent, attempt.TTFTMs = ttftPresent, ttftMs
	if classification.outcome == channelmetrics.OutcomeSuccess {
		applyLegacyChannelMetricUsage(&attempt, logRow, legacy.other)
	}

	samples := []channelmetrics.Sample{attempt}
	if classification.upstreamStarted {
		upstream := channelmetrics.NewLiveSample(channelmetrics.ScopeUpstreamCall, classification.outcome)
		upstream.DataOrigin = channelmetrics.DataOriginLegacy
		upstream.OccurredAt = occurredAt
		upstream.RequestID = attempt.RequestID
		upstream.AttemptSeq = legacy.attemptSeq
		upstream.CallIndex = 1
		upstream.RetryPlanned = retryPlanned
		applyLegacyChannelMetricDimensions(&upstream, logRow, legacy)
		upstream.FailureOwner = classification.owner
		upstream.PartialResponse = classification.partial
		upstream.UpstreamStarted = true
		upstream.ErrorStage = classification.stage
		upstream.LatencyPresent, upstream.LatencyMs = latencyPresent, latencyMs
		upstream.ResponseHeaderPresent, upstream.ResponseHeaderMs = ttftPresent, ttftMs
		if classification.status.Present {
			upstream.NormalizedStatus = classification.status
			if classification.outcome == channelmetrics.OutcomeHTTPError {
				upstream.UpstreamStatus = classification.status
			}
		}
		samples = append(samples, upstream)
	}

	if legacy.isLast {
		final := channelmetrics.NewLiveSample(channelmetrics.ScopeFinalRequest, classification.outcome)
		final.DataOrigin = channelmetrics.DataOriginLegacy
		final.OccurredAt = occurredAt
		final.RequestID = attempt.RequestID
		final.RetryCount = legacy.attemptSeq - 1
		final.LastStartedAttemptPresent = true
		final.LastStartedAttemptSeq = legacy.attemptSeq
		applyLegacyChannelMetricDimensions(&final, logRow, legacy)
		final.FailureOwner = classification.owner
		final.PartialResponse = classification.partial
		final.ErrorStage = classification.stage
		final.LatencyPresent, final.LatencyMs = latencyPresent, latencyMs
		if classification.outcome == channelmetrics.OutcomeSuccess {
			final.ClientStatus = channelmetrics.PresentStatus(http.StatusOK)
		} else if classification.status.Present {
			final.ClientStatus = classification.status
		}
		samples = append(samples, final)
	}

	if classification.outcome == channelmetrics.OutcomeSuccess {
		return samples, nil
	}
	failure := &model.ChannelFailureEvent{
		EventId:   channelmetrics.SHA256String(fmt.Sprintf("legacy-log:v1:%d", logRow.Id)),
		CreatedAt: logRow.CreatedAt, RequestId: strings.TrimSpace(logRow.RequestId),
		AttemptSeq: legacy.attemptSeq, RetryPlanned: retryPlanned, IsLastStartedAttempt: legacy.isLast,
		CausalCallPresent: classification.upstreamStarted,
		ChannelId:         logRow.ChannelId, ChannelNameSnapshot: channelmetrics.TruncateUTF8(legacy.channelName, 191), ChannelType: legacy.channelType,
		RequestedModel: channelmetrics.TruncateUTF8(legacy.requestModel, 191), RequestedModelHash: channelMetricOptionalHash(legacy.requestModel),
		UpstreamModel: channelmetrics.TruncateUTF8(legacy.upstreamModel, 191), UpstreamModelHash: channelMetricOptionalHash(legacy.upstreamModel),
		Group: channelmetrics.TruncateUTF8(legacy.group, 64), TrafficSource: string(legacy.trafficSource), DataOrigin: string(channelmetrics.DataOriginLegacy),
		Outcome: string(classification.outcome), FailureOwner: string(classification.owner), QualityEligible: classification.qualityEligible,
		PartialResponse: classification.partial, ErrorStage: string(classification.stage),
		LatencyMs: latencyMs, TtftPresent: ttftPresent, TtftMs: ttftMs,
		MaskedErrorSummary: channelmetrics.TruncateUTF8(logRow.Content, 512),
	}
	if failure.CausalCallPresent {
		failure.CausalCallIndex = 1
	}
	if classification.status.Present {
		failure.NormalizedStatusPresent = true
		failure.NormalizedStatusCode = classification.status.Code
		if classification.outcome == channelmetrics.OutcomeHTTPError {
			failure.UpstreamStatusPresent = true
			failure.UpstreamStatusCode = classification.status.Code
		}
		if legacy.isLast {
			failure.ClientStatusPresent = true
			failure.ClientStatusCode = classification.status.Code
		}
	}
	return samples, failure
}

func applyLegacyChannelMetricDimensions(sample *channelmetrics.Sample, logRow *model.Log, legacy channelMetricLegacyLogContext) {
	sample.ChannelPresent = true
	sample.ChannelID = logRow.ChannelId
	sample.ChannelNameSnapshot = legacy.channelName
	sample.ChannelType = legacy.channelType
	sample.RequestedModelPresent = legacy.requestModel != ""
	sample.RequestedModel = legacy.requestModel
	sample.UpstreamModelPresent = legacy.upstreamModel != ""
	sample.UpstreamModel = legacy.upstreamModel
	sample.Group = legacy.group
	sample.TrafficSource = legacy.trafficSource
	sample.Stream = logRow.IsStream
}

func applyLegacyChannelMetricUsage(sample *channelmetrics.Sample, logRow *model.Log, other map[string]interface{}) {
	input := int64(logRow.PromptTokens)
	explicitInput := false
	if explicit, ok := legacyMetricNonNegativeInt64(other["input_tokens_total"]); ok {
		input = explicit
		explicitInput = true
	}
	cacheRead, _ := legacyMetricNonNegativeInt64(other["cache_tokens"])
	cacheWrite, hasCacheWrite := legacyMetricNonNegativeInt64(other["cache_write_tokens"])
	if !hasCacheWrite {
		cache5m, _ := legacyMetricNonNegativeInt64(other["cache_creation_tokens_5m"])
		cache1h, _ := legacyMetricNonNegativeInt64(other["cache_creation_tokens_1h"])
		cacheWrite = cache5m + cache1h
		cacheAggregate, _ := legacyMetricNonNegativeInt64(other["cache_creation_tokens"])
		if cacheAggregate > cacheWrite {
			cacheWrite = cacheAggregate
		}
	}
	output := int64(logRow.CompletionTokens)
	quota := int64(logRow.Quota)
	if input < 0 {
		input = 0
	}
	if output < 0 {
		output = 0
	}
	if quota < 0 {
		quota = 0
	}
	// Anthropic 用量语义中的 prompt_tokens 是未缓存输入；OpenAI 语义通常已经是总输入。
	if !explicitInput && (strings.EqualFold(legacyMetricString(other["usage_semantic"]), "anthropic") || legacyMetricBool(other["claude"])) {
		input += cacheRead + cacheWrite
	}
	if input < cacheRead+cacheWrite {
		input = cacheRead + cacheWrite
	}
	sample.UsagePresent = true
	sample.InputTokensTotal = input
	sample.UncachedInputTokens = input - cacheRead - cacheWrite
	sample.OutputTokens = output
	sample.CacheReadTokens = cacheRead
	sample.CacheWriteTokens = cacheWrite
	sample.ChargedQuota = quota
	if common.QuotaPerUnit > 0 {
		sample.ChargedMicroUSD = int64(float64(quota)/common.QuotaPerUnit*1_000_000 + 0.5)
	}
}

func classifyChannelMetricLegacyLog(logRow *model.Log, other map[string]interface{}) channelMetricLegacyOutcome {
	statusCode := legacyMetricInt(other["status_code"])
	status := channelmetrics.StatusCode{}
	if statusCode >= 100 && statusCode <= 999 {
		status = channelmetrics.PresentStatus(statusCode)
	}
	streamStatus := legacyMetricNestedString(other, "stream_status", "status")
	if logRow.Type == model.LogTypeConsume && streamStatus != "error" {
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeSuccess, owner: channelmetrics.FailureOwnerNone, qualityEligible: true, upstreamStarted: true, status: status}
	}
	if statusCode == 499 {
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeClientCancelled, owner: channelmetrics.FailureOwnerClient, stage: channelmetrics.ErrorStageStream, partial: true, upstreamStarted: true, status: status}
	}
	if streamStatus == "error" {
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeStreamError, owner: channelmetrics.FailureOwnerChannel, stage: channelmetrics.ErrorStageStream, qualityEligible: true, partial: true, upstreamStarted: true, status: status}
	}

	errorCode := types.ErrorCode(legacyMetricString(other["error_code"]))
	switch errorCode {
	case types.ErrorCodeDoRequestFailed, types.ErrorCodeChannelResponseTimeExceeded:
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeTransportError, owner: channelmetrics.FailureOwnerChannel, stage: channelmetrics.ErrorStageConnect, qualityEligible: true, upstreamStarted: true, status: status}
	case types.ErrorCodeReadResponseBodyFailed, types.ErrorCodeBadResponse, types.ErrorCodeBadResponseBody, types.ErrorCodeEmptyResponse:
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeProtocolError, owner: channelmetrics.FailureOwnerChannel, stage: channelmetrics.ErrorStageParse, qualityEligible: true, upstreamStarted: true, status: status}
	case types.ErrorCodeReadRequestBodyFailed, types.ErrorCodeConvertRequestFailed, types.ErrorCodeJsonMarshalFailed, types.ErrorCodeBadRequestBody:
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeLocalError, owner: channelmetrics.FailureOwnerGateway, stage: channelmetrics.ErrorStagePreUpstream, status: status}
	case types.ErrorCodeInvalidRequest, types.ErrorCodePromptBlocked:
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeLocalError, owner: channelmetrics.FailureOwnerClient, stage: channelmetrics.ErrorStagePreUpstream, status: status}
	case types.ErrorCodeGetChannelFailed:
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeDispatchError, owner: channelmetrics.FailureOwnerGateway, stage: channelmetrics.ErrorStageChannelSelect, status: status}
	}
	if statusCode >= 400 && statusCode <= 599 {
		owner := channelmetrics.FailureOwnerChannel
		eligible := true
		if statusCode == http.StatusBadRequest || statusCode == http.StatusNotFound || statusCode == http.StatusUnprocessableEntity {
			owner = channelmetrics.FailureOwnerClient
			eligible = false
		}
		return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeHTTPError, owner: owner, stage: channelmetrics.ErrorStageUpstream, qualityEligible: eligible, upstreamStarted: true, status: status}
	}
	return channelMetricLegacyOutcome{outcome: channelmetrics.OutcomeLocalError, owner: channelmetrics.FailureOwnerUnknown, stage: channelmetrics.ErrorStagePreUpstream, status: status}
}

func legacyMetricTrafficSource(logRow *model.Log, other map[string]interface{}) channelmetrics.TrafficSource {
	if legacyMetricBool(other["is_task"]) {
		return channelmetrics.TrafficSourceTask
	}
	// 老版本渠道测试日志可能还没有 request_path，但消费日志的内容和令牌名
	// 都固定为“模型测试”；必须识别出来，避免历史回填污染真实转发统计。
	if logRow != nil && strings.TrimSpace(logRow.TokenName) == "模型测试" && strings.TrimSpace(logRow.Content) == "模型测试" {
		return channelmetrics.TrafficSourceProbe
	}
	path := strings.ToLower(strings.TrimSpace(legacyMetricString(other["request_path"])))
	if strings.Contains(path, "playground") || path == "/pg" || strings.HasPrefix(path, "/pg/") {
		return channelmetrics.TrafficSourcePlayground
	}
	if strings.Contains(path, "channel/test") || strings.Contains(path, "channel_test") {
		return channelmetrics.TrafficSourceProbe
	}
	return channelmetrics.TrafficSourceRelay
}

func legacyMetricAttemptSeq(other map[string]interface{}) int {
	admin, ok := other["admin_info"].(map[string]interface{})
	if !ok {
		return 1
	}
	used, ok := admin["use_channel"].([]interface{})
	if !ok || len(used) == 0 {
		return 1
	}
	return len(used)
}

func legacyMetricLatency(logRow *model.Log, other map[string]interface{}) (int64, bool) {
	if milliseconds, ok := legacyMetricNonNegativeInt64(other["use_time_ms"]); ok {
		return milliseconds, true
	}
	if logRow.UseTime > 0 {
		return int64(logRow.UseTime) * 1000, true
	}
	return 0, false
}

func legacyMetricNestedString(values map[string]interface{}, outer string, inner string) string {
	nested, ok := values[outer].(map[string]interface{})
	if !ok {
		return ""
	}
	return legacyMetricString(nested[inner])
}

func legacyMetricString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func legacyMetricInt(value interface{}) int {
	parsed, ok := legacyMetricInt64(value)
	if !ok || parsed > int64(math.MaxInt) || parsed < int64(math.MinInt) {
		return 0
	}
	return int(parsed)
}

func legacyMetricNonNegativeInt64(value interface{}) (int64, bool) {
	parsed, ok := legacyMetricInt64(value)
	if !ok || parsed < 0 {
		return 0, false
	}
	return parsed, true
}

func legacyMetricInt64(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case uint64:
		if typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return legacyMetricFloat64(float64(typed))
	case float64:
		return legacyMetricFloat64(typed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		return legacyMetricFloat64(parsed)
	default:
		return 0, false
	}
}

func legacyMetricFloat64(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value > float64(math.MaxInt64) || value < float64(math.MinInt64) {
		return 0, false
	}
	return int64(math.Round(value)), true
}

func legacyMetricBool(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed
	default:
		return false
	}
}
