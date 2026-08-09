package channelmetrics

import (
	"errors"
	"fmt"
	"time"
)

// Scope 表示指标样本所属的生命周期层级。
type Scope string

const (
	ScopeFinalRequest   Scope = "final_request"
	ScopeChannelAttempt Scope = "channel_attempt"
	ScopeUpstreamCall   Scope = "upstream_call"
)

func (s Scope) Valid() bool {
	switch s {
	case ScopeFinalRequest, ScopeChannelAttempt, ScopeUpstreamCall:
		return true
	default:
		return false
	}
}

// Outcome 表示业务结果；HTTP 2xx 并不天然等于 success。
type Outcome string

const (
	OutcomeSuccess         Outcome = "success"
	OutcomeHTTPError       Outcome = "http_error"
	OutcomeTransportError  Outcome = "transport_error"
	OutcomeProtocolError   Outcome = "protocol_error"
	OutcomeStreamError     Outcome = "stream_error"
	OutcomeLocalError      Outcome = "local_error"
	OutcomeDispatchError   Outcome = "dispatch_error"
	OutcomeClientCancelled Outcome = "client_cancelled"
)

func (o Outcome) Valid() bool {
	switch o {
	case OutcomeSuccess,
		OutcomeHTTPError,
		OutcomeTransportError,
		OutcomeProtocolError,
		OutcomeStreamError,
		OutcomeLocalError,
		OutcomeDispatchError,
		OutcomeClientCancelled:
		return true
	default:
		return false
	}
}

// FailureOwner 表示失败的责任归属。成功样本使用空值。
type FailureOwner string

const (
	FailureOwnerNone    FailureOwner = ""
	FailureOwnerChannel FailureOwner = "channel"
	FailureOwnerClient  FailureOwner = "client"
	FailureOwnerGateway FailureOwner = "gateway"
	FailureOwnerUnknown FailureOwner = "unknown"
)

func (o FailureOwner) Valid() bool {
	switch o {
	case FailureOwnerNone, FailureOwnerChannel, FailureOwnerClient, FailureOwnerGateway, FailureOwnerUnknown:
		return true
	default:
		return false
	}
}

// TrafficSource 用于隔离真实业务、主动探测和调试流量。
type TrafficSource string

const (
	TrafficSourceRelay      TrafficSource = "relay"
	TrafficSourceProbe      TrafficSource = "probe"
	TrafficSourceTask       TrafficSource = "task"
	TrafficSourcePlayground TrafficSource = "playground"
)

func (s TrafficSource) Valid() bool {
	switch s {
	case TrafficSourceRelay, TrafficSourceProbe, TrafficSourceTask, TrafficSourcePlayground:
		return true
	default:
		return false
	}
}

// DataOrigin 区分原生实时采集与精度较低的历史回填。
type DataOrigin string

const (
	DataOriginLive   DataOrigin = "live"
	DataOriginLegacy DataOrigin = "legacy"
)

func (o DataOrigin) Valid() bool {
	return o == DataOriginLive || o == DataOriginLegacy
}

// ErrorStage 是可扩展的短字符串枚举，调用方可以在兼容范围内补充新阶段。
type ErrorStage string

const (
	ErrorStageNone            ErrorStage = ""
	ErrorStageAuth            ErrorStage = "auth"
	ErrorStageDispatch        ErrorStage = "dispatch"
	ErrorStageChannelSelect   ErrorStage = "channel_select"
	ErrorStagePreUpstream     ErrorStage = "pre_upstream"
	ErrorStageConnect         ErrorStage = "connect"
	ErrorStageUpstream        ErrorStage = "upstream_response"
	ErrorStageStream          ErrorStage = "stream_transfer"
	ErrorStageParse           ErrorStage = "parse"
	ErrorStageSettlement      ErrorStage = "settlement"
	ErrorStageUnfinalizedCall ErrorStage = "unfinalized_call"
)

// StatusCode 显式保存状态码是否适用于当前样本。
//
// Present=false 表示未知或不适用，此时 Code 会在采集时归零。
// 对 UpstreamStatus 而言，Present=true 且 Code=0 专门表示没有收到 HTTP 响应。
type StatusCode struct {
	Present bool `json:"present"`
	Code    int  `json:"code"`
}

func PresentStatus(code int) StatusCode {
	return StatusCode{Present: true, Code: code}
}

func (s StatusCode) NoHTTPResponse() bool {
	return s.Present && s.Code == 0
}

