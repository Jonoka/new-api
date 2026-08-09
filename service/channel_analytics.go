package service

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	"gorm.io/gorm"
)

var ErrInvalidChannelAnalyticsQuery = errors.New("渠道统计查询参数无效")

const (
	channelAnalyticsMaxListItems              = 100
	channelAnalyticsMaxPage                   = 1_000_000
	channelAnalyticsFailureModelSnapshotBytes = 191
	channelAnalyticsFailureGroupSnapshotBytes = 64
)

var allChannelAnalyticsOutcomes = []string{
	string(channelmetrics.OutcomeSuccess),
	string(channelmetrics.OutcomeHTTPError),
	string(channelmetrics.OutcomeTransportError),
	string(channelmetrics.OutcomeProtocolError),
	string(channelmetrics.OutcomeStreamError),
	string(channelmetrics.OutcomeLocalError),
	string(channelmetrics.OutcomeDispatchError),
	string(channelmetrics.OutcomeClientCancelled),
}

var channelAnalyticsUncoveredRelayTypes = []int{
	constant.ChannelTypeMidjourney,
	constant.ChannelTypeMidjourneyPlus,
	constant.ChannelTypeBaidu,
	constant.ChannelTypeAli,
	constant.ChannelTypeXunfei,
	constant.ChannelTypeAws,
	constant.ChannelTypeDify,
	constant.ChannelTypeVolcEngine,
	constant.ChannelTypeCoze,
	constant.ChannelTypeReplicate,
}

var channelAnalyticsUncoveredTaskTypes = []int{
	constant.ChannelTypeOpenAI,
	constant.ChannelTypeAli,
	constant.ChannelTypeGemini,
	constant.ChannelTypeMiniMax,
	constant.ChannelTypeSunoAPI,
	constant.ChannelTypeVertexAi,
	constant.ChannelTypeXai,
	constant.ChannelTypeKling,
	constant.ChannelTypeJimeng,
	constant.ChannelTypeVidu,
	constant.ChannelTypeDoubaoVideo,
	constant.ChannelTypeSora,
}

// ParseChannelAnalyticsQuery 对所有公共过滤参数执行同一套严格解析。
func ParseChannelAnalyticsQuery(values url.Values) (dto.ChannelAnalyticsQuery, error) {
	return parseChannelAnalyticsQuery(values, channelMetricsEffectiveSetting().RetentionDays)
}

// ParseChannelAnalyticsFailureQuery 使用失败明细自身的保留期，避免第 8～14 天
// 的失败事件被 5 分钟聚合桶的较短保留期误拒绝。
func ParseChannelAnalyticsFailureQuery(values url.Values) (dto.ChannelAnalyticsQuery, error) {
	return parseChannelAnalyticsQuery(values, channelMetricsEffectiveSetting().FailureRetentionDays)
}

// ParseChannelAnalyticsFilterModelsQuery 解析大规模模型筛选项查询。
func ParseChannelAnalyticsFilterModelsQuery(values url.Values) (dto.ChannelAnalyticsFilterModelsQuery, error) {
	query := dto.ChannelAnalyticsFilterModelsQuery{
		ModelDimension: strings.ToLower(strings.TrimSpace(values.Get("model_dimension"))),
		Query:          strings.TrimSpace(values.Get("q")),
		Page:           1,
		PageSize:       50,
	}
	if query.ModelDimension == "" {
		query.ModelDimension = "requested"
	}
	if query.ModelDimension != "requested" && query.ModelDimension != "upstream" {
		return dto.ChannelAnalyticsFilterModelsQuery{}, invalidChannelAnalyticsQuery("model_dimension 仅支持 requested 或 upstream")
	}
	if len(query.Query) > 191 {
		return dto.ChannelAnalyticsFilterModelsQuery{}, invalidChannelAnalyticsQuery("q 不能超过 191 字节")
	}
	var err error
	if rawPage := strings.TrimSpace(values.Get("page")); rawPage != "" {
		query.Page, err = strconv.Atoi(rawPage)
		if err != nil || query.Page < 1 || query.Page > channelAnalyticsMaxPage {
			return dto.ChannelAnalyticsFilterModelsQuery{}, invalidChannelAnalyticsQuery("page 必须在 1 到 %d 之间", channelAnalyticsMaxPage)
		}
	}
	if rawPageSize := strings.TrimSpace(values.Get("page_size")); rawPageSize != "" {
		query.PageSize, err = strconv.Atoi(rawPageSize)
		if err != nil || query.PageSize < 1 || query.PageSize > 100 {
			return dto.ChannelAnalyticsFilterModelsQuery{}, invalidChannelAnalyticsQuery("page_size 必须在 1 到 100 之间")
		}
	}
	return query, nil
}

