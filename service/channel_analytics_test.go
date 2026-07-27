package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	"github.com/QuantumNous/new-api/setting/channel_metrics_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelAnalyticsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", channelmetrics.SHA256String(t.Name())[:16])
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Group{}, &model.GroupAlias{}))
	require.NoError(t, model.MigrateChannelAnalyticsLogDB(db))

	oldDB, oldLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestParseChannelAnalyticsQueryRejectsUnboundedAndInvalidFilters(t *testing.T) {
	now := time.Now().Unix()
	_, err := ParseChannelAnalyticsQuery(url.Values{
		"start_timestamp": {fmt.Sprintf("%d", now-8*24*60*60)},
		"end_timestamp":   {fmt.Sprintf("%d", now)},
	})
	require.ErrorIs(t, err, ErrInvalidChannelAnalyticsQuery)

	_, err = ParseChannelAnalyticsQuery(url.Values{"metric_scope": {"unknown"}})
	require.ErrorIs(t, err, ErrInvalidChannelAnalyticsQuery)

	query, err := ParseChannelAnalyticsQuery(url.Values{
		"channel_ids":    {"1,2", "2,3"},
		"stream":         {"false"},
		"traffic_source": {"relay"},
	})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, query.ChannelIds)
	require.NotNil(t, query.Stream)
	assert.False(t, *query.Stream)
	assert.Equal(t, []string{"relay"}, query.TrafficSources)

	query, err = ParseChannelAnalyticsQuery(url.Values{"upstream_status_codes": {"5xx"}})
	require.NoError(t, err)
	require.Len(t, query.UpstreamStatusCodes, 100)
	assert.Equal(t, 500, query.UpstreamStatusCodes[0])
	assert.Equal(t, 599, query.UpstreamStatusCodes[99])

	modelHash := channelmetrics.SHA256String("超长模型")
	query, err = ParseChannelAnalyticsQuery(url.Values{"requested_model_hashes": {strings.ToUpper(modelHash)}})
	require.NoError(t, err)
	assert.Equal(t, []string{modelHash}, query.RequestedModelHash)
	_, err = ParseChannelAnalyticsQuery(url.Values{"requested_model_hashes": {"not-a-hash"}})
	require.ErrorIs(t, err, ErrInvalidChannelAnalyticsQuery)

	_, err = ParseChannelAnalyticsQuery(url.Values{"page": {"9223372036854775807"}})
	require.ErrorIs(t, err, ErrInvalidChannelAnalyticsQuery)

	failureQuery, err := ParseChannelAnalyticsFailureQuery(url.Values{
		"start_timestamp": {fmt.Sprintf("%d", now-10*24*60*60)},
		"end_timestamp":   {fmt.Sprintf("%d", now)},
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, failureQuery.EndTimestamp-failureQuery.StartTimestamp, int64(11*24*60*60))
}

func TestChannelAnalyticsPaginationRejectsOverflowingOffsets(t *testing.T) {
	channels := []dto.ChannelAnalyticsChannelItem{{ChannelId: 1}}
	models := []dto.ChannelAnalyticsModelItem{{ChannelAnalyticsChannelItem: dto.ChannelAnalyticsChannelItem{ChannelId: 1}}}
	assert.Empty(t, paginateChannelItems(channels, int(^uint(0)>>1), 100))
	assert.Empty(t, paginateModelItems(models, int(^uint(0)>>1), 100))
}

func TestChannelAnalyticsSummaryKeepsScopesAndCancellationSeparate(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	rows := []model.ChannelMetricBucket{
		channelAnalyticsTestBucket(bucketTs, "final-success", string(channelmetrics.ScopeFinalRequest), string(channelmetrics.OutcomeSuccess), 8, 8),
		channelAnalyticsTestBucket(bucketTs, "final-cancel", string(channelmetrics.ScopeFinalRequest), string(channelmetrics.OutcomeClientCancelled), 2, 0),
		channelAnalyticsTestBucket(bucketTs, "attempt-success", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeSuccess), 7, 7),
		channelAnalyticsTestBucket(bucketTs, "attempt-failure", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeHTTPError), 2, 0),
		channelAnalyticsTestBucket(bucketTs, "attempt-cancel", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeClientCancelled), 1, 0),
		channelAnalyticsTestBucket(bucketTs, "call", string(channelmetrics.ScopeUpstreamCall), string(channelmetrics.OutcomeSuccess), 12, 12),
	}
	rows[2].QualityEligibleCount = 7
	rows[2].QualitySuccessCount = 7
	rows[2].UsageSampleCount = 7
	rows[2].CacheHitRequestCount = 4
	rows[2].InputTokensTotal = 1000
	rows[2].UncachedInputTokens = 600
	rows[2].OutputTokens = 250
	rows[2].CacheReadTokens = 400
	rows[2].CacheWriteTokens = 50
	rows[2].ChargedQuota = 1234
	rows[2].ChargedMicroUsd = 3456
	rows[2].LatencySumMs = 5600
	rows[2].LatencyCount = 7
	rows[2].LatencyBucket1S = 7
	rows[2].TtftSumMs = 2100
	rows[2].TtftCount = 7
	rows[2].TtftBucket500Ms = 7
	rows[3].QualityEligibleCount = 2
	rows[3].LatencySumMs = 4000
	rows[3].LatencyCount = 2
	rows[3].LatencyBucket2S = 2
	rows[3].TtftSumMs = 1200
	rows[3].TtftCount = 2
	rows[3].TtftBucket1S = 2
	require.NoError(t, db.Create(&rows).Error)

	query := channelAnalyticsTestQuery(bucketTs)
	response, err := GetChannelAnalyticsSummary(query)
	require.NoError(t, err)
	assert.Equal(t, int64(10), response.Summary.FinalRequestCount)
	assert.Equal(t, int64(10), response.Summary.ChannelAttemptCount)
	assert.Equal(t, int64(12), response.Summary.UpstreamCallCount)
	assert.Equal(t, int64(2), response.Summary.FailedAttemptCount)
	require.NotNil(t, response.Summary.ClientSuccessRate)
	assert.InDelta(t, 1, *response.Summary.ClientSuccessRate, 0.0001)
	require.NotNil(t, response.Summary.AttemptSuccessRate)
	assert.InDelta(t, float64(7)/9, *response.Summary.AttemptSuccessRate, 0.0001)
	require.NotNil(t, response.Summary.ChannelQualitySuccessRate)
	assert.InDelta(t, float64(7)/9, *response.Summary.ChannelQualitySuccessRate, 0.0001)
	assert.Equal(t, int64(1250), response.Summary.TotalTokens)
	assert.Equal(t, int64(400), response.Summary.CacheReadTokens)
	require.NotNil(t, response.Summary.P95LatencyMs)
	assert.Equal(t, int64(2000), *response.Summary.P95LatencyMs)
	require.NotNil(t, response.Summary.AvgTtftMs)
	assert.InDelta(t, float64(3300)/9, *response.Summary.AvgTtftMs, 0.0001)
	require.NotNil(t, response.Summary.P95TtftMs)
	assert.Equal(t, int64(1000), *response.Summary.P95TtftMs)
}

