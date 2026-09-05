package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayRequestParseErrorStatusAndRetryBoundary(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "invalid request", err: errors.New("invalid JSON"), wantStatus: http.StatusBadRequest},
		{name: "oversized request", err: common.ErrRequestBodyTooLarge, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "wrapped oversized request", err: errors.Join(errors.New("read failed"), common.ErrRequestBodyTooLarge), wantStatus: http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			relayErr := relayRequestParseError(tt.err)
			require.Equal(t, tt.wantStatus, relayErr.StatusCode)
			require.True(t, types.IsSkipRetryError(relayErr))
		})
	}
}

func TestRelayRejectsMalformedRequestBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{"))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), string(types.ErrorCodeInvalidRequest))
}

func TestRelayPreservesRequestBodyTooLargeStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousMaxBodyMB := constant.MaxRequestBodyMB
	constant.MaxRequestBodyMB = 1
	t.Cleanup(func() { constant.MaxRequestBodyMB = previousMaxBodyMB })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })

	Relay(c, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Contains(t, recorder.Body.String(), string(types.ErrorCodeReadRequestBodyFailed))
}

func TestRelayRejectsNonObjectRequestBeforeUpstream(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		path   string
		format types.RelayFormat
	}{
		{name: "rerank", path: "/v1/rerank", format: types.RelayFormatRerank},
		{name: "embedding", path: "/v1/embeddings", format: types.RelayFormatEmbedding},
	} {
		for _, body := range []string{"null", "[]", "42", `"text"`} {
			t.Run(fixture.name+"/"+body, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				context, _ := gin.CreateTestContext(recorder)
				context.Request = httptest.NewRequest(http.MethodPost, fixture.path, strings.NewReader(body))
				context.Request.Header.Set("Content-Type", "application/json")
				t.Cleanup(func() { common.CleanupBodyStorage(context) })

				Relay(context, fixture.format)

				require.Equal(t, http.StatusBadRequest, recorder.Code)
				require.Contains(t, recorder.Body.String(), string(types.ErrorCodeInvalidRequest))
			})
		}
	}
}