func parseChannelAnalyticsQuery(values url.Values, retentionDays int) (dto.ChannelAnalyticsQuery, error) {
	setting := channelMetricsEffectiveSetting()
	bucketSeconds := int64(setting.BucketSeconds)
	bucketLevel := setting.BucketLevel

	now := time.Now().Unix()
	endTimestamp, err := parseOptionalPositiveInt64(values.Get("end_timestamp"), now, "end_timestamp")
	if err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if endTimestamp > now {
		endTimestamp = now
	}
	startTimestamp, err := parseOptionalPositiveInt64(values.Get("start_timestamp"), endTimestamp-24*60*60, "start_timestamp")
	if err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if startTimestamp >= endTimestamp {
		return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("start_timestamp 必须小于 end_timestamp")
	}
	if retentionDays <= 0 {
		retentionDays = setting.RetentionDays
	}
	if endTimestamp-startTimestamp > int64(retentionDays)*24*60*60 {
		return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("查询范围不能超过 %d 天", retentionDays)
	}

	granularity := strings.ToLower(strings.TrimSpace(values.Get("granularity")))
	if granularity != "" && granularity != "auto" && granularity != bucketLevel {
		return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("当前仅支持 %s 粒度", bucketLevel)
	}

	query := dto.ChannelAnalyticsQuery{
		StartTimestamp: floorTimestamp(startTimestamp, bucketSeconds),
		EndTimestamp:   ceilTimestamp(endTimestamp, bucketSeconds),
		BucketLevel:    bucketLevel,
		BucketSeconds:  bucketSeconds,
		MetricScope:    strings.TrimSpace(values.Get("metric_scope")),
		ModelDimension: strings.ToLower(strings.TrimSpace(values.Get("model_dimension"))),
		Page:           1,
		PageSize:       30,
		SortBy:         strings.ToLower(strings.TrimSpace(values.Get("sort_by"))),
		SortOrder:      strings.ToLower(strings.TrimSpace(values.Get("sort_order"))),
	}
	if query.EndTimestamp <= query.StartTimestamp {
		query.EndTimestamp = query.StartTimestamp + bucketSeconds
	}
	if query.ModelDimension == "" {
		query.ModelDimension = "requested"
	}
	if query.ModelDimension != "requested" && query.ModelDimension != "upstream" {
		return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("model_dimension 仅支持 requested 或 upstream")
	}
	if query.MetricScope != "" && !channelmetrics.Scope(query.MetricScope).Valid() {
		return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("未知 metric_scope：%s", query.MetricScope)
	}

	if query.ChannelIds, err = parseIntList(values, "channel_ids", false); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.ChannelTypes, err = parseIntList(values, "channel_types", true); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.ClientStatusCodes, err = parseStatusCodeList(values, "client_status_codes", false); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.UpstreamStatusCodes, err = parseStatusCodeList(values, "upstream_status_codes", true); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.Groups, err = parseStringList(values, "groups"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	query.Groups, err = expandChannelAnalyticsGroupIdentifiers(query.Groups)
	if err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.RequestedModels, err = parseStringList(values, "requested_models"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.UpstreamModels, err = parseStringList(values, "upstream_models"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.RequestedModelHash, err = parseSHA256List(values, "requested_model_hashes"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.UpstreamModelHash, err = parseSHA256List(values, "upstream_model_hashes"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.Outcomes, err = parseStringList(values, "outcome"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.FailureOwners, err = parseStringList(values, "failure_owner"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.ErrorStages, err = parseStringList(values, "error_stage"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.TrafficSources, err = parseStringList(values, "traffic_source"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if query.DataOrigins, err = parseStringList(values, "data_origin"); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if len(query.TrafficSources) == 0 {
		query.TrafficSources = []string{string(channelmetrics.TrafficSourceRelay)}
	}
	if len(query.DataOrigins) == 0 {
		query.DataOrigins = []string{string(channelmetrics.DataOriginLive)}
	}
	if err := validateChannelAnalyticsEnums(query); err != nil {
		return dto.ChannelAnalyticsQuery{}, err
	}
	if len(query.RequestedModelHash) == 0 {
		for _, value := range query.RequestedModels {
			query.RequestedModelHash = append(query.RequestedModelHash, channelmetrics.SHA256String(value))
		}
	}
	if len(query.UpstreamModelHash) == 0 {
		for _, value := range query.UpstreamModels {
			query.UpstreamModelHash = append(query.UpstreamModelHash, channelmetrics.SHA256String(value))
		}
	}

	if rawStream := strings.TrimSpace(values.Get("stream")); rawStream != "" {
		stream, parseErr := strconv.ParseBool(rawStream)
		if parseErr != nil {
			return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("stream 必须为 true 或 false")
		}
		query.Stream = &stream
	}
	if rawPage := strings.TrimSpace(values.Get("page")); rawPage != "" {
		query.Page, err = strconv.Atoi(rawPage)
		if err != nil || query.Page < 1 || query.Page > channelAnalyticsMaxPage {
			return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("page 必须在 1 到 %d 之间", channelAnalyticsMaxPage)
		}
	}
	if rawPageSize := strings.TrimSpace(values.Get("page_size")); rawPageSize != "" {
		query.PageSize, err = strconv.Atoi(rawPageSize)
		if err != nil || query.PageSize < 1 || query.PageSize > 100 {
			return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("page_size 必须在 1 到 100 之间")
		}
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}
	if query.SortOrder != "asc" && query.SortOrder != "desc" {
		return dto.ChannelAnalyticsQuery{}, invalidChannelAnalyticsQuery("sort_order 仅支持 asc 或 desc")
	}
	return query, nil
}

func GetChannelAnalyticsSummary(query dto.ChannelAnalyticsQuery) (dto.ChannelAnalyticsSummaryResponse, error) {
	if err := validateCompositeChannelAnalyticsQuery(query); err != nil {
		return dto.ChannelAnalyticsSummaryResponse{}, err
	}
	finalFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeFinalRequest))
	attemptFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeChannelAttempt))
	callFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeUpstreamCall))

	finalTotal, err := model.AggregateChannelMetricTotals(model.LOG_DB, finalFilter)
	if err != nil {
		return dto.ChannelAnalyticsSummaryResponse{}, err
	}
	attemptTotal, err := model.AggregateChannelMetricTotals(model.LOG_DB, attemptFilter)
	if err != nil {
		return dto.ChannelAnalyticsSummaryResponse{}, err
	}
	callTotal, err := model.AggregateChannelMetricTotals(model.LOG_DB, callFilter)
	if err != nil {
		return dto.ChannelAnalyticsSummaryResponse{}, err
	}
	finalEligible, err := model.AggregateChannelMetricTotals(model.LOG_DB, withoutClientCancelled(finalFilter))
	if err != nil {
		return dto.ChannelAnalyticsSummaryResponse{}, err
	}
	attemptEligible, err := model.AggregateChannelMetricTotals(model.LOG_DB, withoutClientCancelled(attemptFilter))
	if err != nil {
		return dto.ChannelAnalyticsSummaryResponse{}, err
	}

	summary := dto.ChannelAnalyticsSummary{
		FinalRequestCount:         finalTotal.EventCount,
		ChannelAttemptCount:       attemptTotal.EventCount,
		UpstreamCallCount:         callTotal.EventCount,
		FailedAttemptCount:        channelAnalyticsMaxInt64(0, attemptEligible.EventCount-attemptEligible.SuccessCount),
		RetryCount:                attemptTotal.NonFirstAttemptCount,
		ClientSuccessRate:         ratioPointer(finalEligible.SuccessCount, finalEligible.EventCount),
		ChannelQualitySuccessRate: ratioPointer(attemptTotal.QualitySuccessCount, attemptTotal.QualityEligibleCount),
		AttemptSuccessRate:        ratioPointer(attemptEligible.SuccessCount, attemptEligible.EventCount),
		RetryRate:                 ratioPointer(attemptTotal.NonFirstAttemptCount, attemptTotal.EventCount),
		UsageSampleCount:          attemptTotal.UsageSampleCount,
		InputTokensTotal:          attemptTotal.InputTokensTotal,
		UncachedInputTokens:       attemptTotal.UncachedInputTokens,
		OutputTokens:              attemptTotal.OutputTokens,
		TotalTokens:               attemptTotal.InputTokensTotal + attemptTotal.OutputTokens,
		CacheReadTokens:           attemptTotal.CacheReadTokens,
		CacheWriteTokens:          attemptTotal.CacheWriteTokens,
		CacheRequestHitRate:       ratioPointer(attemptTotal.CacheHitRequestCount, attemptTotal.UsageSampleCount),
		CacheTokenHitRate:         ratioPointer(attemptTotal.CacheReadTokens, attemptTotal.InputTokensTotal),
		ChargedQuota:              attemptTotal.ChargedQuota,
		ChargedMicroUsd:           attemptTotal.ChargedMicroUsd,
		AvgLatencyMs:              averagePointer(attemptTotal.LatencySumMs, attemptTotal.LatencyCount),
		P95LatencyMs:              percentileLatency(attemptTotal.LatencyHistogram(), attemptTotal.LatencyCount, 0.95),
		AvgTtftMs:                 averagePointer(attemptTotal.TtftSumMs, attemptTotal.TtftCount),
		P95TtftMs:                 percentileLatency(attemptTotal.TtftHistogram(), attemptTotal.TtftCount, 0.95),
	}
	meta, err := channelAnalyticsMeta(query, channelAnalyticsMetricFilter(query, ""), true)
	if err != nil {
		return dto.ChannelAnalyticsSummaryResponse{}, err
	}
	return dto.ChannelAnalyticsSummaryResponse{Summary: summary, Meta: meta}, nil
}

func GetChannelAnalyticsTrend(query dto.ChannelAnalyticsQuery) (dto.ChannelAnalyticsTrendResponse, error) {
	if err := validateTrendChannelAnalyticsQuery(query); err != nil {
		return dto.ChannelAnalyticsTrendResponse{}, err
	}
	points := make(map[int64]*dto.ChannelAnalyticsTrendPoint)
	pointAt := func(timestamp int64) *dto.ChannelAnalyticsTrendPoint {
		point := points[timestamp]
		if point == nil {
			point = &dto.ChannelAnalyticsTrendPoint{BucketTs: timestamp}
			points[timestamp] = point
		}
		return point
	}

	if query.MetricScope != "" {
		rows, err := model.AggregateChannelMetricTrend(model.LOG_DB, channelAnalyticsMetricFilter(query, query.MetricScope))
		if err != nil {
			return dto.ChannelAnalyticsTrendResponse{}, err
		}
		for _, row := range rows {
			point := pointAt(row.BucketTs)
			point.EventCount = row.EventCount
			point.SuccessCount = row.SuccessCount
			point.AvgLatencyMs = averagePointer(row.LatencySumMs, row.LatencyCount)
			switch query.MetricScope {
			case string(channelmetrics.ScopeFinalRequest):
				point.FinalRequestCount = row.EventCount
			case string(channelmetrics.ScopeChannelAttempt):
				point.ChannelAttemptCount = row.EventCount
				point.TotalTokens = row.InputTokensTotal + row.OutputTokens
			case string(channelmetrics.ScopeUpstreamCall):
				point.UpstreamCallCount = row.EventCount
			}
		}
	} else {
		finalRows, err := model.AggregateChannelMetricTrend(model.LOG_DB, channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeFinalRequest)))
		if err != nil {
			return dto.ChannelAnalyticsTrendResponse{}, err
		}
		attemptFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeChannelAttempt))
		attemptRows, err := model.AggregateChannelMetricTrend(model.LOG_DB, attemptFilter)
		if err != nil {
			return dto.ChannelAnalyticsTrendResponse{}, err
		}
		eligibleAttemptRows, err := model.AggregateChannelMetricTrend(model.LOG_DB, withoutClientCancelled(attemptFilter))
		if err != nil {
			return dto.ChannelAnalyticsTrendResponse{}, err
		}
		callRows, err := model.AggregateChannelMetricTrend(model.LOG_DB, channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeUpstreamCall)))
		if err != nil {
			return dto.ChannelAnalyticsTrendResponse{}, err
		}
		for _, row := range finalRows {
			pointAt(row.BucketTs).FinalRequestCount = row.EventCount
		}
		for _, row := range attemptRows {
			point := pointAt(row.BucketTs)
			point.ChannelAttemptCount = row.EventCount
			point.TotalTokens = row.InputTokensTotal + row.OutputTokens
		}
		for _, row := range eligibleAttemptRows {
			pointAt(row.BucketTs).FailedAttemptCount = channelAnalyticsMaxInt64(0, row.EventCount-row.SuccessCount)
		}
		for _, row := range callRows {
			pointAt(row.BucketTs).UpstreamCallCount = row.EventCount
		}
	}

	resultPoints := make([]dto.ChannelAnalyticsTrendPoint, 0, len(points))
	for _, point := range points {
		resultPoints = append(resultPoints, *point)
	}
	sort.Slice(resultPoints, func(i, j int) bool { return resultPoints[i].BucketTs < resultPoints[j].BucketTs })
	meta, err := channelAnalyticsMeta(query, channelAnalyticsMetricFilter(query, query.MetricScope), query.MetricScope == "" || query.MetricScope == string(channelmetrics.ScopeUpstreamCall))
	if err != nil {
		return dto.ChannelAnalyticsTrendResponse{}, err
	}
	return dto.ChannelAnalyticsTrendResponse{Points: resultPoints, Meta: meta}, nil
}

