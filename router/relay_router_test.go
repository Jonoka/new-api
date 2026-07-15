package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
