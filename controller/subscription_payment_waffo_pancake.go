package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

type SubscriptionWaffoPancakePayRequest struct {
	PlanId    int                  `json:"plan_id"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

func SubscriptionRequestWaffoPancakePay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionWaffoPancakePayRequest
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

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

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

	// WAFFO_PANCAKE_SUB- prefix (vs. wallet's WAFFO_PANCAKE-) drives webhook
	// dispatch in WaffoPancakeWebhook.
	tradeNo := fmt.Sprintf("WAFFO_PANCAKE_SUB-%d-%d-%s", userId, time.Now().UnixMilli(), randstr.String(6))
	discount, err := model.CalculatePromoCodeDiscount(req.PromoCode, model.PromoCodeTargetSubscription, plan.Id, planPriceUSD)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	payMoney := planPriceUSD
	if discount != nil {
		payMoney = discount.PaidAmount
	}
	if payMoney < 0 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderWaffoPancake, payMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	totalPayMoney := payMoney
	if invoiceAmounts.Required {
		totalPayMoney = invoiceAmounts.TotalPayment
	}

	if totalPayMoney >= 0.01 {
		if strings.TrimSpace(plan.WaffoPancakeProductId) == "" {
			common.ApiErrorMsg(c, "该套餐未配置 WaffoPancakeProductId")
			return
		}
		// Plan targets its own Pancake product, so we only require credentials
		// here — not the gateway-level WaffoPancakeProductID.
		if strings.TrimSpace(setting.WaffoPancakeMerchantID) == "" ||
			strings.TrimSpace(setting.WaffoPancakePrivateKey) == "" {
			common.ApiErrorMsg(c, "Waffo Pancake 未配置或密钥无效")
			return
		}
	}

	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           totalPayMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	model.ApplyPromoCodeResultToSubscriptionOrder(order, discount)
	applyInvoiceToSubscriptionOrder(order, invoiceAmounts, planPriceUSD, payMoney, subscriptionPaidQuotaFromUSD(payMoney))
	if err := order.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅订单创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	if totalPayMoney < 0.01 {
		if err := model.CompleteFreeSubscriptionOrder(tradeNo, model.PaymentProviderWaffoPancake); err != nil {
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

	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(c.Request.Context(), &service.WaffoPancakeCreateSessionParams{
		ProductID:     plan.WaffoPancakeProductId,
		BuyerIdentity: service.WaffoPancakeBuyerIdentityFromUserID(user.Id),
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      decimal.NewFromFloat(totalPayMoney).StringFixed(2),
			TaxCategory: "saas",
		},
		BuyerEmail:              getWaffoPancakeBuyerEmail(user),
		ExpiresInSeconds:        &expiresInSeconds,
		OrderMerchantExternalID: tradeNo,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅结账会话创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		order.Status = common.TopUpStatusFailed
		_ = order.Update()
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅订单创建成功 user_id=%d plan_id=%d trade_no=%s session_id=%s money=%.2f", userId, plan.Id, tradeNo, session.SessionID, totalPayMoney))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url":     session.CheckoutURL,
			"session_id":       session.SessionID,
			"expires_at":       session.ExpiresAt,
			"order_id":         tradeNo,
			"token":            session.Token,
			"token_expires_at": session.TokenExpiresAt,
		},
	})
}
