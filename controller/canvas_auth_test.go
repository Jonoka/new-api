package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const canvasTestOrigin = "https://canvas-2.example.test"
const canvasTestState = "sssssssssssssssssssssssssssssssssssssssssss"
const canvasTestVerifier = "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv"

func setupCanvasAuth(t *testing.T) (*gin.Engine, []*http.Cookie, *model.User) {
	t.Helper()
	redisURL := os.Getenv("NEW_API_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("NEW_API_TEST_REDIS_URL is required for real Redis SSO tests")
	}
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	require.NoError(t, client.Ping(context.Background()).Err())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSubscription{}))
	oldDB, oldRDB, oldSSORDB := model.DB, common.RDB, common.CanvasSSORDB
	oldEnabled, oldSQLite, oldOrigin := common.RedisEnabled, common.UsingSQLite, common.CanvasSSOOrigin
	model.DB, common.RDB = db, client
	common.RedisEnabled, common.UsingSQLite, common.CanvasSSOOrigin = true, true, canvasTestOrigin
	common.InitCanvasSSORedisClient()
	ssoClient := common.CanvasSSORDB
	user := &model.User{Id: 72439, Username: "canvas-test", DisplayName: "Shared Name", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "canvas-test"}
	require.NoError(t, db.Create(user).Error)
	// Seed stale session cache explicitly so UserSessionAuth does not start a background cache fill.
	cacheKey := fmt.Sprintf("user:%d", user.Id)
	require.NoError(t, client.HSet(context.Background(), cacheKey, map[string]interface{}{
		"Id": user.Id, "Username": user.Username, "Role": user.Role, "Status": user.Status, "Group": user.Group,
	}).Err())
	t.Cleanup(func() {
		_ = client.Del(context.Background(), cacheKey).Err()
		_ = client.Close()
		_ = ssoClient.Close()
		_ = sqlDB.Close()
		model.DB, common.RDB, common.CanvasSSORDB = oldDB, oldRDB, oldSSORDB
		common.RedisEnabled, common.UsingSQLite, common.CanvasSSOOrigin = oldEnabled, oldSQLite, oldOrigin
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("canvas-sso-test-session-only"))))
	router.GET("/test-login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", user.Id)
		session.Set("username", user.Username)
		session.Set("role", user.Role)
		session.Set("status", user.Status)
		session.Set("group", user.Group)
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	auth := router.Group("/canvas/auth", CanvasAuthBoundary)
	auth.OPTIONS("/authorize", func(c *gin.Context) {})
	auth.POST("/authorize", middleware.UserSessionAuth(), CanvasAuthorize)
	auth.POST("/exchange", CanvasExchange)
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/test-login", nil))
	return router, login.Result().Cookies(), user
}

func canvasAuthRequest(router *gin.Engine, path, origin string, body []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for _, value := range cookies {
		req.AddCookie(value)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func issueCanvasCode(t *testing.T, router *gin.Engine, cookies []*http.Cookie) string {
	t.Helper()
	digest := sha256.Sum256([]byte(canvasTestVerifier))
	body, err := common.Marshal(canvasAuthorizeRequest{canvasTestState, base64.RawURLEncoding.EncodeToString(digest[:]), "S256", canvasTestOrigin})
	require.NoError(t, err)
	response := canvasAuthRequest(router, "/canvas/auth/authorize", canvasTestOrigin, body, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Code      string `json:"code"`
			State     string `json:"state"`
			ExpiresIn int    `json:"expires_in"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	require.Len(t, envelope.Data.Code, 43)
	require.Equal(t, canvasTestState, envelope.Data.State)
	require.Equal(t, 60, envelope.Data.ExpiresIn)
	client := common.RDB
	t.Cleanup(func() { _ = client.Del(context.Background(), canvasCodeKey(envelope.Data.Code)).Err() })
	return envelope.Data.Code
}

func canvasExchangeBody(t *testing.T, code, state, verifier, audience string) []byte {
	t.Helper()
	body, err := common.Marshal(canvasExchangeRequest{code, state, verifier, audience})
	require.NoError(t, err)
	return body
}

func TestCanvasAuthSingleUseAndFreshRole(t *testing.T) {
	router, cookies, user := setupCanvasAuth(t)
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		code := issueCanvasCode(t, router, cookies)
		ttl, err := common.RDB.TTL(context.Background(), canvasCodeKey(code)).Result()
		require.NoError(t, err)
		require.Greater(t, ttl, time.Duration(0))
		require.LessOrEqual(t, ttl, canvasCodeTTL)
		require.NoError(t, model.DB.Model(user).Update("role", role).Error)
		body := canvasExchangeBody(t, code, canvasTestState, canvasTestVerifier, canvasTestOrigin)
		response := canvasAuthRequest(router, "/canvas/auth/exchange", "", body, nil)
		require.Equal(t, http.StatusOK, response.Code)
		var envelope struct {
			Success bool           `json:"success"`
			Data    canvasIdentity `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &envelope))
		require.True(t, envelope.Success)
		require.Equal(t, user.Id, envelope.Data.ID)
		require.Equal(t, role, envelope.Data.Role)
		require.NotContains(t, response.Body.String(), "password")
		require.NotContains(t, response.Body.String(), "access_token")
		replay := canvasAuthRequest(router, "/canvas/auth/exchange", "", body, nil)
		require.Equal(t, http.StatusBadRequest, replay.Code)
	}
}

