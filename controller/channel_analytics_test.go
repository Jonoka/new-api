package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestChannelAnalyticsControllerRejectsInvalidQueryWithHTTP400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/summary", GetChannelAnalyticsSummary)

	request := httptest.NewRequest(http.MethodGet, "/summary?start_timestamp=200&end_timestamp=100", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
}

func TestChannelAnalyticsControllerRejectsInvalidChannelID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/channels/:id/models", GetChannelAnalyticsModels)

	request := httptest.NewRequest(http.MethodGet, "/channels/not-a-number/models", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
}