func GetChannelAnalyticsChannels(query dto.ChannelAnalyticsQuery) (dto.ChannelAnalyticsChannelsResponse, error) {
	if err := validateChannelTableAnalyticsQuery(query); err != nil {
		return dto.ChannelAnalyticsChannelsResponse{}, err
	}
	attemptFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeChannelAttempt))
	attemptFilter.ChannelPresent = boolPointer(true)
	rows, err := model.AggregateChannelMetricsByChannel(model.LOG_DB, attemptFilter)
	if err != nil {
		return dto.ChannelAnalyticsChannelsResponse{}, err
	}
	eligibleRows, err := model.AggregateChannelMetricsByChannel(model.LOG_DB, withoutClientCancelled(attemptFilter))
	if err != nil {
		return dto.ChannelAnalyticsChannelsResponse{}, err
	}
	eligibleByChannel := mergeChannelAggregateRows(eligibleRows)
	mergedRows := mergeChannelAggregateRows(rows)

	ids := make([]int, 0, len(mergedRows))
	for channelID := range mergedRows {
		ids = append(ids, channelID)
	}
	channelMeta, err := currentChannelMetadata(ids)
	if err != nil {
		return dto.ChannelAnalyticsChannelsResponse{}, err
	}
	items := make([]dto.ChannelAnalyticsChannelItem, 0, len(mergedRows))
	for channelID, row := range mergedRows {
		eligible := eligibleByChannel[channelID]
		item := channelAnalyticsItemFromAggregate(row, eligible)
		if current, ok := channelMeta[channelID]; ok {
			item.ChannelName = current.Name
			item.Group = current.Group
			item.ChannelType = current.Type
			item.ChannelTypeName = constant.GetChannelTypeName(current.Type)
		} else {
			item.ChannelTypeName = constant.GetChannelTypeName(item.ChannelType)
		}
		items = append(items, item)
	}
	sortChannelAnalyticsItems(items, query.SortBy, query.SortOrder)
	total := len(items)
	items = paginateChannelItems(items, query.Page, query.PageSize)

	pageChannelIDs := make([]int, 0, len(items))
	for _, item := range items {
		pageChannelIDs = append(pageChannelIDs, item.ChannelId)
	}
	if len(pageChannelIDs) > 0 {
		statusFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeUpstreamCall))
		statusFilter.ChannelPresent = boolPointer(true)
		statusFilter.ChannelIds = pageChannelIDs
		statusRows, queryErr := model.AggregateChannelMetricStatusCodesByChannel(model.LOG_DB, statusFilter)
		if queryErr != nil {
			return dto.ChannelAnalyticsChannelsResponse{}, queryErr
		}
		statusByChannel := make(map[int][]dto.ChannelAnalyticsStatusCode)
		for _, row := range statusRows {
			statusByChannel[row.ChannelId] = append(statusByChannel[row.ChannelId], statusCodeFromAggregate(row))
		}
		failureFilter := channelAnalyticsFailureFilter(query)
		failureFilter.ChannelIds = pageChannelIDs
		failureRows, queryErr := model.GetLatestChannelFailureTimes(model.LOG_DB, failureFilter, false)
		if queryErr != nil {
			return dto.ChannelAnalyticsChannelsResponse{}, queryErr
		}
		failureByChannel := make(map[int]int64, len(failureRows))
		for _, row := range failureRows {
			failureByChannel[row.ChannelId] = row.CreatedAt
		}
		for index := range items {
			statuses := statusByChannel[items[index].ChannelId]
			sort.Slice(statuses, func(i, j int) bool { return statuses[i].Count > statuses[j].Count })
			if len(statuses) > 3 {
				statuses = statuses[:3]
			}
			items[index].TopStatusCodes = statuses
			items[index].LastFailureAt = failureByChannel[items[index].ChannelId]
		}
	}
	meta, err := channelAnalyticsMeta(query, attemptFilter, true)
	if err != nil {
		return dto.ChannelAnalyticsChannelsResponse{}, err
	}
	return dto.ChannelAnalyticsChannelsResponse{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Meta: meta}, nil
}

func GetChannelAnalyticsModels(channelID int, query dto.ChannelAnalyticsQuery) (dto.ChannelAnalyticsModelsResponse, error) {
	if channelID <= 0 {
		return dto.ChannelAnalyticsModelsResponse{}, invalidChannelAnalyticsQuery("渠道 ID 必须为正整数")
	}
	if err := validateChannelTableAnalyticsQuery(query); err != nil {
		return dto.ChannelAnalyticsModelsResponse{}, err
	}
	if len(query.ChannelIds) > 0 && (len(query.ChannelIds) != 1 || query.ChannelIds[0] != channelID) {
		return dto.ChannelAnalyticsModelsResponse{}, invalidChannelAnalyticsQuery("channel_ids 与路径中的渠道 ID 不一致")
	}
	query.ChannelIds = []int{channelID}
	upstream := query.ModelDimension == "upstream"
	attemptFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeChannelAttempt))
	attemptFilter.ChannelPresent = boolPointer(true)
	if upstream {
		attemptFilter.UpstreamModelPresent = boolPointer(true)
	} else {
		attemptFilter.RequestedModelPresent = boolPointer(true)
	}
	rows, err := model.AggregateChannelMetricsByModel(model.LOG_DB, attemptFilter, upstream)
	if err != nil {
		return dto.ChannelAnalyticsModelsResponse{}, err
	}
	eligibleRows, err := model.AggregateChannelMetricsByModel(model.LOG_DB, withoutClientCancelled(attemptFilter), upstream)
	if err != nil {
		return dto.ChannelAnalyticsModelsResponse{}, err
	}
	mergedRows := mergeModelAggregateRows(rows, upstream)
	eligibleByModel := mergeModelAggregateRows(eligibleRows, upstream)

	currentMeta, err := currentChannelMetadata([]int{channelID})
	if err != nil {
		return dto.ChannelAnalyticsModelsResponse{}, err
	}
	channelName := fmt.Sprintf("渠道 #%d", channelID)
	channelType := 0
	channelGroup := ""
	if current, ok := currentMeta[channelID]; ok {
		channelName, channelType, channelGroup = current.Name, current.Type, current.Group
	} else {
		for _, row := range mergedRows {
			if row.ChannelNameSnapshot != "" {
				channelName = row.ChannelNameSnapshot
			}
			channelType = row.ChannelType
			break
		}
	}
	items := make([]dto.ChannelAnalyticsModelItem, 0, len(mergedRows))
	for modelHash, row := range mergedRows {
		base := channelAnalyticsItemFromAggregate(row, eligibleByModel[modelHash])
		base.ChannelId = channelID
		base.ChannelName = channelName
		base.ChannelType = channelType
		base.ChannelTypeName = constant.GetChannelTypeName(channelType)
		base.Group = channelGroup
		item := dto.ChannelAnalyticsModelItem{ChannelAnalyticsChannelItem: base}
		item.ModelHash = modelHash
		if upstream {
			item.UpstreamModel = row.UpstreamModel
		} else {
			item.RequestedModel = row.RequestedModel
		}
		items = append(items, item)
	}
	sortChannelAnalyticsModelItems(items, query.SortBy, query.SortOrder)
	total := len(items)
	items = paginateModelItems(items, query.Page, query.PageSize)

	if len(items) > 0 {
		statusFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeUpstreamCall))
		statusFilter.ChannelIds = []int{channelID}
		statusFilter.ChannelPresent = boolPointer(true)
		if upstream {
			statusFilter.UpstreamModelPresent = boolPointer(true)
		} else {
			statusFilter.RequestedModelPresent = boolPointer(true)
		}
		statusRows, queryErr := model.AggregateChannelMetricStatusCodesByModel(model.LOG_DB, statusFilter, upstream)
		if queryErr != nil {
			return dto.ChannelAnalyticsModelsResponse{}, queryErr
		}
		statusByModel := make(map[string][]dto.ChannelAnalyticsStatusCode)
		for _, row := range statusRows {
			modelHash := row.RequestedModelHash
			if upstream {
				modelHash = row.UpstreamModelHash
			}
			statusByModel[modelHash] = append(statusByModel[modelHash], statusCodeFromAggregate(row))
		}
		failureRows, queryErr := model.GetLatestChannelFailureTimesByModel(model.LOG_DB, channelAnalyticsFailureFilter(query), upstream)
		if queryErr != nil {
			return dto.ChannelAnalyticsModelsResponse{}, queryErr
		}
		failureByModel := make(map[string]int64)
		for _, row := range failureRows {
			modelName, modelHash := row.RequestedModel, row.RequestedModelHash
			if upstream {
				modelName, modelHash = row.UpstreamModel, row.UpstreamModelHash
			}
			if modelHash == "" && modelName != "" {
				modelHash = channelmetrics.SHA256String(modelName)
			}
			failureByModel[modelHash] = row.CreatedAt
		}
		for index := range items {
			modelHash := items[index].ModelHash
			statuses := statusByModel[modelHash]
			sort.Slice(statuses, func(i, j int) bool { return statuses[i].Count > statuses[j].Count })
			if len(statuses) > 3 {
				statuses = statuses[:3]
			}
			items[index].TopStatusCodes = statuses
			items[index].LastFailureAt = failureByModel[modelHash]
		}
	}
	meta, err := channelAnalyticsMeta(query, attemptFilter, true)
	if err != nil {
		return dto.ChannelAnalyticsModelsResponse{}, err
	}
	return dto.ChannelAnalyticsModelsResponse{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Meta: meta}, nil
}

