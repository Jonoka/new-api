package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type invoiceExternalPaymentRequest struct {
	Orders        []model.InvoiceOrderReference `json:"orders"`
	Invoice       model.InvoiceRequest          `json:"invoice"`
	PaymentMethod string                        `json:"payment_method"`
	TradeType     string                        `json:"trade_type"`
}

func availableInvoicePayMethods() ([]map[string]string, []setting.BepusdtChain) {
	methods := []map[string]string{{
		"name": "余额", "type": model.PaymentMethodBalance,
		"provider": model.PaymentProviderBalance, "color": "rgba(var(--semi-blue-5), 1)",
	}}
	if !operation_setting.IsPaymentComplianceConfirmed() {
		return methods, []setting.BepusdtChain{}
	}
	if isEpayTopUpEnabled() {
		for _, configured := range operation_setting.PayMethods {
			paymentType := strings.TrimSpace(configured["type"])
			if paymentType == "" || paymentType == model.PaymentMethodBalance || paymentType == model.PaymentMethodBepusdt || paymentType == model.PaymentMethodOkpay {
				continue
			}
			name := strings.TrimSpace(configured["name"])
			if name == "" {
				name = paymentType
			}
			color := strings.TrimSpace(configured["color"])
			if color == "" {
				color = "rgba(var(--semi-blue-5), 1)"
			}
			methods = append(methods, map[string]string{
				"name": name, "type": paymentType,
				"provider": model.PaymentProviderEpay, "color": color,
			})
		}
	}
	chains := []setting.BepusdtChain{}
	if isBepusdtTopUpEnabled() {
		chains = setting.GetBepusdtChains()
		methods = append(methods, map[string]string{
			"name": "USDT", "type": model.PaymentMethodBepusdt,
			"provider": model.PaymentProviderBepusdt, "color": "#26A17B",
		})
	}
	if isOkpayTopUpEnabled() {
		methods = append(methods, map[string]string{
			"name": "OKPay", "type": model.PaymentMethodOkpay,
			"provider": model.PaymentProviderOkpay, "color": "#4F46E5",
		})
	}
	return methods, chains
}

func invoicePaymentResponse(record *model.InvoiceRecord, checkout gin.H, amountText string) gin.H {
	response := gin.H{
		"completed": record != nil && record.PaymentStatus == model.InvoicePaymentStatusSuccess,
		"trade_no":  "",
		"invoice":   record,
	}
	if record != nil {
		response["trade_no"] = record.SourceId
	}
	if checkout != nil {
		response["checkout"] = checkout
	}
	if amountText != "" {
		response["amount_text"] = amountText
	}
	return response
}

func invoicePaymentCNYText(record *model.InvoiceRecord) string {
	if record == nil {
		return ""
	}
	return invoicePaymentCNYAmount(record) + " CNY"
}

func markInvoicePaymentFailed(record *model.InvoiceRecord, reason error) {
	if record == nil {
		return
	}
	payload := "支付拉起失败"
	if reason != nil {
		payload = reason.Error()
	}
	_ = model.UpdateInvoiceExternalPaymentStatus(
		record.SourceId,
		record.PaymentProvider,
		model.InvoicePaymentStatusFailed,
		payload,
	)
}

func invoicePaymentCNYAmount(record *model.InvoiceRecord) string {
	if record == nil {
		return ""
	}
	return decimal.NewFromInt(record.PaymentAmountMinor).Div(decimal.NewFromInt(100)).StringFixed(2)
}

