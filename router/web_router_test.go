package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type webRouterTestFS struct {
	http.FileSystem
}

func (f *webRouterTestFS) Exists(prefix string, requestPath string) bool {
	name := strings.TrimPrefix(requestPath, prefix)
	file, err := f.Open(name)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

func newWebRouterTestFS() static.ServeFileSystem {
	return &webRouterTestFS{FileSystem: http.FS(fstest.MapFS{
		"assets/classic.js":    &fstest.MapFile{Data: []byte("classic")},
		"static/js/default.js": &fstest.MapFile{Data: []byte("default")},
	})}
}

func TestRegisterWebMiddlewareLimitsPagesButNotExistingStaticAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRedisEnabled := common.RedisEnabled
	originalEnabled := common.GlobalWebRateLimitEnable
	originalLimit := common.GlobalWebRateLimitNum
	originalDuration := common.GlobalWebRateLimitDuration
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		common.GlobalWebRateLimitEnable = originalEnabled
		common.GlobalWebRateLimitNum = originalLimit
		common.GlobalWebRateLimitDuration = originalDuration
	})

	common.RedisEnabled = false
	common.GlobalWebRateLimitEnable = true
	common.GlobalWebRateLimitNum = 1
	common.GlobalWebRateLimitDuration = 180

	router := gin.New()
	registerWebMiddleware(router, newWebRouterTestFS())
	router.NoRoute(func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, requestPath := range []string{"/assets/classic.js", "/static/js/default.js"} {
		for i := 0; i < 2; i++ {
			response := performWebRouterRequest(router, requestPath, "192.0.2.205:12345")
			require.Equal(t, http.StatusOK, response.Code, requestPath)
		}
	}

	require.Equal(t, http.StatusOK, performWebRouterRequest(router, "/console/log", "192.0.2.205:12345").Code)
	require.Equal(t, http.StatusTooManyRequests, performWebRouterRequest(router, "/console/log", "192.0.2.205:12345").Code)
	require.Equal(t, http.StatusOK, performWebRouterRequest(router, "/static/js/missing.js", "192.0.2.206:12345").Code)
	require.Equal(t, http.StatusTooManyRequests, performWebRouterRequest(router, "/static/js/missing.js", "192.0.2.206:12345").Code)
}

func performWebRouterRequest(router http.Handler, requestPath string, remoteAddr string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, requestPath, nil)
	request.RemoteAddr = remoteAddr
	router.ServeHTTP(recorder, request)
	return recorder
}
