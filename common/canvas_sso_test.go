package common

import (
	"testing"

	"github.com/go-redis/redis/v8"
)

func TestValidateCanvasSSOOrigin(t *testing.T) {
	for _, origin := range []string{"", "https://canvas-2.jo2api.com", "https://canvas.test:9443", "https://canvas.test:1", "https://canvas.test:65535", "https://[::1]:9443"} {
		if err := ValidateCanvasSSOOrigin(origin); err != nil {
			t.Errorf("valid origin rejected: %s", origin)
		}
	}
	for _, origin := range []string{"http://canvas.test", "//canvas.test", "https://user@canvas.test", "https://canvas.test/", "https://canvas.test/path", "https://canvas.test?", "https://canvas.test?q=x", "https://canvas.test#fragment", "https://canvas.test#", "https://CANVAS.test", " https://canvas.test", "https://canvas.test\r\n", "https://canvas.test:", "https://canvas.test:443", "https://canvas.test:0443", "https://canvas.test:09443", "https://canvas.test:0", "https://canvas.test:65536", "https://canvas.test:99999999999999999999", "https://[::1]:", "https://[::1]:443"} {
		if ValidateCanvasSSOOrigin(origin) == nil {
			t.Errorf("invalid origin accepted: %q", origin)
		}
	}
}

func TestCanvasSSORedisUsesIsolatedRetryPolicy(t *testing.T) {
	originalRDB, originalSSORDB, originalOrigin := RDB, CanvasSSORDB, CanvasSSOOrigin
	RDB = redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 9, MaxRetries: 3})
	CanvasSSOOrigin = "https://canvas.test"
	InitCanvasSSORedisClient()
	t.Cleanup(func() {
		_ = CanvasSSORDB.Close()
		_ = RDB.Close()
		RDB, CanvasSSORDB, CanvasSSOOrigin = originalRDB, originalSSORDB, originalOrigin
	})
	if RDB.Options().MaxRetries != 3 || CanvasSSORDB.Options().MaxRetries != 0 {
		t.Fatal("SSO must disable retries without changing the shared client's retry policy")
	}
	if CanvasSSORDB == RDB || CanvasSSORDB.Options().Addr != RDB.Options().Addr || CanvasSSORDB.Options().DB != RDB.Options().DB {
		t.Fatal("SSO must use a separate handle for the same Redis datastore")
	}
}
