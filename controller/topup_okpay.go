package controller

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// OkpayPayRequest 用户发起 OKPay 支付请求
type OkpayPayRequest struct {
	Amount    int64  `json:"amount"`
	PromoCode string `json:"promo_code"`
}

// OkpayAmountRequest 金额计算请求
type OkpayAmountRequest struct {
	Amount    int64  `json:"amount"`
	PromoCode string `json:"promo_code"`
}

// generateOkpaySignature 按 OKPay 规范生成 MD5 签名
// 1. 所有非空参数按 key 排序
// 2. 拼接 key=value&key=value
// 3. 末尾加 &token=MerchantToken
// 4. MD5 → 大写 hex
func generateOkpaySignature(params map[string]string, merchantToken string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		if strings.EqualFold(k, "sign") {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+strings.TrimSpace(params[k]))
	}
	query := strings.Join(pairs, "&")
	query += "&token=" + strings.TrimSpace(merchantToken)

	hash := md5.Sum([]byte(query))
	return strings.ToUpper(fmt.Sprintf("%x", hash))
}

// verifyOkpayCallbackSignature 验证回调签名
func verifyOkpayCallbackSignature(formValues url.Values, merchantToken string) bool {
	params := make(map[string]string)
	for key := range formValues {
		value := strings.TrimSpace(formValues.Get(key))
		if strings.EqualFold(key, "sign") || value == "" {
			continue
		}
		params[key] = value
	}
	expected := generateOkpaySignature(params, merchantToken)
	actual := strings.TrimSpace(formValues.Get("sign"))
	return strings.EqualFold(expected, actual)
}

// getOkpayPayMoney 计算 OKPay 支付金额
func getOkpayPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dExchangeRate := decimal.NewFromFloat(setting.OkpayExchangeRate)

	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dExchangeRate).Mul(dTopupGroupRatio).Mul(dDiscount)
	return payMoney.InexactFloat64()
}

// getOkpayMinTopup 获取最低充值额度
func getOkpayMinTopup() int64 {
	minTopup := setting.OkpayMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

// RequestOkpayAmount 计算 OKPay 支付金额
func RequestOkpayAmount(c *gin.Context) {
	var req OkpayAmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getOkpayMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getOkpayMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getOkpayPayMoney(req.Amount, group)
	discount, err := model.CalculatePromoCodeDiscount(req.PromoCode, model.PromoCodeTargetTopUp, 0, payMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if discount != nil {
		payMoney = discount.PaidAmount
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    strconv.FormatFloat(payMoney, 'f', 2, 64),
	})
}

// RequestOkpayPay 创建 OKPay 支付订单
func RequestOkpayPay(c *gin.Context) {
	var req OkpayPayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getOkpayMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getOkpayMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getOkpayPayMoney(req.Amount, group)
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

	tradeNo := fmt.Sprintf("USR%dNO%s%d", id, common.GetRandomString(6), time.Now().Unix())

	amount := normalizeTopUpAmountForStorage(req.Amount)
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodOkpay,
		PaymentProvider: model.PaymentProviderOkpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	model.ApplyPromoCodeResultToTopUp(topUp, discount)
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// 处理 0 元优惠订单
	if payMoney < 0.01 {
		completedTopUp, quotaToAdd, completedNow, err := model.CompleteFreeTopUp(tradeNo, model.PaymentProviderOkpay)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 0元优惠充值完成失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
			return
		}
		if completedNow {
			model.RecordTopupLog(completedTopUp.UserId, fmt.Sprintf("使用优惠码充值成功，充值金额: %v，支付金额：0.00", logger.LogQuota(quotaToAdd)), c.ClientIP(), completedTopUp.PaymentMethod, "promo")
		}
		c.JSON(http.StatusOK, freeTopUpResponse(completedTopUp, quotaToAdd, discount))
		return
	}

	// 调用 OKPay API 创建支付
	callBackAddress := service.GetCallbackAddress()
	callbackUrl := callBackAddress + "/api/okpay/notify"
	redirectUrl := paymentReturnPath("/console/log")

	// OKPay 金额需要 8 位小数
	dPayMoney := decimal.NewFromFloat(payMoney)

	payload := map[string]string{
		"unique_id":    tradeNo,
		"amount":       dPayMoney.StringFixed(8),
		"return_url":   redirectUrl,
		"callback_url": callbackUrl,
		"coin":         strings.ToUpper(strings.TrimSpace(setting.OkpayCoin)),
		"name":         fmt.Sprintf("TopUp-%s", tradeNo),
		"id":           setting.OkpayMerchantId,
	}
	payload["sign"] = generateOkpaySignature(payload, setting.OkpayMerchantToken)

	// form POST 到 OKPay
	gatewayUrl := strings.TrimRight(setting.OkpayGatewayUrl, "/") + "/payLink"
	formValues := url.Values{}
	for k, v := range payload {
		formValues.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.PostForm(gatewayUrl, formValues)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 请求失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 读取响应失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay API 响应 trade_no=%s status_code=%d body=%q", tradeNo, resp.StatusCode, string(body)))

	if resp.StatusCode/100 != 2 {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	var raw map[string]interface{}
	err = common.Unmarshal(body, &raw)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "解析支付响应失败"})
		return
	}

	// 提取 data.pay_url
	payUrl := ""
	if data, ok := raw["data"].(map[string]interface{}); ok {
		if u, ok := data["pay_url"].(string); ok {
			payUrl = strings.TrimSpace(u)
		}
	}
	// 兼容 data 为数组的情况
	if payUrl == "" {
		if items, ok := raw["data"].([]interface{}); ok && len(items) > 0 {
			if first, ok := items[0].(map[string]interface{}); ok {
				if u, ok := first["pay_url"].(string); ok {
					payUrl = strings.TrimSpace(u)
				}
			}
		}
	}

	if payUrl == "" {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 未返回 pay_url trade_no=%s body=%q", tradeNo, string(body)))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 充值订单创建成功 user_id=%d trade_no=%s amount=%d money=%.2f coin=%s", id, tradeNo, req.Amount, payMoney, setting.OkpayCoin))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url": payUrl,
			"trade_no":    tradeNo,
		},
	})
}

