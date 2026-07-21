package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type ChannelMetricUsage struct {
	InputTokensTotal    int64
	UncachedInputTokens int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheWriteTokens    int64
	ChargedQuota        int64
}

type channelMetricCallState struct {
	index         int
	startedAt     time.Time
	headerAt      time.Time
	status        channelmetrics.StatusCode
	transportFail bool
}

type channelMetricAttemptState struct {
	seq           int
	startedAt     time.Time
	channelID     int
	channelName   string
	channelType   int
	upstreamModel string
	calls         []channelMetricCallState
	usage         *ChannelMetricUsage
}

type channelMetricFailureDraft struct {
	attemptSeq      int
	createdAt       time.Time
	channelID       int
	channelName     string
	channelType     int
	requestedModel  string
	upstreamModel   string
	group           string
	trafficSource   channelmetrics.TrafficSource
	outcome         channelmetrics.Outcome
	failureOwner    channelmetrics.FailureOwner
	qualityEligible bool
	partialResponse bool
	errorStage      channelmetrics.ErrorStage
	streamEndReason string
	retryPlanned    bool
	retryReason     string
	latencyMs       int64
	ttftPresent     bool
	ttftMs          int64
	maskedError     string
	causalCall      *channelMetricCallState
	normalized      channelmetrics.StatusCode
}

type channelMetricRequestState struct {
	mu sync.Mutex

	requestID      string
	startedAt      time.Time
	requestedModel string
	group          string
	stream         bool
	trafficSource  channelmetrics.TrafficSource

	attemptSeq int
	current    *channelMetricAttemptState
	finished   bool

	finalChannelPresent bool
	finalChannelID      int
	finalChannelName    string
	finalChannelType    int
	finalUpstreamModel  string
	failures            []channelMetricFailureDraft
}

// BeginChannelMetricRequest 必须在错误响应 defer 之前调用，使最终状态码在写回后再采集。
func BeginChannelMetricRequest(c *gin.Context) {
	if c == nil || channelMetricCollector() == nil || !channelMetricsEffectiveSetting().Enabled {
		return
	}
	startedAt := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	state := &channelMetricRequestState{
		requestID:      common.GetContextKeyString(c, common.RequestIdKey),
		startedAt:      startedAt,
		requestedModel: common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		group:          common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		trafficSource:  channelmetrics.TrafficSourceRelay,
	}
	if state.requestID == "" {
		state.requestID = common.GetTimeString() + common.GetRandomString(8)
	}
	common.SetContextKey(c, constant.ContextKeyChannelMetricState, state)
}

// MarkChannelMetricProbeRequest 在 RelayInfo 生成前固定主动探测来源，避免本地失败污染真实转发统计。
func MarkChannelMetricProbeRequest(c *gin.Context) {
	setChannelMetricTrafficSource(c, channelmetrics.TrafficSourceProbe)
}

// MarkChannelMetricTaskRequest 在 RelayInfo 生成前固定异步任务来源。
func MarkChannelMetricTaskRequest(c *gin.Context) {
	setChannelMetricTrafficSource(c, channelmetrics.TrafficSourceTask)
}

// MarkChannelMetricPlaygroundRequest 在 RelayInfo 生成前固定后台调试来源。
func MarkChannelMetricPlaygroundRequest(c *gin.Context) {
	setChannelMetricTrafficSource(c, channelmetrics.TrafficSourcePlayground)
}

func setChannelMetricTrafficSource(c *gin.Context, source channelmetrics.TrafficSource) {
	state := getChannelMetricRequestState(c)
	if state == nil || !source.Valid() {
		return
	}
	state.mu.Lock()
	state.trafficSource = source
	state.mu.Unlock()
}

func BindChannelMetricRelayInfo(c *gin.Context, info *relaycommon.RelayInfo) {
	state := getChannelMetricRequestState(c)
	if state == nil || info == nil {
		return
	}
	state.mu.Lock()
	state.requestedModel = info.OriginModelName
	state.group = info.UsingGroup
	state.stream = info.IsStream
	state.trafficSource = channelMetricTrafficSource(info)
	state.mu.Unlock()
}

