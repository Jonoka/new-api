package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/waffo-com/waffo-go/types/order"
)

func GetTopUpInfo(c *gin.Context) {
	complianceConfirmed := operation_setting.IsPaymentComplianceConfirmed()

	// 获取支付方式
	payMethods := operation_setting.PayMethods
	if !complianceConfirmed {
		payMethods = []map[string]string{}
	}

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if isStripeTopUpEnabled() {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.Itoa(setting.StripeMinTopUp),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	// Waffo Pancake displayed above the legacy Waffo gateway.
	enableWaffoPancake := isWaffoPancakeTopUpEnabled()
	if enableWaffoPancake {
		hasWaffoPancake := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffoPancake {
				hasWaffoPancake = true
				break
			}
		}

		if !hasWaffoPancake {
			payMethods = append(payMethods, map[string]string{
				"name":      "Waffo Pancake",
				"type":      model.PaymentMethodWaffoPancake,
				"color":     "rgba(var(--semi-orange-5), 1)",
				"min_topup": strconv.Itoa(setting.WaffoPancakeMinTopUp),
			})
		}
	}

	// 如果启用了 Waffo 支付，添加到支付方法列表
	enableWaffo := isWaffoTopUpEnabled()
	if enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffo {
				hasWaffo = true
				break
			}
		}

		if !hasWaffo {
			waffoMethod := map[string]string{
				"name":      "Waffo (Global Payment)",
				"type":      model.PaymentMethodWaffo,
				"color":     "rgba(var(--semi-blue-5), 1)",
				"min_topup": strconv.Itoa(setting.WaffoMinTopUp),
			}
			payMethods = append(payMethods, waffoMethod)
		}
	}

	// 如果启用了 Bepusdt (USDT) 支付，添加到支付方法列表
	enableBepusdt := isBepusdtTopUpEnabled()
	var bepusdtChains []setting.BepusdtChain
	if enableBepusdt {
		bepusdtChains = setting.GetBepusdtChains()
		hasBepusdt := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodBepusdt {
				hasBepusdt = true
				break
			}
		}
		if !hasBepusdt {
			payMethods = append(payMethods, map[string]string{
				"name":      "USDT",
				"type":      model.PaymentMethodBepusdt,
				"color":     "#26A17B",
				"min_topup": strconv.Itoa(setting.BepusdtMinTopUp),
			})
		}
	}

	// 如果启用了 OKPay 支付，添加到支付方法列表
	enableOkpay := isOkpayTopUpEnabled()
	if enableOkpay {
		hasOkpay := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodOkpay {
				hasOkpay = true
				break
			}
		}
		if !hasOkpay {
			payMethods = append(payMethods, map[string]string{
				"name":      "OKPay",
				"type":      model.PaymentMethodOkpay,
				"color":     "#4F46E5",
				"min_topup": strconv.Itoa(setting.OkpayMinTopUp),
			})
		}
	}

	data := gin.H{
		"enable_online_topup":              isEpayTopUpEnabled(),
		"enable_stripe_topup":              isStripeTopUpEnabled(),
		"enable_creem_topup":               isCreemTopUpEnabled(),
		"enable_waffo_topup":               enableWaffo,
		"enable_waffo_pancake_topup":       enableWaffoPancake,
		"enable_bepusdt_topup":             enableBepusdt,
		"enable_okpay_topup":               enableOkpay,
		"enable_redemption":                complianceConfirmed,
		"payment_compliance_confirmed":     complianceConfirmed,
		"payment_compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products":          setting.CreemProducts,
		"bepusdt_chains":          bepusdtChains,
		"bepusdt_min_topup":       setting.BepusdtMinTopUp,
		"okpay_min_topup":         setting.OkpayMinTopUp,
		"pay_methods":             payMethods,
		"min_topup":               operation_setting.MinTopUp,
		"stripe_min_topup":        setting.StripeMinTopUp,
		"waffo_min_topup":         setting.WaffoMinTopUp,
		"waffo_pancake_min_topup": setting.WaffoPancakeMinTopUp,
		"amount_options":          operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                operation_setting.GetPaymentSetting().AmountDiscount,
		"topup_link":              common.TopUpLink,
		"invoice":                 model.InvoiceConfigSnapshot(),
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64                `json:"amount"`
	PaymentMethod string               `json:"payment_method"`
	PromoCode     string               `json:"promo_code"`
	Invoice       model.InvoiceRequest `json:"invoice"`
}