func RequestInvoiceExternalPayment(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var req invoiceExternalPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	req.Invoice.Required = true
	req.PaymentMethod = strings.TrimSpace(req.PaymentMethod)
	req.TradeType = strings.TrimSpace(req.TradeType)
	if req.PaymentMethod == "" || req.PaymentMethod == model.PaymentMethodBalance {
		common.ApiErrorMsg(c, "余额支付请使用 /api/user/invoice/request")
		return
	}

	options := model.InvoiceExternalPaymentOptions{
		PaymentMethod: req.PaymentMethod,
		RequestIP:     c.ClientIP(),
	}
	switch req.PaymentMethod {
	case model.PaymentMethodBepusdt:
		if !isBepusdtTopUpEnabled() {
			common.ApiErrorMsg(c, "BEPUSDT 支付不可用")
			return
		}
		chains := setting.GetBepusdtChains()
		if req.TradeType == "" && len(chains) > 0 {
			req.TradeType = chains[0].TradeType
		}
		if !isValidBepusdtTradeType(req.TradeType) {
			common.ApiErrorMsg(c, "不支持的支付链")
			return
		}
		options.PaymentProvider = model.PaymentProviderBepusdt
	case model.PaymentMethodOkpay:
		if !isOkpayTopUpEnabled() {
			common.ApiErrorMsg(c, "OKPay 支付不可用")
			return
		}
		options.PaymentProvider = model.PaymentProviderOkpay
		options.ProviderMerchantId = setting.OkpayMerchantId
	default:
		if !isEpayTopUpEnabled() || !operation_setting.ContainsPayMethod(req.PaymentMethod) {
			common.ApiErrorMsg(c, "支付方式不存在或不可用")
			return
		}
		options.PaymentProvider = model.PaymentProviderEpay
		options.ProviderMerchantId = operation_setting.EpayId
	}
	options.ProviderPayload = common.GetJsonString(map[string]string{
		"payment_provider": options.PaymentProvider,
		"payment_method":   options.PaymentMethod,
		"trade_type":       req.TradeType,
	})

	record, err := model.CreateCombinedInvoiceExternalPayment(c.GetInt("id"), req.Orders, req.Invoice, options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if record.PaymentStatus == model.InvoicePaymentStatusSuccess {
		common.ApiSuccess(c, invoicePaymentResponse(record, nil, invoicePaymentCNYText(record)))
		return
	}

	callbackBase := strings.TrimRight(service.GetCallbackAddress(), "/")
	switch options.PaymentProvider {
	case model.PaymentProviderEpay:
		client := GetEpayClient()
		if client == nil {
			markInvoicePaymentFailed(record, errors.New("易支付客户端初始化失败"))
			common.ApiErrorMsg(c, "当前管理员未配置支付信息")
			return
		}
		notifyURL, notifyErr := url.Parse(callbackBase + "/api/invoice/epay/notify")
		returnURL, returnErr := url.Parse(callbackBase + "/api/invoice/epay/return")
		if notifyErr != nil || returnErr != nil {
			markInvoicePaymentFailed(record, errors.New("易支付回调地址配置错误"))
			common.ApiErrorMsg(c, "回调地址配置错误")
			return
		}
		uri, params, err := client.Purchase(&epay.PurchaseArgs{
			Type: req.PaymentMethod, ServiceTradeNo: record.SourceId,
			Name:   "Invoice-" + record.SourceId,
			Money:  decimal.NewFromInt(record.PaymentAmountMinor).Div(decimal.NewFromInt(100)).StringFixed(2),
			Device: epay.PC, NotifyUrl: notifyURL, ReturnUrl: returnURL,
		})
		if err != nil {
			markInvoicePaymentFailed(record, err)
			common.ApiErrorMsg(c, fmt.Sprintf("拉起支付失败，支付记录 %s 已保留", record.SourceId))
			return
		}
		common.ApiSuccess(c, invoicePaymentResponse(record, gin.H{"type": "form", "url": uri, "params": params}, invoicePaymentCNYText(record)))
	case model.PaymentProviderBepusdt:
		payment, err := createBepusdtTransactionDetails(
			c, record.SourceId,
			decimal.NewFromInt(record.PaymentAmountMinor).Div(decimal.NewFromInt(100)).InexactFloat64(),
			req.TradeType, callbackBase+"/api/invoice/bepusdt/notify",
			paymentReturnPath("/console/invoice?pay=pending"), "Invoice-"+record.SourceId,
		)
		if err != nil {
			markInvoicePaymentFailed(record, err)
			common.ApiErrorMsg(c, fmt.Sprintf("拉起支付失败，支付记录 %s 已保留", record.SourceId))
			return
		}
		if payment.Data.TradeId == "" || (payment.Data.OrderId != "" && payment.Data.OrderId != record.SourceId) ||
			(payment.Data.Fiat != "" && !strings.EqualFold(payment.Data.Fiat, "CNY")) ||
			(payment.Data.TradeType != "" && payment.Data.TradeType != req.TradeType) {
			snapshotErr := errors.New("BEPUSDT 支付网关返回快照无效")
			markInvoicePaymentFailed(record, snapshotErr)
			common.ApiErrorMsg(c, fmt.Sprintf("支付网关返回快照无效，支付记录 %s 已保留", record.SourceId))
			return
		}
		if payment.Data.Amount != "" {
			returnedAmount, parseErr := decimal.NewFromString(payment.Data.Amount)
			expectedAmount := decimal.NewFromInt(record.PaymentAmountMinor).Div(decimal.NewFromInt(100))
			if parseErr != nil || !returnedAmount.Equal(expectedAmount) {
				markInvoicePaymentFailed(record, errors.New("BEPUSDT 支付网关返回金额不匹配"))
				common.ApiErrorMsg(c, fmt.Sprintf("支付网关返回金额不匹配，支付记录 %s 已保留", record.SourceId))
				return
			}
		}
		record, err = model.UpdateInvoicePaymentProviderSnapshot(record.SourceId, model.InvoicePaymentCallback{
			PaymentProvider: model.PaymentProviderBepusdt, PaymentMethod: model.PaymentMethodBepusdt,
			ProviderOrderId: payment.Data.TradeId, ProviderAmount: invoicePaymentCNYAmount(record),
			ProviderCurrency: "CNY", ProviderPayload: common.GetJsonString(payment.Data),
		})
		if err != nil {
			markInvoicePaymentFailed(record, err)
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, invoicePaymentResponse(record, gin.H{"type": "redirect", "url": payment.Data.PaymentUrl}, invoicePaymentCNYText(record)))
	case model.PaymentProviderOkpay:
		fiatAmount := decimal.NewFromInt(record.PaymentAmountMinor).Div(decimal.NewFromInt(100)).InexactFloat64()
		paymentAmount := getOkpayPaymentAmountFromFiat(fiatAmount)
		amount := decimal.NewFromFloat(paymentAmount.CoinAmount).StringFixed(8)
		record, err = model.UpdateInvoicePaymentProviderSnapshot(record.SourceId, model.InvoicePaymentCallback{
			PaymentProvider: model.PaymentProviderOkpay, PaymentMethod: model.PaymentMethodOkpay,
			ProviderMerchantId: setting.OkpayMerchantId, ProviderAmount: amount,
			ProviderCurrency: paymentAmount.Coin,
		})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		payment, err := createOkpayPaymentLink(
			c, record.SourceId, paymentAmount, "Invoice-"+record.SourceId,
			callbackBase+"/api/invoice/okpay/notify", paymentReturnPath("/console/invoice?pay=pending"),
		)
		if err != nil {
			markInvoicePaymentFailed(record, err)
			common.ApiErrorMsg(c, fmt.Sprintf("拉起支付失败，支付记录 %s 已保留", record.SourceId))
			return
		}
		record, err = model.UpdateInvoicePaymentProviderSnapshot(record.SourceId, model.InvoicePaymentCallback{
			PaymentProvider: model.PaymentProviderOkpay, PaymentMethod: model.PaymentMethodOkpay,
			ProviderOrderId: payment.ProviderOrderId, ProviderMerchantId: setting.OkpayMerchantId,
			ProviderAmount: payment.Amount, ProviderCurrency: payment.PaymentAmount.Coin,
		})
		if err != nil {
			markInvoicePaymentFailed(record, err)
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, invoicePaymentResponse(record, gin.H{"type": "redirect", "url": payment.PaymentUrl}, fmt.Sprintf("%s %s", payment.Amount, payment.PaymentAmount.Coin)))
	}
}

func CancelInvoiceExternalPayment(c *gin.Context) {
	record, err := model.CancelInvoiceExternalPayment(c.GetInt("id"), c.Param("trade_no"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, record)
}

func GetInvoiceExternalPayment(c *gin.Context) {
	record, err := model.GetUserInvoicePaymentByTradeNo(c.GetInt("id"), c.Param("trade_no"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	amountText := invoicePaymentCNYText(record)
	if record.PaymentProvider == model.PaymentProviderOkpay && record.ProviderAmount != "" && record.ProviderCurrency != "" {
		amountText = record.ProviderAmount + " " + record.ProviderCurrency
	}
	common.ApiSuccess(c, invoicePaymentResponse(record, nil, amountText))
}

func parseInvoiceEpayParams(c *gin.Context) (map[string]string, error) {
	if err := c.Request.ParseForm(); err != nil {
		return nil, err
	}
	params := make(map[string]string, len(c.Request.Form))
	for key := range c.Request.Form {
		params[key] = c.Request.Form.Get(key)
	}
	return params, nil
}

func completeInvoiceEpay(params map[string]string) (*model.InvoiceRecord, error) {
	client := GetEpayClient()
	if client == nil {
		return nil, errors.New("易支付未配置")
	}
	verified, err := client.Verify(params)
	if err != nil || !verified.VerifyStatus {
		return nil, errors.New("易支付回调验签失败")
	}
	if verified.TradeStatus != epay.StatusTradeSuccess {
		return nil, nil
	}
	LockOrder(verified.ServiceTradeNo)
	defer UnlockOrder(verified.ServiceTradeNo)
	record, _, err := model.CompleteInvoiceExternalPayment(verified.ServiceTradeNo, model.InvoicePaymentCallback{
		PaymentProvider: model.PaymentProviderEpay, PaymentMethod: verified.Type,
		ProviderOrderId: verified.TradeNo, ProviderMerchantId: params["pid"],
		ProviderAmount: verified.Money, ProviderCurrency: "CNY",
		ProviderPayload: common.GetJsonString(params),
	})
	return record, err
}

func InvoiceEpayNotify(c *gin.Context) {
	if !isEpayWebhookConfigured() {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	params, err := parseInvoiceEpayParams(c)
	if err != nil || len(params) == 0 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	record, err := completeInvoiceEpay(params)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("发票易支付回调拒绝 error=%q", err.Error()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	_ = record
	_, _ = c.Writer.Write([]byte("success"))
}

func InvoiceEpayReturn(c *gin.Context) {
	params, err := parseInvoiceEpayParams(c)
	if err != nil || len(params) == 0 || !isEpayWebhookConfigured() {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/invoice?pay=fail"))
		return
	}
	record, err := completeInvoiceEpay(params)
	if err != nil {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/invoice?pay=fail"))
		return
	}
	if record == nil {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/invoice?pay=pending"))
		return
	}
	c.Redirect(http.StatusFound, paymentReturnPath("/console/invoice?pay=success"))
}

func InvoiceBepusdtNotify(c *gin.Context) {
	if !isBepusdtWebhookConfigured() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	parsed, err := parseBepusdtNotifyPayload(c)
	if err != nil || len(parsed.Params) == 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if !verifyBepusdtNotifyParamsSignature(parsed.Params, setting.BepusdtAuthToken) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	payload := parsed.Payload
	if payload.Status == 3 {
		_ = model.UpdateInvoiceExternalPaymentStatus(payload.OrderId, model.PaymentProviderBepusdt, model.InvoicePaymentStatusExpired, common.GetJsonString(parsed.Params))
		_, _ = c.Writer.Write([]byte("ok"))
		return
	}
	if payload.Status != 2 {
		_, _ = c.Writer.Write([]byte("ok"))
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(parsed.Params["fiat"]))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(parsed.Params["currency"]))
	}
	if currency == "" {
		currency = "CNY"
	}
	LockOrder(payload.OrderId)
	defer UnlockOrder(payload.OrderId)
	_, _, err = model.CompleteInvoiceExternalPayment(payload.OrderId, model.InvoicePaymentCallback{
		PaymentProvider: model.PaymentProviderBepusdt, PaymentMethod: model.PaymentMethodBepusdt,
		ProviderOrderId: payload.TradeId, ProviderAmount: fmt.Sprintf("%v", payload.Amount),
		ProviderCurrency: currency, ProviderPayload: common.GetJsonString(parsed.Params),
	})
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("发票 BEPUSDT 回调拒绝 trade_no=%s error=%q", payload.OrderId, err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	_, _ = c.Writer.Write([]byte("ok"))
}

func InvoiceOkpayNotify(c *gin.Context) {
	if !isOkpayWebhookConfigured() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	values, body, err := parseOkpayCallbackValues(c)
	if err != nil || len(values) == 0 {
		writeOkpayCallbackStatus(c, false)
		return
	}
	orderedSource := body
	if len(strings.TrimSpace(string(orderedSource))) == 0 {
		orderedSource = []byte(c.Request.URL.RawQuery)
	}
	if !verifyOkpayCallbackSignature(values, setting.OkpayMerchantToken, parseOkpayCallbackOrderedPairs(orderedSource)) {
		writeOkpayCallbackStatus(c, false)
		return
	}
	requestStatus := values.Get("status")
	paymentStatus := getOkpayCallbackValue(values, "data[status]", "payment_status", "trade_status", "order_status")
	if !isOkpayCallbackSuccess(requestStatus, paymentStatus) {
		writeOkpayCallbackStatus(c, true)
		return
	}
	uniqueID := getOkpayCallbackValue(values, "data[unique_id]", "unique_id", "trade_no", "out_trade_no")
	providerOrderID := getOkpayCallbackValue(values, "data[order_id]", "order_id", "trade_id")
	tradeNo := uniqueID
	if tradeNo == "" && providerOrderID != "" {
		record, lookupErr := model.GetInvoicePaymentByProviderOrderId(model.PaymentProviderOkpay, providerOrderID)
		if lookupErr != nil {
			writeOkpayCallbackStatus(c, false)
			return
		}
		tradeNo = record.SourceId
	}
	if tradeNo == "" {
		writeOkpayCallbackStatus(c, false)
		return
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	_, _, err = model.CompleteInvoiceExternalPayment(tradeNo, model.InvoicePaymentCallback{
		PaymentProvider: model.PaymentProviderOkpay, PaymentMethod: model.PaymentMethodOkpay,
		ProviderOrderId: providerOrderID, ProviderMerchantId: values.Get("id"),
		ProviderAmount: values.Get("data[amount]"), ProviderCurrency: values.Get("data[coin]"),
		ProviderPayload: common.GetJsonString(values),
	})
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("发票 OKPay 回调拒绝 trade_no=%s error=%q", tradeNo, err.Error()))
		writeOkpayCallbackStatus(c, false)
		return
	}
	writeOkpayCallbackStatus(c, true)
}
