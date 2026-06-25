package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMarkActualStreamFromResponseSyncsContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	common.SetContextKey(c, constant.ContextKeyIsStream, false)

	info := &relaycommon.RelayInfo{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
	}

	markActualStreamFromResponse(c, info, resp)

	require.True(t, info.IsStream)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyIsStream))
}

func TestMarkActualStreamFromResponseIgnoresNonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	common.SetContextKey(c, constant.ContextKeyIsStream, false)

	info := &relaycommon.RelayInfo{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	markActualStreamFromResponse(c, info, resp)

	require.False(t, info.IsStream)
	require.False(t, common.GetContextKeyBool(c, constant.ContextKeyIsStream))
}
