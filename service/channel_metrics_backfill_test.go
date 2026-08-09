package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	"github.com/QuantumNous/new-api/setting/channel_metrics_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelMetricSamplesFromLegacyConsumeLogPreserveOperationsDimensions(t *testing.T) {
	logRow := &model.Log{
		Id: 10, CreatedAt: 1000, Type: model.LogTypeConsume, ChannelId: 7,
		ModelName: "gpt-4.1", Group: "vip", PromptTokens: 120,
		CompletionTokens: 30, Quota: 500, UseTime: 2, IsStream: true,
		RequestId: "request-1",
	}
	other := map[string]interface{}{
		"upstream_model_name": "gpt-4.1-2025-04-14",
		"input_tokens_total":  150,
		"cache_tokens":        40,
		"cache_write_tokens":  10,
		"use_time_ms":         1234,
		"frt":                 321,
	}
	legacy := channelMetricLegacyLogContext{
		channelName: "主渠道", channelType: 1,
		requestModel: logRow.ModelName, upstreamModel: "gpt-4.1-2025-04-14", group: logRow.Group,
		trafficSource: channelmetrics.TrafficSourceRelay, attemptSeq: 2, isLast: true, other: other,
	}

	samples, failure := channelMetricSamplesFromLegacyLog(logRow, legacy)
	require.Nil(t, failure)
	require.Len(t, samples, 3)
	attempt := samples[0]
	assert.Equal(t, channelmetrics.ScopeChannelAttempt, attempt.Scope)
	assert.Equal(t, channelmetrics.DataOriginLegacy, attempt.DataOrigin)
	assert.Equal(t, "vip", attempt.Group)
	assert.Equal(t, "gpt-4.1", attempt.RequestedModel)
	assert.Equal(t, "gpt-4.1-2025-04-14", attempt.UpstreamModel)
	assert.EqualValues(t, 150, attempt.InputTokensTotal)
	assert.EqualValues(t, 100, attempt.UncachedInputTokens)
	assert.EqualValues(t, 40, attempt.CacheReadTokens)
	assert.EqualValues(t, 10, attempt.CacheWriteTokens)
	assert.EqualValues(t, 30, attempt.OutputTokens)
	assert.EqualValues(t, 1234, attempt.LatencyMs)
	assert.EqualValues(t, 321, attempt.TTFTMs)
	assert.NoError(t, attempt.Validate())
	assert.Equal(t, 200, samples[2].ClientStatus.Code)
	assert.NoError(t, samples[2].Validate())
}

func TestChannelMetricSamplesFromLegacyErrorExposeStatusAndOrigin(t *testing.T) {
	logRow := &model.Log{
		Id: 11, CreatedAt: 1100, Type: model.LogTypeError, ChannelId: 8,
		ModelName: "claude-sonnet", Group: "default", RequestId: "request-2",
		Content: "bad response status code 503", Other: common.MapToJsonStr(map[string]interface{}{"status_code": 503, "use_time_ms": 900}),
	}
	other, err := common.StrToMap(logRow.Other)
	require.NoError(t, err)
	legacy := channelMetricLegacyLogContext{
		channelName: "Claude", channelType: 14,
		requestModel: logRow.ModelName, upstreamModel: logRow.ModelName, group: logRow.Group,
		trafficSource: channelmetrics.TrafficSourceRelay, attemptSeq: 1, isLast: true, other: other,
	}

	samples, failure := channelMetricSamplesFromLegacyLog(logRow, legacy)
	require.Len(t, samples, 3)
	require.NotNil(t, failure)
	assert.Equal(t, channelmetrics.OutcomeHTTPError, samples[0].Outcome)
	assert.True(t, samples[0].QualityEligible)
	assert.Equal(t, 503, samples[1].UpstreamStatus.Code)
	assert.Equal(t, string(channelmetrics.DataOriginLegacy), failure.DataOrigin)
	assert.True(t, failure.UpstreamStatusPresent)
	assert.Equal(t, 503, failure.UpstreamStatusCode)
	assert.True(t, failure.ClientStatusPresent)
	assert.Equal(t, 503, failure.ClientStatusCode)
	for _, sample := range samples {
		assert.NoError(t, sample.Validate())
	}
}