// OkpayNotify 处理 OKPay 回调通知
func OkpayNotify(c *gin.Context) {
	if !isOkpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay webhook 读取请求体失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay webhook 收到请求 client_ip=%s body=%q", c.ClientIP(), string(bodyBytes)))

	// 解析 form body
	formValues, err := url.ParseQuery(strings.TrimSpace(string(bodyBytes)))
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay webhook 解析 form 失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	sign := strings.TrimSpace(formValues.Get("sign"))
	if sign == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 缺少 sign client_ip=%s", c.ClientIP()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 验证签名
	if !verifyOkpayCallbackSignature(formValues, setting.OkpayMerchantToken) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 验签失败 client_ip=%s sign=%q", c.ClientIP(), sign))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	requestStatus := strings.TrimSpace(formValues.Get("status"))
	paymentStatus := strings.TrimSpace(formValues.Get("data[status]"))
	uniqueID := strings.TrimSpace(formValues.Get("data[unique_id]"))
	orderID := strings.TrimSpace(formValues.Get("data[order_id]"))

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay webhook 验签成功 unique_id=%s order_id=%s status=%s payment_status=%s client_ip=%s", uniqueID, orderID, requestStatus, paymentStatus, c.ClientIP()))

	// 判断支付状态: request status 必须为 "success"，data[status] 为 "1" 表示成功
	if !strings.EqualFold(requestStatus, "success") || paymentStatus != "1" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 订单非成功状态 unique_id=%s status=%s payment_status=%s", uniqueID, requestStatus, paymentStatus))
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	tradeNo := uniqueID
	if tradeNo == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 缺少 unique_id order_id=%s", orderID))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 充值订单不存在 trade_no=%s order_id=%s", tradeNo, orderID))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 充值订单状态非 pending，忽略 trade_no=%s status=%s", tradeNo, topUp.Status))
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	err = model.RechargeOkpay(tradeNo, c.ClientIP())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 充值处理失败 trade_no=%s order_id=%s client_ip=%s error=%q", tradeNo, orderID, c.ClientIP(), err.Error()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 充值成功 trade_no=%s order_id=%s amount=%s coin=%s client_ip=%s",
		tradeNo, orderID,
		strings.TrimSpace(formValues.Get("data[amount]")),
		strings.TrimSpace(formValues.Get("data[coin]")),
		c.ClientIP()))
	_, _ = c.Writer.Write([]byte("success"))
}
