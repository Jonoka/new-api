package service

import (
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
)

const (
	channelAnalyticsMaxStabilityWindows = 5
	// 低于该可归因样本数时，成功率仍返回，但调用方必须标为样本不足。
	channelAnalyticsMinimumStabilitySamples int64 = 10
)

type channelAnalyticsStabilityDimension struct {
	group   bool
	channel bool
	model   bool
}

var channelAnalyticsStabilityDimensions = map[string]channelAnalyticsStabilityDimension{
	"group":               {group: true},
	"channel":             {channel: true},
	"model":               {model: true},
	"group_model":         {group: true, model: true},
	"group_channel":       {group: true, channel: true},
	"channel_model":       {channel: true, model: true},
	"group_channel_model": {group: true, channel: true, model: true},
}

// ParseChannelAnalyticsStabilityQuery 解析运维矩阵查询。
// windows 是以同一结束时间为右边界的重叠窗口，例如 15m,1h,6h,24h。
func ParseChannelAnalyticsStabilityQuery(values url.Values) (dto.ChannelAnalyticsStabilityQuery, error) {
	setting := channelMetricsEffectiveSetting()
	retentionSeconds := int64(setting.RetentionDays) * 24 * 60 * 60
	if retentionSeconds <= 0 {
		return dto.ChannelAnalyticsStabilityQuery{}, invalidChannelAnalyticsQuery("渠道指标保留期无效")
	}
	windows, err := parseChannelAnalyticsStabilityWindows(values, int64(setting.BucketSeconds), retentionSeconds)
	if err != nil {
		return dto.ChannelAnalyticsStabilityQuery{}, err
	}
	maxWindow := int64(0)
	for _, window := range windows {
		if window > maxWindow {
			maxWindow = window
		}
	}

	now := time.Now().Unix()
	endTimestamp, err := parseOptionalPositiveInt64(values.Get("end_timestamp"), now, "end_timestamp")
	if err != nil {
		return dto.ChannelAnalyticsStabilityQuery{}, err
	}
	if endTimestamp > now {
		endTimestamp = now
	}
	cloned := cloneChannelAnalyticsValues(values)
	cloned.Set("end_timestamp", strconv.FormatInt(endTimestamp, 10))
	cloned.Set("start_timestamp", strconv.FormatInt(endTimestamp-maxWindow, 10))
	// 运维矩阵默认同时读取实时事实和幂等历史回填；回填以切换点截断，
	// 因而两个来源可以安全相加。显式传参时仍完全尊重调用方选择。
	if !channelAnalyticsHasNonEmptyListValue(values["data_origin"]) {
		cloned["data_origin"] = []string{
			string(channelmetrics.DataOriginLive),
			string(channelmetrics.DataOriginLegacy),
		}
	}
	query, err := parseChannelAnalyticsQuery(cloned, setting.RetentionDays)
	if err != nil {
		return dto.ChannelAnalyticsStabilityQuery{}, err
	}
	// 公共解析器会为普通趋势查询向外扩到完整桶；稳定性窗口本身已经是
	// 桶粒度的整数倍，因此以对齐后的结束时间反推可保持窗口长度精确。
	query.StartTimestamp = query.EndTimestamp - maxWindow
	dimension := strings.ToLower(strings.TrimSpace(values.Get("dimension")))
	if dimension == "" {
		dimension = "group_channel_model"
	}
	if _, ok := channelAnalyticsStabilityDimensions[dimension]; !ok {
		return dto.ChannelAnalyticsStabilityQuery{}, invalidChannelAnalyticsQuery("不支持的 dimension：%s", dimension)
	}
	if err := validateChannelAnalyticsStabilityQuery(query); err != nil {
		return dto.ChannelAnalyticsStabilityQuery{}, err
	}
	return dto.ChannelAnalyticsStabilityQuery{
		ChannelAnalyticsQuery: query,
		Dimension:             dimension,
		WindowSeconds:         windows,
	}, nil
}

func channelAnalyticsHasNonEmptyListValue(values []string) bool {
	for _, raw := range values {
		for _, value := range strings.Split(raw, ",") {
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
	}
	return false
}

func cloneChannelAnalyticsValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values)+2)
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}