func GetChannelAnalyticsStatusCodes(query dto.ChannelAnalyticsQuery) (dto.ChannelAnalyticsStatusResponse, error) {
	if query.MetricScope == "" {
		query.MetricScope = string(channelmetrics.ScopeUpstreamCall)
	}
	if err := validateStatusChannelAnalyticsQuery(query); err != nil {
		return dto.ChannelAnalyticsStatusResponse{}, err
	}
	client := query.MetricScope == string(channelmetrics.ScopeFinalRequest)
	filter := channelAnalyticsMetricFilter(query, query.MetricScope)
	rows, err := model.AggregateChannelMetricStatusCodes(model.LOG_DB, filter, client)
	if err != nil {
		return dto.ChannelAnalyticsStatusResponse{}, err
	}
	items := make([]dto.ChannelAnalyticsStatusCode, 0, len(rows))
	for _, row := range rows {
		items = append(items, statusCodeFromAggregate(row))
	}
	sort.Slice(items, func(i, j int) bool {
		comparison := 0
		if query.SortBy == "status_code" {
			comparison = compareInt64(int64(items[i].StatusCode), int64(items[j].StatusCode))
		} else {
			comparison = compareInt64(items[i].Count, items[j].Count)
		}
		if comparison == 0 && items[i].StatusPresent != items[j].StatusPresent {
			if items[i].StatusPresent {
				comparison = 1
			} else {
				comparison = -1
			}
		}
		if comparison == 0 {
			comparison = compareInt64(int64(items[i].StatusCode), int64(items[j].StatusCode))
		}
		if query.SortOrder == "asc" {
			return comparison < 0
		}
		return comparison > 0
	})

	stageFilter := filter
	stageFilter.Outcomes = failureOutcomes(filter.Outcomes)
	stageRows, err := model.AggregateChannelMetricErrorStages(model.LOG_DB, stageFilter)
	if err != nil {
		return dto.ChannelAnalyticsStatusResponse{}, err
	}
	stages := make([]dto.ChannelAnalyticsErrorStage, 0, len(stageRows))
	for _, row := range stageRows {
		if row.EventCount == 0 {
			continue
		}
		stage := row.ErrorStage
		if stage == "" {
			stage = "unknown"
		}
		stages = append(stages, dto.ChannelAnalyticsErrorStage{ErrorStage: stage, Count: row.EventCount})
	}
	sort.Slice(stages, func(i, j int) bool { return stages[i].Count > stages[j].Count })
	meta, err := channelAnalyticsMeta(query, filter, !client)
	if err != nil {
		return dto.ChannelAnalyticsStatusResponse{}, err
	}
	return dto.ChannelAnalyticsStatusResponse{Items: items, ErrorStages: stages, Meta: meta}, nil
}

func GetChannelAnalyticsFailures(query dto.ChannelAnalyticsQuery) (dto.ChannelAnalyticsFailuresResponse, error) {
	if err := validateFailureChannelAnalyticsQuery(query); err != nil {
		return dto.ChannelAnalyticsFailuresResponse{}, err
	}
	filter := channelAnalyticsFailureFilter(query)
	filter.Limit = query.PageSize
	filter.Offset = (query.Page - 1) * query.PageSize
	rows, total, err := model.QueryChannelFailureEvents(model.LOG_DB, filter)
	if err != nil {
		return dto.ChannelAnalyticsFailuresResponse{}, err
	}
	items := make([]dto.ChannelAnalyticsFailureItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.ChannelAnalyticsFailureItem{
			EventId:                 row.EventId,
			CreatedAt:               row.CreatedAt,
			RequestId:               row.RequestId,
			AttemptSeq:              row.AttemptSeq,
			RetryPlanned:            row.RetryPlanned,
			IsLastStartedAttempt:    row.IsLastStartedAttempt,
			ChannelId:               row.ChannelId,
			ChannelName:             row.ChannelNameSnapshot,
			ChannelType:             row.ChannelType,
			RequestedModel:          row.RequestedModel,
			RequestedModelHash:      row.RequestedModelHash,
			UpstreamModel:           row.UpstreamModel,
			UpstreamModelHash:       row.UpstreamModelHash,
			Group:                   row.Group,
			TrafficSource:           row.TrafficSource,
			DataOrigin:              row.DataOrigin,
			Outcome:                 row.Outcome,
			FailureOwner:            row.FailureOwner,
			QualityEligible:         row.QualityEligible,
			PartialResponse:         row.PartialResponse,
			ErrorStage:              row.ErrorStage,
			StreamEndReason:         row.StreamEndReason,
			UpstreamStatusPresent:   row.UpstreamStatusPresent,
			UpstreamStatusCode:      row.UpstreamStatusCode,
			NormalizedStatusPresent: row.NormalizedStatusPresent,
			NormalizedStatusCode:    row.NormalizedStatusCode,
			ClientStatusPresent:     row.ClientStatusPresent,
			ClientStatusCode:        row.ClientStatusCode,
			LatencyMs:               row.LatencyMs,
			TtftPresent:             row.TtftPresent,
			TtftMs:                  row.TtftMs,
			RetryReason:             row.RetryReason,
			ErrorSummary:            row.MaskedErrorSummary,
		})
	}
	metaFilter := channelAnalyticsMetricFilter(query, string(channelmetrics.ScopeChannelAttempt))
	meta, err := channelAnalyticsMeta(query, metaFilter, true)
	if err != nil {
		return dto.ChannelAnalyticsFailuresResponse{}, err
	}
	return dto.ChannelAnalyticsFailuresResponse{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Meta: meta}, nil
}

func GetChannelAnalyticsFilters() (dto.ChannelAnalyticsFiltersResponse, error) {
	setting := channelMetricsEffectiveSetting()
	var channels []model.Channel
	if err := model.DB.Omit("key").Order("name ASC").Find(&channels).Error; err != nil {
		return dto.ChannelAnalyticsFiltersResponse{}, err
	}
	channelOptions := make([]dto.ChannelAnalyticsFilterChannel, 0, len(channels))
	typeSet := make(map[int]struct{})
	for _, channel := range channels {
		channelOptions = append(channelOptions, dto.ChannelAnalyticsFilterChannel{
			ChannelId: channel.Id, ChannelName: channel.Name, ChannelType: channel.Type,
		})
		typeSet[channel.Type] = struct{}{}
	}
	types := make([]dto.ChannelAnalyticsFilterType, 0, len(typeSet))
	for channelType := range typeSet {
		types = append(types, dto.ChannelAnalyticsFilterType{Value: channelType, Label: constant.GetChannelTypeName(channelType)})
	}
	sort.Slice(types, func(i, j int) bool { return types[i].Value < types[j].Value })
	groupRows, err := model.GetChannelMetricGroupOptions(model.LOG_DB, setting.BucketLevel, 1000)
	if err != nil {
		return dto.ChannelAnalyticsFiltersResponse{}, err
	}
	groupNames, err := model.GetGroupDisplayNameMap()
	if err != nil {
		return dto.ChannelAnalyticsFiltersResponse{}, err
	}
	groups := make([]dto.ChannelAnalyticsFilterGroup, 0, len(groupRows))
	seenGroups := make(map[string]struct{}, len(groupRows))
	for _, row := range groupRows {
		code := row.Group
		if entity, resolveErr := model.GetGroupByCodeOrAlias(row.Group); resolveErr == nil {
			code = entity.Code
		}
		if _, exists := seenGroups[code]; exists {
			continue
		}
		seenGroups[code] = struct{}{}
		name := groupNames[code]
		if name == "" {
			name = groupNames[row.Group]
		}
		if name == "" {
			name = code
		}
		groups = append(groups, dto.ChannelAnalyticsFilterGroup{Code: code, Name: name})
	}
	requestedRows, err := model.GetChannelMetricModelOptions(model.LOG_DB, setting.BucketLevel, false, 1000)
	if err != nil {
		return dto.ChannelAnalyticsFiltersResponse{}, err
	}
	upstreamRows, err := model.GetChannelMetricModelOptions(model.LOG_DB, setting.BucketLevel, true, 1000)
	if err != nil {
		return dto.ChannelAnalyticsFiltersResponse{}, err
	}
	requestedModels := make([]string, 0, len(requestedRows))
	requestedModelOptions := make([]dto.ChannelAnalyticsFilterModel, 0, len(requestedRows))
	for _, row := range requestedRows {
		// 旧字符串契约无法表达被截断模型的完整哈希，只保留未截断项。
		if channelmetrics.SHA256String(row.Model) == row.ModelHash {
			requestedModels = append(requestedModels, row.Model)
		}
		requestedModelOptions = append(requestedModelOptions, dto.ChannelAnalyticsFilterModel{
			Value: row.ModelHash, Label: row.Model, Model: row.Model, ModelHash: row.ModelHash,
		})
	}
	upstreamModels := make([]string, 0, len(upstreamRows))
	upstreamModelOptions := make([]dto.ChannelAnalyticsFilterModel, 0, len(upstreamRows))
	for _, row := range upstreamRows {
		if channelmetrics.SHA256String(row.Model) == row.ModelHash {
			upstreamModels = append(upstreamModels, row.Model)
		}
		upstreamModelOptions = append(upstreamModelOptions, dto.ChannelAnalyticsFilterModel{
			Value: row.ModelHash, Label: row.Model, Model: row.Model, ModelHash: row.ModelHash,
		})
	}
	now := time.Now().Unix()
	query := dto.ChannelAnalyticsQuery{
		StartTimestamp: now - int64(setting.RetentionDays)*24*60*60,
		EndTimestamp:   now,
		BucketLevel:    setting.BucketLevel,
		BucketSeconds:  int64(setting.BucketSeconds),
		TrafficSources: []string{string(channelmetrics.TrafficSourceRelay)},
		DataOrigins:    []string{string(channelmetrics.DataOriginLive)},
	}
	meta, err := channelAnalyticsMeta(query, channelAnalyticsMetricFilter(query, ""), true)
	if err != nil {
		return dto.ChannelAnalyticsFiltersResponse{}, err
	}
	return dto.ChannelAnalyticsFiltersResponse{
		Channels:              channelOptions,
		ChannelTypes:          types,
		Groups:                groups,
		RequestedModels:       requestedModels,
		UpstreamModels:        upstreamModels,
		RequestedModelOptions: requestedModelOptions,
		UpstreamModelOptions:  upstreamModelOptions,
		Outcomes:              append([]string(nil), allChannelAnalyticsOutcomes...),
		TrafficSources: []string{
			string(channelmetrics.TrafficSourceRelay), string(channelmetrics.TrafficSourceProbe),
			string(channelmetrics.TrafficSourceTask), string(channelmetrics.TrafficSourcePlayground),
		},
		DataOrigins: []string{string(channelmetrics.DataOriginLive), string(channelmetrics.DataOriginLegacy)},
		Meta:        meta,
	}, nil
}

