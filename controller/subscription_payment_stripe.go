package controller

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/thanhpk/randstr"
)

type SubscriptionStripePayRequest struct {
	PlanId    int                  `json:"plan_id"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

func SubscriptionRequestStripePay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionStripePayRequest
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
	if plan.StripePriceId == "" && strings.TrimSpace(req.PromoCode) == "" {
		common.ApiErrorMsg(c, "该套餐未配置 StripePriceId")
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

	reference := fmt.Sprintf("sub-stripe-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "sub_ref_" + common.Sha1([]byte(reference))

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
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderStripe, payMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	totalPayMoney := payMoney
	if invoiceAmounts.Required {
		totalPayMoney = invoiceAmounts.TotalPayment
	}

	stripePriceId := plan.StripePriceId
	if discount != nil || invoiceAmounts.Required {
		stripePriceId = ""
	}
	if totalPayMoney >= 0.01 {
		if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
			common.ApiErrorMsg(c, "Stripe 未配置或密钥无效")
			return
		}
		if setting.StripeWebhookSecret == "" {
			common.ApiErrorMsg(c, "Stripe Webhook 未配置")
			return
		}
	}

	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           totalPayMoney,
		TradeNo:         referenceId,
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		RequestIP:       c.ClientIP(),
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	model.ApplyPromoCodeResultToSubscriptionOrder(order, discount)
	applyInvoiceToSubscriptionOrder(order, invoiceAmounts, planPriceUSD, payMoney, subscriptionPaidQuotaFromUSD(payMoney))
	if err := order.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	if totalPayMoney < 0.01 {
		if err := model.CompleteFreeSubscriptionOrder(referenceId, model.PaymentProviderStripe); err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":   "success",
			"completed": true,
			"data": gin.H{
				"completed": true,
				"trade_no":  referenceId,
				"discount":  discount,
			},
		})
		return
	}

	payLink, err := genStripeSubscriptionLink(referenceId, user.StripeCustomer, user.Email, stripePriceId, plan.Title, totalPayMoney)
	if err != nil {
		_ = model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅支付链接创建失败 trade_no=%s plan_id=%d error=%q", referenceId, plan.Id, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func genStripeSubscriptionLink(referenceId string, customerId string, email string, priceId string, planTitle string, payMoney float64) (string, error) {
	stripe.Key = setting.StripeApiSecret
	lineItem := &stripe.CheckoutSessionLineItemParams{
		Quantity: stripe.Int64(1),
	}
	mode := stripe.CheckoutSessionModeSubscription
	if priceId != "" {
		lineItem.Price = stripe.String(priceId)
	} else {
		mode = stripe.CheckoutSessionModePayment
		lineItem.PriceData = &stripe.CheckoutSessionLineItemPriceDataParams{
			Currency: stripe.String("usd"),
			ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
				Name: stripe.String(planTitle),
			},
			UnitAmount: stripe.Int64(int64(decimal.NewFromFloat(payMoney).Mul(decimal.NewFromInt(100)).Round(0).IntPart())),
		}
	}

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(paymentReturnPath("/console/topup")),
		CancelURL:         stripe.String(paymentReturnPath("/console/topup")),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			lineItem,
		},
		Mode: stripe.String(string(mode)),
	}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}
		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	result, err := session.New(params)
	if err != nil {
		return "", err
	}
	return result.URL, nil
}
