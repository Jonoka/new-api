package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetRevenueStats(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	if startTimestamp <= 0 || endTimestamp <= 0 || endTimestamp <= startTimestamp {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid time range",
		})
		return
	}

	granularity := c.DefaultQuery("granularity", "day")
	if granularity != "hour" && granularity != "day" {
		granularity = "day"
	}

	// timezone_offset: client's UTC offset in seconds (e.g. +28800 for UTC+8)
	tzOffset, _ := strconv.ParseInt(c.DefaultQuery("timezone_offset", "0"), 10, 64)
	if tzOffset < -43200 || tzOffset > 50400 {
		tzOffset = 0
	}

	stats, err := model.GetRevenueStats(startTimestamp, endTimestamp, granularity, tzOffset)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}
