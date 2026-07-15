package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type invoiceOrderSelectionRequest struct {
	Orders []model.InvoiceOrderReference `json:"orders"`
}

type applyInvoiceOrdersRequest struct {
	Orders  []model.InvoiceOrderReference `json:"orders"`
	Invoice model.InvoiceRequest          `json:"invoice"`
}

func GetInvoiceOrders(c *gin.Context) {
	orders, err := model.GetRecentInvoiceOrders(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"window_days": 30,
		"currency":    "CNY",
		"orders":      orders,
	})
}

func PreviewInvoiceOrders(c *gin.Context) {
	var req invoiceOrderSelectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
		return
	}
	preview, err := model.PreviewInvoiceOrders(c.GetInt("id"), req.Orders)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, preview)
}

func ApplyInvoiceOrders(c *gin.Context) {
	var req applyInvoiceOrdersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
		return
	}
	// 该接口本身就是发票申请动作，不要求客户端重复传 required=true。
	req.Invoice.Required = true
	record, err := model.CreateCombinedInvoiceWithBalance(c.GetInt("id"), req.Orders, req.Invoice)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, record)
}