func parseChannelAnalyticsStabilityWindows(values url.Values, bucketSeconds int64, retentionSeconds int64) ([]int64, error) {
	if bucketSeconds <= 0 {
		return nil, invalidChannelAnalyticsQuery("渠道指标桶粒度无效")
	}
	rawValues := values["windows"]
	if len(rawValues) == 0 {
		defaults := []int64{15 * 60, 60 * 60, 6 * 60 * 60, 24 * 60 * 60, 7 * 24 * 60 * 60}
		windows := make([]int64, 0, len(defaults))
		for _, value := range defaults {
			if value >= bucketSeconds && value <= retentionSeconds && value%bucketSeconds == 0 {
				windows = append(windows, value)
			}
		}
		if len(windows) == 0 {
			fallback := retentionSeconds / bucketSeconds * bucketSeconds
			if fallback <= 0 {
				return nil, invalidChannelAnalyticsQuery("渠道指标桶粒度不能大于保留期")
			}
			windows = append(windows, fallback)
		}
		return windows, nil
	}

	result := make([]int64, 0, channelAnalyticsMaxStabilityWindows)
	seen := make(map[int64]struct{})
	for _, rawValue := range rawValues {
		for _, item := range strings.Split(rawValue, ",") {
			seconds, err := parseChannelAnalyticsWindow(strings.TrimSpace(item))
			if err != nil {
				return nil, err
			}
			if seconds < bucketSeconds || seconds > retentionSeconds || seconds%bucketSeconds != 0 {
				return nil, invalidChannelAnalyticsQuery("windows 必须是 %d 秒的整数倍，且不超过 %d 天", bucketSeconds, retentionSeconds/(24*60*60))
			}
			if _, exists := seen[seconds]; exists {
				continue
			}
			seen[seconds] = struct{}{}
			result = append(result, seconds)
			if len(result) > channelAnalyticsMaxStabilityWindows {
				return nil, invalidChannelAnalyticsQuery("windows 最多支持 %d 个时间窗", channelAnalyticsMaxStabilityWindows)
			}
		}
	}
	if len(result) == 0 {
		return nil, invalidChannelAnalyticsQuery("windows 不能为空")
	}
	return result, nil
}

func parseChannelAnalyticsWindow(raw string) (int64, error) {
	if raw == "" {
		return 0, invalidChannelAnalyticsQuery("windows 包含空值")
	}
	lower := strings.ToLower(raw)
	multiplier := int64(1)
	number := lower
	switch lower[len(lower)-1] {
	case 'm':
		multiplier, number = 60, lower[:len(lower)-1]
	case 'h':
		multiplier, number = 60*60, lower[:len(lower)-1]
	case 'd':
		multiplier, number = 24*60*60, lower[:len(lower)-1]
	case 's':
		number = lower[:len(lower)-1]
	}
	value, err := strconv.ParseInt(number, 10, 64)
	if err != nil || value <= 0 || value > int64(^uint64(0)>>1)/multiplier {
		return 0, invalidChannelAnalyticsQuery("无法识别时间窗：%s", raw)
	}
	return value * multiplier, nil
}

func validateChannelAnalyticsStabilityQuery(query dto.ChannelAnalyticsQuery) error {
	if query.MetricScope != "" {
		return invalidChannelAnalyticsQuery("stability 的 metric_scope 固定为 channel_attempt")
	}
	if len(query.ClientStatusCodes) > 0 || len(query.UpstreamStatusCodes) > 0 {
		return invalidChannelAnalyticsQuery("stability 不支持状态码过滤")
	}
	allowedSort := map[string]bool{
		"": true, "request_count": true, "channel_attempt_count": true,
		"failure_count": true, "quality_success_rate": true, "attempt_success_rate": true,
		"retry_count": true, "retry_rate": true, "p95_latency_ms": true, "p95_ttft_ms": true,
		"input_tokens_total": true, "output_tokens": true, "total_tokens": true,
		"cache_read_tokens": true, "cache_token_hit_rate": true,
		"charged_quota": true, "charged_micro_usd": true,
	}
	if !allowedSort[query.SortBy] {
		return invalidChannelAnalyticsQuery("不支持的 sort_by：%s", query.SortBy)
	}
	return nil
}