func TestChannelAnalyticsChannelStatusAndFailureDrilldown(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	require.NoError(t, db.Create(&model.Channel{
		Id: 11, Name: "当前渠道名", Type: constant.ChannelTypeOpenAI, Key: "test-key", Group: "default",
	}).Error)

	oldName := channelAnalyticsTestBucket(bucketTs, "attempt-old", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeSuccess), 3, 3)
	oldName.ChannelId, oldName.ChannelNameSnapshot, oldName.ChannelType = 11, "历史渠道名", constant.ChannelTypeOpenAI
	oldName.ChannelPresent = true
	oldName.QualityEligibleCount, oldName.QualitySuccessCount = 3, 3
	newName := channelAnalyticsTestBucket(bucketTs, "attempt-new", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeHTTPError), 2, 0)
	newName.ChannelId, newName.ChannelNameSnapshot, newName.ChannelType = 11, "重命名后渠道", constant.ChannelTypeOpenAI
	newName.ChannelPresent = true
	newName.QualityEligibleCount = 2
	status429 := channelAnalyticsTestBucket(bucketTs, "status-429", string(channelmetrics.ScopeUpstreamCall), string(channelmetrics.OutcomeHTTPError), 2, 0)
	status429.ChannelId, status429.ChannelNameSnapshot, status429.ChannelType = 11, "重命名后渠道", constant.ChannelTypeOpenAI
	status429.ChannelPresent, status429.UpstreamStatusPresent, status429.UpstreamStatusCode = true, true, 429
	status200 := channelAnalyticsTestBucket(bucketTs, "status-200", string(channelmetrics.ScopeUpstreamCall), string(channelmetrics.OutcomeSuccess), 3, 3)
	status200.ChannelId, status200.ChannelNameSnapshot, status200.ChannelType = 11, "重命名后渠道", constant.ChannelTypeOpenAI
	status200.ChannelPresent, status200.UpstreamStatusPresent, status200.UpstreamStatusCode = true, true, 200
	require.NoError(t, db.Create(&[]model.ChannelMetricBucket{oldName, newName, status429, status200}).Error)
	require.NoError(t, db.Create(&model.ChannelFailureEvent{
		EventId: "failure-1", CreatedAt: bucketTs + 10, RequestId: "req-1", AttemptSeq: 1,
		ChannelId: 11, ChannelNameSnapshot: "重命名后渠道", ChannelType: constant.ChannelTypeOpenAI,
		TrafficSource: "relay", Outcome: "http_error", ErrorStage: "upstream_response",
		UpstreamStatusPresent: true, UpstreamStatusCode: 429, MaskedErrorSummary: "已脱敏错误",
	}).Error)

	query := channelAnalyticsTestQuery(bucketTs)
	channels, err := GetChannelAnalyticsChannels(query)
	require.NoError(t, err)
	require.Len(t, channels.Items, 1)
	item := channels.Items[0]
	assert.Equal(t, "当前渠道名", item.ChannelName)
	assert.Equal(t, int64(5), item.ChannelAttemptCount)
	assert.Equal(t, int64(2), item.FailureCount)
	require.NotNil(t, item.ChannelQualitySuccessRate)
	assert.InDelta(t, 0.6, *item.ChannelQualitySuccessRate, 0.0001)
	assert.Equal(t, bucketTs+10, item.LastFailureAt)
	require.Len(t, item.TopStatusCodes, 2)
	assert.Equal(t, 200, item.TopStatusCodes[0].StatusCode)

	query.MetricScope = string(channelmetrics.ScopeUpstreamCall)
	statuses, err := GetChannelAnalyticsStatusCodes(query)
	require.NoError(t, err)
	require.Len(t, statuses.Items, 2)
	assert.Equal(t, int64(3), statuses.Items[0].Count)

	query.MetricScope = ""
	failures, err := GetChannelAnalyticsFailures(query)
	require.NoError(t, err)
	require.Len(t, failures.Items, 1)
	assert.Equal(t, "已脱敏错误", failures.Items[0].ErrorSummary)
	assert.Equal(t, int64(1), failures.Total)
}

