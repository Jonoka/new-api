package controller

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func getOkpayFiatPayMoneyFromUSD(amountUSD float64) (float64, error) {
	rate := setting.OkpayExchangeRate
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, fmt.Errorf("OKPay 充值价格配置错误")
	}
	return decimal.NewFromFloat(amountUSD).
		Mul(decimal.NewFromFloat(rate)).
		Round(2).
		InexactFloat64(), nil
}

func getSubscriptionOkpayPayMoney(plan *model.SubscriptionPlan, amountUSD float64) (float64, error) {
	if plan == nil {
		return 0, fmt.Errorf("subscription plan is nil")
	}
	if model.NormalizeSubscriptionPlanCurrency(plan.Currency) == model.SubscriptionCurrencyCNY {
		return model.SubscriptionPlanCurrencyAmountFromUSD(amountUSD, model.SubscriptionCurrencyCNY)
	}
	return getOkpayFiatPayMoneyFromUSD(amountUSD)
}

func convertSubscriptionDiscountToOkpayMoney(plan *model.SubscriptionPlan, discount *model.PromoCodeDiscountResult) (*model.PromoCodeDiscountResult, error) {
	if discount == nil {
		return nil, nil
	}
	if model.NormalizeSubscriptionPlanCurrency(plan.Currency) == model.SubscriptionCurrencyCNY {
		return convertSubscriptionDiscountToCNYPlanMoney(plan, discount)
	}
	return convertSubscriptionDiscountAmountWithError(discount, getOkpayFiatPayMoneyFromUSD)
}

// SubscriptionRequestOkpayPay 创建 OKPay 订阅支付订单。
func SubscriptionRequestOkpayPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	if !isOkpayTopUpEnabled() {
		common.ApiErrorMsg(c, "OKPay 未配置或密钥无效")
		return
	}

	var req SubscriptionOkpayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	planPriceUSD, err := model.SubscriptionPlanPriceUSD(plan)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if planPriceUSD < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}

	userId := c.GetInt("id")
	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	discount, err := model.CalculatePromoCodeDiscount(req.PromoCode, model.PromoCodeTargetSubscription, plan.Id, planPriceUSD)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	paidUSD := planPriceUSD
	if discount != nil {
		paidUSD = discount.PaidAmount
	}
	if paidUSD < 0 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}

	okpayDiscount, err := convertSubscriptionDiscountToOkpayMoney(plan, discount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	payMoney, err := getSubscriptionOkpayPayMoney(plan, paidUSD)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if okpayDiscount != nil {
		payMoney = okpayDiscount.PaidAmount
	}

	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderOkpay, payMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	totalPayMoney := payMoney
	if invoiceAmounts.Required {
		totalPayMoney = invoiceAmounts.TotalPayment
	}
	paymentAmount := okpayPaymentAmount{}
	providerAmount := ""
	providerCurrency := ""
	if totalPayMoney >= 0.01 {
		paymentAmount = getOkpayPaymentAmountFromFiat(totalPayMoney)
		providerAmount = decimal.NewFromFloat(paymentAmount.CoinAmount).StringFixed(8)
		providerCurrency = strings.ToUpper(strings.TrimSpace(paymentAmount.Coin))
	}

	tradeNo := fmt.Sprintf("OKPAY_SUBUSR%dNO%s%d", userId, common.GetRandomString(6), time.Now().UnixNano())
	order := &model.SubscriptionOrder{
		UserId:           userId,
		PlanId:           plan.Id,
		Money:            totalPayMoney,
		TradeNo:          tradeNo,
		PaymentMethod:    model.PaymentMethodOkpay,
		PaymentProvider:  model.PaymentProviderOkpay,
		ProviderAmount:   providerAmount,
		ProviderCurrency: providerCurrency,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
	}
	if discount == nil {
		order.AffiliateSourceQuota = subscriptionPaidQuotaFromUSD(paidUSD)
	}
	model.ApplyPromoCodeResultToSubscriptionOrder(order, okpayDiscount)
	applyInvoiceToSubscriptionOrder(order, invoiceAmounts, payMoney, payMoney, subscriptionPaidQuotaFromUSD(paidUSD))
	if err := order.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 订阅订单创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}

	if totalPayMoney < 0.01 {
		if err := model.CompleteFreeSubscriptionOrder(tradeNo, model.PaymentProviderOkpay); err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":   "success",
			"completed": true,
			"data": gin.H{
				"completed": true,
				"trade_no":  tradeNo,
				"discount":  discount,
			},
		})
		return
	}

	callbackUrl := service.GetCallbackAddress() + "/api/okpay/notify"
	redirectUrl := paymentReturnPath("/console/topup")
	payment, err := createOkpayPaymentLink(c, tradeNo, paymentAmount, fmt.Sprintf("SUB:%s", plan.Title), callbackUrl, redirectUrl)
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderOkpay)
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 订阅拉起支付失败 user_id=%d plan_id=%d trade_no=%s money=%.2f error=%q", userId, plan.Id, tradeNo, totalPayMoney, err.Error()))
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.UpdateSubscriptionOrderProviderSnapshot(tradeNo, model.PaymentProviderOkpay, payment.ProviderOrderId, payment.Amount, payment.PaymentAmount.Coin); err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderOkpay)
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 保存订阅第三方订单号失败 user_id=%d plan_id=%d trade_no=%s provider_order_id=%s error=%q", userId, plan.Id, tradeNo, payment.ProviderOrderId, err.Error()))
		common.ApiErrorMsg(c, "保存支付订单失败")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 订阅订单创建成功 user_id=%d plan_id=%d trade_no=%s provider_order_id=%s fiat_money=%.2f CNY coin_amount=%s coin=%s", userId, plan.Id, tradeNo, payment.ProviderOrderId, totalPayMoney, payment.Amount, payment.PaymentAmount.Coin))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url":       payment.PaymentUrl,
			"trade_no":          tradeNo,
			"provider_order_id": payment.ProviderOrderId,
			"amount":            payment.Amount,
			"amount_text":       fmt.Sprintf("%s %s", payment.Amount, payment.PaymentAmount.Coin),
			"coin":              payment.PaymentAmount.Coin,
			"fiat_amount":       strconv.FormatFloat(payment.PaymentAmount.FiatAmount, 'f', 2, 64),
			"fiat_currency":     "CNY",
			"rate":              strconv.FormatFloat(payment.PaymentAmount.Rate, 'f', -1, 64),
			"rate_source":       payment.PaymentAmount.RateSource,
			"auto_rate_failed":  payment.PaymentAmount.AutoRateFailed,
			"discount":          discount,
			"invoice":           invoiceAmounts,
		},
	})
}