// GetChannelAnalyticsStability 返回分组、渠道、模型任意安全组合的多时间窗矩阵。
func GetChannelAnalyticsStability(query dto.ChannelAnalyticsStabilityQuery) (dto.ChannelAnalyticsStabilityResponse, error) {
	spec, ok := channelAnalyticsStabilityDimensions[query.Dimension]
	if !ok || len(query.WindowSeconds) == 0 {
		return dto.ChannelAnalyticsStabilityResponse{}, ErrInvalidChannelAnalyticsQuery
	}
	selection := model.ChannelMetricDimensionSelection{
		Group:   spec.group,
		Channel: spec.channel,
	}
	if spec.model {
		selection.UpstreamModel = query.ModelDimension == "upstream"
		selection.RequestedModel = !selection.UpstreamModel
	}

	itemsByKey := make(map[string]*dto.ChannelAnalyticsStabilityItem)
	channelIDs := make(map[int]struct{})
	for windowIndex, windowSeconds := range query.WindowSeconds {
		windowQuery := query.ChannelAnalyticsQuery
		windowQuery.StartTimestamp = windowQuery.EndTimestamp - windowSeconds
		filter := channelAnalyticsMetricFilter(windowQuery, string(channelmetrics.ScopeChannelAttempt))
		if spec.channel {
			filter.ChannelPresent = boolPointer(true)
		}
		if spec.model {
			if selection.UpstreamModel {
				filter.UpstreamModelPresent = boolPointer(true)
			} else {
				filter.RequestedModelPresent = boolPointer(true)
			}
		}
		rows, err := model.AggregateChannelMetricsByDimensions(model.LOG_DB, filter, selection)
		if err != nil {
			return dto.ChannelAnalyticsStabilityResponse{}, err
		}
		upstreamFilter := filter
		upstreamFilter.MetricScopes = []string{string(channelmetrics.ScopeUpstreamCall)}
		upstreamRows, err := model.AggregateChannelMetricsByDimensions(model.LOG_DB, upstreamFilter, selection)
		if err != nil {
			return dto.ChannelAnalyticsStabilityResponse{}, err
		}
		mergedRows := mergeChannelAnalyticsStabilityRows(rows, spec, selection.UpstreamModel)
		upstreamByKey := mergeChannelAnalyticsStabilityRows(upstreamRows, spec, selection.UpstreamModel)
		for key, row := range mergedRows {
			item := itemsByKey[key]
			if item == nil {
				created := channelAnalyticsStabilityItemFromRow(key, row, spec, selection.UpstreamModel, query)
				item = &created
				itemsByKey[key] = item
			}
			item.Windows[windowIndex] = channelAnalyticsStabilityWindowFromRows(
				windowSeconds,
				windowQuery.EndTimestamp-windowSeconds,
				windowQuery.EndTimestamp,
				row,
				upstreamByKey[key],
			)
			if item.ChannelId > 0 {
				channelIDs[item.ChannelId] = struct{}{}
			}
		}
	}

	ids := make([]int, 0, len(channelIDs))
	for channelID := range channelIDs {
		ids = append(ids, channelID)
	}
	currentChannels, err := currentChannelMetadata(ids)
	if err != nil {
		return dto.ChannelAnalyticsStabilityResponse{}, err
	}
	groupNames, err := model.GetGroupDisplayNameMap()
	if err != nil {
		return dto.ChannelAnalyticsStabilityResponse{}, err
	}
	items := make([]dto.ChannelAnalyticsStabilityItem, 0, len(itemsByKey))
	for _, item := range itemsByKey {
		if current, exists := currentChannels[item.ChannelId]; exists {
			item.ChannelName = current.Name
			item.ChannelType = current.Type
			item.ChannelTypeName = constant.GetChannelTypeName(current.Type)
		}
		if item.Group != "" {
			item.GroupName = groupNames[item.Group]
			if item.GroupName == "" {
				item.GroupName = item.Group
			}
		}
		items = append(items, *item)
	}
	sortChannelAnalyticsStabilityItems(items, query.SortBy, query.SortOrder)
	total := len(items)
	items = paginateChannelAnalyticsStabilityItems(items, query.Page, query.PageSize)
	meta, err := channelAnalyticsMeta(query.ChannelAnalyticsQuery, channelAnalyticsMetricFilter(query.ChannelAnalyticsQuery, string(channelmetrics.ScopeChannelAttempt)), true)
	if err != nil {
		return dto.ChannelAnalyticsStabilityResponse{}, err
	}
	return dto.ChannelAnalyticsStabilityResponse{
		Dimension: query.Dimension,
		Items:     items,
		Total:     total,
		Page:      query.Page,
		PageSize:  query.PageSize,
		Meta:      meta,
	}, nil
}