func TestChannelAnalyticsFailuresIncludeLegacyBackfillEvents(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	legacy := model.ChannelFailureEvent{
		EventId: "legacy-failure", CreatedAt: bucketTs + 10, RequestId: "legacy-request", AttemptSeq: 1,
		ChannelId: 51, ChannelNameSnapshot: "历史渠道", ChannelType: constant.ChannelTypeOpenAI,
		RequestedModel: "legacy-model", RequestedModelHash: channelmetrics.SHA256String("legacy-model"),
		TrafficSource: string(channelmetrics.TrafficSourceRelay), DataOrigin: string(channelmetrics.DataOriginLegacy),
		Outcome: string(channelmetrics.OutcomeHTTPError), ErrorStage: string(channelmetrics.ErrorStageUpstream),
		UpstreamStatusPresent: true, UpstreamStatusCode: 503,
	}
	live := legacy
	live.EventId, live.RequestId, live.DataOrigin = "live-failure", "live-request", string(channelmetrics.DataOriginLive)
	require.NoError(t, db.Create(&[]model.ChannelFailureEvent{legacy, live}).Error)

	query := channelAnalyticsTestQuery(bucketTs)
	query.DataOrigins = []string{string(channelmetrics.DataOriginLegacy)}
	response, err := GetChannelAnalyticsFailures(query)
	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, "legacy-failure", response.Items[0].EventId)
	assert.Equal(t, string(channelmetrics.DataOriginLegacy), response.Items[0].DataOrigin)

	query.DataOrigins = []string{string(channelmetrics.DataOriginLive)}
	response, err = GetChannelAnalyticsFailures(query)
	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, "live-failure", response.Items[0].EventId)
	assert.Equal(t, string(channelmetrics.DataOriginLive), response.Items[0].DataOrigin)
}

func TestChannelAnalyticsFailuresMatchLongModelByFullHashAndSnapshot(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	requestedModel := strings.Repeat("长模型-", 50)
	require.Greater(t, len(requestedModel), channelAnalyticsFailureModelSnapshotBytes)
	require.NoError(t, db.Create(&model.ChannelFailureEvent{
		EventId: "long-model-failure", CreatedAt: bucketTs + 10, RequestId: "long-model-request", AttemptSeq: 1,
		ChannelId: 52, ChannelNameSnapshot: "长模型渠道", ChannelType: constant.ChannelTypeOpenAI,
		RequestedModel:     channelmetrics.TruncateUTF8(requestedModel, channelAnalyticsFailureModelSnapshotBytes),
		RequestedModelHash: channelmetrics.SHA256String(requestedModel),
		TrafficSource:      string(channelmetrics.TrafficSourceRelay), DataOrigin: string(channelmetrics.DataOriginLive),
		Outcome: string(channelmetrics.OutcomeHTTPError), ErrorStage: string(channelmetrics.ErrorStageUpstream),
	}).Error)

	query := channelAnalyticsTestQuery(bucketTs)
	query.RequestedModels = []string{requestedModel}
	query.RequestedModelHash = []string{channelmetrics.SHA256String(requestedModel)}
	response, err := GetChannelAnalyticsFailures(query)
	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, "long-model-failure", response.Items[0].EventId)
}

func TestChannelAnalyticsModelsKeepDeletedChannelSnapshotAndUpstreamFailureTime(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	bucket := channelAnalyticsTestBucket(bucketTs, "deleted-channel-model", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeSuccess), 1, 1)
	bucket.ChannelPresent = true
	bucket.ChannelId = 99
	bucket.ChannelNameSnapshot = "已删除渠道快照"
	bucket.ChannelType = constant.ChannelTypeOpenAI
	bucket.RequestedModelPresent = true
	bucket.RequestedModel = "request-model"
	bucket.RequestedModelHash = channelmetrics.SHA256String("request-model")
	bucket.UpstreamModelPresent = true
	bucket.UpstreamModel = "upstream-model"
	bucket.UpstreamModelHash = channelmetrics.SHA256String("upstream-model")
	require.NoError(t, db.Create(&bucket).Error)
	require.NoError(t, db.Create(&model.ChannelFailureEvent{
		EventId: "deleted-channel-failure", CreatedAt: bucketTs + 20, RequestId: "req-deleted", AttemptSeq: 1,
		ChannelId: 99, ChannelNameSnapshot: "已删除渠道快照", ChannelType: constant.ChannelTypeOpenAI,
		RequestedModel: "request-model", UpstreamModel: "upstream-model",
		TrafficSource: "relay", Outcome: "http_error", ErrorStage: "upstream_response",
		MaskedErrorSummary: "已脱敏",
	}).Error)

	query := channelAnalyticsTestQuery(bucketTs)
	query.ModelDimension = "upstream"
	response, err := GetChannelAnalyticsModels(99, query)
	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	assert.Equal(t, "已删除渠道快照", response.Items[0].ChannelName)
	assert.Equal(t, "upstream-model", response.Items[0].UpstreamModel)
	assert.Equal(t, bucketTs+20, response.Items[0].LastFailureAt)
}