func BeginChannelMetricAttempt(c *gin.Context, info *relaycommon.RelayInfo, channelID int, channelName string, channelType int) {
	state := getChannelMetricRequestState(c)
	if state == nil || info == nil {
		return
	}
	now := time.Now()
	state.mu.Lock()
	state.attemptSeq++
	state.current = &channelMetricAttemptState{
		seq:           state.attemptSeq,
		startedAt:     now,
		channelID:     channelID,
		channelName:   channelName,
		channelType:   channelType,
		upstreamModel: info.OriginModelName,
	}
	state.finalChannelPresent = true
	state.finalChannelID = channelID
	state.finalChannelName = channelName
	state.finalChannelType = channelType
	state.mu.Unlock()
}

// BeginChannelMetricUpstreamCall 返回当前尝试内从 1 开始的调用序号；0 表示未启用采集。
func BeginChannelMetricUpstreamCall(c *gin.Context) int {
	state := getChannelMetricRequestState(c)
	if state == nil {
		return 0
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.current == nil {
		return 0
	}
	index := len(state.current.calls) + 1
	state.current.calls = append(state.current.calls, channelMetricCallState{index: index, startedAt: time.Now()})
	return index
}

// CompleteChannelMetricUpstreamHeader 只记录 transport/header 证据，业务结果在尝试结束后判定。
func CompleteChannelMetricUpstreamHeader(c *gin.Context, callIndex int, statusCode int, transportErr error) {
	if callIndex <= 0 {
		return
	}
	state := getChannelMetricRequestState(c)
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.current == nil || callIndex > len(state.current.calls) {
		return
	}
	call := &state.current.calls[callIndex-1]
	call.transportFail = transportErr != nil
	if statusCode >= 100 {
		call.headerAt = time.Now()
		call.status = channelmetrics.PresentStatus(statusCode)
		return
	}
	if transportErr != nil {
		call.status = channelmetrics.PresentStatus(0)
	}
}

func AttachChannelMetricUsage(c *gin.Context, usage ChannelMetricUsage) {
	state := getChannelMetricRequestState(c)
	if state == nil {
		return
	}
	usage = normalizeChannelMetricUsage(usage)
	state.mu.Lock()
	if state.current != nil {
		copyUsage := usage
		state.current.usage = &copyUsage
	}
	state.mu.Unlock()
}

// AttachChannelMetricUsageAfterSettlement 始终记录上游返回的用量，只有结算成功时
// 才把额度计入已收费额度，避免计费故障把 Token 和缓存数据一并丢失。
func AttachChannelMetricUsageAfterSettlement(c *gin.Context, usage ChannelMetricUsage, quota int, settleErr error) {
	if settleErr == nil {
		usage.ChargedQuota = int64(quota)
	}
	AttachChannelMetricUsage(c, usage)
}

func FinishChannelMetricAttempt(c *gin.Context, info *relaycommon.RelayInfo, relayErr *types.NewAPIError, retryPlanned bool, retryReason string) {
	state := getChannelMetricRequestState(c)
	if state == nil || info == nil {
		return
	}
	finishedAt := time.Now()

	state.mu.Lock()
	attempt := state.current
	if attempt == nil {
		state.mu.Unlock()
		return
	}
	state.current = nil
	attempt.upstreamModel = info.UpstreamModelName
	if attempt.upstreamModel == "" {
		attempt.upstreamModel = info.OriginModelName
	}
	state.finalUpstreamModel = attempt.upstreamModel
	outcome, owner, stage, eligible := classifyChannelMetricAttempt(c, info, relayErr, len(attempt.calls) > 0)
	partial := info.HasSendResponse() || info.SendResponseCount > 0 || info.ReceivedResponseCount > 0
	latency := nonNegativeMilliseconds(finishedAt.Sub(attempt.startedAt))
	ttft := info.FirstResponseLatencyMilliseconds()
	ttftPresent := ttft > 0
	streamEndReason := ""
	if info.StreamStatus != nil {
		streamEndReason = string(info.StreamStatus.EndReason)
	}
	requestedModel := state.requestedModel
	group := info.UsingGroup
	if group == "" {
		group = state.group
	}
	state.group = group
	trafficSource := state.trafficSource

	if outcome != channelmetrics.OutcomeSuccess {
		draft := channelMetricFailureDraft{
			attemptSeq: attempt.seq, createdAt: finishedAt,
			channelID: attempt.channelID, channelName: attempt.channelName, channelType: attempt.channelType,
			requestedModel: requestedModel, upstreamModel: attempt.upstreamModel, group: group,
			trafficSource: trafficSource, outcome: outcome, failureOwner: owner,
			qualityEligible: eligible, partialResponse: partial, errorStage: stage,
			streamEndReason: streamEndReason, retryPlanned: retryPlanned, retryReason: retryReason,
			latencyMs: latency, ttftPresent: ttftPresent, ttftMs: ttft,
		}
		if relayErr != nil {
			draft.maskedError = channelmetrics.TruncateUTF8(relayErr.MaskSensitiveErrorWithStatusCode(), 512)
			if relayErr.StatusCode >= 100 && relayErr.StatusCode <= 999 {
				draft.normalized = channelmetrics.PresentStatus(relayErr.StatusCode)
			}
		}
		if causalCall := selectCausalChannelMetricCall(attempt.calls, outcome, relayErr); causalCall != nil {
			causal := *causalCall
			draft.causalCall = &causal
		}
		state.failures = append(state.failures, draft)
	}
	state.mu.Unlock()

	attemptSample := channelmetrics.NewLiveSample(channelmetrics.ScopeChannelAttempt, outcome)
	attemptSample.OccurredAt = finishedAt
	attemptSample.RequestID = state.requestID
	attemptSample.AttemptSeq = attempt.seq
	attemptSample.RetryPlanned = retryPlanned
	applyChannelMetricAttemptDimensions(&attemptSample, attempt, requestedModel, group, trafficSource, info.IsStream)
	attemptSample.FailureOwner = owner
	attemptSample.QualityEligible = eligible
	attemptSample.PartialResponse = partial
	attemptSample.UpstreamStarted = len(attempt.calls) > 0
	attemptSample.ErrorStage = stage
	attemptSample.StreamEndReason = streamEndReason
	attemptSample.LatencyPresent = true
	attemptSample.LatencyMs = latency
	attemptSample.TTFTPresent = ttftPresent
	attemptSample.TTFTMs = ttft
	if outcome == channelmetrics.OutcomeSuccess && attempt.usage != nil {
		applyChannelMetricUsage(&attemptSample, *attempt.usage)
	}
	recordChannelMetric(attemptSample)
	recordChannelMetricCalls(state.requestID, attempt, requestedModel, group, trafficSource, info.IsStream, finishedAt, outcome, owner, stage, partial, relayErr)
}

func FinishChannelMetricRequest(c *gin.Context, info *relaycommon.RelayInfo, relayErr *types.NewAPIError) {
	state := getChannelMetricRequestState(c)
	if state == nil {
		return
	}
	finishedAt := time.Now()
	state.mu.Lock()
	if state.finished {
		state.mu.Unlock()
		return
	}
	state.finished = true
	requestedModel := state.requestedModel
	group := state.group
	stream := state.stream
	trafficSource := state.trafficSource
	requestID := state.requestID
	startedAt := state.startedAt
	attemptSeq := state.attemptSeq
	finalChannelPresent := state.finalChannelPresent
	finalChannelID := state.finalChannelID
	finalChannelName := state.finalChannelName
	finalChannelType := state.finalChannelType
	finalUpstreamModel := state.finalUpstreamModel
	failures := append([]channelMetricFailureDraft(nil), state.failures...)
	state.mu.Unlock()

	outcome, _, stage, _ := classifyChannelMetricAttempt(c, info, relayErr, attemptSeq > 0)
	status := 0
	if c != nil && c.Writer != nil {
		status = c.Writer.Status()
	}

	sample := channelmetrics.NewLiveSample(channelmetrics.ScopeFinalRequest, outcome)
	sample.OccurredAt = finishedAt
	sample.RequestID = requestID
	sample.RetryCount = channelMetricMaxInt(attemptSeq-1, 0)
	if attemptSeq > 0 {
		sample.LastStartedAttemptPresent = true
		sample.LastStartedAttemptSeq = attemptSeq
	}
	sample.RequestedModelPresent = requestedModel != ""
	sample.RequestedModel = requestedModel
	sample.UpstreamModelPresent = finalUpstreamModel != ""
	sample.UpstreamModel = finalUpstreamModel
	sample.Group = group
	sample.TrafficSource = trafficSource
	sample.Stream = stream
	sample.PartialResponse = info != nil && (info.HasSendResponse() || info.SendResponseCount > 0 || info.ReceivedResponseCount > 0)
	sample.ChannelPresent = finalChannelPresent
	sample.ChannelID = finalChannelID
	sample.ChannelNameSnapshot = finalChannelName
	sample.ChannelType = finalChannelType
	if status >= 100 && status <= 999 {
		sample.ClientStatus = channelmetrics.PresentStatus(status)
	}
	sample.ErrorStage = stage
	sample.LatencyPresent = true
	sample.LatencyMs = nonNegativeMilliseconds(finishedAt.Sub(startedAt))
	recordChannelMetric(sample)

	if len(failures) > 0 {
		enqueueChannelFailureEvents(buildChannelFailureEvents(failures, requestID, attemptSeq, status))
	}
}

func getChannelMetricRequestState(c *gin.Context) *channelMetricRequestState {
	if c == nil {
		return nil
	}
	state, ok := common.GetContextKeyType[*channelMetricRequestState](c, constant.ContextKeyChannelMetricState)
	if !ok {
		return nil
	}
	return state
}

func channelMetricTrafficSource(info *relaycommon.RelayInfo) channelmetrics.TrafficSource {
	if info == nil {
		return channelmetrics.TrafficSourceRelay
	}
	if info.IsChannelTest {
		return channelmetrics.TrafficSourceProbe
	}
	if info.IsPlayground {
		return channelmetrics.TrafficSourcePlayground
	}
	// RelayFormatTask / RelayFormatMjProxy 目前都通过 TaskRelayInfo 表示任务型请求。
	// 部分任务入口由 genBaseRelayInfo 构造，RelayFormat 可能尚未回填，不能因此
	// 将真实任务流量误归类为普通 relay。
	if info.RelayFormat == types.RelayFormatTask || info.TaskRelayInfo != nil {
		return channelmetrics.TrafficSourceTask
	}
	return channelmetrics.TrafficSourceRelay
}

func classifyChannelMetricAttempt(c *gin.Context, info *relaycommon.RelayInfo, relayErr *types.NewAPIError, upstreamStarted bool) (channelmetrics.Outcome, channelmetrics.FailureOwner, channelmetrics.ErrorStage, bool) {
	if requestWasCancelled(c) || (info != nil && info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone) {
		return channelmetrics.OutcomeClientCancelled, channelmetrics.FailureOwnerClient, channelmetrics.ErrorStageStream, false
	}
	if info != nil && info.IsStream && info.StreamStatus != nil && (!info.StreamStatus.IsNormalEnd() || info.StreamStatus.HasErrors()) {
		return channelmetrics.OutcomeStreamError, channelmetrics.FailureOwnerChannel, channelmetrics.ErrorStageStream, upstreamStarted
	}
	if relayErr == nil {
		return channelmetrics.OutcomeSuccess, channelmetrics.FailureOwnerNone, channelmetrics.ErrorStageNone, upstreamStarted
	}

	code := relayErr.GetErrorCode()
	status := relayErr.StatusCode
	switch code {
	case types.ErrorCodeDoRequestFailed, types.ErrorCodeChannelResponseTimeExceeded:
		return channelmetrics.OutcomeTransportError, channelmetrics.FailureOwnerChannel, channelmetrics.ErrorStageConnect, upstreamStarted
	case types.ErrorCodeReadResponseBodyFailed, types.ErrorCodeBadResponse, types.ErrorCodeBadResponseBody, types.ErrorCodeEmptyResponse:
		return channelmetrics.OutcomeProtocolError, channelmetrics.FailureOwnerChannel, channelmetrics.ErrorStageParse, upstreamStarted
	case types.ErrorCodeReadRequestBodyFailed, types.ErrorCodeConvertRequestFailed, types.ErrorCodeJsonMarshalFailed, types.ErrorCodeBadRequestBody:
		return channelmetrics.OutcomeLocalError, channelmetrics.FailureOwnerGateway, channelmetrics.ErrorStagePreUpstream, false
	case types.ErrorCodeInvalidRequest, types.ErrorCodePromptBlocked:
		return channelmetrics.OutcomeLocalError, channelmetrics.FailureOwnerClient, channelmetrics.ErrorStagePreUpstream, false
	case types.ErrorCodeGetChannelFailed:
		return channelmetrics.OutcomeDispatchError, channelmetrics.FailureOwnerGateway, channelmetrics.ErrorStageChannelSelect, false
	}
	if status >= 400 && status <= 599 && upstreamStarted {
		owner := channelmetrics.FailureOwnerChannel
		eligible := true
		if status == http.StatusBadRequest || status == http.StatusNotFound || status == http.StatusUnprocessableEntity {
			owner = channelmetrics.FailureOwnerClient
			eligible = false
		}
		return channelmetrics.OutcomeHTTPError, owner, channelmetrics.ErrorStageUpstream, eligible
	}
	return channelmetrics.OutcomeLocalError, channelmetrics.FailureOwnerUnknown, channelmetrics.ErrorStagePreUpstream, false
}

func recordChannelMetricCalls(requestID string, attempt *channelMetricAttemptState, requestedModel string, group string, source channelmetrics.TrafficSource, stream bool, finishedAt time.Time, attemptOutcome channelmetrics.Outcome, owner channelmetrics.FailureOwner, stage channelmetrics.ErrorStage, partial bool, relayErr *types.NewAPIError) {
	for index := range attempt.calls {
		call := attempt.calls[index]
		outcome := channelmetrics.OutcomeSuccess
		callOwner := channelmetrics.FailureOwnerNone
		callStage := channelmetrics.ErrorStageNone
		if !call.status.Present {
			outcome = channelmetrics.OutcomeTransportError
			callOwner = channelmetrics.FailureOwnerUnknown
			callStage = channelmetrics.ErrorStageUnfinalizedCall
		} else if call.transportFail || call.status.NoHTTPResponse() {
			outcome = channelmetrics.OutcomeTransportError
			callOwner = channelmetrics.FailureOwnerChannel
			callStage = channelmetrics.ErrorStageConnect
		} else if call.status.Present && call.status.Code >= 400 {
			outcome = channelmetrics.OutcomeHTTPError
			callOwner = channelmetrics.FailureOwnerChannel
			callStage = channelmetrics.ErrorStageUpstream
		}
		isLast := index == len(attempt.calls)-1
		if isLast && outcome == channelmetrics.OutcomeSuccess && channelMetricCallInheritsAttemptOutcome(attemptOutcome) {
			outcome, callOwner, callStage = attemptOutcome, owner, stage
		}
		end := call.headerAt
		if isLast || end.IsZero() {
			end = finishedAt
		}
		sample := channelmetrics.NewLiveSample(channelmetrics.ScopeUpstreamCall, outcome)
		sample.OccurredAt = finishedAt
		sample.RequestID = requestID
		sample.AttemptSeq = attempt.seq
		sample.CallIndex = call.index
		applyChannelMetricAttemptDimensions(&sample, attempt, requestedModel, group, source, stream)
		sample.FailureOwner = callOwner
		sample.PartialResponse = isLast && partial
		sample.ErrorStage = callStage
		sample.UpstreamStarted = true
		sample.UpstreamStatus = call.status
		if isLast && relayErr != nil && relayErr.StatusCode >= 100 && relayErr.StatusCode <= 999 {
			sample.NormalizedStatus = channelmetrics.PresentStatus(relayErr.StatusCode)
		}
		if !call.headerAt.IsZero() {
			sample.ResponseHeaderPresent = true
			sample.ResponseHeaderMs = nonNegativeMilliseconds(call.headerAt.Sub(call.startedAt))
		}
		sample.LatencyPresent = true
		sample.LatencyMs = nonNegativeMilliseconds(end.Sub(call.startedAt))
		recordChannelMetric(sample)
	}
}

func channelMetricCallInheritsAttemptOutcome(outcome channelmetrics.Outcome) bool {
	switch outcome {
	case channelmetrics.OutcomeHTTPError,
		channelmetrics.OutcomeTransportError,
		channelmetrics.OutcomeProtocolError,
		channelmetrics.OutcomeStreamError,
		channelmetrics.OutcomeClientCancelled:
		return true
	default:
		// 选渠、请求转换和其他网关本地失败不应污染已经正常完成的上游调用。
		return false
	}
}

// selectCausalChannelMetricCall 只在证据足够时返回根因调用。
// 多调用尝试不能简单把最后一次调用当作根因，否则 continuation/fallback
// 等正常二次请求会被错误归因。
func selectCausalChannelMetricCall(calls []channelMetricCallState, outcome channelmetrics.Outcome, relayErr *types.NewAPIError) *channelMetricCallState {
	if len(calls) == 0 {
		return nil
	}
	if len(calls) == 1 {
		switch outcome {
		case channelmetrics.OutcomeHTTPError,
			channelmetrics.OutcomeTransportError,
			channelmetrics.OutcomeProtocolError,
			channelmetrics.OutcomeStreamError:
			return &calls[0]
		default:
			return nil
		}
	}

	matching := make([]int, 0, 1)
	for index := range calls {
		call := calls[index]
		switch outcome {
		case channelmetrics.OutcomeTransportError:
			if call.transportFail || call.status.NoHTTPResponse() {
				matching = append(matching, index)
			}
		case channelmetrics.OutcomeHTTPError:
			if !call.status.Present {
				continue
			}
			if relayErr != nil && relayErr.StatusCode >= 400 && relayErr.StatusCode <= 599 {
				if call.status.Code == relayErr.StatusCode {
					matching = append(matching, index)
				}
			} else if call.status.Code >= 400 && call.status.Code <= 599 {
				matching = append(matching, index)
			}
		}
	}
	if len(matching) != 1 {
		return nil
	}
	return &calls[matching[0]]
}

func applyChannelMetricAttemptDimensions(sample *channelmetrics.Sample, attempt *channelMetricAttemptState, requestedModel string, group string, source channelmetrics.TrafficSource, stream bool) {
	sample.ChannelPresent = true
	sample.ChannelID = attempt.channelID
	sample.ChannelNameSnapshot = attempt.channelName
	sample.ChannelType = attempt.channelType
	sample.RequestedModelPresent = requestedModel != ""
	sample.RequestedModel = requestedModel
	sample.UpstreamModelPresent = attempt.upstreamModel != ""
	sample.UpstreamModel = attempt.upstreamModel
	sample.Group = group
	sample.TrafficSource = source
	sample.Stream = stream
}

func applyChannelMetricUsage(sample *channelmetrics.Sample, usage ChannelMetricUsage) {
	sample.UsagePresent = true
	sample.InputTokensTotal = usage.InputTokensTotal
	sample.UncachedInputTokens = usage.UncachedInputTokens
	sample.OutputTokens = usage.OutputTokens
	sample.CacheReadTokens = usage.CacheReadTokens
	sample.CacheWriteTokens = usage.CacheWriteTokens
	sample.ChargedQuota = usage.ChargedQuota
	if common.QuotaPerUnit > 0 {
		sample.ChargedMicroUSD = int64(float64(usage.ChargedQuota)/common.QuotaPerUnit*1_000_000 + 0.5)
	}
}

func normalizeChannelMetricUsage(usage ChannelMetricUsage) ChannelMetricUsage {
	usage.InputTokensTotal = channelMetricMaxInt64(usage.InputTokensTotal, 0)
	usage.CacheReadTokens = channelMetricMaxInt64(usage.CacheReadTokens, 0)
	usage.CacheWriteTokens = channelMetricMaxInt64(usage.CacheWriteTokens, 0)
	usage.OutputTokens = channelMetricMaxInt64(usage.OutputTokens, 0)
	usage.ChargedQuota = channelMetricMaxInt64(usage.ChargedQuota, 0)
	if usage.InputTokensTotal < usage.CacheReadTokens+usage.CacheWriteTokens {
		usage.InputTokensTotal = usage.CacheReadTokens + usage.CacheWriteTokens
	}
	usage.UncachedInputTokens = usage.InputTokensTotal - usage.CacheReadTokens - usage.CacheWriteTokens
	return usage
}

func buildChannelFailureEvents(drafts []channelMetricFailureDraft, requestID string, lastAttemptSeq int, clientStatus int) []model.ChannelFailureEvent {
	events := make([]model.ChannelFailureEvent, 0, len(drafts))
	for _, draft := range drafts {
		isLast := draft.attemptSeq == lastAttemptSeq
		event := model.ChannelFailureEvent{
			EventId:   channelmetrics.SHA256String(fmt.Sprintf("v1:%s:%d:%d:%s", requestID, draft.attemptSeq, draft.channelID, draft.outcome)),
			CreatedAt: draft.createdAt.Unix(), RequestId: channelmetrics.TruncateUTF8(requestID, 128), AttemptSeq: draft.attemptSeq,
			RetryPlanned: draft.retryPlanned, IsLastStartedAttempt: isLast,
			ChannelId: draft.channelID, ChannelNameSnapshot: channelmetrics.TruncateUTF8(draft.channelName, 191), ChannelType: draft.channelType,
			RequestedModel: channelmetrics.TruncateUTF8(draft.requestedModel, 191), RequestedModelHash: channelMetricOptionalHash(draft.requestedModel),
			UpstreamModel: channelmetrics.TruncateUTF8(draft.upstreamModel, 191), UpstreamModelHash: channelMetricOptionalHash(draft.upstreamModel),
			Group:         channelmetrics.TruncateUTF8(draft.group, 64),
			TrafficSource: string(draft.trafficSource), DataOrigin: string(channelmetrics.DataOriginLive), Outcome: string(draft.outcome),
			FailureOwner: string(draft.failureOwner), QualityEligible: draft.qualityEligible,
			PartialResponse: draft.partialResponse, ErrorStage: channelmetrics.TruncateUTF8(string(draft.errorStage), 32),
			StreamEndReason: channelmetrics.TruncateUTF8(draft.streamEndReason, 64), LatencyMs: draft.latencyMs,
			TtftPresent: draft.ttftPresent, TtftMs: draft.ttftMs,
			RetryReason: channelmetrics.TruncateUTF8(draft.retryReason, 128), MaskedErrorSummary: channelmetrics.TruncateUTF8(draft.maskedError, 512),
			NormalizedStatusPresent: draft.normalized.Present, NormalizedStatusCode: draft.normalized.Code,
		}
		if draft.causalCall != nil {
			event.CausalCallPresent = true
			event.CausalCallIndex = draft.causalCall.index
			event.UpstreamStatusPresent = draft.causalCall.status.Present
			event.UpstreamStatusCode = draft.causalCall.status.Code
		}
		if isLast && clientStatus >= 100 && clientStatus <= 999 {
			event.ClientStatusPresent = true
			event.ClientStatusCode = clientStatus
		}
		events = append(events, event)
	}
	return events
}

func channelMetricOptionalHash(value string) string {
	if value == "" {
		return ""
	}
	return channelmetrics.SHA256String(value)
}

func requestWasCancelled(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	err := c.Request.Context().Err()
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func nonNegativeMilliseconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return duration.Milliseconds()
}

func channelMetricMaxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func channelMetricMaxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