func TestCanvasAuthConcurrentRedemption(t *testing.T) {
	router, cookies, _ := setupCanvasAuth(t)
	code := issueCanvasCode(t, router, cookies)
	body := canvasExchangeBody(t, code, canvasTestState, canvasTestVerifier, canvasTestOrigin)
	var wg sync.WaitGroup
	statuses := make(chan int, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- canvasAuthRequest(router, "/canvas/auth/exchange", "", body, nil).Code
		}()
	}
	wg.Wait()
	close(statuses)
	successes := 0
	for status := range statuses {
		if status == http.StatusOK {
			successes++
		} else {
			require.Equal(t, http.StatusBadRequest, status)
		}
	}
	require.Equal(t, 1, successes)
}

func TestCanvasAuthRejectsProofAndRevokedIdentity(t *testing.T) {
	for _, reason := range []string{"state", "verifier", "audience", "expired", "disabled", "deleted", "role"} {
		t.Run(reason, func(t *testing.T) {
			router, cookies, user := setupCanvasAuth(t)
			code := issueCanvasCode(t, router, cookies)
			state, verifier, audience := canvasTestState, canvasTestVerifier, canvasTestOrigin
			want := http.StatusBadRequest
			switch reason {
			case "state":
				state = strings.Repeat("x", 43)
			case "verifier":
				verifier = strings.Repeat("x", 43)
			case "audience":
				audience = "https://other.example.test"
			case "expired":
				require.NoError(t, common.RDB.ExpireAt(context.Background(), canvasCodeKey(code), time.Unix(1, 0)).Err())
			case "disabled":
				require.NoError(t, model.DB.Model(user).Update("status", common.UserStatusDisabled).Error)
				want = http.StatusForbidden
			case "deleted":
				require.NoError(t, model.DB.Delete(user).Error)
				want = http.StatusForbidden
			case "role":
				require.NoError(t, model.DB.Model(user).Update("role", 999).Error)
				want = http.StatusForbidden
			}
			response := canvasAuthRequest(router, "/canvas/auth/exchange", "", canvasExchangeBody(t, code, state, verifier, audience), nil)
			require.Equal(t, want, response.Code)
			if reason != "audience" {
				replay := canvasAuthRequest(router, "/canvas/auth/exchange", "", canvasExchangeBody(t, code, canvasTestState, canvasTestVerifier, canvasTestOrigin), nil)
				require.Equal(t, http.StatusBadRequest, replay.Code)
			}
		})
	}
}

