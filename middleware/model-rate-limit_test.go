package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func TestBuildModelRequestRateLimitRuleUsesMostSpecificOverride(t *testing.T) {
	originalGroup := setting.ModelRequestRateLimitGroup
	originalUserGroup := setting.ModelRequestRateLimitUserGroup
	originalTotal := setting.ModelRequestRateLimitCount
	originalSuccess := setting.ModelRequestRateLimitSuccessCount
	defer func() {
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = originalGroup
		setting.ModelRequestRateLimitUserGroup = originalUserGroup
		setting.ModelRequestRateLimitCount = originalTotal
		setting.ModelRequestRateLimitSuccessCount = originalSuccess
		setting.ModelRequestRateLimitMutex.Unlock()
	}()

	setting.ModelRequestRateLimitCount = 10
	setting.ModelRequestRateLimitSuccessCount = 20
	if err := setting.UpdateModelRequestRateLimitGroupByJSONString(`{"codex":[30,40]}`); err != nil {
		t.Fatalf("failed to set request group limits: %v", err)
	}
	if err := setting.UpdateModelRequestRateLimitUserGroupByJSONString(`{"vip":{"global":[50,60],"groups":{"codex":[70,80]}}}`); err != nil {
		t.Fatalf("failed to set user group limits: %v", err)
	}

	rule := buildModelRequestRateLimitRule("vip", "codex")
	want := modelRequestRateLimitRule{
		name:            "user_group_request_group",
		scope:           "user_group:vip:request_group:codex",
		totalMaxCount:   70,
		successMaxCount: 80,
	}
	if rule != want {
		t.Fatalf("unexpected selected rule, got %#v want %#v", rule, want)
	}
}

func TestBuildModelRequestRateLimitRuleUserGroupGlobalOverridesBaseGlobal(t *testing.T) {
	originalGroup := setting.ModelRequestRateLimitGroup
	originalUserGroup := setting.ModelRequestRateLimitUserGroup
	originalTotal := setting.ModelRequestRateLimitCount
	originalSuccess := setting.ModelRequestRateLimitSuccessCount
	defer func() {
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = originalGroup
		setting.ModelRequestRateLimitUserGroup = originalUserGroup
		setting.ModelRequestRateLimitCount = originalTotal
		setting.ModelRequestRateLimitSuccessCount = originalSuccess
		setting.ModelRequestRateLimitMutex.Unlock()
	}()

	setting.ModelRequestRateLimitCount = 10
	setting.ModelRequestRateLimitSuccessCount = 20
	if err := setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`); err != nil {
		t.Fatalf("failed to clear request group limits: %v", err)
	}
	if err := setting.UpdateModelRequestRateLimitUserGroupByJSONString(`{"vip":{"global":[50,60]}}`); err != nil {
		t.Fatalf("failed to set user group limits: %v", err)
	}

	rule := buildModelRequestRateLimitRule("vip", "codex")
	want := modelRequestRateLimitRule{
		name:            "user_group",
		scope:           "user_group:vip",
		totalMaxCount:   50,
		successMaxCount: 60,
	}
	if rule != want {
		t.Fatalf("unexpected selected rule, got %#v want %#v", rule, want)
	}
}

func TestBuildModelRequestRateLimitKeyEscapesScopes(t *testing.T) {
	key := buildModelRequestRateLimitKey("success", "user_group:vip:a/request_group:codex plus", "42")
	if key != "rateLimit:model_request:v2:success:user_group%3Avip%3Aa%2Frequest_group%3Acodex+plus:user:42" {
		t.Fatalf("unexpected escaped redis key: %s", key)
	}

	memoryKey := buildModelRequestRateLimitMemoryKey("total", "user_group:vip:a/request_group:codex plus", "42")
	if memoryKey != "MRRL:v2:total:user_group%3Avip%3Aa%2Frequest_group%3Acodex+plus:user:42" {
		t.Fatalf("unexpected escaped memory key: %s", memoryKey)
	}
}

func TestMemoryRateLimitHandlerBlocksWhenSuccessLimitAlreadyReached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)

	userId := "42"
	rule := modelRequestRateLimitRule{
		name:            "global",
		scope:           "global",
		totalMaxCount:   0,
		successMaxCount: 1,
	}
	successKey := buildModelRequestRateLimitMemoryKey("success", rule.scope, userId)
	if !inMemoryRateLimiter.Request(successKey, rule.successMaxCount, 60) {
		t.Fatal("expected setup request to be recorded")
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 42)
		c.Next()
	})
	router.Use(memoryRateLimitHandler(60, rule))
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", recorder.Code)
	}
}

func TestMemoryRateLimitHandlerRecordsSuccessForFinalSelectedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter.Init(time.Minute)

	originalGroup := setting.ModelRequestRateLimitGroup
	defer func() {
		setting.ModelRequestRateLimitMutex.Lock()
		setting.ModelRequestRateLimitGroup = originalGroup
		setting.ModelRequestRateLimitMutex.Unlock()
	}()
	if err := setting.UpdateModelRequestRateLimitGroupByJSONString(`{"codex-final":[0,1]}`); err != nil {
		t.Fatalf("failed to set request group limits: %v", err)
	}

	userId := "987654"
	initialRule := modelRequestRateLimitRule{
		name:            "global",
		scope:           "global",
		totalMaxCount:   0,
		successMaxCount: 100,
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 987654)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, "default")
		c.Next()
	})
	router.Use(memoryRateLimitHandler(60, initialRule))
	router.GET("/test", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, "codex-final")
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	finalSuccessKey := buildModelRequestRateLimitMemoryKey("success", "request_group:codex-final", userId)
	if inMemoryRateLimiter.Allow(finalSuccessKey, 1, 60) {
		t.Fatal("expected final request group success counter to be full")
	}
}