func channelAnalyticsStabilityItemFromRow(key string, row model.ChannelMetricAggregateRow, spec channelAnalyticsStabilityDimension, upstream bool, query dto.ChannelAnalyticsStabilityQuery) dto.ChannelAnalyticsStabilityItem {
	item := dto.ChannelAnalyticsStabilityItem{
		Key:     key,
		Windows: make([]dto.ChannelAnalyticsStabilityWindow, len(query.WindowSeconds)),
	}
	for index, windowSeconds := range query.WindowSeconds {
		item.Windows[index] = dto.ChannelAnalyticsStabilityWindow{
			WindowSeconds:      windowSeconds,
			StartTimestamp:     query.EndTimestamp - windowSeconds,
			EndTimestamp:       query.EndTimestamp,
			MinimumSampleCount: channelAnalyticsMinimumStabilitySamples,
		}
	}
	if spec.group {
		item.Group = row.Group
	}
	if spec.channel {
		item.ChannelId = row.ChannelId
		item.ChannelName = row.ChannelNameSnapshot
		item.ChannelType = row.ChannelType
		item.ChannelTypeName = constant.GetChannelTypeName(row.ChannelType)
	}
	if spec.model {
		if upstream {
			item.UpstreamModel, item.ModelHash = row.UpstreamModel, row.UpstreamModelHash
		} else {
			item.RequestedModel, item.ModelHash = row.RequestedModel, row.RequestedModelHash
		}
	}
	return item
}

