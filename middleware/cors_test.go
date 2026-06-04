package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCORSPreflightAllowsCanvasJsonRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS())
	router.POST("/canvas/v1/images/generations", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodOptions, "/canvas/v1/images/generations?group=Image2", nil)
	request.Header.Set("Origin", "https://canvas.maolaoapi.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, "https://canvas.maolaoapi.com", recorder.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers")), "content-type")
}