func TestChannelAnalyticsLongModelFilterUsesFullHash(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	fullModel := strings.Repeat("长模型", 80)
	modelSnapshot := channelmetrics.TruncateUTF8(fullModel, channel_metrics_setting.DefaultSetting().ModelSnapshotMaxBytes)
	modelHash := channelmetrics.SHA256String(fullModel)
	require.NotEqual(t, channelmetrics.SHA256String(modelSnapshot), modelHash)
	require.NoError(t, db.Create(&model.Channel{
		Id: 88, Name: "长模型渠道", Type: constant.ChannelTypeOpenAI, Key: "test-key", Group: "default",
	}).Error)

	bucket := channelAnalyticsTestBucket(bucketTs, "long-model", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeHTTPError), 1, 0)
	bucket.ChannelPresent, bucket.ChannelId, bucket.ChannelNameSnapshot = true, 88, "长模型渠道"
	bucket.ChannelType = constant.ChannelTypeOpenAI
	bucket.RequestedModelPresent = true
	bucket.RequestedModel = modelSnapshot
	bucket.RequestedModelHash = modelHash
	require.NoError(t, db.Create(&bucket).Error)
	require.NoError(t, db.Create(&model.ChannelFailureEvent{
		EventId: "long-model-failure", CreatedAt: bucketTs + 10, RequestId: "long-model-request", AttemptSeq: 1,
		ChannelId: 88, ChannelNameSnapshot: "长模型渠道", ChannelType: constant.ChannelTypeOpenAI,
		RequestedModel: modelSnapshot, RequestedModelHash: modelHash,
		TrafficSource: "relay", Outcome: "http_error", ErrorStage: "upstream_response",
	}).Error)

	filters, err := GetChannelAnalyticsFilters()
	require.NoError(t, err)
	require.NotEmpty(t, filters.RequestedModelOptions)
	assert.NotContains(t, filters.RequestedModels, modelSnapshot, "旧筛选契约不应暴露无法精确查询的截断快照")
	var found bool
	for _, option := range filters.RequestedModelOptions {
		if option.ModelHash == modelHash {
			found = true
			assert.Equal(t, modelSnapshot, option.Model)
			assert.Equal(t, modelHash, option.Value)
		}
	}
	assert.True(t, found, "筛选项必须保留完整模型哈希")

	query := channelAnalyticsTestQuery(bucketTs)
	query.RequestedModelHash = []string{modelHash}
	models, err := GetChannelAnalyticsModels(88, query)
	require.NoError(t, err)
	require.Len(t, models.Items, 1)
	assert.Equal(t, modelHash, models.Items[0].ModelHash)
	assert.Equal(t, bucketTs+10, models.Items[0].LastFailureAt)
	failures, err := GetChannelAnalyticsFailures(query)
	require.NoError(t, err)
	require.Len(t, failures.Items, 1)
	assert.Equal(t, modelHash, failures.Items[0].RequestedModelHash)
}

func TestChannelAnalyticsMetaSeparatesRuntimeFlushHealthFromWindowQuality(t *testing.T) {
	setupChannelAnalyticsTestDB(t)
	collector := channelmetrics.NewCollector(channelmetrics.DefaultConfig(), channelmetrics.SinkFunc(func(context.Context, channelmetrics.MetricBatch) error {
		return errors.New("模拟日志库写入失败")
	}))
	require.NoError(t, collector.Record(channelmetrics.NewLiveSample(channelmetrics.ScopeFinalRequest, channelmetrics.OutcomeSuccess)))
	require.Error(t, collector.Flush(context.Background()))

	runtime := &channelMetricsRuntime{collector: collector, setting: channel_metrics_setting.DefaultSetting()}
	channelMetricsRuntimeMu.Lock()
	previous := channelMetricsCurrent
	channelMetricsCurrent = runtime
	channelMetricsRuntimeMu.Unlock()
	t.Cleanup(func() {
		channelMetricsRuntimeMu.Lock()
		if channelMetricsCurrent == runtime {
			channelMetricsCurrent = previous
		}
		channelMetricsRuntimeMu.Unlock()
	})

	now := time.Now().Unix()
	query := dto.ChannelAnalyticsQuery{
		StartTimestamp: now - 3600, EndTimestamp: now,
		BucketLevel: "5m", BucketSeconds: 300,
		TrafficSources: []string{string(channelmetrics.TrafficSourceRelay)},
		DataOrigins:    []string{string(channelmetrics.DataOriginLive)},
	}
	meta, err := channelAnalyticsMeta(query, channelAnalyticsMetricFilter(query, ""), false)
	require.NoError(t, err)
	assert.True(t, meta.Partial)
	assert.EqualValues(t, 1, meta.RuntimePendingBatchCount)
	assert.EqualValues(t, 1, meta.RuntimeFlushFailureCount)
	assert.Positive(t, meta.RuntimeLastFlushErrorAt)
	assert.Zero(t, meta.InvalidSampleCount, "进程累计质量值不应灌入任意查询窗口")
}

func TestChannelAnalyticsMetaMarksLegacyQueryPartialUntilBackfillCompletes(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	now := time.Now().Unix()
	query := dto.ChannelAnalyticsQuery{
		StartTimestamp: now - 3600, EndTimestamp: now,
		BucketLevel: "5m", BucketSeconds: 300,
		TrafficSources: []string{string(channelmetrics.TrafficSourceRelay)},
		DataOrigins:    []string{string(channelmetrics.DataOriginLive), string(channelmetrics.DataOriginLegacy)},
	}
	meta, err := channelAnalyticsMeta(query, channelAnalyticsMetricFilter(query, ""), false)
	require.NoError(t, err)
	assert.True(t, meta.Partial, "请求历史数据但回填任务尚未建立时不能宣称数据完整")

	require.NoError(t, model.SaveChannelMetricBackfillJob(db, &model.ChannelMetricBackfillJob{
		JobId: channelMetricLegacyBackfillJobId, Status: model.ChannelMetricBackfillStatusCompleted,
		CreatedAt: now, UpdatedAt: now, CompletedAt: now,
	}))
	meta, err = channelAnalyticsMeta(query, channelAnalyticsMetricFilter(query, ""), false)
	require.NoError(t, err)
	assert.False(t, meta.Partial)
	require.NotNil(t, meta.Backfill)
	assert.Equal(t, model.ChannelMetricBackfillStatusCompleted, meta.Backfill.Status)
}

