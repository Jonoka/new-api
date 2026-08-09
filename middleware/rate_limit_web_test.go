package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestGlobalWebRateLimitDoesNotCountStaticAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalRedisEnabled := common.RedisEnabled
	originalEnabled := common.GlobalWebRateLimitEnable
	originalLimit := common.GlobalWebRateLimitNum
	originalDuration := common.GlobalWebRateLimitDuration
	defer func() {
		common.RedisEnabled = originalRedisEnabled
		common.GlobalWebRateLimitEnable = originalEnabled
		common.GlobalWebRateLimitNum = originalLimit
		common.GlobalWebRateLimitDuration = originalDuration
	}()

	common.RedisEnabled = false
	common.GlobalWebRateLimitEnable = true
	common.GlobalWebRateLimitNum = 1
	common.GlobalWebRateLimitDuration = 180
	inMemoryRateLimiter = common.InMemoryRateLimiter{}

	router := gin.New()
	router.Use(GlobalWebRateLimit())
	router.Any("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for _, requestPath := range []string{
		"/assets/index.js",
		"/assets/index.js.LICENSE.txt",
		"/cover.webp",
	} {
		for i := 0; i < 2; i++ {
			if status := performWebRateLimitRequest(router, http.MethodGet, requestPath); status != http.StatusNoContent {
				t.Fatalf("static request %s was rate limited on attempt %d: status %d", requestPath, i+1, status)
			}
		}
	}

	if status := performWebRateLimitRequest(router, http.MethodGet, "/console/log"); status != http.StatusNoContent {
		t.Fatalf("first document request returned status %d", status)
	}
	if status := performWebRateLimitRequest(router, http.MethodGet, "/console/log"); status != http.StatusTooManyRequests {
		t.Fatalf("second document request returned status %d, want %d", status, http.StatusTooManyRequests)
	}
}

func TestStaticWebAssetDetectionOnlyBypassesSafeReads(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{method: http.MethodGet, path: "/assets/app", want: true},
		{method: http.MethodHead, path: "/logo.svg", want: true},
		{method: http.MethodPost, path: "/assets/app.js", want: false},
		{method: http.MethodGet, path: "/console/log", want: false},
		{method: http.MethodGet, path: "/pricing", want: false},
	}

	for _, test := range tests {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(test.method, test.path, nil)
		if got := isStaticWebAssetRequest(ctx); got != test.want {
			t.Fatalf("isStaticWebAssetRequest(%s %s) = %t, want %t", test.method, test.path, got, test.want)
		}
	}
}

func performWebRateLimitRequest(router http.Handler, method string, requestPath string) int {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, requestPath, nil)
	request.RemoteAddr = "192.0.2.10:12345"
	router.ServeHTTP(recorder, request)
	return recorder.Code
}