// Sample 是只包含值类型的不可变采集快照。
// 调用方应在构造完成后按值传入 Collector.Record，不要持有可变 RelayInfo 指针。
type Sample struct {
	OccurredAt time.Time `json:"occurred_at"`

	Scope      Scope  `json:"metric_scope"`
	RequestID  string `json:"request_id"`
	AttemptSeq int    `json:"attempt_seq"`
	CallIndex  int    `json:"call_index"`
	RetryCount int    `json:"retry_count"`

	RetryPlanned bool `json:"retry_planned"`
	// LastStartedAttempt 只适用于 final_request，用于描述请求实际启动到哪次尝试。
	LastStartedAttemptPresent bool `json:"last_started_attempt_present"`
	LastStartedAttemptSeq     int  `json:"last_started_attempt_seq"`

	ChannelPresent      bool   `json:"channel_present"`
	ChannelID           int    `json:"channel_id"`
	ChannelNameSnapshot string `json:"channel_name_snapshot"`
	ChannelType         int    `json:"channel_type"`

	RequestedModelPresent bool   `json:"requested_model_present"`
	RequestedModel        string `json:"requested_model"`
	UpstreamModelPresent  bool   `json:"upstream_model_present"`
	UpstreamModel         string `json:"upstream_model"`
	Group                 string `json:"group"`

	TrafficSource TrafficSource `json:"traffic_source"`
	DataOrigin    DataOrigin    `json:"data_origin"`
	Stream        bool          `json:"stream"`
	Outcome       Outcome       `json:"outcome"`
	FailureOwner  FailureOwner  `json:"failure_owner"`

	QualityEligible bool `json:"quality_eligible"`
	PartialResponse bool `json:"partial_response"`
	UpstreamStarted bool `json:"upstream_started"`

	ErrorStage      ErrorStage `json:"error_stage"`
	StreamEndReason string     `json:"stream_end_reason"`

	ClientStatus     StatusCode `json:"client_status"`
	UpstreamStatus   StatusCode `json:"upstream_status"`
	NormalizedStatus StatusCode `json:"normalized_status"`

	LatencyPresent bool  `json:"latency_present"`
	LatencyMs      int64 `json:"latency_ms"`
	TTFTPresent    bool  `json:"ttft_present"`
	TTFTMs         int64 `json:"ttft_ms"`
	// ResponseHeader 只描述 transport 收到响应头的耗时，不等于流式 TTFT。
	ResponseHeaderPresent bool  `json:"response_header_present"`
	ResponseHeaderMs      int64 `json:"response_header_ms"`

	// UsagePresent 只有在最终成功且结算完成的 channel_attempt 上才能为 true。
	UsagePresent        bool  `json:"usage_present"`
	InputTokensTotal    int64 `json:"input_tokens_total"`
	UncachedInputTokens int64 `json:"uncached_input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	CacheWriteTokens    int64 `json:"cache_write_tokens"`
	ChargedQuota        int64 `json:"charged_quota"`
	ChargedMicroUSD     int64 `json:"charged_micro_usd"`
}

var (
	ErrInvalidSample          = errors.New("无效的渠道指标样本")
	ErrCollectorCapacity      = errors.New("渠道指标热桶容量已满")
	ErrSinkNotConfigured      = errors.New("渠道指标持久化接收器未配置")
	ErrDimensionHashCollision = errors.New("渠道指标维度哈希冲突")
)

// NewLiveSample 创建带实时业务默认值的样本骨架。
func NewLiveSample(scope Scope, outcome Outcome) Sample {
	return Sample{
		Scope:         scope,
		Outcome:       outcome,
		TrafficSource: TrafficSourceRelay,
		DataOrigin:    DataOriginLive,
	}
}

// Validate 校验会影响统计口径的约束。TrafficSource 和 DataOrigin 的空值
// 分别按 relay、live 处理，以降低插桩遗漏造成的污染风险。
func (s Sample) Validate() error {
	s = normalizeSample(s)
	return validateSample(s)
}

func normalizeSample(s Sample) Sample {
	if s.TrafficSource == "" {
		s.TrafficSource = TrafficSourceRelay
	}
	if s.DataOrigin == "" {
		s.DataOrigin = DataOriginLive
	}
	if !s.ChannelPresent {
		s.ChannelID = 0
		s.ChannelNameSnapshot = ""
		s.ChannelType = 0
	}
	if !s.RequestedModelPresent {
		s.RequestedModel = ""
	}
	if !s.UpstreamModelPresent {
		s.UpstreamModel = ""
	}
	if !s.ClientStatus.Present {
		s.ClientStatus.Code = 0
	}
	if !s.UpstreamStatus.Present {
		s.UpstreamStatus.Code = 0
	}
	if !s.NormalizedStatus.Present {
		s.NormalizedStatus.Code = 0
	}
	if !s.LatencyPresent && s.LatencyMs > 0 {
		s.LatencyPresent = true
	}
	if !s.TTFTPresent && s.TTFTMs > 0 {
		s.TTFTPresent = true
	}
	return s
}

func validateSample(s Sample) error {
	if !s.Scope.Valid() {
		return invalidSample("未知 metric_scope %q", s.Scope)
	}
	if !s.Outcome.Valid() {
		return invalidSample("未知 outcome %q", s.Outcome)
	}
	if !s.FailureOwner.Valid() {
		return invalidSample("未知 failure_owner %q", s.FailureOwner)
	}
	if !s.TrafficSource.Valid() {
		return invalidSample("未知 traffic_source %q", s.TrafficSource)
	}
	if !s.DataOrigin.Valid() {
		return invalidSample("未知 data_origin %q", s.DataOrigin)
	}
	if s.AttemptSeq < 0 || s.CallIndex < 0 || s.RetryCount < 0 {
		return invalidSample("尝试序号、调用序号和重试次数不能为负数")
	}
	if (s.Scope == ScopeChannelAttempt || s.Scope == ScopeUpstreamCall) && s.AttemptSeq < 1 {
		return invalidSample("%s 的 attempt_seq 必须从 1 开始", s.Scope)
	}
	if s.LastStartedAttemptPresent && (s.Scope != ScopeFinalRequest || s.LastStartedAttemptSeq < 1) {
		return invalidSample("last_started_attempt 只适用于 final_request 且序号必须大于 0")
	}
	if (s.Scope == ScopeChannelAttempt || s.Scope == ScopeUpstreamCall) && !s.ChannelPresent {
		return invalidSample("%s 必须包含渠道维度", s.Scope)
	}
	if s.Scope == ScopeFinalRequest && (s.UpstreamStatus.Present || s.NormalizedStatus.Present) {
		return invalidSample("final_request 不允许记录上游状态码")
	}
	if s.Scope == ScopeChannelAttempt && (s.ClientStatus.Present || s.UpstreamStatus.Present || s.NormalizedStatus.Present) {
		return invalidSample("channel_attempt 不允许记录客户端或完整上游状态码")
	}
	if s.Scope == ScopeUpstreamCall && s.ClientStatus.Present {
		return invalidSample("upstream_call 不允许记录客户端状态码")
	}
	if err := validateStatus("client_status", s.ClientStatus, false); err != nil {
		return err
	}
	if err := validateStatus("upstream_status", s.UpstreamStatus, true); err != nil {
		return err
	}
	if err := validateStatus("normalized_status", s.NormalizedStatus, false); err != nil {
		return err
	}
	if s.Scope == ScopeUpstreamCall && s.Outcome == OutcomeSuccess && s.UpstreamStatus.NoHTTPResponse() {
		return invalidSample("成功的 upstream_call 不能使用无响应状态码 0")
	}
	if s.Outcome == OutcomeClientCancelled && s.QualityEligible {
		return invalidSample("client_cancelled 不能进入渠道质量分母")
	}
	if s.Outcome == OutcomeSuccess && s.FailureOwner != FailureOwnerNone {
		return invalidSample("成功样本不能设置 failure_owner")
	}
	if s.LatencyPresent && s.LatencyMs < 0 {
		return invalidSample("latency_ms 不能为负数")
	}
	if s.TTFTPresent && s.TTFTMs < 0 {
		return invalidSample("ttft_ms 不能为负数")
	}
	if s.ResponseHeaderPresent && s.ResponseHeaderMs < 0 {
		return invalidSample("response_header_ms 不能为负数")
	}
	if hasNegativeUsage(s) {
		return invalidSample("Token、额度和金额不能为负数")
	}
	if s.UsagePresent && (s.Scope != ScopeChannelAttempt || s.Outcome != OutcomeSuccess) {
		return invalidSample("用量只能附加到成功的 channel_attempt")
	}
	if !s.UsagePresent && hasUsageValue(s) {
		return invalidSample("存在用量数值时必须设置 usage_present")
	}
	return nil
}

func validateStatus(name string, status StatusCode, allowZero bool) error {
	if !status.Present {
		return nil
	}
	if allowZero && status.Code == 0 {
		return nil
	}
	if status.Code < 100 || status.Code > 999 {
		return invalidSample("%s=%d 不在有效范围", name, status.Code)
	}
	return nil
}

func hasNegativeUsage(s Sample) bool {
	return s.InputTokensTotal < 0 ||
		s.UncachedInputTokens < 0 ||
		s.OutputTokens < 0 ||
		s.CacheReadTokens < 0 ||
		s.CacheWriteTokens < 0 ||
		s.ChargedQuota < 0 ||
		s.ChargedMicroUSD < 0
}

func hasUsageValue(s Sample) bool {
	return s.InputTokensTotal != 0 ||
		s.UncachedInputTokens != 0 ||
		s.OutputTokens != 0 ||
		s.CacheReadTokens != 0 ||
		s.CacheWriteTokens != 0 ||
		s.ChargedQuota != 0 ||
		s.ChargedMicroUSD != 0
}

func invalidSample(format string, args ...any) error {
	return fmt.Errorf("%w：%s", ErrInvalidSample, fmt.Sprintf(format, args...))
}
