package dto

// ChannelAnalyticsQuery 是所有渠道可观测性接口共享的已校验查询条件。
type ChannelAnalyticsQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	BucketLevel    string
	BucketSeconds  int64
	MetricScope    string
	ModelDimension string

	ChannelIds          []int
	ChannelTypes        []int
	Groups              []string
	RequestedModels     []string
	UpstreamModels      []string
	RequestedModelHash  []string
	UpstreamModelHash   []string
	Outcomes            []string
	FailureOwners       []string
	ErrorStages         []string
	ClientStatusCodes   []int
	UpstreamStatusCodes []int
	TrafficSources      []string
	DataOrigins         []string
	Stream              *bool

	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// ChannelAnalyticsMeta 描述统计覆盖范围和数据质量，避免把缺失数据展示为真实零值。
type ChannelAnalyticsMeta struct {
	GeneratedAt                 int64  `json:"generated_at"`
	ReliableFromTs              int64  `json:"reliable_from_ts"`
	DataStartTs                 int64  `json:"data_start_ts"`
	DataEndTs                   int64  `json:"data_end_ts"`
	LastFlushedAt               int64  `json:"last_flushed_at"`
	RuntimePendingBatchCount    int64  `json:"runtime_pending_batch_count"`
	RuntimeFlushFailureCount    int64  `json:"runtime_flush_failure_count"`
	RuntimeLastFlushErrorAt     int64  `json:"runtime_last_flush_error_at"`
	BucketLevel                 string `json:"bucket_level"`
	BucketSeconds               int64  `json:"bucket_seconds"`
	RetentionDays               int    `json:"retention_days"`
	Partial                     bool   `json:"partial"`
	DetailAvailable             bool   `json:"detail_available"`
	UncoveredChannelTypes       []int  `json:"uncovered_channel_types"`
	InvalidSampleCount          int64  `json:"invalid_sample_count"`
	DimensionOverflowCount      int64  `json:"dimension_overflow_count"`
	DroppedMetricEventCount     int64  `json:"dropped_metric_event_count"`
	DroppedFailureEventCount    int64  `json:"dropped_failure_event_count"`
	DimensionHashCollisionCount int64  `json:"dimension_hash_collision_count"`
}

type ChannelAnalyticsSummary struct {
	FinalRequestCount         int64    `json:"final_request_count"`
	ChannelAttemptCount       int64    `json:"channel_attempt_count"`
	UpstreamCallCount         int64    `json:"upstream_call_count"`
	FailedAttemptCount        int64    `json:"failed_attempt_count"`
	RetryCount                int64    `json:"retry_count"`
	ClientSuccessRate         *float64 `json:"client_success_rate"`
	ChannelQualitySuccessRate *float64 `json:"channel_quality_success_rate"`
	AttemptSuccessRate        *float64 `json:"attempt_success_rate"`
	RetryRate                 *float64 `json:"retry_rate"`
	UsageSampleCount          int64    `json:"usage_sample_count"`
	InputTokensTotal          int64    `json:"input_tokens_total"`
	UncachedInputTokens       int64    `json:"uncached_input_tokens"`
	OutputTokens              int64    `json:"output_tokens"`
	TotalTokens               int64    `json:"total_tokens"`
	CacheReadTokens           int64    `json:"cache_read_tokens"`
	CacheWriteTokens          int64    `json:"cache_write_tokens"`
	CacheRequestHitRate       *float64 `json:"cache_request_hit_rate"`
	CacheTokenHitRate         *float64 `json:"cache_token_hit_rate"`
	ChargedQuota              int64    `json:"charged_quota"`
	ChargedMicroUsd           int64    `json:"charged_micro_usd"`
	AvgLatencyMs              *float64 `json:"avg_latency_ms"`
	P95LatencyMs              *int64   `json:"p95_latency_ms"`
	AvgTtftMs                 *float64 `json:"avg_ttft_ms"`
	P95TtftMs                 *int64   `json:"p95_ttft_ms"`
}