func channelAnalyticsStabilityWindowFromRows(windowSeconds int64, startTimestamp int64, endTimestamp int64, row model.ChannelMetricAggregateRow, upstream model.ChannelMetricAggregateRow) dto.ChannelAnalyticsStabilityWindow {
	eligibleCount := channelAnalyticsMaxInt64(0, row.EventCount-row.ClientCancelledCount)
	return dto.ChannelAnalyticsStabilityWindow{
		WindowSeconds:              windowSeconds,
		StartTimestamp:             startTimestamp,
		EndTimestamp:               endTimestamp,
		ChannelAttemptCount:        row.EventCount,
		FailureCount:               channelAnalyticsMaxInt64(0, eligibleCount-row.SuccessCount),
		QualityEligibleCount:       row.QualityEligibleCount,
		QualitySuccessCount:        row.QualitySuccessCount,
		QualitySuccessRate:         ratioPointer(row.QualitySuccessCount, row.QualityEligibleCount),
		AttemptEligibleCount:       eligibleCount,
		AttemptSuccessCount:        row.SuccessCount,
		AttemptSuccessRate:         ratioPointer(row.SuccessCount, eligibleCount),
		RetryCount:                 row.NonFirstAttemptCount,
		RetryRate:                  ratioPointer(row.NonFirstAttemptCount, row.EventCount),
		PartialResponseCount:       row.PartialResponseCount,
		UpstreamCallCount:          upstream.EventCount,
		UpstreamStatusSampleCount:  upstream.UpstreamStatusSampleCount,
		UpstreamStatusCoverageRate: ratioPointer(upstream.UpstreamStatusSampleCount, upstream.EventCount),
		Upstream429Count:           upstream.Upstream429Count,
		Upstream4xxCount:           upstream.Upstream4xxCount,
		Upstream5xxCount:           upstream.Upstream5xxCount,
		HTTPErrorCount:             row.HTTPErrorCount,
		TransportErrorCount:        row.TransportErrorCount,
		ProtocolErrorCount:         row.ProtocolErrorCount,
		StreamErrorCount:           row.StreamErrorCount,
		LocalErrorCount:            row.LocalErrorCount,
		DispatchErrorCount:         row.DispatchErrorCount,
		ClientCancelledCount:       row.ClientCancelledCount,
		LiveEventCount:             row.LiveEventCount,
		LegacyEventCount:           row.LegacyEventCount,
		LiveEventRate:              ratioPointer(row.LiveEventCount, row.EventCount),
		LegacyEventRate:            ratioPointer(row.LegacyEventCount, row.EventCount),
		MinimumSampleCount:         channelAnalyticsMinimumStabilitySamples,
		SampleSufficient:           row.QualityEligibleCount >= channelAnalyticsMinimumStabilitySamples,
		UsageSampleCount:           row.UsageSampleCount,
		UsageSuccessCoverageRate:   ratioPointer(row.SuccessUsageSampleCount, row.SuccessCount),
		InputTokensTotal:           row.InputTokensTotal,
		UncachedInputTokens:        row.UncachedInputTokens,
		OutputTokens:               row.OutputTokens,
		TotalTokens:                row.InputTokensTotal + row.OutputTokens,
		CacheReadTokens:            row.CacheReadTokens,
		CacheWriteTokens:           row.CacheWriteTokens,
		CacheRequestHitRate:        ratioPointer(row.CacheHitRequestCount, row.UsageSampleCount),
		CacheTokenHitRate:          ratioPointer(row.CacheReadTokens, row.InputTokensTotal),
		LatencySampleCount:         row.LatencyCount,
		LatencyCoverageRate:        ratioPointer(row.LatencyCount, row.EventCount),
		AvgLatencyMs:               averagePointer(row.LatencySumMs, row.LatencyCount),
		P95LatencyMs:               percentileLatency(row.LatencyHistogram(), row.LatencyCount, 0.95),
		TtftSampleCount:            row.TtftCount,
		TtftCoverageRate:           ratioPointer(row.TtftCount, row.EventCount),
		AvgTtftMs:                  averagePointer(row.TtftSumMs, row.TtftCount),
		P95TtftMs:                  percentileLatency(row.TtftHistogram(), row.TtftCount, 0.95),
		ChargedQuota:               row.ChargedQuota,
		ChargedMicroUsd:            row.ChargedMicroUsd,
		LastFailureBucketTs:        row.LastFailureBucketTs,
	}
}

func mergeChannelAnalyticsStabilityRows(rows []model.ChannelMetricAggregateRow, spec channelAnalyticsStabilityDimension, upstream bool) map[string]model.ChannelMetricAggregateRow {
	result := make(map[string]model.ChannelMetricAggregateRow)
	for _, row := range rows {
		key := channelAnalyticsStabilityKey(row, spec, upstream)
		current := result[key]
		if current.Group == "" {
			current.Group, current.GroupHash = row.Group, row.GroupHash
		}
		if current.ChannelNameSnapshot == "" {
			current.ChannelNameSnapshot = row.ChannelNameSnapshot
		}
		if current.ChannelType == 0 {
			current.ChannelType = row.ChannelType
		}
		current.ChannelId = row.ChannelId
		if current.RequestedModel == "" {
			current.RequestedModel, current.RequestedModelHash = row.RequestedModel, row.RequestedModelHash
		}
		if current.UpstreamModel == "" {
			current.UpstreamModel, current.UpstreamModelHash = row.UpstreamModel, row.UpstreamModelHash
		}
		mergeChannelMetricAggregate(&current, row)
		result[key] = current
	}
	return result
}

func channelAnalyticsStabilityKey(row model.ChannelMetricAggregateRow, spec channelAnalyticsStabilityDimension, upstream bool) string {
	parts := []string{"ops"}
	if spec.group {
		groupHash := row.GroupHash
		if groupHash == "" {
			groupHash = channelmetrics.SHA256String(row.Group)
		}
		parts = append(parts, "g:"+groupHash)
	}
	if spec.channel {
		parts = append(parts, fmt.Sprintf("c:%d", row.ChannelId))
	}
	if spec.model {
		modelHash := row.RequestedModelHash
		if upstream {
			modelHash = row.UpstreamModelHash
		}
		parts = append(parts, "m:"+modelHash)
	}
	return channelmetrics.SHA256String(strings.Join(parts, "|"))
}

