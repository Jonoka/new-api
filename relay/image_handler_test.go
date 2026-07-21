package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPrepareImagePassthroughBodyPreservesPayloadSize(t *testing.T) {
	payload := []byte("multipart-image-payload")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(payload))
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{}

	reader, err := prepareImagePassthroughBody(c, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)

	require.Equal(t, payload, forwarded)
	require.Equal(t, int64(len(payload)), info.UpstreamRequestBodySize)
}

func TestResolveImageSettlementCount(t *testing.T) {
	tests := []struct {
		name      string
		requested uint
		ratios    map[string]float64
		settled   uint
		delivered uint
	}{
		{name: "无实际数量时使用请求数量", requested: 4, settled: 4, delivered: 4},
		{name: "少交付时按实际数量结算", requested: 4, ratios: map[string]float64{"n": 1}, settled: 1, delivered: 1},
		{name: "多交付时按请求数量封顶", requested: 2, ratios: map[string]float64{"n": 4}, settled: 2, delivered: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settled, delivered := resolveImageSettlementCount(test.requested, test.ratios)
			require.Equal(t, test.settled, settled)
			require.Equal(t, test.delivered, delivered)
		})
	}
}
