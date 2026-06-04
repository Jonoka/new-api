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
	config.AllowHeaders = []string{"*"}
	config.AllowOriginWithContextFunc = func(c *gin.Context, origin string) bool {
		return isAllowedCredentialOrigin(c, origin)
	}
	return cors.New(config)
}

func isAllowedCredentialOrigin(c *gin.Context, origin string) bool {
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
	return host == "maolaoapi.com" || strings.HasSuffix(host, ".maolaoapi.com")
}

func PoweredBy() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-New-Api-Version", common.Version)
		c.Next()
	}
}