func TestSortChannelAnalyticsItemsByFailureCount(t *testing.T) {
	items := []dto.ChannelAnalyticsChannelItem{
		{ChannelId: 1, FailureCount: 2},
		{ChannelId: 2, FailureCount: 7},
		{ChannelId: 3, FailureCount: 1},
	}

	sortChannelAnalyticsItems(items, "failure_count", "desc")

	assert.Equal(t, []int{2, 1, 3}, []int{items[0].ChannelId, items[1].ChannelId, items[2].ChannelId})
}

func TestValidateChannelTableAnalyticsQueryAllowsChannelNameSort(t *testing.T) {
	assert.NoError(t, validateChannelTableAnalyticsQuery(dto.ChannelAnalyticsQuery{SortBy: "channel_name"}))
}

func TestSortChannelAnalyticsItemsByChannelName(t *testing.T) {
	items := []dto.ChannelAnalyticsChannelItem{
		{ChannelId: 6, ChannelName: "beta"},
		{ChannelId: 9, ChannelName: " 天才程序员 / Codex-Plus "},
		{ChannelId: 4, ChannelName: "Alpha"},
		{ChannelId: 7, ChannelName: "天才程序员 / Codex-Pro"},
		{ChannelId: 3, ChannelName: " alpha "},
		{ChannelId: 8, ChannelName: "  "},
	}

	sortChannelAnalyticsItems(items, "channel_name", "asc")
	assert.Equal(t, []int{3, 4, 6, 9, 7, 8}, channelAnalyticsItemIDs(items))

	sortChannelAnalyticsItems(items, "channel_name", "desc")
	assert.Equal(t, []int{7, 9, 6, 4, 3, 8}, channelAnalyticsItemIDs(items))
}

func TestChannelNameSortRunsBeforePagination(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	channels := []model.Channel{
		{Id: 4, Name: "天才程序员 / Codex-Pro", Type: constant.ChannelTypeOpenAI, Key: "key-4", Group: "default"},
		{Id: 2, Name: "beta", Type: constant.ChannelTypeOpenAI, Key: "key-2", Group: "default"},
		{Id: 3, Name: "天才程序员 / Codex-Plus", Type: constant.ChannelTypeOpenAI, Key: "key-3", Group: "default"},
		{Id: 1, Name: "alpha", Type: constant.ChannelTypeOpenAI, Key: "key-1", Group: "default"},
	}
	require.NoError(t, db.Create(&channels).Error)

	buckets := make([]model.ChannelMetricBucket, 0, len(channels))
	for index, channel := range channels {
		bucket := channelAnalyticsTestBucket(
			bucketTs,
			fmt.Sprintf("channel-name-page-%d", channel.Id),
			string(channelmetrics.ScopeChannelAttempt),
			string(channelmetrics.OutcomeSuccess),
			int64((index+1)*10),
			int64((index+1)*10),
		)
		bucket.ChannelPresent = true
		bucket.ChannelId = channel.Id
		bucket.ChannelNameSnapshot = fmt.Sprintf("历史名称 %d", channel.Id)
		bucket.ChannelType = channel.Type
		buckets = append(buckets, bucket)
	}
	require.NoError(t, db.Create(&buckets).Error)

	query := channelAnalyticsTestQuery(bucketTs)
	query.SortBy = "channel_name"
	query.SortOrder = "asc"
	query.Page = 2
	query.PageSize = 2
	response, err := GetChannelAnalyticsChannels(query)
	require.NoError(t, err)

	assert.Equal(t, 4, response.Total)
	assert.Equal(t, []int{3, 4}, channelAnalyticsItemIDs(response.Items))
}

func channelAnalyticsItemIDs(items []dto.ChannelAnalyticsChannelItem) []int {
	ids := make([]int, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ChannelId)
	}
	return ids
}

func TestParseChannelAnalyticsStabilityQueryUsesBoundedWindowsAndHistoricalOrigin(t *testing.T) {
	query, err := ParseChannelAnalyticsStabilityQuery(url.Values{
		"dimension": {"group_channel_model"},
		"windows":   {"15m,1h", "6h"},
	})
	require.NoError(t, err)
	assert.Equal(t, []int64{900, 3600, 21600}, query.WindowSeconds)
	assert.Equal(t, "group_channel_model", query.Dimension)
	assert.Equal(t, []string{"live", "legacy"}, query.DataOrigins)
	assert.Equal(t, int64(21600), query.EndTimestamp-query.StartTimestamp)

	_, err = ParseChannelAnalyticsStabilityQuery(url.Values{"dimension": {"unsafe_sql"}})
	require.ErrorIs(t, err, ErrInvalidChannelAnalyticsQuery)
	_, err = ParseChannelAnalyticsStabilityQuery(url.Values{"windows": {"1m"}})
	require.ErrorIs(t, err, ErrInvalidChannelAnalyticsQuery)

	query, err = ParseChannelAnalyticsStabilityQuery(url.Values{"data_origin": {"  , "}})
	require.NoError(t, err)
	assert.Equal(t, []string{"live", "legacy"}, query.DataOrigins)
}

