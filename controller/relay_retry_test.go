package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func buildRelayRetryTestContext() *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return ctx
}

func TestShouldRetryWithReasonRetriesTransportFailure(t *testing.T) {
	ctx := buildRelayRetryTestContext()
	err := types.NewOpenAIError(
		errors.New("dial tcp 127.0.0.1:9: connect: connection refused"),
		types.ErrorCodeDoRequestFailed,
		http.StatusInternalServerError,
	)

	decision := shouldRetryWithReason(ctx, err, 2)

	require.True(t, decision.Retry)
	require.Equal(t, "transport_error_retry", decision.Reason)
}

func TestShouldRetryWithReasonRetriesConfiguredBadRequest(t *testing.T) {
	originalRanges := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: 400, End: 400},
	}
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = originalRanges
	})

	err := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)
	decision := shouldRetryWithReason(buildRelayRetryTestContext(), err, 2)

	require.True(t, decision.Retry)
	require.Equal(t, "status_code_retry", decision.Reason)
}

func TestShouldRetryWithReasonFastForwardsAfterAuthUnavailable(t *testing.T) {
	t.Run("multi group token uses original token group after distributor selects first group", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set(string(constant.ContextKeyUsingGroup), "svip")
		ctx.Set(string(constant.ContextKeyAutoGroup), "svip")
		ctx.Set("relay_info", &relaycommon.RelayInfo{TokenGroup: "svip,Codex-Plus,Codex-Pro"})
		err := types.NewOpenAIError(
			errors.New("auth_unavailable: no auth available (providers=codex, model=gpt-5.6-sol)"),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusServiceUnavailable,
		)

		decision := shouldRetryWithReason(ctx, err, 2)

		require.True(t, decision.Retry)
		require.Equal(t, "auth_unavailable_same_group_fallback", decision.Reason)
		require.Equal(t, 0, ctx.GetInt("auto_group_index"))
	})

	t.Run("auto group retries auth error even when non-stream timing is marked", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set(string(constant.ContextKeyUsingGroup), "auto")
		info := &relaycommon.RelayInfo{}
		info.ResetFirstResponseTiming(time.Now())
		info.SetFirstResponseTime()
		ctx.Set("relay_info", info)
		err := types.NewOpenAIError(
			errors.New("auth_unavailable: no auth available (providers=codex, model=gpt-5.4)"),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusServiceUnavailable,
		)

		decision := shouldRetryWithReason(ctx, err, 2)

		require.True(t, decision.Retry)
		require.Equal(t, "auth_unavailable_same_group_fallback", decision.Reason)
		require.Equal(t, 0, ctx.GetInt("auto_group_index"))

		secondDecision := shouldRetryWithReason(ctx, err, 2)
		require.True(t, secondDecision.Retry)
		require.Equal(t, "auth_unavailable_fast_group_fallback", secondDecision.Reason)
		require.Equal(t, 1, ctx.GetInt("auto_group_index"))
	})
}

func TestShouldRetryWithReasonReportsBlockingReason(t *testing.T) {
	err := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)

	t.Run("no remaining retries", func(t *testing.T) {
		decision := shouldRetryWithReason(buildRelayRetryTestContext(), err, 0)
		require.False(t, decision.Retry)
		require.Equal(t, "retry_exhausted", decision.Reason)
	})

	t.Run("specific channel", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("specific_channel_id", "13")
		decision := shouldRetryWithReason(ctx, err, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "specific_channel", decision.Reason)
	})

	t.Run("channel affinity skip", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("channel_affinity_skip_retry_on_failure", true)
		decision := shouldRetryWithReason(ctx, err, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "channel_affinity_skip", decision.Reason)
	})

	t.Run("upstream timing and response header without client body do not block retry", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		info := &relaycommon.RelayInfo{}
		info.ResetFirstResponseTiming(time.Now())
		info.SetFirstResponseTime()
		ctx.Set("relay_info", info)
		ctx.Writer.WriteHeader(http.StatusOK)
		ctx.Writer.WriteHeaderNow()
		err := types.NewOpenAIError(
			errors.New("Service temporarily unavailable"),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusServiceUnavailable,
		)

		decision := shouldRetryWithReason(ctx, err, 2)

		require.True(t, decision.Retry)
		require.Equal(t, "status_code_retry", decision.Reason)
	})

	t.Run("stream already started", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		info := &relaycommon.RelayInfo{}
		info.ResetFirstResponseTiming(time.Now())
		info.SetFirstResponseTime()
		info.ReceivedResponseCount = 1
		ctx.Set("relay_info", info)
		decision := shouldRetryWithReason(ctx, err, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "no_retry_after_stream_started", decision.Reason)
	})
}

func TestShouldMarkLitePoolExhausted(t *testing.T) {
	ctx := buildRelayRetryTestContext()
	ctx.Set("use_channel", []string{"33", "14"})
	relayInfo := &relaycommon.RelayInfo{OriginModelName: "gpt-image-2-lite"}

	require.True(t, shouldMarkLitePoolExhausted(ctx, relayInfo, types.NewOpenAIError(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway), true))
	require.False(t, shouldMarkLitePoolExhausted(ctx, relayInfo, types.NewOpenAIError(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway), false))
	require.False(t, shouldMarkLitePoolExhausted(ctx, &relaycommon.RelayInfo{OriginModelName: "gpt-image-2-pro"}, types.NewOpenAIError(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway), true))
	require.False(t, shouldMarkLitePoolExhausted(ctx, relayInfo, types.NewOpenAIError(errors.New("invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest), true))
	require.False(t, shouldMarkLitePoolExhausted(ctx, relayInfo, types.NewOpenAIError(errors.New("quota"), types.ErrorCodeInsufficientUserQuota, http.StatusServiceUnavailable), true))

	wrapped := markLitePoolExhausted(types.NewOpenAIError(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway))
	require.Equal(t, types.ErrorCodeLitePoolExhausted, wrapped.GetErrorCode())
	require.Equal(t, string(types.ErrorCodeLitePoolExhausted), fmt.Sprint(wrapped.ToOpenAIError().Code))
}

func TestAppendErrorLogRequestConversion(t *testing.T) {
	other := map[string]interface{}{}
	appendErrorLogRequestConversion(&relaycommon.RelayInfo{
		RequestConversionChain: []types.RelayFormat{
			types.RelayFormatClaude,
			types.RelayFormatOpenAI,
		},
	}, other)

	require.Equal(t, []string{"Claude Messages", "OpenAI Compatible"}, other["request_conversion"])
}
