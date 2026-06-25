package relay

import (
	"net/http"
	"strings"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func markActualStreamFromResponse(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) {
	if c == nil || info == nil || resp == nil || resp.StatusCode != http.StatusOK {
		return
	}
	if !isStreamResponseContentType(resp.Header.Get("Content-Type")) {
		return
	}

	info.IsStream = true
	appcommon.SetContextKey(c, constant.ContextKeyIsStream, true)
}

func isStreamResponseContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/event-stream") || strings.Contains(ct, "stream")
}