func sortChannelAnalyticsStabilityItems(items []dto.ChannelAnalyticsStabilityItem, sortBy string, sortOrder string) {
	if sortBy == "" {
		sortBy = "request_count"
	}
	descending := sortOrder != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		left, right := firstChannelAnalyticsStabilityWindow(items[i]), firstChannelAnalyticsStabilityWindow(items[j])
		leftMissing := channelAnalyticsStabilitySortValueMissing(left, sortBy)
		rightMissing := channelAnalyticsStabilitySortValueMissing(right, sortBy)
		if leftMissing != rightMissing {
			// 无样本不是“最差成功率”或“最低延迟”，无论升降序都放在末尾。
			return !leftMissing
		}
		comparison := compareChannelAnalyticsStabilityWindows(left, right, sortBy)
		if comparison == 0 {
			comparison = strings.Compare(items[i].Key, items[j].Key)
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
}

func channelAnalyticsStabilitySortValueMissing(window dto.ChannelAnalyticsStabilityWindow, sortBy string) bool {
	switch sortBy {
	case "quality_success_rate":
		return window.QualitySuccessRate == nil
	case "attempt_success_rate":
		return window.AttemptSuccessRate == nil
	case "retry_rate":
		return window.RetryRate == nil
	case "p95_latency_ms":
		return window.P95LatencyMs == nil
	case "p95_ttft_ms":
		return window.P95TtftMs == nil
	case "cache_token_hit_rate":
		return window.CacheTokenHitRate == nil
	default:
		return false
	}
}

func firstChannelAnalyticsStabilityWindow(item dto.ChannelAnalyticsStabilityItem) dto.ChannelAnalyticsStabilityWindow {
	if len(item.Windows) == 0 {
		return dto.ChannelAnalyticsStabilityWindow{}
	}
	return item.Windows[0]
}

func compareChannelAnalyticsStabilityWindows(left dto.ChannelAnalyticsStabilityWindow, right dto.ChannelAnalyticsStabilityWindow, sortBy string) int {
	switch sortBy {
	case "failure_count":
		return compareInt64(left.FailureCount, right.FailureCount)
	case "quality_success_rate":
		return compareOptionalFloat(left.QualitySuccessRate, right.QualitySuccessRate)
	case "attempt_success_rate":
		return compareOptionalFloat(left.AttemptSuccessRate, right.AttemptSuccessRate)
	case "retry_count":
		return compareInt64(left.RetryCount, right.RetryCount)
	case "retry_rate":
		return compareOptionalFloat(left.RetryRate, right.RetryRate)
	case "p95_latency_ms":
		return compareOptionalInt64(left.P95LatencyMs, right.P95LatencyMs)
	case "p95_ttft_ms":
		return compareOptionalInt64(left.P95TtftMs, right.P95TtftMs)
	case "input_tokens_total":
		return compareInt64(left.InputTokensTotal, right.InputTokensTotal)
	case "output_tokens":
		return compareInt64(left.OutputTokens, right.OutputTokens)
	case "total_tokens":
		return compareInt64(left.TotalTokens, right.TotalTokens)
	case "cache_read_tokens":
		return compareInt64(left.CacheReadTokens, right.CacheReadTokens)
	case "cache_token_hit_rate":
		return compareOptionalFloat(left.CacheTokenHitRate, right.CacheTokenHitRate)
	case "charged_quota":
		return compareInt64(left.ChargedQuota, right.ChargedQuota)
	case "charged_micro_usd":
		return compareInt64(left.ChargedMicroUsd, right.ChargedMicroUsd)
	default:
		return compareInt64(left.ChannelAttemptCount, right.ChannelAttemptCount)
	}
}

func paginateChannelAnalyticsStabilityItems(items []dto.ChannelAnalyticsStabilityItem, page int, pageSize int) []dto.ChannelAnalyticsStabilityItem {
	if page <= 0 || pageSize <= 0 || (page > 1 && page-1 > len(items)/pageSize) {
		return []dto.ChannelAnalyticsStabilityItem{}
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []dto.ChannelAnalyticsStabilityItem{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