func expandChannelAnalyticsGroupIdentifiers(groups []string) ([]string, error) {
	if len(groups) == 0 {
		return groups, nil
	}
	result := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		identifiers := []string{group}
		if !strings.EqualFold(strings.TrimSpace(group), "auto") {
			resolved, err := model.ResolveGroupLogIdentifiers(group)
			if err == nil {
				identifiers = resolved
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
		for _, identifier := range identifiers {
			if _, exists := seen[identifier]; exists {
				continue
			}
			seen[identifier] = struct{}{}
			result = append(result, identifier)
			if len(result) > channelAnalyticsMaxListItems*10 {
				return nil, invalidChannelAnalyticsQuery("groups 展开历史别名后最多允许 %d 项", channelAnalyticsMaxListItems*10)
			}
		}
	}
	return result, nil
}

// GetChannelAnalyticsFilterModels 返回不会因模型数量超过 1000 而静默截断的筛选项。
func GetChannelAnalyticsFilterModels(query dto.ChannelAnalyticsFilterModelsQuery) (dto.ChannelAnalyticsFilterModelsResponse, error) {
	if query.ModelDimension != "requested" && query.ModelDimension != "upstream" {
		return dto.ChannelAnalyticsFilterModelsResponse{}, ErrInvalidChannelAnalyticsQuery
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		return dto.ChannelAnalyticsFilterModelsResponse{}, ErrInvalidChannelAnalyticsQuery
	}
	offset := (query.Page - 1) * query.PageSize
	rows, total, err := model.SearchChannelMetricModelOptions(
		model.LOG_DB,
		channelMetricsEffectiveSetting().BucketLevel,
		query.ModelDimension == "upstream",
		query.Query,
		offset,
		query.PageSize,
	)
	if err != nil {
		return dto.ChannelAnalyticsFilterModelsResponse{}, err
	}
	items := make([]dto.ChannelAnalyticsFilterModel, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.ChannelAnalyticsFilterModel{
			Value: row.ModelHash, Label: row.Model, Model: row.Model, ModelHash: row.ModelHash,
		})
	}
	return dto.ChannelAnalyticsFilterModelsResponse{
		ModelDimension: query.ModelDimension,
		Query:          query.Query,
		Items:          items,
		Total:          total,
		Page:           query.Page,
		PageSize:       query.PageSize,
	}, nil
}

func channelAnalyticsMetricFilter(query dto.ChannelAnalyticsQuery, scope string) model.ChannelMetricBucketFilter {
	modelSnapshotBytes := channelMetricsEffectiveSetting().ModelSnapshotMaxBytes
	requestedModelSnapshots := make([]string, 0, len(query.RequestedModels))
	for _, requestedModel := range query.RequestedModels {
		requestedModelSnapshots = append(requestedModelSnapshots, channelmetrics.TruncateUTF8(requestedModel, modelSnapshotBytes))
	}
	upstreamModelSnapshots := make([]string, 0, len(query.UpstreamModels))
	for _, upstreamModel := range query.UpstreamModels {
		upstreamModelSnapshots = append(upstreamModelSnapshots, channelmetrics.TruncateUTF8(upstreamModel, modelSnapshotBytes))
	}
	filter := model.ChannelMetricBucketFilter{
		StartTs:              query.StartTimestamp,
		EndTs:                query.EndTimestamp,
		BucketLevel:          query.BucketLevel,
		TrafficSources:       append([]string(nil), query.TrafficSources...),
		DataOrigins:          append([]string(nil), query.DataOrigins...),
		ChannelIds:           append([]int(nil), query.ChannelIds...),
		ChannelTypes:         append([]int(nil), query.ChannelTypes...),
		Groups:               append([]string(nil), query.Groups...),
		RequestedModels:      requestedModelSnapshots,
		RequestedModelHashes: append([]string(nil), query.RequestedModelHash...),
		UpstreamModels:       upstreamModelSnapshots,
		UpstreamModelHashes:  append([]string(nil), query.UpstreamModelHash...),
		Outcomes:             append([]string(nil), query.Outcomes...),
		ErrorStages:          append([]string(nil), query.ErrorStages...),
		FailureOwners:        append([]string(nil), query.FailureOwners...),
		ClientStatusCodes:    append([]int(nil), query.ClientStatusCodes...),
		UpstreamStatusCodes:  append([]int(nil), query.UpstreamStatusCodes...),
		Stream:               query.Stream,
	}
	for _, group := range query.Groups {
		filter.GroupHashes = append(filter.GroupHashes, channelmetrics.SHA256String(group))
	}
	if scope != "" {
		filter.MetricScopes = []string{scope}
	}
	return filter
}

func channelAnalyticsFailureFilter(query dto.ChannelAnalyticsQuery) model.ChannelFailureEventFilter {
	requestedModelSnapshots := make([]string, 0, len(query.RequestedModels))
	for _, requestedModel := range query.RequestedModels {
		requestedModelSnapshots = append(requestedModelSnapshots, channelmetrics.TruncateUTF8(requestedModel, channelAnalyticsFailureModelSnapshotBytes))
	}
	upstreamModelSnapshots := make([]string, 0, len(query.UpstreamModels))
	for _, upstreamModel := range query.UpstreamModels {
		upstreamModelSnapshots = append(upstreamModelSnapshots, channelmetrics.TruncateUTF8(upstreamModel, channelAnalyticsFailureModelSnapshotBytes))
	}
	groupSnapshots := make([]string, 0, len(query.Groups))
	for _, group := range query.Groups {
		groupSnapshots = append(groupSnapshots, channelmetrics.TruncateUTF8(group, channelAnalyticsFailureGroupSnapshotBytes))
	}
	return model.ChannelFailureEventFilter{
		StartTs:              query.StartTimestamp,
		EndTs:                query.EndTimestamp,
		ChannelIds:           append([]int(nil), query.ChannelIds...),
		ChannelTypes:         append([]int(nil), query.ChannelTypes...),
		TrafficSources:       append([]string(nil), query.TrafficSources...),
		DataOrigins:          append([]string(nil), query.DataOrigins...),
		Groups:               groupSnapshots,
		RequestedModels:      requestedModelSnapshots,
		RequestedModelHashes: append([]string(nil), query.RequestedModelHash...),
		UpstreamModels:       upstreamModelSnapshots,
		UpstreamModelHashes:  append([]string(nil), query.UpstreamModelHash...),
		Outcomes:             append([]string(nil), query.Outcomes...),
		FailureOwners:        append([]string(nil), query.FailureOwners...),
		ErrorStages:          append([]string(nil), query.ErrorStages...),
		UpstreamStatusCodes:  append([]int(nil), query.UpstreamStatusCodes...),
		ClientStatusCodes:    append([]int(nil), query.ClientStatusCodes...),
	}
}