type ChannelAnalyticsSummaryResponse struct {
	Summary ChannelAnalyticsSummary `json:"summary"`
	Meta    ChannelAnalyticsMeta    `json:"meta"`
}

type ChannelAnalyticsTrendPoint struct {
	BucketTs            int64    `json:"bucket_ts"`
	FinalRequestCount   int64    `json:"final_request_count"`
	ChannelAttemptCount int64    `json:"channel_attempt_count"`
	UpstreamCallCount   int64    `json:"upstream_call_count"`
	FailedAttemptCount  int64    `json:"failed_attempt_count"`
	TotalTokens         int64    `json:"total_tokens"`
	EventCount          int64    `json:"event_count,omitempty"`
	SuccessCount        int64    `json:"success_count,omitempty"`
	AvgLatencyMs        *float64 `json:"avg_latency_ms,omitempty"`
}

type ChannelAnalyticsTrendResponse struct {
	Points []ChannelAnalyticsTrendPoint `json:"points"`
	Meta   ChannelAnalyticsMeta         `json:"meta"`
}

type ChannelAnalyticsStatusCode struct {
	StatusPresent bool   `json:"status_present"`
	StatusCode    int    `json:"status_code"`
	Label         string `json:"label"`
	Count         int64  `json:"count"`
}

type ChannelAnalyticsErrorStage struct {
	ErrorStage string `json:"error_stage"`
	Count      int64  `json:"count"`
}

type ChannelAnalyticsChannelItem struct {
	ChannelId                 int                          `json:"channel_id"`
	ChannelName               string                       `json:"channel_name"`
	ChannelType               int                          `json:"channel_type"`
	ChannelTypeName           string                       `json:"channel_type_name"`
	Group                     string                       `json:"group,omitempty"`
	ChannelAttemptCount       int64                        `json:"channel_attempt_count"`
	FailureCount              int64                        `json:"failure_count"`
	RetryCount                int64                        `json:"retry_count"`
	ChannelQualitySuccessRate *float64                     `json:"channel_quality_success_rate"`
	AttemptSuccessRate        *float64                     `json:"attempt_success_rate"`
	UsageSampleCount          int64                        `json:"usage_sample_count"`
	InputTokensTotal          int64                        `json:"input_tokens_total"`
	UncachedInputTokens       int64                        `json:"uncached_input_tokens"`
	OutputTokens              int64                        `json:"output_tokens"`
	CacheReadTokens           int64                        `json:"cache_read_tokens"`
	CacheWriteTokens          int64                        `json:"cache_write_tokens"`
	CacheRequestHitRate       *float64                     `json:"cache_request_hit_rate"`
	CacheTokenHitRate         *float64                     `json:"cache_token_hit_rate"`
	AvgLatencyMs              *float64                     `json:"avg_latency_ms"`
	P95LatencyMs              *int64                       `json:"p95_latency_ms"`
	AvgTtftMs                 *float64                     `json:"avg_ttft_ms"`
	P95TtftMs                 *int64                       `json:"p95_ttft_ms"`
	ChargedQuota              int64                        `json:"charged_quota"`
	ChargedMicroUsd           int64                        `json:"charged_micro_usd"`
	LastFailureAt             int64                        `json:"last_failure_at"`
	TopStatusCodes            []ChannelAnalyticsStatusCode `json:"top_status_codes"`
}

type ChannelAnalyticsModelItem struct {
	ChannelAnalyticsChannelItem
	RequestedModel string `json:"requested_model,omitempty"`
	UpstreamModel  string `json:"upstream_model,omitempty"`
	ModelHash      string `json:"model_hash,omitempty"`
}

type ChannelAnalyticsChannelsResponse struct {
	Items    []ChannelAnalyticsChannelItem `json:"items"`
	Total    int                           `json:"total"`
	Page     int                           `json:"page"`
	PageSize int                           `json:"page_size"`
	Meta     ChannelAnalyticsMeta          `json:"meta"`
}

