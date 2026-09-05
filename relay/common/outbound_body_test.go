package common

import (
	"io"
	"testing"

	basecommon "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestNewOutboundJSONBodyReturnsExactReplayableBytes(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"model":"mapped-model","input":"final transformed bytes"}`)
	body, size, factory, owner, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	require.EqualValues(t, len(payload), size)

	first, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, payload, first)

	for i := 0; i < 2; i++ {
		replay, err := factory()
		require.NoError(t, err)
		got, err := io.ReadAll(replay)
		require.NoError(t, err)
		require.Equal(t, payload, got)
		require.NoError(t, replay.Close())
	}

	require.NoError(t, owner.Close())
	_, err = factory()
	require.ErrorIs(t, err, basecommon.ErrStorageClosed)
}