func channelAnalyticsMeta(query dto.ChannelAnalyticsQuery, filter model.ChannelMetricBucketFilter, includeTransportCoverage bool) (dto.ChannelAnalyticsMeta, error) {
	bounds, err := model.GetChannelMetricBounds(model.LOG_DB, filter)
	if err != nil {
		return dto.ChannelAnalyticsMeta{}, err
	}
	reliableFilter := model.ChannelMetricBucketFilter{
		BucketLevel:    query.BucketLevel,
		TrafficSources: []string{string(channelmetrics.TrafficSourceRelay)},
		DataOrigins:    []string{string(channelmetrics.DataOriginLive)},
	}
	reliableBounds, err := model.GetChannelMetricBounds(model.LOG_DB, reliableFilter)
	if err != nil {
		return dto.ChannelAnalyticsMeta{}, err
	}
	quality, err := model.GetChannelMetricDataQuality(model.LOG_DB, query.StartTimestamp, query.EndTimestamp)
	if err != nil {
		return dto.ChannelAnalyticsMeta{}, err
	}
	runtimeQuality := GetChannelMetricsQuality()

	uncovered := []int{}
	if includeTransportCoverage {
		uncovered, err = activeUncoveredChannelTypes(query)
		if err != nil {
			return dto.ChannelAnalyticsMeta{}, err
		}
	}
	dataEnd := int64(0)
	if bounds.DataEndTs > 0 {
		dataEnd = bounds.DataEndTs + query.BucketSeconds
		if dataEnd > query.EndTimestamp {
			dataEnd = query.EndTimestamp
		}
		if now := time.Now().Unix(); dataEnd > now {
			dataEnd = now
		}
	}
	setting := channelMetricsEffectiveSetting()
	failureCutoff := time.Now().AddDate(0, 0, -setting.FailureRetentionDays).Unix()
	meta := dto.ChannelAnalyticsMeta{
		GeneratedAt:                 time.Now().Unix(),
		ReliableFromTs:              reliableBounds.DataStartTs,
		DataStartTs:                 bounds.DataStartTs,
		DataEndTs:                   dataEnd,
		LastFlushedAt:               quality.LastFlushedAt,
		RuntimePendingBatchCount:    runtimeQuality.PendingBatchCount,
		RuntimeFlushFailureCount:    runtimeQuality.FlushFailureCount,
		RuntimeLastFlushErrorAt:     runtimeQuality.LastFlushErrorAtUnix,
		BucketLevel:                 query.BucketLevel,
		BucketSeconds:               query.BucketSeconds,
		RetentionDays:               setting.RetentionDays,
		DetailAvailable:             query.StartTimestamp >= failureCutoff,
		UncoveredChannelTypes:       uncovered,
		InvalidSampleCount:          quality.InvalidSampleCount,
		DimensionOverflowCount:      quality.DimensionOverflowCount,
		DroppedMetricEventCount:     quality.DroppedMetricEventCount,
		DroppedFailureEventCount:    quality.DroppedFailureEventCount,
		DimensionHashCollisionCount: quality.DimensionHashCollisionCount,
	}
	backfillJob, backfillErr := model.GetChannelMetricBackfillJob(model.LOG_DB, channelMetricLegacyBackfillJobId)
	if backfillErr == nil {
		meta.Backfill = &dto.ChannelAnalyticsBackfillMeta{
			Status: backfillJob.Status, BackfillStartTs: backfillJob.BackfillStartTs, LiveCutoverTs: backfillJob.LiveCutoverTs,
			TotalRows: backfillJob.TotalRows, ScannedRows: backfillJob.ScannedRows, ConvertedRows: backfillJob.ConvertedRows,
			SkippedRows: backfillJob.SkippedRows, MetricBucketCount: backfillJob.MetricBucketCount,
			FailureEventCount: backfillJob.FailureEventCount, LastError: backfillJob.LastError,
			UpdatedAt: backfillJob.UpdatedAt, CompletedAt: backfillJob.CompletedAt,
		}
	} else if !errors.Is(backfillErr, gorm.ErrRecordNotFound) {
		return dto.ChannelAnalyticsMeta{}, backfillErr
	}
	backfillAffectsQuery := false
	for _, origin := range query.DataOrigins {
		if origin == string(channelmetrics.DataOriginLegacy) {
			backfillAffectsQuery = true
			break
		}
	}
	meta.Partial = len(uncovered) > 0 ||
		(backfillAffectsQuery && (meta.Backfill == nil || meta.Backfill.Status != model.ChannelMetricBackfillStatusCompleted)) ||
		meta.RuntimePendingBatchCount > 0 ||
		meta.RuntimeLastFlushErrorAt > runtimeQuality.LastFlushedAtUnix ||
		meta.InvalidSampleCount > 0 ||
		meta.DimensionOverflowCount > 0 ||
		meta.DroppedMetricEventCount > 0 ||
		meta.DroppedFailureEventCount > 0 ||
		meta.DimensionHashCollisionCount > 0
	return meta, nil
}

func activeUncoveredChannelTypes(query dto.ChannelAnalyticsQuery) ([]int, error) {
	filterSet := make(map[int]struct{})
	for _, value := range query.ChannelTypes {
		filterSet[value] = struct{}{}
	}
	sourceSet := make(map[string]struct{}, len(query.TrafficSources))
	for _, source := range query.TrafficSources {
		sourceSet[source] = struct{}{}
	}
	candidateSet := make(map[int]struct{})
	if _, probe := sourceSet[string(channelmetrics.TrafficSourceProbe)]; probe {
		for _, channelType := range append(append([]int{}, channelAnalyticsUncoveredRelayTypes...), channelAnalyticsUncoveredTaskTypes...) {
			candidateSet[channelType] = struct{}{}
		}
	} else {
		if _, relay := sourceSet[string(channelmetrics.TrafficSourceRelay)]; relay {
			for _, channelType := range channelAnalyticsUncoveredRelayTypes {
				candidateSet[channelType] = struct{}{}
			}
		}
		if _, playground := sourceSet[string(channelmetrics.TrafficSourcePlayground)]; playground {
			for _, channelType := range channelAnalyticsUncoveredRelayTypes {
				candidateSet[channelType] = struct{}{}
			}
		}
		if _, task := sourceSet[string(channelmetrics.TrafficSourceTask)]; task {
			for _, channelType := range channelAnalyticsUncoveredTaskTypes {
				candidateSet[channelType] = struct{}{}
			}
		}
	}
	candidates := make([]int, 0, len(candidateSet))
	for channelType := range candidateSet {
		if len(filterSet) > 0 {
			if _, ok := filterSet[channelType]; !ok {
				continue
			}
		}
		candidates = append(candidates, channelType)
	}
	if len(candidates) == 0 {
		return []int{}, nil
	}
	var active []int
	dbQuery := model.DB.Model(&model.Channel{}).Distinct("type").Where("type IN ?", candidates)
	if len(query.ChannelIds) > 0 {
		dbQuery = dbQuery.Where("id IN ?", query.ChannelIds)
	}
	if err := dbQuery.Pluck("type", &active).Error; err != nil {
		return nil, err
	}
	sort.Ints(active)
	return active, nil
}

func currentChannelMetadata(ids []int) (map[int]model.Channel, error) {
	result := make(map[int]model.Channel)
	if len(ids) == 0 {
		return result, nil
	}
	var channels []model.Channel
	if err := model.DB.Omit("key").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		result[channel.Id] = channel
	}
	return result, nil
}

func channelAnalyticsItemFromAggregate(row model.ChannelMetricAggregateRow, eligible model.ChannelMetricAggregateRow) dto.ChannelAnalyticsChannelItem {
	return dto.ChannelAnalyticsChannelItem{
		ChannelId:                 row.ChannelId,
		ChannelName:               row.ChannelNameSnapshot,
		ChannelType:               row.ChannelType,
		ChannelAttemptCount:       row.EventCount,
		FailureCount:              channelAnalyticsMaxInt64(0, eligible.EventCount-eligible.SuccessCount),
		RetryCount:                row.NonFirstAttemptCount,
		ChannelQualitySuccessRate: ratioPointer(row.QualitySuccessCount, row.QualityEligibleCount),
		AttemptSuccessRate:        ratioPointer(eligible.SuccessCount, eligible.EventCount),
		UsageSampleCount:          row.UsageSampleCount,
		InputTokensTotal:          row.InputTokensTotal,
		UncachedInputTokens:       row.UncachedInputTokens,
		OutputTokens:              row.OutputTokens,
		CacheReadTokens:           row.CacheReadTokens,
		CacheWriteTokens:          row.CacheWriteTokens,
		CacheRequestHitRate:       ratioPointer(row.CacheHitRequestCount, row.UsageSampleCount),
		CacheTokenHitRate:         ratioPointer(row.CacheReadTokens, row.InputTokensTotal),
		AvgLatencyMs:              averagePointer(row.LatencySumMs, row.LatencyCount),
		P95LatencyMs:              percentileLatency(row.LatencyHistogram(), row.LatencyCount, 0.95),
		AvgTtftMs:                 averagePointer(row.TtftSumMs, row.TtftCount),
		P95TtftMs:                 percentileLatency(row.TtftHistogram(), row.TtftCount, 0.95),
		ChargedQuota:              row.ChargedQuota,
		ChargedMicroUsd:           row.ChargedMicroUsd,
		TopStatusCodes:            []dto.ChannelAnalyticsStatusCode{},
	}
}

func statusCodeFromAggregate(row model.ChannelMetricAggregateRow) dto.ChannelAnalyticsStatusCode {
	label := "未知 / 不适用"
	if row.StatusPresent {
		if row.StatusCode == 0 {
			label = "无 HTTP 响应"
		} else {
			label = strconv.Itoa(row.StatusCode)
		}
	}
	return dto.ChannelAnalyticsStatusCode{
		StatusPresent: row.StatusPresent,
		StatusCode:    row.StatusCode,
		Label:         label,
		Count:         row.EventCount,
	}
}

func mergeChannelAggregateRows(rows []model.ChannelMetricAggregateRow) map[int]model.ChannelMetricAggregateRow {
	result := make(map[int]model.ChannelMetricAggregateRow)
	for _, row := range rows {
		current := result[row.ChannelId]
		if current.ChannelNameSnapshot == "" {
			current.ChannelNameSnapshot = row.ChannelNameSnapshot
		}
		if current.ChannelType == 0 {
			current.ChannelType = row.ChannelType
		}
		current.ChannelId = row.ChannelId
		mergeChannelMetricAggregate(&current, row)
		result[row.ChannelId] = current
	}
	return result
}

func mergeModelAggregateRows(rows []model.ChannelMetricAggregateRow, upstream bool) map[string]model.ChannelMetricAggregateRow {
	result := make(map[string]model.ChannelMetricAggregateRow)
	for _, row := range rows {
		key := row.RequestedModelHash
		if upstream {
			key = row.UpstreamModelHash
		}
		current := result[key]
		if current.ChannelNameSnapshot == "" {
			current.ChannelNameSnapshot = row.ChannelNameSnapshot
		}
		if current.ChannelType == 0 {
			current.ChannelType = row.ChannelType
		}
		current.RequestedModelHash = row.RequestedModelHash
		current.UpstreamModelHash = row.UpstreamModelHash
		if current.RequestedModel == "" {
			current.RequestedModel = row.RequestedModel
		}
		if current.UpstreamModel == "" {
			current.UpstreamModel = row.UpstreamModel
		}
		mergeChannelMetricAggregate(&current, row)
		result[key] = current
	}
	return result
}