func TestChannelMetricSamplesFromLegacyLocalErrorDoNotInventUpstreamCall(t *testing.T) {
	logRow := &model.Log{
		Id: 12, CreatedAt: 1200, Type: model.LogTypeError, ChannelId: 8,
		ModelName: "gpt-local", Group: "default", RequestId: "request-local",
		Other: common.MapToJsonStr(map[string]interface{}{
			"error_code":  string(types.ErrorCodeConvertRequestFailed),
			"status_code": 500,
		}),
	}
	other, err := common.StrToMap(logRow.Other)
	require.NoError(t, err)
	legacy := channelMetricLegacyLogContext{
		channelName: "本地转换", channelType: 1,
		requestModel: logRow.ModelName, upstreamModel: logRow.ModelName, group: logRow.Group,
		trafficSource: channelmetrics.TrafficSourceRelay, attemptSeq: 1, isLast: true, other: other,
	}

	samples, failure := channelMetricSamplesFromLegacyLog(logRow, legacy)
	require.Len(t, samples, 2)
	assert.Equal(t, channelmetrics.ScopeChannelAttempt, samples[0].Scope)
	assert.False(t, samples[0].UpstreamStarted)
	assert.Equal(t, channelmetrics.ScopeFinalRequest, samples[1].Scope)
	require.NotNil(t, failure)
	assert.False(t, failure.CausalCallPresent)
	for _, sample := range samples {
		assert.NoError(t, sample.Validate())
	}
}

func TestLegacyAnthropicUsageAddsCachedTokensToTotalInput(t *testing.T) {
	logRow := &model.Log{PromptTokens: 100, CompletionTokens: 20, Quota: 10}
	sample := channelmetrics.NewLiveSample(channelmetrics.ScopeChannelAttempt, channelmetrics.OutcomeSuccess)
	sample.AttemptSeq = 1
	sample.ChannelPresent = true
	sample.ChannelID = 1
	applyLegacyChannelMetricUsage(&sample, logRow, map[string]interface{}{
		"usage_semantic":     "anthropic",
		"cache_tokens":       float64(40),
		"cache_write_tokens": float64(10),
	})
	assert.EqualValues(t, 150, sample.InputTokensTotal)
	assert.EqualValues(t, 100, sample.UncachedInputTokens)
	assert.NoError(t, sample.Validate())

	legacySplit := channelmetrics.NewLiveSample(channelmetrics.ScopeChannelAttempt, channelmetrics.OutcomeSuccess)
	legacySplit.AttemptSeq = 1
	legacySplit.ChannelPresent = true
	legacySplit.ChannelID = 1
	applyLegacyChannelMetricUsage(&legacySplit, logRow, map[string]interface{}{
		"claude":                   true,
		"cache_tokens":             float64(40),
		"cache_creation_tokens":    float64(50),
		"cache_creation_tokens_5m": float64(10),
		"cache_creation_tokens_1h": float64(20),
	})
	assert.EqualValues(t, 190, legacySplit.InputTokensTotal)
	assert.EqualValues(t, 50, legacySplit.CacheWriteTokens)
	assert.EqualValues(t, 100, legacySplit.UncachedInputTokens)
}

func TestLegacyTaskBillingAdjustmentIsSkipped(t *testing.T) {
	logRow := &model.Log{
		Id: 12, Type: model.LogTypeConsume, ChannelId: 3,
		Other: common.MapToJsonStr(map[string]interface{}{
			"task_id": "task-1", "pre_consumed_quota": 10, "actual_quota": 20,
		}),
	}
	legacy := buildChannelMetricLegacyLogContext(logRow, nil, nil)
	assert.True(t, legacy.skip)

	taskLog := &model.Log{Id: 13, Type: model.LogTypeConsume, ChannelId: 3, Other: common.MapToJsonStr(map[string]interface{}{"is_task": true})}
	taskLegacy := buildChannelMetricLegacyLogContext(taskLog, nil, nil)
	assert.False(t, taskLegacy.skip)
	assert.Equal(t, channelmetrics.TrafficSourceTask, taskLegacy.trafficSource)
}

