package types

import (
	"errors"
	"net/http"
	"testing"
)

func TestNewOpenAIErrorPreservesTypedErrorCode(t *testing.T) {
	err := NewOpenAIError(errors.New("dial failed"), ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	if got := err.GetErrorCode(); got != ErrorCodeDoRequestFailed {
		t.Fatalf("expected %q, got %q", ErrorCodeDoRequestFailed, got)
	}
}

func TestNewOpenAIErrorPreservesErrorCodeWhenWrappingNewAPIError(t *testing.T) {
	inner := NewError(errors.New("dial failed"), ErrorCodeDoRequestFailed)
	err := NewOpenAIError(inner, ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	if got := err.GetErrorCode(); got != ErrorCodeDoRequestFailed {
		t.Fatalf("expected %q, got %q", ErrorCodeDoRequestFailed, got)
	}
}