type AmountRequest struct {
	Amount    int64                `json:"amount"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

type RetryTopUpPaymentRequest struct {
	TradeNo string `json:"trade_no"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func getPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(operation_setting.Price)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getEpayPayMoneyFromUSD(amount float64) float64 {
	return decimal.NewFromFloat(amount).
		Mul(decimal.NewFromFloat(operation_setting.Price)).
		Round(2).
		InexactFloat64()
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func normalizeTopUpAmountForStorage(amount int64) int64 {
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return amount
	}
	dAmount := decimal.NewFromInt(amount)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	return dAmount.Div(dQuotaPerUnit).IntPart()
}

func freeTopUpResponse(topUp *model.TopUp, quotaToAdd int, discount *model.PromoCodeDiscountResult) gin.H {
	data := gin.H{
		"completed":    true,
		"trade_no":     topUp.TradeNo,
		"quota_to_add": quotaToAdd,
	}
	if discount != nil {
		data["discount"] = discount
	}
	return gin.H{
		"message":   "success",
		"data":      data,
		"completed": true,
	}
}

func ensureRetryableTopUpForUser(c *gin.Context, tradeNo string) (*model.TopUp, bool) {
	if tradeNo == "" {
		common.ApiErrorMsg(c, "未提供订单号")
		return nil, false
	}
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.UserId != c.GetInt("id") {
		common.ApiErrorMsg(c, "充值订单不存在")
		return nil, false
	}
	if topUp.Status != common.TopUpStatusPending {
		common.ApiErrorMsg(c, "订单状态不是待支付，无法重新支付")
		return nil, false
	}
	if topUp.Money < 0.01 {
		common.ApiErrorMsg(c, "0 元订单无需重新支付")
		return nil, false
	}
	return topUp, true
}

func retryStripeGatewayPayMoney(topUp *model.TopUp) float64 {
	if topUp == nil {
		return 0
	}
	payMoney := topUp.ActualMoney
	if payMoney <= 0 {
		payMoney = topUp.Money
	}
	if topUp.InvoiceRequired && topUp.InvoiceFeeAmount > 0 {
		payMoney = decimal.NewFromFloat(payMoney).
			Add(decimal.NewFromFloat(model.AmountCNYToPaymentCurrency(topUp.InvoiceFeeAmount, model.PaymentProviderStripe))).
			Round(2).
			InexactFloat64()
	}
	return payMoney
}

func retryEpayTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	if !operation_setting.ContainsPayMethod(topUp.PaymentMethod) {
		common.ApiErrorMsg(c, "支付方式不存在")
		return
	}
	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置支付信息")
		return
	}
	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(paymentReturnPath("/console/log"))
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           topUp.PaymentMethod,
		ServiceTradeNo: topUp.TradeNo,
		Name:           fmt.Sprintf("TUC%d", topUp.Amount),
		Money:          strconv.FormatFloat(topUp.Money, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 重新拉起支付失败 user_id=%d trade_no=%s payment_method=%s error=%q", topUp.UserId, topUp.TradeNo, topUp.PaymentMethod, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 重新拉起支付成功 user_id=%d trade_no=%s payment_method=%s money=%.2f uri=%q", topUp.UserId, topUp.TradeNo, topUp.PaymentMethod, topUp.Money, uri))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func retryStripeTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	user, err := model.GetUserById(topUp.UserId, false)
	if err != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	payMoney := retryStripeGatewayPayMoney(topUp)
	payLink, err := genStripeLink(topUp.TradeNo, user.StripeCustomer, user.Email, topUp.Amount, payMoney, "", "")
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 重新创建 Checkout Session 失败 user_id=%d trade_no=%s error=%q", topUp.UserId, topUp.TradeNo, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Stripe 重新拉起支付成功 user_id=%d trade_no=%s pay_money=%.2f", topUp.UserId, topUp.TradeNo, payMoney))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func retryCreemTopUpPayment(c *gin.Context, _ *model.TopUp) {
	common.ApiErrorMsg(c, "Creem 固定产品订单暂不支持重新支付，请重新选择产品下单")
}

func retryWaffoTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	if !setting.WaffoEnabled {
		common.ApiErrorMsg(c, "Waffo 支付未启用")
		return
	}
	user, err := model.GetUserById(topUp.UserId, false)
	if err != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	sdk, err := getWaffoSDK()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo 重新支付 SDK 初始化失败 user_id=%d trade_no=%s error=%q", topUp.UserId, topUp.TradeNo, err.Error()))
		common.ApiErrorMsg(c, "支付配置错误")
		return
	}

	callbackAddr := service.GetCallbackAddress()
	notifyUrl := callbackAddr + "/api/waffo/webhook"
	if setting.WaffoNotifyUrl != "" {
		notifyUrl = setting.WaffoNotifyUrl
	}
	returnUrl := paymentReturnPath("/console/topup?show_history=true")
	if setting.WaffoReturnUrl != "" {
		returnUrl = setting.WaffoReturnUrl
	}

	currency := getWaffoCurrency()
	createParams := &order.CreateOrderParams{
		PaymentRequestID: topUp.TradeNo,
		MerchantOrderID:  topUp.TradeNo,
		OrderAmount:      formatWaffoAmount(topUp.Money, currency),
		OrderCurrency:    currency,
		OrderDescription: fmt.Sprintf("Recharge %d credits", topUp.Amount),
		OrderRequestedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		NotifyURL:        notifyUrl,
		MerchantInfo: &order.MerchantInfo{
			MerchantID: setting.WaffoMerchantId,
		},
		UserInfo: &order.UserInfo{
			UserID:       strconv.Itoa(user.Id),
			UserEmail:    getWaffoUserEmail(user),
			UserTerminal: "WEB",
		},
		PaymentInfo: &order.PaymentInfo{
			ProductName: "ONE_TIME_PAYMENT",
		},
		SuccessRedirectURL: returnUrl,
		FailedRedirectURL:  returnUrl,
	}
	resp, err := sdk.Order().Create(c.Request.Context(), createParams, nil)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo 重新创建订单失败 user_id=%d trade_no=%s error=%q", topUp.UserId, topUp.TradeNo, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if !resp.IsSuccess() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo 重新创建订单业务失败 user_id=%d trade_no=%s code=%s message=%q response=%q", topUp.UserId, topUp.TradeNo, resp.Code, resp.Message, common.GetJsonString(resp)))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	orderData := resp.GetData()
	paymentUrl := orderData.FetchRedirectURL()
	if paymentUrl == "" {
		paymentUrl = orderData.OrderAction
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo 重新拉起支付成功 user_id=%d trade_no=%s money=%.2f", topUp.UserId, topUp.TradeNo, topUp.Money))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url": paymentUrl,
			"order_id":    topUp.TradeNo,
		},
	})
}

func retryWaffoPancakeTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	if !isWaffoPancakeTopUpEnabled() {
		common.ApiErrorMsg(c, "Waffo Pancake 配置不完整")
		return
	}
	user, err := model.GetUserById(topUp.UserId, false)
	if err != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(c.Request.Context(), &service.WaffoPancakeCreateSessionParams{
		ProductID:     setting.WaffoPancakeProductID,
		BuyerIdentity: getWaffoPancakeBuyerIdentity(user),
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      formatWaffoPancakeAmount(topUp.Money),
			TaxCategory: "saas",
		},
		BuyerEmail:              getWaffoPancakeBuyerEmail(user),
		ExpiresInSeconds:        &expiresInSeconds,
		OrderMerchantExternalID: topUp.TradeNo,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 重新创建结账会话失败 user_id=%d trade_no=%s error=%q", topUp.UserId, topUp.TradeNo, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 重新拉起支付成功 user_id=%d trade_no=%s session_id=%s money=%.2f", topUp.UserId, topUp.TradeNo, session.SessionID, topUp.Money))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url":     session.CheckoutURL,
			"session_id":       session.SessionID,
			"expires_at":       session.ExpiresAt,
			"order_id":         topUp.TradeNo,
			"token":            session.Token,
			"token_expires_at": session.TokenExpiresAt,
		},
	})
}

func retryBepusdtTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	chains := setting.GetBepusdtChains()
	if len(chains) == 0 {
		common.ApiErrorMsg(c, "管理员未配置 USDT 链")
		return
	}
	callBackAddress := service.GetCallbackAddress()
	notifyUrl := callBackAddress + "/api/bepusdt/notify"
	redirectUrl := paymentReturnPath("/console/log")
	tradeType := chains[0].TradeType
	paymentUrl, err := createBepusdtTransaction(c, topUp.TradeNo, topUp.Money, tradeType, notifyUrl, redirectUrl)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 重新拉起支付失败 user_id=%d trade_no=%s trade_type=%s money=%.2f error=%q", topUp.UserId, topUp.TradeNo, tradeType, topUp.Money, err.Error()))
		common.ApiErrorMsg(c, fmt.Sprintf("拉起支付失败: %s", err.Error()))
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt 重新拉起支付成功 user_id=%d trade_no=%s trade_type=%s money=%.2f CNY", topUp.UserId, topUp.TradeNo, tradeType, topUp.Money))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url": paymentUrl,
			"trade_no":    topUp.TradeNo,
		},
	})
}

func retryOkpayTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	callBackAddress := service.GetCallbackAddress()
	callbackUrl := callBackAddress + "/api/okpay/notify"
	redirectUrl := paymentReturnPath("/console/log")
	paymentAmount := getOkpayPaymentAmountFromFiat(topUp.Money)
	payment, err := createOkpayPaymentLink(c, topUp.TradeNo, paymentAmount, fmt.Sprintf("TopUp-%s", topUp.TradeNo), callbackUrl, redirectUrl)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 重新拉起支付失败 user_id=%d trade_no=%s error=%q", topUp.UserId, topUp.TradeNo, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.UpdateTopUpProviderSnapshot(topUp.TradeNo, model.PaymentProviderOkpay, payment.ProviderOrderId, payment.Amount, payment.PaymentAmount.Coin); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 重新支付保存网关快照失败 user_id=%d trade_no=%s provider_order_id=%s error=%q", topUp.UserId, topUp.TradeNo, payment.ProviderOrderId, err.Error()))
		common.ApiErrorMsg(c, "保存支付订单失败")
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 重新拉起支付成功 user_id=%d trade_no=%s provider_order_id=%s fiat_money=%.2f CNY coin_amount=%s coin=%s", topUp.UserId, topUp.TradeNo, payment.ProviderOrderId, topUp.Money, payment.Amount, payment.PaymentAmount.Coin))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url":       payment.PaymentUrl,
			"trade_no":          topUp.TradeNo,
			"provider_order_id": payment.ProviderOrderId,
			"amount":            payment.Amount,
			"amount_text":       fmt.Sprintf("%s %s", payment.Amount, payment.PaymentAmount.Coin),
			"coin":              payment.PaymentAmount.Coin,
			"fiat_amount":       strconv.FormatFloat(payment.PaymentAmount.FiatAmount, 'f', 2, 64),
			"fiat_currency":     "CNY",
			"rate":              strconv.FormatFloat(payment.PaymentAmount.Rate, 'f', -1, 64),
			"rate_source":       payment.PaymentAmount.RateSource,
			"auto_rate_failed":  payment.PaymentAmount.AutoRateFailed,
		},
	})
}

