package common

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestGenRelayInfoCanvasProxySkipsTokenQuotaWithoutPlaygroundFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/canvas/v1/chat/completions?group=vip", nil)

	info := GenRelayInfoOpenAI(ctx, nil)

	require.False(t, info.IsPlayground)
	require.True(t, info.SkipTokenQuota)
	require.Equal(t, "/v1/chat/completions?group=vip", info.RequestURLPath)
}

func TestRelayInfoTimingDiagnosticsMilliseconds(t *testing.T) {
	start := time.Now().Add(-200 * time.Millisecond)
	upstreamStart := start.Add(25 * time.Millisecond)
	info := &RelayInfo{
		StartTime:              start,
		FirstResponseStartTime: upstreamStart,
		FirstResponseTime:      upstreamStart.Add(30 * time.Millisecond),
	}
	info.EnableTimingDiagnostics(upstreamStart)
	info.TimingDiagnostics.gotConn = upstreamStart.Add(5 * time.Millisecond)
	info.TimingDiagnostics.wroteRequest = upstreamStart.Add(10 * time.Millisecond)
	info.TimingDiagnostics.gotFirstResponseByte = upstreamStart.Add(20 * time.Millisecond)
	info.TimingDiagnostics.clientDoReturn = upstreamStart.Add(25 * time.Millisecond)
	info.TimingDiagnostics.firstSSEData = upstreamStart.Add(30 * time.Millisecond)
	info.TimingDiagnostics.firstDownstreamWrite = upstreamStart.Add(35 * time.Millisecond)

	diagnostics := info.TimingDiagnosticsMilliseconds()

	require.Equal(t, float64(25), diagnostics["before_do_request_ms"])
	require.Equal(t, float64(5), diagnostics["got_conn_ms"])
	require.Equal(t, float64(10), diagnostics["wrote_request_ms"])
	require.Equal(t, float64(20), diagnostics["got_first_response_byte_ms"])
	require.Equal(t, float64(25), diagnostics["client_do_return_ms"])
	require.Equal(t, float64(30), diagnostics["first_sse_data_ms"])
	require.Equal(t, float64(35), diagnostics["first_downstream_write_ms"])
	require.Contains(t, diagnostics, "total_ms")
}

func TestRelayInfoTimingDiagnosticsFirstSSEDataRecordsOnce(t *testing.T) {
	info := &RelayInfo{}
	info.EnableTimingDiagnostics(time.Now().Add(-10 * time.Millisecond))

	info.MarkTimingFirstSSEData()
	first := info.TimingDiagnostics.firstSSEData
	require.False(t, first.IsZero())

	time.Sleep(time.Millisecond)
	info.MarkTimingFirstSSEData()

	require.Equal(t, first, info.TimingDiagnostics.firstSSEData)
}

func TestRelayInfoResetAttemptStateClearsRetryScopedState(t *testing.T) {
	info := &RelayInfo{
		IsStream:              true,
		SendResponseCount:     3,
		ReceivedResponseCount: 4,
		StreamStatus:          NewStreamStatus(),
	}
	info.StreamStatus.RecordError("previous attempt")
	info.EnableTimingDiagnostics(time.Now().Add(-time.Second))

	start := time.Now()
	info.ResetAttemptState(start)

	require.Equal(t, 0, info.SendResponseCount)
	require.Equal(t, 0, info.ReceivedResponseCount)
	require.Nil(t, info.TimingDiagnostics)
	require.NotNil(t, info.StreamStatus)
	require.False(t, info.StreamStatus.HasErrors())
	require.Equal(t, start, info.FirstResponseStartTime)
	require.False(t, info.HasSendResponse())
}
