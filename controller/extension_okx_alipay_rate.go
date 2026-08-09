package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type okxAlipayRateConfigRequest struct {
	RateAPIURL      string  `json:"rate_api_url"`
	Side            string  `json:"side"`
	Tier            int     `json:"tier"`
	AdjustmentType  string  `json:"adjustment_type"`
	AdjustmentValue float64 `json:"adjustment_value"`
}

func okxAlipayRateConfigResponse(config extension.OkxAlipayRateConfig) gin.H {
	config = extension.NormalizeOkxAlipayRateConfig(config)
	return gin.H{
		"rate_api_url":     config.RateAPIURL,
		"side":             config.Side,
		"tier":             config.Tier,
		"adjustment_type":  config.AdjustmentType,
		"adjustment_value": strconv.FormatFloat(config.AdjustmentValue, 'f', -1, 64),
	}
}

func okxAlipayRateQuoteResponse(quote extension.OkxAlipayRateQuote) gin.H {
	return gin.H{
		"raw_rate":         strconv.FormatFloat(quote.RawRate, 'f', -1, 64),
		"adjusted_rate":    strconv.FormatFloat(quote.AdjustedRate, 'f', -1, 64),
		"source":           quote.Source,
		"side":             quote.Side,
		"tier":             quote.Tier,
		"adjustment_type":  quote.AdjustmentType,
		"adjustment_value": strconv.FormatFloat(quote.AdjustmentValue, 'f', -1, 64),
		"rate_api_url":     quote.RateAPIURL,
		"order_id":         quote.OrderID,
		"nick_name":        quote.NickName,
	}
}

func GetOkxAlipayRateConfig(c *gin.Context) {
	common.ApiSuccess(c, okxAlipayRateConfigResponse(extension.GetOkxAlipayRateConfig()))
}

func SaveOkxAlipayRateConfig(c *gin.Context) {
	var req okxAlipayRateConfigRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	config := extension.NormalizeOkxAlipayRateConfig(extension.OkxAlipayRateConfig{
		RateAPIURL:      req.RateAPIURL,
		Side:            req.Side,
		Tier:            req.Tier,
		AdjustmentType:  req.AdjustmentType,
		AdjustmentValue: req.AdjustmentValue,
	})
	if err := extension.ValidateOkxAlipayRateConfig(config); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.UpdateOptionsBulk(config.OptionValues()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, okxAlipayRateConfigResponse(config))
}

func PreviewOkxAlipayRate(c *gin.Context) {
	config := extension.GetOkxAlipayRateConfig()
	quote, err := extension.FetchOkxAlipayRateQuote(config)
	if err != nil {
		common.ApiErrorMsg(c, "获取 OKX 支付宝汇率失败: "+err.Error())
		return
	}
	common.ApiSuccess(c, okxAlipayRateQuoteResponse(quote))
}
