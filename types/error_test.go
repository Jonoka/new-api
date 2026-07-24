package types

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrOptionWithHideErrMsgPreservesCause(t *testing.T) {
	original := fmt.Errorf("post upstream: %w", context.Canceled)
	relayErr := NewError(
		original,
		ErrorCodeDoRequestFailed,
		ErrOptionWithHideErrMsg("upstream error: do request failed"),
	)

	require.Equal(t, "upstream error: do request failed", relayErr.Error())
	require.ErrorIs(t, relayErr, context.Canceled)
}
