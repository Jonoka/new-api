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

	stats, err := model.GetRevenueStats(startTimestamp, endTimestamp, granularity)
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
