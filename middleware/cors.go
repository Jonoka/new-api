package middleware

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{
		"Accept",
		"Authorization",
		"Cache-Control",
		"Content-Length",
		"Content-Type",
		"New-API-User",
		"Origin",
		"X-API-Key",
		"X-Requested-With",
		"anthropic-beta",
		"anthropic-version",
	}
	config.AllowOriginWithContextFunc = func(c *gin.Context, origin string) bool {
		return isAllowedCredentialOrigin(c, origin)
	}
	return cors.New(config)
}

func isAllowedCredentialOrigin(c *gin.Context, origin string) bool {
	if common.CanvasSSOOrigin != "" && origin == common.CanvasSSOOrigin {
		return true
	}
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	requestHost := strings.ToLower(c.Request.Host)
	if requestHost != "" {
		if host == strings.ToLower(strings.Split(requestHost, ":")[0]) {
			return true
		}
	}
	return isAllowedCanvasHost(host)
}

func isAllowedCanvasHost(host string) bool {
	allowedDomains := []string{"jo2api.com", "maolaoapi.com"}
	for _, domain := range allowedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func PoweredBy() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-New-Api-Version", common.Version)
		c.Next()
	}
}