type ChannelAnalyticsModelsResponse struct {
	Items    []ChannelAnalyticsModelItem `json:"items"`
	Total    int                         `json:"total"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
	Meta     ChannelAnalyticsMeta        `json:"meta"`
}

type ChannelAnalyticsStatusResponse struct {
	Items       []ChannelAnalyticsStatusCode `json:"items"`
	ErrorStages []ChannelAnalyticsErrorStage `json:"error_stages"`
	Meta        ChannelAnalyticsMeta         `json:"meta"`
}

type ChannelAnalyticsFailureItem struct {
	EventId                 string `json:"event_id"`
	CreatedAt               int64  `json:"created_at"`
	RequestId               string `json:"request_id"`
	AttemptSeq              int    `json:"attempt_seq"`
	RetryPlanned            bool   `json:"retry_planned"`
	IsLastStartedAttempt    bool   `json:"is_last_started_attempt"`
	ChannelId               int    `json:"channel_id"`
	ChannelName             string `json:"channel_name"`
	ChannelType             int    `json:"channel_type"`
	RequestedModel          string `json:"requested_model"`
	RequestedModelHash      string `json:"requested_model_hash"`
	UpstreamModel           string `json:"upstream_model"`
	UpstreamModelHash       string `json:"upstream_model_hash"`
	Group                   string `json:"group"`
	TrafficSource           string `json:"traffic_source"`
	Outcome                 string `json:"outcome"`
	FailureOwner            string `json:"failure_owner"`
	QualityEligible         bool   `json:"quality_eligible"`
	PartialResponse         bool   `json:"partial_response"`
	ErrorStage              string `json:"error_stage"`
	StreamEndReason         string `json:"stream_end_reason"`
	UpstreamStatusPresent   bool   `json:"upstream_status_present"`
	UpstreamStatusCode      int    `json:"upstream_status_code"`
	NormalizedStatusPresent bool   `json:"normalized_status_present"`
	NormalizedStatusCode    int    `json:"normalized_status_code"`
	ClientStatusPresent     bool   `json:"client_status_present"`
	ClientStatusCode        int    `json:"client_status_code"`
	LatencyMs               int64  `json:"latency_ms"`
	TtftPresent             bool   `json:"ttft_present"`
	TtftMs                  int64  `json:"ttft_ms"`
	RetryReason             string `json:"retry_reason"`
	ErrorSummary            string `json:"error_summary"`
}

type ChannelAnalyticsFailuresResponse struct {
	Items    []ChannelAnalyticsFailureItem `json:"items"`
	Total    int64                         `json:"total"`
	Page     int                           `json:"page"`
	PageSize int                           `json:"page_size"`
	Meta     ChannelAnalyticsMeta          `json:"meta"`
}

type ChannelAnalyticsFilterChannel struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	ChannelType int    `json:"channel_type"`
}

type ChannelAnalyticsFilterType struct {
	Value int    `json:"value"`
	Label string `json:"label"`
}

type ChannelAnalyticsFilterModel struct {
	Value     string `json:"value"`
	Label     string `json:"label"`
	Model     string `json:"model"`
	ModelHash string `json:"model_hash"`
}

type ChannelAnalyticsFiltersResponse struct {
	Channels              []ChannelAnalyticsFilterChannel `json:"channels"`
	ChannelTypes          []ChannelAnalyticsFilterType    `json:"channel_types"`
	RequestedModels       []string                        `json:"requested_models"`
	UpstreamModels        []string                        `json:"upstream_models"`
	RequestedModelOptions []ChannelAnalyticsFilterModel   `json:"requested_model_options"`
	UpstreamModelOptions  []ChannelAnalyticsFilterModel   `json:"upstream_model_options"`
	Outcomes              []string                        `json:"outcomes"`
	TrafficSources        []string                        `json:"traffic_sources"`
	DataOrigins           []string                        `json:"data_origins"`
	Meta                  ChannelAnalyticsMeta            `json:"meta"`
}
