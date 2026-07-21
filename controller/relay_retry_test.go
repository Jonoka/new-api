package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
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

func TestShouldRetryWithReasonReportsBlockingReason(t *testing.T) {
	err := types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)

	t.Run("no remaining retries", func(t *testing.T) {
		decision := shouldRetryWithReason(buildRelayRetryTestContext(), err, 0)
		require.False(t, decision.Retry)
		require.Equal(t, "retry_exhausted", decision.Reason)
	})

	t.Run("channel error with no remaining retries", func(t *testing.T) {
		channelErr := types.NewError(errors.New("channel unavailable"), types.ErrorCode("channel:unavailable"))
		decision := shouldRetryWithReason(buildRelayRetryTestContext(), channelErr, 0)
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

	t.Run("specific channel with channel error", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		ctx.Set("specific_channel_id", "13")
		channelErr := types.NewError(errors.New("channel unavailable"), types.ErrorCode("channel:unavailable"))
		decision := shouldRetryWithReason(ctx, channelErr, 2)
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

	t.Run("stream already started", func(t *testing.T) {
		ctx := buildRelayRetryTestContext()
		info := &relaycommon.RelayInfo{}
		info.ResetFirstResponseTiming(time.Now())
		info.SetFirstResponseTime()
		info.ReceivedResponseCount = 1
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)
		decision := shouldRetryWithReason(ctx, err, 2)
		require.False(t, decision.Retry)
		require.Equal(t, "no_retry_after_stream_started", decision.Reason)
	})
}

func TestRepeatedChannelRetryDelay(t *testing.T) {
	t.Run("skip different channel", func(t *testing.T) {
		err := &types.NewAPIError{StatusCode: http.StatusTooManyRequests}
		require.Zero(t, repeatedChannelRetryDelay(err, 1, false))
	})

	t.Run("skip non rate limit error", func(t *testing.T) {
		err := &types.NewAPIError{StatusCode: http.StatusBadGateway}
		require.Zero(t, repeatedChannelRetryDelay(err, 1, true))
	})

	t.Run("use exponential fallback", func(t *testing.T) {
		err := &types.NewAPIError{StatusCode: http.StatusTooManyRequests}
		require.Equal(t, 500*time.Millisecond, repeatedChannelRetryDelay(err, 1, true))
		require.Equal(t, time.Second, repeatedChannelRetryDelay(err, 2, true))
		require.Equal(t, 2*time.Second, repeatedChannelRetryDelay(err, 3, true))
	})

	t.Run("prefer bounded retry after", func(t *testing.T) {
		err := &types.NewAPIError{StatusCode: http.StatusTooManyRequests, RetryAfter: 30 * time.Second}
		require.Equal(t, 10*time.Second, repeatedChannelRetryDelay(err, 1, true))
	})

	t.Run("recognize rate limit before status mapping", func(t *testing.T) {
		err := &types.NewAPIError{
			StatusCode:         http.StatusServiceUnavailable,
			OriginalStatusCode: http.StatusTooManyRequests,
		}
		require.Equal(t, repeatedChannelRetryBaseDelay, repeatedChannelRetryDelay(err, 1, true))
	})
}

func TestChannelRetryStateIsIsolatedByChannel(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	states := make(map[int]channelRetryState)

	recordChannelRetryState(states, 342, &types.NewAPIError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 10 * time.Second,
	}, now)
	recordChannelRetryState(states, 351, &types.NewAPIError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: time.Second,
	}, now)

	require.Equal(t, 8*time.Second, channelRetryDelay(states, 342, now.Add(2*time.Second)))
	require.Zero(t, channelRetryDelay(states, 351, now.Add(2*time.Second)))
}

func TestChannelRetryStateSurvivesOtherChannelFailure(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	states := make(map[int]channelRetryState)

	recordChannelRetryState(states, 342, &types.NewAPIError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 10 * time.Second,
	}, now)
	recordChannelRetryState(states, 351, &types.NewAPIError{StatusCode: http.StatusBadGateway}, now)

	require.Equal(t, 9*time.Second, channelRetryDelay(states, 342, now.Add(time.Second)))
	require.Zero(t, channelRetryDelay(states, 351, now.Add(time.Second)))
}

func TestChannelRetryStateUsesPerChannelBackoff(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	states := make(map[int]channelRetryState)
	rateLimitErr := &types.NewAPIError{StatusCode: http.StatusTooManyRequests}

	recordChannelRetryState(states, 342, rateLimitErr, now)
	require.Equal(t, 500*time.Millisecond, channelRetryDelay(states, 342, now))

	recordChannelRetryState(states, 342, rateLimitErr, now.Add(time.Second))
	require.Equal(t, time.Second, channelRetryDelay(states, 342, now.Add(time.Second)))
}

func TestWaitForRelayRetryStopsOnCanceledRequest(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := buildRelayRetryTestContext()
	ctx.Request = ctx.Request.WithContext(requestContext)

	startedAt := time.Now()
	require.False(t, waitForRelayRetry(ctx, time.Second))
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
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

func TestTaskErrorToChannelMetricErrorPreservesLocalClassification(t *testing.T) {
	upstream := taskErrorToChannelMetricError(&dto.TaskError{
		Error: errors.New("upstream failed"), StatusCode: http.StatusBadGateway,
	})
	require.Equal(t, types.ErrorCodeBadResponseStatusCode, upstream.GetErrorCode())
	require.Equal(t, http.StatusBadGateway, upstream.StatusCode)

	local := taskErrorToChannelMetricError(&dto.TaskError{
		Error: errors.New("convert failed"), StatusCode: http.StatusBadRequest, LocalError: true,
	})
	require.Equal(t, types.ErrorCodeConvertRequestFailed, local.GetErrorCode())
	require.Equal(t, http.StatusBadRequest, local.StatusCode)
}
