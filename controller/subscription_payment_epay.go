package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type SubscriptionEpayPayRequest struct {
	PlanId        int    `json:"plan_id"`
	PaymentMethod string `json:"payment_method"`
	PromoCode     string `json:"promo_code"`
}

func SubscriptionRequestEpay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionEpayPayRequest
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
	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		common.ApiErrorMsg(c, "支付方式不存在")
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

	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("SUBUSR%dNO%s", userId, tradeNo)

	discount, err := model.CalculatePromoCodeDiscount(req.PromoCode, model.PromoCodeTargetSubscription, plan.Id, planPriceUSD)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	basePayMoney := planPriceUSD
	if discount != nil {
		basePayMoney = discount.PaidAmount
	}
	if basePayMoney < 0 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	payMoney := getEpayPayMoneyFromUSD(basePayMoney)
	epayDiscount := convertSubscriptionDiscountToEpayMoney(discount)

	var returnUrl *url.URL
	var notifyUrl *url.URL
	var client *epay.Client
	if payMoney >= 0.01 {
		callBackAddress := service.GetCallbackAddress()
		returnUrl, err = url.Parse(callBackAddress + "/api/subscription/epay/return")
		if err != nil {
			common.ApiErrorMsg(c, "回调地址配置错误")
			return
		}
		notifyUrl, err = url.Parse(callBackAddress + "/api/subscription/epay/notify")
		if err != nil {
			common.ApiErrorMsg(c, "回调地址配置错误")
			return
		}

		client = GetEpayClient()
		if client == nil {
			common.ApiErrorMsg(c, "当前管理员未配置支付信息")
			return
		}
	}

	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if discount == nil {
		order.AffiliateSourceQuota = subscriptionPaidQuotaFromUSD(basePayMoney)
	}
	model.ApplyPromoCodeResultToSubscriptionOrder(order, epayDiscount)
	if err := order.Insert(); err != nil {
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	if payMoney < 0.01 {
		if err := model.CompleteFreeSubscriptionOrder(tradeNo, model.PaymentProviderEpay); err != nil {
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

	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("SUB:%s", plan.Title),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderEpay)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func convertSubscriptionDiscountToEpayMoney(discount *model.PromoCodeDiscountResult) *model.PromoCodeDiscountResult {
	return convertSubscriptionDiscountAmount(discount, getEpayPayMoneyFromUSD)
}

func convertSubscriptionDiscountToBepusdtMoney(discount *model.PromoCodeDiscountResult) *model.PromoCodeDiscountResult {
	return convertSubscriptionDiscountAmount(discount, getBepusdtPayMoneyFromUSD)
}

func convertSubscriptionDiscountToPlanCurrency(discount *model.PromoCodeDiscountResult, currency string) (*model.PromoCodeDiscountResult, error) {
	if discount == nil {
		return nil, nil
	}
	converted, err := convertSubscriptionDiscountAmountWithError(discount, func(amount float64) (float64, error) {
		return model.SubscriptionPlanCurrencyAmountFromUSD(amount, currency)
	})
	if err != nil {
		return nil, err
	}
	return converted, nil
}

func convertSubscriptionDiscountAmount(discount *model.PromoCodeDiscountResult, convert func(float64) float64) *model.PromoCodeDiscountResult {
	converted, _ := convertSubscriptionDiscountAmountWithError(discount, func(amount float64) (float64, error) {
		return convert(amount), nil
	})
	return converted
}

func convertSubscriptionDiscountAmountWithError(discount *model.PromoCodeDiscountResult, convert func(float64) (float64, error)) (*model.PromoCodeDiscountResult, error) {
	if discount == nil {
		return nil, nil
	}
	converted := *discount
	var err error
	if converted.OriginalAmount, err = convert(discount.OriginalAmount); err != nil {
		return nil, err
	}
	if converted.PaidAmount, err = convert(discount.PaidAmount); err != nil {
		return nil, err
	}
	if converted.DiscountAmount, err = convert(discount.DiscountAmount); err != nil {
		return nil, err
	}
	// 返佣按套餐美元价折后金额计算，订单金额按实际网关实收金额记录。
	converted.ActualPaidQuota = discount.ActualPaidQuota
	return &converted, nil
}

func subscriptionPaidQuotaFromUSD(amount float64) int {
	if amount <= 0 {
		return 0
	}
	return int(amount * common.QuotaPerUnit)
}

func SubscriptionEpayNotify(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
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

	if len(params) == 0 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	client := GetEpayClient()
	if client == nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	LockOrder(verifyInfo.ServiceTradeNo)
	defer UnlockOrder(verifyInfo.ServiceTradeNo)

	if err := model.CompleteSubscriptionOrder(verifyInfo.ServiceTradeNo, common.GetJsonString(verifyInfo), model.PaymentProviderEpay, verifyInfo.Type); err != nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	_, _ = c.Writer.Write([]byte("success"))
}

// SubscriptionEpayReturn handles browser return after payment.
// It verifies the payload and completes the order, then redirects to console.
func SubscriptionEpayReturn(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
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

	if len(params) == 0 {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}

	client := GetEpayClient()
	if client == nil {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		if err := model.CompleteSubscriptionOrder(verifyInfo.ServiceTradeNo, common.GetJsonString(verifyInfo), model.PaymentProviderEpay, verifyInfo.Type); err != nil {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=success"))
		return
	}
	c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=pending"))
}
