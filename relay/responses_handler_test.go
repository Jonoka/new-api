package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSyncResponsesStreamStateFromBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name     string
		body     []byte
		initial  bool
		expected bool
	}{
		{name: "stream true", body: []byte(`{"stream":true}`), initial: false, expected: true},
		{name: "stream false", body: []byte(`{"stream":false}`), initial: true, expected: false},
		{name: "stream absent", body: []byte(`{"model":"gpt-5"}`), initial: true, expected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{IsStream: tc.initial}

			syncResponsesStreamStateFromBody(c, info, tc.body)

			require.Equal(t, tc.expected, info.IsStream)
			if _, ok := common.GetContextKey(c, appconstant.ContextKeyIsStream); ok {
				require.Equal(t, tc.expected, common.GetContextKeyBool(c, appconstant.ContextKeyIsStream))
			}
		})
	}
}
