package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetChannelAnalyticsSummary(c *gin.Context) {
	query, ok := parseChannelAnalyticsQuery(c)
	if !ok {
		return
	}
	response, err := service.GetChannelAnalyticsSummary(query)
	writeChannelAnalyticsResponse(c, response, err)
}

func GetChannelAnalyticsTrend(c *gin.Context) {
	query, ok := parseChannelAnalyticsQuery(c)
	if !ok {
		return
	}
	response, err := service.GetChannelAnalyticsTrend(query)
	writeChannelAnalyticsResponse(c, response, err)
}

func GetChannelAnalyticsChannels(c *gin.Context) {
	query, ok := parseChannelAnalyticsQuery(c)
	if !ok {
		return
	}
	response, err := service.GetChannelAnalyticsChannels(query)
	writeChannelAnalyticsResponse(c, response, err)
}

func GetChannelAnalyticsModels(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		writeChannelAnalyticsError(c, http.StatusBadRequest, service.ErrInvalidChannelAnalyticsQuery)
		return
	}
	query, ok := parseChannelAnalyticsQuery(c)
	if !ok {
		return
	}
	response, err := service.GetChannelAnalyticsModels(channelID, query)
	writeChannelAnalyticsResponse(c, response, err)
}

func GetChannelAnalyticsStability(c *gin.Context) {
	query, err := service.ParseChannelAnalyticsStabilityQuery(c.Request.URL.Query())
	if err != nil {
		writeChannelAnalyticsError(c, http.StatusBadRequest, err)
		return
	}
	response, err := service.GetChannelAnalyticsStability(query)
	writeChannelAnalyticsResponse(c, response, err)
}

func GetChannelAnalyticsStatusCodes(c *gin.Context) {
	query, ok := parseChannelAnalyticsQuery(c)
	if !ok {
		return
	}
	response, err := service.GetChannelAnalyticsStatusCodes(query)
	writeChannelAnalyticsResponse(c, response, err)
}

func GetChannelAnalyticsFailures(c *gin.Context) {
	query, err := service.ParseChannelAnalyticsFailureQuery(c.Request.URL.Query())
	if err != nil {
		writeChannelAnalyticsError(c, http.StatusBadRequest, err)
		return
	}
	response, err := service.GetChannelAnalyticsFailures(query)
	writeChannelAnalyticsResponse(c, response, err)
}

func GetChannelAnalyticsFilters(c *gin.Context) {
	response, err := service.GetChannelAnalyticsFilters()
	writeChannelAnalyticsResponse(c, response, err)
}

func GetChannelAnalyticsFilterModels(c *gin.Context) {
	query, err := service.ParseChannelAnalyticsFilterModelsQuery(c.Request.URL.Query())
	if err != nil {
		writeChannelAnalyticsError(c, http.StatusBadRequest, err)
		return
	}
	response, err := service.GetChannelAnalyticsFilterModels(query)
	writeChannelAnalyticsResponse(c, response, err)
}

func parseChannelAnalyticsQuery(c *gin.Context) (query dto.ChannelAnalyticsQuery, ok bool) {
	query, err := service.ParseChannelAnalyticsQuery(c.Request.URL.Query())
	if err != nil {
		writeChannelAnalyticsError(c, http.StatusBadRequest, err)
		return query, false
	}
	return query, true
}

func writeChannelAnalyticsResponse(c *gin.Context, data interface{}, err error) {
	if err == nil {
		common.ApiSuccess(c, data)
		return
	}
	if errors.Is(err, service.ErrInvalidChannelAnalyticsQuery) {
		writeChannelAnalyticsError(c, http.StatusBadRequest, err)
		return
	}
	writeChannelAnalyticsError(c, http.StatusInternalServerError, err)
}

func writeChannelAnalyticsError(c *gin.Context, status int, err error) {
	message := "渠道统计查询失败"
	if err != nil {
		message = err.Error()
	}
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
	})
}
