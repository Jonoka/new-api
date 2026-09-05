package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetRelayRouterRegistersCanvasPricingRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	SetRelayRouter(engine)

	for _, route := range engine.Routes() {
		if route.Method == "GET" && route.Path == "/canvas/v1/pricing" {
			require.Contains(t, route.Handler, "controller.GetPricing")
			return
		}
	}
	t.Fatal("GET /canvas/v1/pricing route is not registered")
}

func TestCanvasSSORoutesKeepExactOriginBeforeGlobalCORS(t *testing.T) {
	originalOrigin, originalLimit := common.CanvasSSOOrigin, common.GlobalApiRateLimitEnable
	common.CanvasSSOOrigin, common.GlobalApiRateLimitEnable = "https://canvas-2.jo2api.com", false
	t.Cleanup(func() { common.CanvasSSOOrigin, common.GlobalApiRateLimitEnable = originalOrigin, originalLimit })
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("canvas-route-test-only"))))
	SetRelayRouter(engine)
	for _, method := range []string{http.MethodPost, http.MethodOptions} {
		request := httptest.NewRequest(method, "/canvas/auth/authorize", strings.NewReader(`{}`))
		// This sibling is accepted by legacy relay CORS, but never for SSO issuance.
		request.Header.Set("Origin", "https://sibling.jo2api.com")
		request.Header.Set("Access-Control-Request-Method", "POST")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
	preflight := httptest.NewRequest(http.MethodOptions, "/canvas/auth/authorize", nil)
	preflight.Header.Set("Origin", common.CanvasSSOOrigin)
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, preflight)
	require.Equal(t, http.StatusNoContent, response.Code)

	request := httptest.NewRequest(http.MethodPost, "/canvas/v1/responses?group=default", strings.NewReader(`{"model":"test"}`))
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	common.CanvasSSOOrigin = ""
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/canvas/auth/exchange", strings.NewReader(`{}`)))
	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestCanvasSSOLaunchReturnsToCurrentThemeWithoutLoggingOut(t *testing.T) {
	originalOrigin, originalLaunch, originalTheme := common.CanvasSSOOrigin, common.CanvasSSOLaunchEnabled, common.GetTheme()
	common.CanvasSSOOrigin, common.CanvasSSOLaunchEnabled = "https://canvas-2.jo2api.com", true
	t.Cleanup(func() {
		common.CanvasSSOOrigin, common.CanvasSSOLaunchEnabled = originalOrigin, originalLaunch
		common.SetTheme(originalTheme)
	})
	engine := gin.New()
	SetRelayRouter(engine)
	for _, theme := range []string{"default", "classic"} {
		common.SetTheme(theme)
		path := "/canvas"
		if theme == "classic" {
			path = "/console/canvas"
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/canvas/auth/launch", nil))
		require.Equal(t, http.StatusSeeOther, response.Code)
		require.Equal(t, path, response.Header().Get("Location"))
		require.Empty(t, response.Result().Cookies())
		for _, launchEnabled := range []bool{true, false} {
			common.CanvasSSOLaunchEnabled = launchEnabled
			response = httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/canvas/auth/launch?canvas_resume=1&canvas_next=https%3A%2F%2Fevil.test&group=vip", nil))
			require.Equal(t, path+"?canvas_resume=1&group=vip", response.Header().Get("Location"))
			response = httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/canvas/auth/launch?canvas_resume=1&canvas_next=%2Fprojects%2F1&group=vip", nil))
			require.Equal(t, path+"?canvas_next=%2Fprojects%2F1&canvas_resume=1&group=vip", response.Header().Get("Location"))
		}
	}
}

func TestSetRelayRouterRegistersAPIImageTaskRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	for _, route := range []string{
		"POST /v1/images/tasks",
		"GET /v1/images/tasks/:task_id",
		"GET /v1/images/tasks/:task_id/content/:index",
	} {
		require.True(t, routes[route], route)
	}
}
