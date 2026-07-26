package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetInvoiceConfig(c *gin.Context) {
	config := model.InvoiceConfigSnapshot()
	payMethods, bepusdtChains := availableInvoicePayMethods()
	config["pay_methods"] = payMethods
	config["bepusdt_chains"] = bepusdtChains
	common.ApiSuccess(c, config)
}

func GetUserInvoices(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	records, total, err := model.GetUserInvoiceRecords(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, pageInfo)
}

func AdminListInvoices(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	records, total, err := model.GetAllInvoiceRecords(c.Query("status"), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, pageInfo)
}

type AdminUpdateInvoiceRequest struct {
	DownloadUrl string `json:"download_url"`
	Status      string `json:"status"`
	AdminRemark string `json:"admin_remark"`
}

func AdminUpdateInvoice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	var req AdminUpdateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if err := model.UpdateInvoiceRecord(id, req.DownloadUrl, req.Status, req.AdminRemark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

type AdminDeleteInvoicesRequest struct {
	Ids []int `json:"ids"`
}

func AdminDeleteInvoice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	deleted, err := model.DeleteInvoiceRecords([]int{id})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, deleted)
}

func AdminDeleteInvoices(c *gin.Context) {
	var req AdminDeleteInvoicesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	deleted, err := model.DeleteInvoiceRecords(req.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, deleted)
}
