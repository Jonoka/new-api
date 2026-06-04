package relay

import (
	"net/http"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/assert"
)

func TestShouldTreatOpenAIImageAcceptedAsSuccess(t *testing.T) {
	assert.True(t, shouldTreatOpenAIImageAcceptedAsSuccess(&relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}, http.StatusAccepted))
	assert.False(t, shouldTreatOpenAIImageAcceptedAsSuccess(&relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeChatCompletions,
	}, http.StatusAccepted))
	assert.False(t, shouldTreatOpenAIImageAcceptedAsSuccess(&relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}, http.StatusOK))
	assert.False(t, shouldTreatOpenAIImageAcceptedAsSuccess(nil, http.StatusAccepted))
}
