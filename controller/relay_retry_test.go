package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
}