func mergeChannelMetricAggregate(target *model.ChannelMetricAggregateRow, source model.ChannelMetricAggregateRow) {
	if source.LastFailureBucketTs > target.LastFailureBucketTs {
		target.LastFailureBucketTs = source.LastFailureBucketTs
	}
	target.EventCount += source.EventCount
	target.SuccessCount += source.SuccessCount
	target.NonFirstAttemptCount += source.NonFirstAttemptCount
	target.RetryPlannedCount += source.RetryPlannedCount
	target.QualityEligibleCount += source.QualityEligibleCount
	target.QualitySuccessCount += source.QualitySuccessCount
	target.PartialResponseCount += source.PartialResponseCount
	target.UsageSampleCount += source.UsageSampleCount
	target.CacheHitRequestCount += source.CacheHitRequestCount
	target.InputTokensTotal += source.InputTokensTotal
	target.UncachedInputTokens += source.UncachedInputTokens
	target.OutputTokens += source.OutputTokens
	target.CacheReadTokens += source.CacheReadTokens
	target.CacheWriteTokens += source.CacheWriteTokens
	target.ChargedQuota += source.ChargedQuota
	target.ChargedMicroUsd += source.ChargedMicroUsd
	target.LatencySumMs += source.LatencySumMs
	target.LatencyCount += source.LatencyCount
	target.TtftSumMs += source.TtftSumMs
	target.TtftCount += source.TtftCount
	target.UpstreamStatusSampleCount += source.UpstreamStatusSampleCount
	target.Upstream429Count += source.Upstream429Count
	target.Upstream4xxCount += source.Upstream4xxCount
	target.Upstream5xxCount += source.Upstream5xxCount
	target.HTTPErrorCount += source.HTTPErrorCount
	target.TransportErrorCount += source.TransportErrorCount
	target.ProtocolErrorCount += source.ProtocolErrorCount
	target.StreamErrorCount += source.StreamErrorCount
	target.LocalErrorCount += source.LocalErrorCount
	target.DispatchErrorCount += source.DispatchErrorCount
	target.ClientCancelledCount += source.ClientCancelledCount
	target.LiveEventCount += source.LiveEventCount
	target.LegacyEventCount += source.LegacyEventCount
	target.SuccessUsageSampleCount += source.SuccessUsageSampleCount
	target.LatencyBucket100Ms += source.LatencyBucket100Ms
	target.LatencyBucket250Ms += source.LatencyBucket250Ms
	target.LatencyBucket500Ms += source.LatencyBucket500Ms
	target.LatencyBucket1S += source.LatencyBucket1S
	target.LatencyBucket2S += source.LatencyBucket2S
	target.LatencyBucket4S += source.LatencyBucket4S
	target.LatencyBucket8S += source.LatencyBucket8S
	target.LatencyBucket15S += source.LatencyBucket15S
	target.LatencyBucket30S += source.LatencyBucket30S
	target.LatencyBucket60S += source.LatencyBucket60S
	target.LatencyBucket120S += source.LatencyBucket120S
	target.LatencyBucket300S += source.LatencyBucket300S
	target.LatencyBucketInf += source.LatencyBucketInf
	target.TtftBucket100Ms += source.TtftBucket100Ms
	target.TtftBucket250Ms += source.TtftBucket250Ms
	target.TtftBucket500Ms += source.TtftBucket500Ms
	target.TtftBucket1S += source.TtftBucket1S
	target.TtftBucket2S += source.TtftBucket2S
	target.TtftBucket4S += source.TtftBucket4S
	target.TtftBucket8S += source.TtftBucket8S
	target.TtftBucket15S += source.TtftBucket15S
	target.TtftBucket30S += source.TtftBucket30S
	target.TtftBucket60S += source.TtftBucket60S
	target.TtftBucket120S += source.TtftBucket120S
	target.TtftBucket300S += source.TtftBucket300S
	target.TtftBucketInf += source.TtftBucketInf
}

func validateCompositeChannelAnalyticsQuery(query dto.ChannelAnalyticsQuery) error {
	if query.MetricScope != "" {
		return invalidChannelAnalyticsQuery("summary 不支持 metric_scope")
	}
	if len(query.ClientStatusCodes) > 0 || len(query.UpstreamStatusCodes) > 0 {
		return invalidChannelAnalyticsQuery("summary 不支持状态码过滤，请使用 status-codes 接口")
	}
	if len(query.ErrorStages) > 0 || len(query.FailureOwners) > 0 {
		return invalidChannelAnalyticsQuery("summary 无法跨 scope 应用错误阶段或归属过滤")
	}
	if query.SortBy != "" {
		return invalidChannelAnalyticsQuery("summary 不支持 sort_by")
	}
	return nil
}

func validateTrendChannelAnalyticsQuery(query dto.ChannelAnalyticsQuery) error {
	if query.SortBy != "" {
		return invalidChannelAnalyticsQuery("trend 不支持 sort_by")
	}
	if query.MetricScope == "" {
		return validateCompositeChannelAnalyticsQuery(query)
	}
	return validateScopeSpecificFilters(query, query.MetricScope)
}

func validateChannelTableAnalyticsQuery(query dto.ChannelAnalyticsQuery) error {
	if query.MetricScope != "" {
		return invalidChannelAnalyticsQuery("渠道和模型接口的 metric_scope 固定为 channel_attempt")
	}
	if len(query.ClientStatusCodes) > 0 || len(query.UpstreamStatusCodes) > 0 {
		return invalidChannelAnalyticsQuery("渠道和模型接口不支持状态码过滤")
	}
	allowedSort := map[string]bool{
		"": true, "request_count": true, "channel_attempt_count": true, "channel_name": true,
		"quality_success_rate": true, "failure_count": true, "p95_latency_ms": true,
		"input_tokens_total": true, "output_tokens": true,
		"cache_read_tokens": true, "charged_quota": true, "charged_micro_usd": true,
	}
	if !allowedSort[query.SortBy] {
		return invalidChannelAnalyticsQuery("不支持的 sort_by：%s", query.SortBy)
	}
	return nil
}

func validateStatusChannelAnalyticsQuery(query dto.ChannelAnalyticsQuery) error {
	if query.MetricScope != string(channelmetrics.ScopeFinalRequest) && query.MetricScope != string(channelmetrics.ScopeUpstreamCall) {
		return invalidChannelAnalyticsQuery("status-codes 的 metric_scope 仅支持 final_request 或 upstream_call")
	}
	if query.SortBy != "" && query.SortBy != "count" && query.SortBy != "status_code" {
		return invalidChannelAnalyticsQuery("status-codes 的 sort_by 仅支持 count 或 status_code")
	}
	return validateScopeSpecificFilters(query, query.MetricScope)
}

func validateFailureChannelAnalyticsQuery(query dto.ChannelAnalyticsQuery) error {
	if query.MetricScope != "" {
		return invalidChannelAnalyticsQuery("failures 不支持 metric_scope")
	}
	if query.Stream != nil {
		return invalidChannelAnalyticsQuery("失败明细当前不支持 stream 过滤")
	}
	if query.SortBy != "" && query.SortBy != "created_at" {
		return invalidChannelAnalyticsQuery("failures 的 sort_by 仅支持 created_at")
	}
	if query.SortOrder != "desc" {
		return invalidChannelAnalyticsQuery("failures 当前仅支持按 created_at 倒序")
	}
	return nil
}

func validateScopeSpecificFilters(query dto.ChannelAnalyticsQuery, scope string) error {
	switch scope {
	case string(channelmetrics.ScopeFinalRequest):
		if len(query.UpstreamStatusCodes) > 0 {
			return invalidChannelAnalyticsQuery("final_request 不支持上游状态码过滤")
		}
		if len(query.ErrorStages) > 0 || len(query.FailureOwners) > 0 {
			return invalidChannelAnalyticsQuery("final_request 不支持错误阶段或责任归属过滤")
		}
	case string(channelmetrics.ScopeChannelAttempt):
		if len(query.ClientStatusCodes) > 0 || len(query.UpstreamStatusCodes) > 0 {
			return invalidChannelAnalyticsQuery("channel_attempt 不支持状态码过滤")
		}
	case string(channelmetrics.ScopeUpstreamCall):
		if len(query.ClientStatusCodes) > 0 {
			return invalidChannelAnalyticsQuery("upstream_call 不支持客户端状态码过滤")
		}
	default:
		return invalidChannelAnalyticsQuery("未知 metric_scope：%s", scope)
	}
	return nil
}

func validateChannelAnalyticsEnums(query dto.ChannelAnalyticsQuery) error {
	for _, outcome := range query.Outcomes {
		if !channelmetrics.Outcome(outcome).Valid() {
			return invalidChannelAnalyticsQuery("未知 outcome：%s", outcome)
		}
	}
	for _, owner := range query.FailureOwners {
		if owner == "" || !channelmetrics.FailureOwner(owner).Valid() {
			return invalidChannelAnalyticsQuery("未知 failure_owner：%s", owner)
		}
	}
	for _, source := range query.TrafficSources {
		if !channelmetrics.TrafficSource(source).Valid() {
			return invalidChannelAnalyticsQuery("未知 traffic_source：%s", source)
		}
	}
	for _, origin := range query.DataOrigins {
		if !channelmetrics.DataOrigin(origin).Valid() {
			return invalidChannelAnalyticsQuery("未知 data_origin：%s", origin)
		}
	}
	for _, stage := range query.ErrorStages {
		if len(stage) > 32 {
			return invalidChannelAnalyticsQuery("error_stage 不能超过 32 字节")
		}
	}
	return nil
}

func withoutClientCancelled(filter model.ChannelMetricBucketFilter) model.ChannelMetricBucketFilter {
	filter.Outcomes = failureAndSuccessOutcomes(filter.Outcomes)
	return filter
}

