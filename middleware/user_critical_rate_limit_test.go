package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserCriticalRateLimitIsolatesUsersAndOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRedisEnabled := common.RedisEnabled
	previousEnable := common.CriticalRateLimitEnable
	previousNum := common.CriticalRateLimitNum
	previousDuration := common.CriticalRateLimitDuration
	previousExpiration := common.RateLimitKeyExpirationDuration
	common.RedisEnabled = false
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 1
	common.CriticalRateLimitDuration = 60
	common.RateLimitKeyExpirationDuration = 0
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.CriticalRateLimitEnable = previousEnable
		common.CriticalRateLimitNum = previousNum
		common.CriticalRateLimitDuration = previousDuration
		common.RateLimitKeyExpirationDuration = previousExpiration
	})

	scope := t.Name() + ":" + common.GetUUID()
	accessTokenLimit := UserCriticalRateLimit(scope + ":access-token")
	affiliateTransferLimit := UserCriticalRateLimit(scope + ":aff-transfer")

	require.Equal(t, http.StatusOK, runUserRateLimit(t, accessTokenLimit, 1))
	require.Equal(t, http.StatusTooManyRequests, runUserRateLimit(t, accessTokenLimit, 1))
	require.Equal(t, http.StatusOK, runUserRateLimit(t, accessTokenLimit, 2))
	require.Equal(t, http.StatusOK, runUserRateLimit(t, affiliateTransferLimit, 1))
	require.Equal(t, http.StatusTooManyRequests, runUserRateLimit(t, affiliateTransferLimit, 1))
}

func TestUserCriticalRateLimitRequiresAuthenticatedUser(t *testing.T) {
	previousRedisEnabled := common.RedisEnabled
	previousEnable := common.CriticalRateLimitEnable
	previousExpiration := common.RateLimitKeyExpirationDuration
	common.RedisEnabled = false
	common.CriticalRateLimitEnable = true
	common.RateLimitKeyExpirationDuration = 0
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.CriticalRateLimitEnable = previousEnable
		common.RateLimitKeyExpirationDuration = previousExpiration
	})

	require.Equal(t, http.StatusUnauthorized, runUserRateLimit(t, UserCriticalRateLimit("access-token"), 0))
}

func runUserRateLimit(t *testing.T, handler gin.HandlerFunc, userID int) int {
	t.Helper()
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.POST("/", func(c *gin.Context) {
		if userID != 0 {
			c.Set("id", userID)
		}
	}, handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	return recorder.Code
}