func TestChannelAnalyticsFilterModelsSupportsSearchPaginationAndLiteralWildcards(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	models := []string{"alpha-model", "alpha%literal", "beta_model"}
	rows := make([]model.ChannelMetricBucket, 0, len(models)+1)
	for index, modelName := range models {
		row := channelAnalyticsTestBucket(bucketTs, fmt.Sprintf("filter-model-%d", index), string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeSuccess), 1, 1)
		row.RequestedModelPresent = true
		row.RequestedModel = modelName
		row.RequestedModelHash = channelmetrics.SHA256String(modelName)
		rows = append(rows, row)
	}
	duplicate := rows[0]
	duplicate.DimensionHash = channelmetrics.SHA256String("filter-model-alpha-duplicate")
	duplicate.Outcome = string(channelmetrics.OutcomeHTTPError)
	rows = append(rows, duplicate)
	require.NoError(t, db.Create(&rows).Error)

	query, err := ParseChannelAnalyticsFilterModelsQuery(url.Values{
		"model_dimension": {"requested"},
		"page":            {"1"},
		"page_size":       {"2"},
	})
	require.NoError(t, err)
	response, err := GetChannelAnalyticsFilterModels(query)
	require.NoError(t, err)
	assert.Equal(t, int64(3), response.Total)
	require.Len(t, response.Items, 2)
	assert.Equal(t, "alpha%literal", response.Items[0].Model)
	assert.Equal(t, "alpha-model", response.Items[1].Model)

	query.Page = 2
	response, err = GetChannelAnalyticsFilterModels(query)
	require.NoError(t, err)
	assert.Equal(t, int64(3), response.Total)
	require.Len(t, response.Items, 1)
	assert.Equal(t, "beta_model", response.Items[0].Model)

	query.Page, query.PageSize, query.Query = 1, 50, "%"
	response, err = GetChannelAnalyticsFilterModels(query)
	require.NoError(t, err)
	assert.Equal(t, int64(1), response.Total, "百分号必须按普通字符搜索，不能扩大为 SQL 通配符")
	require.Len(t, response.Items, 1)
	assert.Equal(t, "alpha%literal", response.Items[0].Model)

	_, err = ParseChannelAnalyticsFilterModelsQuery(url.Values{"model_dimension": {"unsafe"}})
	assert.ErrorIs(t, err, ErrInvalidChannelAnalyticsQuery)
}

func TestChannelAnalyticsStabilitySeparatesActualGroupChannelAndModel(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	require.NoError(t, db.Create(&model.Group{Id: 1, Code: "vip", Name: "VIP 用户", Status: model.GroupStatusActive}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 31, Name: "当前渠道", Type: constant.ChannelTypeOpenAI, Key: "test-key", Group: "default,vip",
	}).Error)

	success := channelAnalyticsTestBucket(bucketTs, "ops-success", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeSuccess), 8, 8)
	failure := channelAnalyticsTestBucket(bucketTs, "ops-failure", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeHTTPError), 2, 0)
	for _, row := range []*model.ChannelMetricBucket{&success, &failure} {
		row.ChannelPresent = true
		row.ChannelId, row.ChannelNameSnapshot, row.ChannelType = 31, "历史渠道", constant.ChannelTypeOpenAI
		row.Group, row.GroupHash = "vip", channelmetrics.SHA256String("vip")
		row.RequestedModelPresent = true
		row.RequestedModel, row.RequestedModelHash = "gpt-ops", channelmetrics.SHA256String("gpt-ops")
		row.QualityEligibleCount = row.EventCount
		row.QualitySuccessCount = row.SuccessCount
		row.LatencyCount = row.EventCount
		row.LatencySumMs = row.EventCount * 500
		row.LatencyBucket500Ms = row.EventCount
	}
	success.UsageSampleCount = 8
	success.InputTokensTotal = 1000
	success.OutputTokens = 200
	success.CacheHitRequestCount = 4
	success.CacheReadTokens = 400
	require.NoError(t, db.Create(&[]model.ChannelMetricBucket{success, failure}).Error)

	base := channelAnalyticsTestQuery(bucketTs)
	base.EndTimestamp = bucketTs + 300
	base.DataOrigins = []string{string(channelmetrics.DataOriginLive), string(channelmetrics.DataOriginLegacy)}
	query := dto.ChannelAnalyticsStabilityQuery{
		ChannelAnalyticsQuery: base,
		Dimension:             "group_channel_model",
		WindowSeconds:         []int64{300},
	}
	response, err := GetChannelAnalyticsStability(query)
	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	item := response.Items[0]
	assert.Equal(t, "vip", item.Group)
	assert.Equal(t, "VIP 用户", item.GroupName)
	assert.Equal(t, 31, item.ChannelId)
	assert.Equal(t, "当前渠道", item.ChannelName)
	assert.Equal(t, "gpt-ops", item.RequestedModel)
	require.Len(t, item.Windows, 1)
	window := item.Windows[0]
	assert.Equal(t, int64(10), window.ChannelAttemptCount)
	assert.Equal(t, int64(2), window.FailureCount)
	require.NotNil(t, window.QualitySuccessRate)
	assert.InDelta(t, 0.8, *window.QualitySuccessRate, 0.0001)
	assert.Equal(t, int64(1200), window.TotalTokens)
	assert.Equal(t, bucketTs, window.LastFailureBucketTs)

	filters, err := GetChannelAnalyticsFilters()
	require.NoError(t, err)
	require.Len(t, filters.Groups, 1)
	assert.Equal(t, dto.ChannelAnalyticsFilterGroup{Code: "vip", Name: "VIP 用户"}, filters.Groups[0])
}