func failureAndSuccessOutcomes(selected []string) []string {
	if len(selected) == 0 {
		selected = allChannelAnalyticsOutcomes
	}
	result := make([]string, 0, len(selected))
	for _, outcome := range selected {
		if outcome != string(channelmetrics.OutcomeClientCancelled) {
			result = append(result, outcome)
		}
	}
	if len(result) == 0 {
		return []string{"__none__"}
	}
	return result
}

func failureOutcomes(selected []string) []string {
	if len(selected) == 0 {
		selected = allChannelAnalyticsOutcomes
	}
	result := make([]string, 0, len(selected))
	for _, outcome := range selected {
		if outcome != string(channelmetrics.OutcomeSuccess) && outcome != string(channelmetrics.OutcomeClientCancelled) {
			result = append(result, outcome)
		}
	}
	if len(result) == 0 {
		return []string{"__none__"}
	}
	return result
}

func parseOptionalPositiveInt64(raw string, fallback int64, name string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, invalidChannelAnalyticsQuery("%s 必须为正整数时间戳", name)
	}
	return value, nil
}

func parseIntList(values url.Values, key string, allowZero bool) ([]int, error) {
	stringsList, err := parseStringList(values, key)
	if err != nil {
		return nil, err
	}
	result := make([]int, 0, len(stringsList))
	for _, raw := range stringsList {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 0 || (!allowZero && value == 0) {
			return nil, invalidChannelAnalyticsQuery("%s 包含无效整数：%s", key, raw)
		}
		result = append(result, value)
	}
	return result, nil
}

func parseStatusCodeList(values url.Values, key string, allowZero bool) ([]int, error) {
	rawValues, err := parseStringList(values, key)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]struct{})
	result := make([]int, 0, len(rawValues))
	appendStatus := func(status int) error {
		if status == 0 && allowZero {
			if _, ok := seen[status]; !ok {
				seen[status] = struct{}{}
				result = append(result, status)
			}
			if len(result) > channelAnalyticsMaxListItems {
				return invalidChannelAnalyticsQuery("%s 展开后最多允许 %d 项", key, channelAnalyticsMaxListItems)
			}
			return nil
		}
		if status < 100 || status > 999 {
			return invalidChannelAnalyticsQuery("%s 包含无效状态码：%d", key, status)
		}
		if _, ok := seen[status]; !ok {
			seen[status] = struct{}{}
			result = append(result, status)
		}
		if len(result) > channelAnalyticsMaxListItems {
			return invalidChannelAnalyticsQuery("%s 展开后最多允许 %d 项", key, channelAnalyticsMaxListItems)
		}
		return nil
	}
	for _, raw := range rawValues {
		normalized := strings.ToLower(strings.TrimSpace(raw))
		if len(normalized) == 3 && normalized[1:] == "xx" && normalized[0] >= '1' && normalized[0] <= '9' {
			base := int(normalized[0]-'0') * 100
			for status := base; status < base+100; status++ {
				if err := appendStatus(status); err != nil {
					return nil, err
				}
			}
			continue
		}
		status, parseErr := strconv.Atoi(normalized)
		if parseErr != nil || status < 0 || (!allowZero && status == 0) {
			return nil, invalidChannelAnalyticsQuery("%s 包含无效状态码：%s", key, raw)
		}
		if err := appendStatus(status); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func parseStringList(values url.Values, key string) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, raw := range values[key] {
		for _, item := range strings.Split(raw, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			result = append(result, item)
			if len(result) > channelAnalyticsMaxListItems {
				return nil, invalidChannelAnalyticsQuery("%s 最多允许 %d 项", key, channelAnalyticsMaxListItems)
			}
		}
	}
	return result, nil
}

func parseSHA256List(values url.Values, key string) ([]string, error) {
	items, err := parseStringList(values, key)
	if err != nil {
		return nil, err
	}
	for index, item := range items {
		item = strings.ToLower(item)
		if len(item) != 64 || strings.IndexFunc(item, func(char rune) bool {
			return (char < '0' || char > '9') && (char < 'a' || char > 'f')
		}) >= 0 {
			return nil, invalidChannelAnalyticsQuery("%s 包含无效 SHA-256：%s", key, item)
		}
		items[index] = item
	}
	return items, nil
}

func floorTimestamp(timestamp int64, bucketSeconds int64) int64 {
	return timestamp - timestamp%bucketSeconds
}

func ceilTimestamp(timestamp int64, bucketSeconds int64) int64 {
	if timestamp%bucketSeconds == 0 {
		return timestamp
	}
	return timestamp + bucketSeconds - timestamp%bucketSeconds
}

func invalidChannelAnalyticsQuery(format string, args ...interface{}) error {
	return fmt.Errorf("%w：%s", ErrInvalidChannelAnalyticsQuery, fmt.Sprintf(format, args...))
}

func ratioPointer(numerator int64, denominator int64) *float64 {
	if denominator <= 0 {
		return nil
	}
	value := float64(numerator) / float64(denominator)
	return &value
}

func averagePointer(sum int64, count int64) *float64 {
	if count <= 0 {
		return nil
	}
	value := float64(sum) / float64(count)
	return &value
}

func percentileLatency(histogram [model.ChannelMetricHistogramBuckets]int64, count int64, percentile float64) *int64 {
	if count <= 0 {
		return nil
	}
	target := int64(float64(count)*percentile + 0.999999)
	if target < 1 {
		target = 1
	}
	var cumulative int64
	for index, bucketCount := range histogram {
		cumulative += bucketCount
		if cumulative >= target {
			value := model.ChannelMetricHistogramUpperBoundsMs[index]
			if index == len(model.ChannelMetricHistogramUpperBoundsMs)-1 {
				value = model.ChannelMetricHistogramUpperBoundsMs[index-1]
			}
			return &value
		}
	}
	return nil
}

func boolPointer(value bool) *bool { return &value }

func channelAnalyticsMaxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func sortChannelAnalyticsItems(items []dto.ChannelAnalyticsChannelItem, sortBy string, sortOrder string) {
	if sortBy == "" {
		sortBy = "request_count"
	}
	descending := sortOrder != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		if sortBy == "channel_name" {
			leftEmpty := strings.TrimSpace(items[i].ChannelName) == ""
			rightEmpty := strings.TrimSpace(items[j].ChannelName) == ""
			if leftEmpty != rightEmpty {
				return !leftEmpty
			}
		}
		comparison := compareChannelAnalyticsItems(items[i], items[j], sortBy)
		if comparison == 0 {
			comparison = compareInt64(int64(items[i].ChannelId), int64(items[j].ChannelId))
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
}

func sortChannelAnalyticsModelItems(items []dto.ChannelAnalyticsModelItem, sortBy string, sortOrder string) {
	if sortBy == "" {
		sortBy = "request_count"
	}
	descending := sortOrder != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		comparison := compareChannelAnalyticsItems(items[i].ChannelAnalyticsChannelItem, items[j].ChannelAnalyticsChannelItem, sortBy)
		if comparison == 0 {
			left := items[i].RequestedModel + items[i].UpstreamModel
			right := items[j].RequestedModel + items[j].UpstreamModel
			comparison = strings.Compare(left, right)
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
}

func compareChannelAnalyticsItems(left dto.ChannelAnalyticsChannelItem, right dto.ChannelAnalyticsChannelItem, sortBy string) int {
	switch sortBy {
	case "channel_name":
		return strings.Compare(normalizeChannelAnalyticsName(left.ChannelName), normalizeChannelAnalyticsName(right.ChannelName))
	case "quality_success_rate":
		return compareOptionalFloat(left.ChannelQualitySuccessRate, right.ChannelQualitySuccessRate)
	case "failure_count":
		return compareInt64(left.FailureCount, right.FailureCount)
	case "p95_latency_ms":
		return compareOptionalInt64(left.P95LatencyMs, right.P95LatencyMs)
	case "input_tokens_total":
		return compareInt64(left.InputTokensTotal, right.InputTokensTotal)
	case "output_tokens":
		return compareInt64(left.OutputTokens, right.OutputTokens)
	case "cache_read_tokens":
		return compareInt64(left.CacheReadTokens, right.CacheReadTokens)
	case "charged_quota":
		return compareInt64(left.ChargedQuota, right.ChargedQuota)
	case "charged_micro_usd":
		return compareInt64(left.ChargedMicroUsd, right.ChargedMicroUsd)
	default:
		return compareInt64(left.ChannelAttemptCount, right.ChannelAttemptCount)
	}
}

func normalizeChannelAnalyticsName(name string) string {
	return strings.Map(func(character rune) rune {
		if character >= 'A' && character <= 'Z' {
			return character + ('a' - 'A')
		}
		return character
	}, strings.TrimSpace(name))
}

func compareInt64(left int64, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func compareOptionalInt64(left *int64, right *int64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	return compareInt64(*left, *right)
}

func compareOptionalFloat(left *float64, right *float64) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	if *left < *right {
		return -1
	}
	if *left > *right {
		return 1
	}
	return 0
}

func paginateChannelItems(items []dto.ChannelAnalyticsChannelItem, page int, pageSize int) []dto.ChannelAnalyticsChannelItem {
	if page <= 0 || pageSize <= 0 || (page > 1 && page-1 > len(items)/pageSize) {
		return []dto.ChannelAnalyticsChannelItem{}
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []dto.ChannelAnalyticsChannelItem{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func paginateModelItems(items []dto.ChannelAnalyticsModelItem, page int, pageSize int) []dto.ChannelAnalyticsModelItem {
	if page <= 0 || pageSize <= 0 || (page > 1 && page-1 > len(items)/pageSize) {
		return []dto.ChannelAnalyticsModelItem{}
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []dto.ChannelAnalyticsModelItem{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
