package router

import (
	"testing"

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