func TestCanvasAuthBoundaryAndFailClosed(t *testing.T) {
	router, cookies, user := setupCanvasAuth(t)
	for _, origin := range []string{"", "null", "https://canvas-2.example.test.evil", "https://other.example.test"} {
		response := canvasAuthRequest(router, "/canvas/auth/authorize", origin, []byte(`{}`), cookies)
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Empty(t, response.Header().Get("Access-Control-Allow-Origin"))
	}
	unauthorized := canvasAuthRequest(router, "/canvas/auth/authorize", canvasTestOrigin, []byte(`{}`), nil)
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)
	require.Equal(t, canvasTestOrigin, unauthorized.Header().Get("Access-Control-Allow-Origin"))
	preflight := httptest.NewRequest(http.MethodOptions, "/canvas/auth/authorize", nil)
	preflight.Header.Set("Origin", canvasTestOrigin)
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, preflight)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	oversized := canvasAuthRequest(router, "/canvas/auth/authorize", canvasTestOrigin, []byte(`{"state":"`+strings.Repeat("x", 2048)+`"}`), cookies)
	require.Equal(t, http.StatusBadRequest, oversized.Code)
	code := issueCanvasCode(t, router, cookies)
	body := canvasExchangeBody(t, code, canvasTestState, canvasTestVerifier, canvasTestOrigin)
	client := common.CanvasSSORDB
	common.CanvasSSORDB = nil
	response := canvasAuthRequest(router, "/canvas/auth/exchange", "", body, nil)
	common.CanvasSSORDB = client
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.NoError(t, model.DB.Model(user).Update("status", common.UserStatusDisabled).Error)
	digest := sha256.Sum256([]byte(canvasTestVerifier))
	issueBody, err := common.Marshal(canvasAuthorizeRequest{canvasTestState, base64.RawURLEncoding.EncodeToString(digest[:]), "S256", canvasTestOrigin})
	require.NoError(t, err)
	denied := canvasAuthRequest(router, "/canvas/auth/authorize", canvasTestOrigin, issueBody, cookies)
	require.Equal(t, http.StatusForbidden, denied.Code)
}

func TestCanvasAuthDestinationValidation(t *testing.T) {
	for _, destination := range []string{"/", "/canvas/project?a=1#node", "/admin"} {
		require.True(t, validCanvasDestination(destination))
	}
	for _, destination := range []string{"https://evil.test", "//evil.test", "/\\evil.test", "/%2fevil.test", "/%5cevil.test", "/%0d%0aX", "/\x00", strings.Repeat("/", 1025)} {
		require.False(t, validCanvasDestination(destination))
	}
}

func TestCanvasAuthConsumedResponseLossDoesNotRetry(t *testing.T) {
	originalRDB, originalSSORDB := common.RDB, common.CanvasSSORDB
	originalEnabled, originalOrigin := common.RedisEnabled, common.CanvasSSOOrigin
	code := strings.Repeat("c", 43)
	key := canvasCodeKey(code)
	expected := fmt.Sprintf("*2\r\n$6\r\ngetdel\r\n$%d\r\n%s\r\n", len(key), key)
	var attempts atomic.Int32
	consumed := make(chan string, 8)
	common.RDB = redis.NewClient(&redis.Options{
		MaxRetries: 3,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			go func() {
				defer server.Close()
				command := make([]byte, len(expected))
				if _, err := io.ReadFull(server, command); err == nil {
					attempts.Add(1)
					consumed <- string(command)
					// Redis consumed the command, but the connection loses its reply.
				}
			}()
			return client, nil
		},
	})
	common.RedisEnabled, common.CanvasSSOOrigin = true, canvasTestOrigin
	common.InitCanvasSSORedisClient()
	t.Cleanup(func() {
		_ = common.CanvasSSORDB.Close()
		_ = common.RDB.Close()
		common.RDB, common.CanvasSSORDB = originalRDB, originalSSORDB
		common.RedisEnabled, common.CanvasSSOOrigin = originalEnabled, originalOrigin
	})
	router := gin.New()
	router.POST("/canvas/auth/exchange", CanvasAuthBoundary, CanvasExchange)
	body := canvasExchangeBody(t, code, canvasTestState, canvasTestVerifier, canvasTestOrigin)
	response := canvasAuthRequest(router, "/canvas/auth/exchange", "", body, nil)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.EqualValues(t, 1, attempts.Load())
	select {
	case command := <-consumed:
		require.Equal(t, expected, command)
	default:
		t.Fatal("the simulated Redis server did not consume GETDEL")
	}
	require.Equal(t, 3, common.RDB.Options().MaxRetries)
}
