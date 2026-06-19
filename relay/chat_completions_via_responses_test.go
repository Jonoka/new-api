package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestSyncResponsesStreamFlagUsesRelayInfo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isStream bool
		initial  *bool
	}{
		{name: "stream overrides nil", isStream: true, initial: nil},
		{name: "stream overrides false", isStream: true, initial: common.GetPointer(false)},
		{name: "non stream overrides nil", isStream: false, initial: nil},
		{name: "non stream overrides true", isStream: false, initial: common.GetPointer(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{IsStream: tc.isStream}
			req := &dto.OpenAIResponsesRequest{Stream: tc.initial}

			syncResponsesStreamFlag(info, req)

			require.NotNil(t, req.Stream)
			require.Equal(t, tc.isStream, *req.Stream)
		})
	}
}
