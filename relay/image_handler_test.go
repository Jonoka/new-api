package relay

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestImageTasksEndpointAccepts202(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{ImageAsyncMode: dto.ImageAsyncModeTasksEndpoint}}}
	require.True(t, isSuccessfulImageResponse(http.StatusAccepted, info))
	require.False(t, isSuccessfulImageResponse(http.StatusAccepted, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}))
}
