package types

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

func TestReadableRelayErrorMessageAddsChineseHintForStreamDisconnect(t *testing.T) {
	relayErr := NewErrorWithStatusCode(
		errors.New("upstream stream disconnected: connection reset by peer"),
		ErrorCodeDoRequestFailed,
		http.StatusInternalServerError,
	)

	require.Equal(t, "upstream stream disconnected: connection reset by peer", relayErr.Error())
	require.Contains(t, relayErr.ErrorWithStatusCode(), "status_code=500")
	require.Contains(t, relayErr.ErrorWithStatusCode(), "upstream stream disconnected: connection reset by peer")
	require.Contains(t, relayErr.ErrorWithStatusCode(), "中文说明：上游流式响应中途断开")
	require.Contains(t, relayErr.MaskSensitiveErrorWithStatusCode(), "中文说明：上游流式响应中途断开")
	require.Contains(t, relayErr.ToOpenAIError().Message, "中文说明：上游流式响应中途断开")
	require.Contains(t, relayErr.ToClaudeError().Message, "中文说明：上游流式响应中途断开")
}

func TestReadableRelayErrorMessageDoesNotDuplicateChineseHint(t *testing.T) {
	message := "upstream stream disconnected: connection reset by peer（中文说明：上游流式响应中途断开）"

	require.Equal(t, message, readableRelayErrorMessage(message))
}