func RetryTopUpPayment(c *gin.Context) {
	var req RetryTopUpPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	tradeNo := strings.TrimSpace(req.TradeNo)
	if tradeNo != "" {
		LockOrder(tradeNo)
		defer UnlockOrder(tradeNo)
	}
	topUp, ok := ensureRetryableTopUpForUser(c, tradeNo)
	if !ok {
		return
	}

	switch topUp.PaymentProvider {
	case model.PaymentProviderEpay, "":
		retryEpayTopUpPayment(c, topUp)
	case model.PaymentProviderStripe:
		retryStripeTopUpPayment(c, topUp)
	case model.PaymentProviderCreem:
		retryCreemTopUpPayment(c, topUp)
	case model.PaymentProviderWaffo:
		retryWaffoTopUpPayment(c, topUp)
	case model.PaymentProviderWaffoPancake:
		retryWaffoPancakeTopUpPayment(c, topUp)
	case model.PaymentProviderBepusdt:
		retryBepusdtTopUpPayment(c, topUp)
	case model.PaymentProviderOkpay:
		retryOkpayTopUpPayment(c, topUp)
	default:
		common.ApiErrorMsg(c, "不支持的支付渠道")
	}
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	originalPayMoney := payMoney
	discount, err := model.CalculatePromoCodeDiscount(req.PromoCode, model.PromoCodeTargetTopUp, 0, payMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if discount != nil {
		payMoney = discount.PaidAmount
	}
	if payMoney < 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderEpay, payMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	totalPayMoney := payMoney
	if invoiceAmounts.Required {
		totalPayMoney = invoiceAmounts.TotalPayment
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(paymentReturnPath("/console/log"))
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil && totalPayMoney >= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	amount := normalizeTopUpAmountForStorage(req.Amount)
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           totalPayMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	model.ApplyPromoCodeResultToTopUp(topUp, discount)
	applyInvoiceToTopUp(topUp, invoiceAmounts, originalPayMoney, payMoney, true)
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 创建充值订单失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	if totalPayMoney < 0.01 {
		completedTopUp, quotaToAdd, completedNow, err := model.CompleteFreeTopUp(tradeNo, model.PaymentProviderEpay)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 0元优惠充值完成失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
			return
		}
		if completedNow {
			model.RecordTopupLog(completedTopUp.UserId, fmt.Sprintf("使用优惠码充值成功，充值金额: %v，支付金额：0.00", logger.LogQuota(quotaToAdd)), c.ClientIP(), completedTopUp.PaymentMethod, "promo")
		}
		c.JSON(http.StatusOK, freeTopUpResponse(completedTopUp, quotaToAdd, discount))
		return
	}

	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          strconv.FormatFloat(totalPayMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderEpay, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 拉起支付失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值订单创建成功 user_id=%d trade_no=%s payment_method=%s amount=%d money=%.2f uri=%q params=%q", id, tradeNo, req.PaymentMethod, req.Amount, totalPayMoney, uri, common.GetJsonString(params)))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

func EpayNotify(c *gin.Context) {
	if !isEpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s params=%q", c.Request.RequestURI, c.ClientIP(), c.Request.Method, common.GetJsonString(params)))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	client := GetEpayClient()
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		return
	}
	verifyInfo, err := client.Verify(params)
	if err == nil && verifyInfo.VerifyStatus {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签成功 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))
	} else {
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.RequestURI, c.ClientIP()))
		}
		return
	}

	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		topUp := model.GetTopUpByTradeNo(verifyInfo.ServiceTradeNo)
		if topUp == nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调订单不存在 trade_no=%s callback_type=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, c.ClientIP(), common.GetJsonString(verifyInfo)))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		if topUp.PaymentProvider != model.PaymentProviderEpay {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 订单支付网关不匹配 trade_no=%s order_provider=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, topUp.PaymentProvider, verifyInfo.Type, c.ClientIP()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		actualPaymentMethod := ""
		if topUp.PaymentMethod != verifyInfo.Type {
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 实际支付方式与订单不同 trade_no=%s order_payment_method=%s actual_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, topUp.PaymentMethod, verifyInfo.Type, c.ClientIP()))
			actualPaymentMethod = verifyInfo.Type
		}
		completedTopUp, quotaToAdd, completedNow, err := model.CompleteEpayTopUp(verifyInfo.ServiceTradeNo, actualPaymentMethod)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 充值处理失败 trade_no=%s user_id=%d client_ip=%s error=%q topup=%q", topUp.TradeNo, topUp.UserId, c.ClientIP(), err.Error(), common.GetJsonString(topUp)))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		if completedNow {
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值成功 trade_no=%s user_id=%d client_ip=%s quota_to_add=%d money=%.2f topup=%q", completedTopUp.TradeNo, completedTopUp.UserId, c.ClientIP(), quotaToAdd, completedTopUp.Money, common.GetJsonString(completedTopUp)))
			model.RecordTopupLog(completedTopUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), completedTopUp.Money), c.ClientIP(), completedTopUp.PaymentMethod, "epay")
		}
		_, _ = c.Writer.Write([]byte("success"))
	} else {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))
		_, _ = c.Writer.Write([]byte("success"))
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	discount, err := model.CalculatePromoCodeDiscount(req.PromoCode, model.PromoCodeTargetTopUp, 0, payMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if discount != nil {
		payMoney = discount.PaidAmount
	}
	if payMoney < 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	invoiceAmounts, err := buildInvoicePaymentPreviewAmounts(req.Invoice, model.PaymentProviderEpay, payMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	totalPayMoney := payMoney
	if invoiceAmounts.Required {
		totalPayMoney = invoiceAmounts.TotalPayment
	}
	response := gin.H{"message": "success", "data": strconv.FormatFloat(totalPayMoney, 'f', 2, 64)}
	if discount != nil {
		response["discount"] = discount
	}
	addInvoiceFieldsToResponse(response, invoiceAmounts)
	c.JSON(http.StatusOK, response)
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
