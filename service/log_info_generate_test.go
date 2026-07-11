package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func buildLogInfoContextForTest() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx
}

func TestGenerateTextOtherInfoCompatibilityRoutingAudit(t *testing.T) {
	ctx := buildLogInfoContextForTest()
	ctx.Set("compatibility_requested_model", "gpt-image-2")
	ctx.Set("compatibility_routed_model", "gpt-image-2-lite")
	info := &relaycommon.RelayInfo{
		StartTime:   time.Now(),
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	other := GenerateTextOtherInfo(ctx, info, 0, 0.4, 0, 0, 0, 0.1, -1)

	require.Equal(t, "gpt-image-2", other["compatibility_requested_model"])
	require.Equal(t, "gpt-image-2-lite", other["compatibility_routed_model"])
}

func TestGenerateTextOtherInfoTimingDiagnosticsDisabled(t *testing.T) {
	oldEnabled := constant.UpstreamTimingDiagnosticsEnabled
	constant.UpstreamTimingDiagnosticsEnabled = false
	t.Cleanup(func() { constant.UpstreamTimingDiagnosticsEnabled = oldEnabled })

	start := time.Now().Add(-100 * time.Millisecond)
	info := &relaycommon.RelayInfo{
		StartTime:   start,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	info.EnableTimingDiagnostics(start.Add(10 * time.Millisecond))
	info.MarkTimingClientDoReturn()

	other := GenerateTextOtherInfo(buildLogInfoContextForTest(), info, 1, 1, 1, 0, 0, 0, 1)

	require.NotContains(t, other, "timing_diagnostics")
}

func TestGenerateTextOtherInfoTimingDiagnosticsEnabled(t *testing.T) {
	oldEnabled := constant.UpstreamTimingDiagnosticsEnabled
	constant.UpstreamTimingDiagnosticsEnabled = true
	t.Cleanup(func() { constant.UpstreamTimingDiagnosticsEnabled = oldEnabled })

	start := time.Now().Add(-100 * time.Millisecond)
	info := &relaycommon.RelayInfo{
		StartTime:   start,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	info.EnableTimingDiagnostics(start.Add(10 * time.Millisecond))
	info.MarkTimingClientDoReturn()

	other := GenerateTextOtherInfo(buildLogInfoContextForTest(), info, 1, 1, 1, 0, 0, 0, 1)

	diagnostics, ok := other["timing_diagnostics"].(map[string]interface{})
	require.True(t, ok)
	require.Contains(t, diagnostics, "before_do_request_ms")
	require.Contains(t, diagnostics, "client_do_return_ms")
	require.Contains(t, diagnostics, "total_ms")
}