func TestLegacyViolationFeeAdjustmentIsSkipped(t *testing.T) {
	logRow := &model.Log{
		Id: 14, Type: model.LogTypeConsume, ChannelId: 3,
		Content: "Violation fee charged",
		Other:   common.MapToJsonStr(map[string]interface{}{"violation_fee": true}),
	}
	legacy := buildChannelMetricLegacyLogContext(logRow, nil, nil)
	assert.True(t, legacy.skip)
}

func TestLegacyChannelTestWithoutRequestPathIsClassifiedAsProbe(t *testing.T) {
	logRow := &model.Log{
		Id: 15, Type: model.LogTypeConsume, ChannelId: 4,
		TokenName: "模型测试", Content: "模型测试",
	}
	legacy := buildChannelMetricLegacyLogContext(logRow, nil, nil)
	assert.False(t, legacy.skip)
	assert.Equal(t, channelmetrics.TrafficSourceProbe, legacy.trafficSource)
}

func TestRunChannelMetricBackfillBatchCreatesLegacyFacts(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.Channel{Id: 21, Name: "历史渠道", Type: 1}).Error)
	logRow := &model.Log{
		CreatedAt: now - 3600, Type: model.LogTypeConsume, ChannelId: 21,
		ModelName: "gpt-history", Group: "ops", PromptTokens: 100,
		CompletionTokens: 20, Quota: 300, RequestId: "history-request",
		Other: common.MapToJsonStr(map[string]interface{}{"use_time_ms": 800, "cache_tokens": 25}),
	}
	require.NoError(t, db.Create(logRow).Error)

	setting := channel_metrics_setting.DefaultSetting().Normalized()
	setting.BackfillBatchSize = 100
	// 回填的有界临时采集器不能受实时容量配置影响，否则维度会落入 __other__。
	setting.MaxActiveDimensionsPerBucket = 1
	setting.MaxHotBuckets = 1
	runtime := &channelMetricsRuntime{setting: setting, startedAt: now, backfillDone: make(chan struct{})}
	job, err := prepareChannelMetricBackfillJob(context.Background(), runtime)
	require.NoError(t, err)
	assert.EqualValues(t, logRow.Id, job.MaxLogId)

	applied, err := runChannelMetricBackfillBatch(context.Background(), setting, job)
	require.NoError(t, err)
	assert.True(t, applied)
	assert.Equal(t, model.ChannelMetricBackfillStatusCompleted, job.Status)

	rows, err := model.QueryChannelMetricBuckets(db, model.ChannelMetricBucketFilter{
		DataOrigins: []string{string(channelmetrics.DataOriginLegacy)},
		ChannelIds:  []int{21},
	})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	var attempts int64
	for _, row := range rows {
		if row.MetricScope == string(channelmetrics.ScopeChannelAttempt) {
			attempts += row.EventCount
			assert.EqualValues(t, 25, row.CacheReadTokens)
			assert.Equal(t, "ops", row.Group)
		}
	}
	assert.EqualValues(t, 1, attempts)

	stored, err := model.GetChannelMetricBackfillJob(db, channelMetricLegacyBackfillJobId)
	require.NoError(t, err)
	assert.Equal(t, model.ChannelMetricBackfillStatusCompleted, stored.Status)
	assert.EqualValues(t, 1, stored.ScannedRows)

	summary, err := GetChannelAnalyticsSummary(dto.ChannelAnalyticsQuery{
		StartTimestamp: floorTimestamp(now-7200, int64(setting.BucketSeconds)),
		EndTimestamp:   ceilTimestamp(now, int64(setting.BucketSeconds)),
		BucketLevel:    setting.BucketLevel, BucketSeconds: int64(setting.BucketSeconds),
		TrafficSources: []string{string(channelmetrics.TrafficSourceRelay)},
		DataOrigins:    []string{string(channelmetrics.DataOriginLegacy)},
	})
	require.NoError(t, err)
	require.NotNil(t, summary.Meta.Backfill)
	assert.Equal(t, model.ChannelMetricBackfillStatusCompleted, summary.Meta.Backfill.Status)
	assert.EqualValues(t, 1, summary.Meta.Backfill.TotalRows)
}