func TestChannelAnalyticsExpandsAndCollapsesHistoricalGroupAliases(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300
	defaultGroup := model.Group{Code: "default", Name: "默认", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&defaultGroup).Error)
	group := model.Group{Code: "2", Name: "特价", Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.GroupAlias{Alias: "group_2", GroupId: group.Id}).Error)

	query, err := ParseChannelAnalyticsQuery(url.Values{"groups": {group.Code}})
	require.NoError(t, err)
	assert.Equal(t, []string{group.Code, "group_2"}, query.Groups)

	current := channelAnalyticsTestBucket(bucketTs, "current-group-code", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeSuccess), 1, 1)
	current.Group, current.GroupHash = group.Code, channelmetrics.SHA256String(group.Code)
	legacy := channelAnalyticsTestBucket(bucketTs, "legacy-group-code", string(channelmetrics.ScopeChannelAttempt), string(channelmetrics.OutcomeSuccess), 1, 1)
	legacy.Group, legacy.GroupHash = "group_2", channelmetrics.SHA256String("group_2")
	require.NoError(t, db.Create(&[]model.ChannelMetricBucket{current, legacy}).Error)

	filters, err := GetChannelAnalyticsFilters()
	require.NoError(t, err)
	require.Len(t, filters.Groups, 1)
	assert.Equal(t, dto.ChannelAnalyticsFilterGroup{Code: group.Code, Name: group.Name}, filters.Groups[0])
}

func TestChannelAnalyticsStabilityReturnsOperationalBreakdownAndCoverage(t *testing.T) {
	db := setupChannelAnalyticsTestDB(t)
	bucketTs := time.Now().Unix() / 300 * 300

	makeRow := func(name string, outcome channelmetrics.Outcome, count int64) model.ChannelMetricBucket {
		row := channelAnalyticsTestBucket(bucketTs, name, string(channelmetrics.ScopeChannelAttempt), string(outcome), count, 0)
		if outcome == channelmetrics.OutcomeSuccess {
			row.SuccessCount = count
			row.UsageSampleCount = count
		}
		row.ChannelPresent = true
		row.ChannelId, row.ChannelNameSnapshot, row.ChannelType = 41, "运维渠道", constant.ChannelTypeOpenAI
		row.Group, row.GroupHash = "ops", channelmetrics.SHA256String("ops")
		row.RequestedModelPresent = true
		row.RequestedModel, row.RequestedModelHash = "gpt-ops-breakdown", channelmetrics.SHA256String("gpt-ops-breakdown")
		if outcome != channelmetrics.OutcomeClientCancelled {
			row.QualityEligibleCount = count
		}
		if outcome == channelmetrics.OutcomeSuccess {
			row.QualitySuccessCount = count
		}
		row.LatencyCount = count
		row.LatencySumMs = count * 500
		row.LatencyBucket500Ms = count
		return row
	}
	makeUpstreamRow := func(name string, outcome channelmetrics.Outcome, count int64, statusCode int, origin channelmetrics.DataOrigin) model.ChannelMetricBucket {
		row := makeRow(name, outcome, count)
		row.MetricScope = string(channelmetrics.ScopeUpstreamCall)
		row.DataOrigin = string(origin)
		if statusCode > 0 {
			row.UpstreamStatusPresent = true
			row.UpstreamStatusCode = statusCode
		}
		return row
	}

	success := makeRow("ops-breakdown-success", channelmetrics.OutcomeSuccess, 12)
	success.TtftCount, success.TtftSumMs, success.TtftBucket250Ms = 12, 2400, 12
	rateLimited := makeRow("ops-breakdown-429", channelmetrics.OutcomeHTTPError, 2)
	rateLimited.DataOrigin = string(channelmetrics.DataOriginLegacy)
	rateLimited.UsageSampleCount = 2
	rateLimited.InputTokensTotal = 100
	serverError := makeRow("ops-breakdown-503", channelmetrics.OutcomeHTTPError, 3)
	transport := makeRow("ops-breakdown-transport", channelmetrics.OutcomeTransportError, 4)
	protocol := makeRow("ops-breakdown-protocol", channelmetrics.OutcomeProtocolError, 5)
	protocol.DataOrigin = string(channelmetrics.DataOriginLegacy)
	stream := makeRow("ops-breakdown-stream", channelmetrics.OutcomeStreamError, 6)
	stream.PartialResponseCount = 6
	cancelled := makeRow("ops-breakdown-cancelled", channelmetrics.OutcomeClientCancelled, 3)
	upstreamSuccess := makeUpstreamRow("ops-upstream-success", channelmetrics.OutcomeSuccess, 12, 200, channelmetrics.DataOriginLive)
	upstreamRateLimited := makeUpstreamRow("ops-upstream-429", channelmetrics.OutcomeHTTPError, 2, 429, channelmetrics.DataOriginLegacy)
	upstreamServerError := makeUpstreamRow("ops-upstream-503", channelmetrics.OutcomeHTTPError, 3, 503, channelmetrics.DataOriginLive)
	upstreamTransport := makeUpstreamRow("ops-upstream-transport", channelmetrics.OutcomeTransportError, 4, 0, channelmetrics.DataOriginLive)
	upstreamProtocol := makeUpstreamRow("ops-upstream-protocol", channelmetrics.OutcomeProtocolError, 5, 0, channelmetrics.DataOriginLegacy)
	upstreamStream := makeUpstreamRow("ops-upstream-stream", channelmetrics.OutcomeStreamError, 6, 0, channelmetrics.DataOriginLive)
	require.NoError(t, db.Create(&[]model.ChannelMetricBucket{
		success, rateLimited, serverError, transport, protocol, stream, cancelled,
		upstreamSuccess, upstreamRateLimited, upstreamServerError, upstreamTransport, upstreamProtocol, upstreamStream,
	}).Error)

	base := channelAnalyticsTestQuery(bucketTs)
	base.EndTimestamp = bucketTs + 300
	base.DataOrigins = []string{string(channelmetrics.DataOriginLive), string(channelmetrics.DataOriginLegacy)}
	response, err := GetChannelAnalyticsStability(dto.ChannelAnalyticsStabilityQuery{
		ChannelAnalyticsQuery: base,
		Dimension:             "group_channel_model",
		WindowSeconds:         []int64{300},
	})
	require.NoError(t, err)
	require.Len(t, response.Items, 1)
	require.Len(t, response.Items[0].Windows, 1)
	window := response.Items[0].Windows[0]

	assert.Equal(t, int64(35), window.ChannelAttemptCount)
	assert.Equal(t, int64(32), window.UpstreamCallCount)
	assert.Equal(t, int64(20), window.FailureCount)
	assert.Equal(t, int64(32), window.AttemptEligibleCount)
	assert.Equal(t, int64(3), window.ClientCancelledCount)
	assert.Equal(t, int64(17), window.UpstreamStatusSampleCount)
	assert.Equal(t, int64(2), window.Upstream429Count)
	assert.Equal(t, int64(2), window.Upstream4xxCount)
	assert.Equal(t, int64(3), window.Upstream5xxCount)
	assert.Equal(t, int64(5), window.HTTPErrorCount)
	assert.Equal(t, int64(4), window.TransportErrorCount)
	assert.Equal(t, int64(5), window.ProtocolErrorCount)
	assert.Equal(t, int64(6), window.StreamErrorCount)
	assert.Equal(t, int64(28), window.LiveEventCount)
	assert.Equal(t, int64(7), window.LegacyEventCount)
	assert.True(t, window.SampleSufficient)
	assert.Equal(t, channelAnalyticsMinimumStabilitySamples, window.MinimumSampleCount)
	require.NotNil(t, window.UpstreamStatusCoverageRate)
	assert.InDelta(t, float64(17)/32, *window.UpstreamStatusCoverageRate, 0.0001)
	require.NotNil(t, window.LiveEventRate)
	assert.InDelta(t, float64(28)/35, *window.LiveEventRate, 0.0001)
	require.NotNil(t, window.LegacyEventRate)
	assert.InDelta(t, float64(7)/35, *window.LegacyEventRate, 0.0001)
	require.NotNil(t, window.UsageSuccessCoverageRate)
	assert.InDelta(t, 1, *window.UsageSuccessCoverageRate, 0.0001)
	require.NotNil(t, window.LatencyCoverageRate)
	assert.InDelta(t, 1, *window.LatencyCoverageRate, 0.0001)
	require.NotNil(t, window.TtftCoverageRate)
	assert.InDelta(t, float64(12)/35, *window.TtftCoverageRate, 0.0001)
}

