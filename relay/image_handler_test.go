package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageTasksEndpointAccepts202(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}}
	require.True(t, isSuccessfulImageResponse(http.StatusAccepted, info))
	require.False(t, isSuccessfulImageResponse(http.StatusAccepted, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}))
}

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
	require.NotNil(t, info.UpstreamRequestBodyFactory)
	replay, err := info.UpstreamRequestBodyFactory()
	require.NoError(t, err)
	replayed, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.NoError(t, replay.Close())
	require.Equal(t, payload, replayed)
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

func TestImageHelperPreservesTypedGeminiImageSizeError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{
		GeminiImageParamCompatEnabled: true,
	})

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		Request:   &dto.ImageRequest{Model: "model", Prompt: "prompt", Size: "not-a-size"},
	}
	err := ImageHelper(c, info)

	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
	require.True(t, types.IsSkipRetryError(apiErr))
}
