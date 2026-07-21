package channelmetrics

import (
	"errors"
	"testing"
)

func TestSampleStatusPresenceSemantics(t *testing.T) {
	upstream := NewLiveSample(ScopeUpstreamCall, OutcomeTransportError)
	upstream.AttemptSeq = 1
	upstream.ChannelPresent = true
	upstream.ChannelID = 1
	upstream.UpstreamStatus = PresentStatus(0)
	if err := upstream.Validate(); err != nil {
		t.Fatalf("无 HTTP 响应的 upstream_call 应合法：%v", err)
	}
	dimension, err := DimensionFromSample(upstream, DefaultSnapshotLimits())
	if err != nil {
		t.Fatalf("生成无响应维度失败：%v", err)
	}
	if !dimension.UpstreamStatus.Present || dimension.UpstreamStatus.Code != 0 || !dimension.UpstreamStatus.NoHTTPResponse() {
		t.Fatalf("无响应状态语义丢失：%+v", dimension.UpstreamStatus)
	}

	unknown := upstream
	unknown.UpstreamStatus = StatusCode{Present: false, Code: 503}
	dimension, err = DimensionFromSample(unknown, DefaultSnapshotLimits())
	if err != nil {
		t.Fatalf("生成未知状态维度失败：%v", err)
	}
	if dimension.UpstreamStatus.Present || dimension.UpstreamStatus.Code != 0 {
		t.Fatalf("不适用状态码必须归零：%+v", dimension.UpstreamStatus)
	}

	successWithoutResponse := upstream
	successWithoutResponse.Outcome = OutcomeSuccess
	if err := successWithoutResponse.Validate(); !errors.Is(err, ErrInvalidSample) {
		t.Fatalf("成功调用使用状态码 0 应失败，实际：%v", err)
	}
}

func TestSampleRejectsCrossScopeStatusAndUsage(t *testing.T) {
	attempt := NewLiveSample(ScopeChannelAttempt, OutcomeSuccess)
	attempt.AttemptSeq = 1
	attempt.ChannelPresent = true
	attempt.ChannelID = 1
	attempt.UpstreamStatus = PresentStatus(200)
	if err := attempt.Validate(); !errors.Is(err, ErrInvalidSample) {
		t.Fatalf("channel_attempt 携带完整上游状态码应失败：%v", err)
	}

	finalRequest := NewLiveSample(ScopeFinalRequest, OutcomeSuccess)
	finalRequest.UsagePresent = true
	finalRequest.InputTokensTotal = 10
	if err := finalRequest.Validate(); !errors.Is(err, ErrInvalidSample) {
		t.Fatalf("final_request 携带结算用量应失败：%v", err)
	}

	cancelled := NewLiveSample(ScopeFinalRequest, OutcomeClientCancelled)
	cancelled.QualityEligible = true
	if err := cancelled.Validate(); !errors.Is(err, ErrInvalidSample) {
		t.Fatalf("客户端取消进入质量分母应失败：%v", err)
	}
}