func TestChannelAnalyticsStabilityAscendingRateKeepsMissingSamplesLast(t *testing.T) {
	lowRate := 0.8
	highRate := 0.99
	items := []dto.ChannelAnalyticsStabilityItem{
		{Key: "missing", Windows: []dto.ChannelAnalyticsStabilityWindow{{}}},
		{Key: "high", Windows: []dto.ChannelAnalyticsStabilityWindow{{QualitySuccessRate: &highRate}}},
		{Key: "low", Windows: []dto.ChannelAnalyticsStabilityWindow{{QualitySuccessRate: &lowRate}}},
	}
	sortChannelAnalyticsStabilityItems(items, "quality_success_rate", "asc")
	assert.Equal(t, []string{"low", "high", "missing"}, []string{items[0].Key, items[1].Key, items[2].Key})
}

func channelAnalyticsTestBucket(bucketTs int64, hash string, scope string, outcome string, events int64, successes int64) model.ChannelMetricBucket {
	return model.ChannelMetricBucket{
		BucketLevel:      "5m",
		BucketTs:         bucketTs,
		DimensionHash:    channelmetrics.SHA256String(hash),
		DimensionVersion: 1,
		MetricScope:      scope,
		TrafficSource:    string(channelmetrics.TrafficSourceRelay),
		DataOrigin:       string(channelmetrics.DataOriginLive),
		Outcome:          outcome,
		EventCount:       events,
		SuccessCount:     successes,
	}
}

func channelAnalyticsTestQuery(bucketTs int64) dto.ChannelAnalyticsQuery {
	return dto.ChannelAnalyticsQuery{
		StartTimestamp: bucketTs,
		EndTimestamp:   bucketTs + 300,
		BucketLevel:    "5m",
		BucketSeconds:  300,
		ModelDimension: "requested",
		TrafficSources: []string{string(channelmetrics.TrafficSourceRelay)},
		DataOrigins:    []string{string(channelmetrics.DataOriginLive)},
		Page:           1,
		PageSize:       30,
		SortOrder:      "desc",
	}
}
