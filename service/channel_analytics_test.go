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
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
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

func TestSortChannelAnalyticsItemsByFailureCount(t *testing.T) {
	items := []dto.ChannelAnalyticsChannelItem{
		{ChannelId: 1, FailureCount: 2},
		{ChannelId: 2, FailureCount: 7},
		{ChannelId: 3, FailureCount: 1},
	}

	sortChannelAnalyticsItems(items, "failure_count", "desc")

	assert.Equal(t, []int{2, 1, 3}, []int{items[0].ChannelId, items[1].ChannelId, items[2].ChannelId})
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
