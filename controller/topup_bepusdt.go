package controller

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// BepusdtPayRequest 用户发起 USDT 支付请求
type BepusdtPayRequest struct {
	Amount    int64  `json:"amount"`
	TradeType string `json:"trade_type"` // e.g. "usdt.trc20"
	PromoCode string `json:"promo_code"`
}

// BepusdtAmountRequest 金额计算请求
type BepusdtAmountRequest struct {
	Amount    int64  `json:"amount"`
	PromoCode string `json:"promo_code"`
}

type SubscriptionBepusdtPayRequest struct {
	PlanId    int    `json:"plan_id"`
	TradeType string `json:"trade_type"`
	PromoCode string `json:"promo_code"`
}

// bepusdtCreateTransactionResp bepusdt API 响应
type bepusdtCreateTransactionResp struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       struct {
		Fiat           string `json:"fiat"`
		TradeType      string `json:"trade_type"`
		TradeId        string `json:"trade_id"`
		OrderId        string `json:"order_id"`
		Amount         string `json:"amount"`
		ActualAmount   string `json:"actual_amount"`
		Token          string `json:"token"`
		ExpirationTime int    `json:"expiration_time"`
		Status         int    `json:"status"`
		PaymentUrl     string `json:"payment_url"`
	} `json:"data"`
}

// bepusdtNotifyPayload 回调通知载荷
type bepusdtNotifyPayload struct {
	TradeId            string      `json:"trade_id"`
	OrderId            string      `json:"order_id"`
	Amount             interface{} `json:"amount"`
	ActualAmount       interface{} `json:"actual_amount"`
	Token              string      `json:"token"`
	BlockTransactionId string      `json:"block_transaction_id"`
	Signature          string      `json:"signature"`
	Status             int         `json:"status"`
}

// generateBepusdtSignature 按 bepusdt 规范生成 MD5 签名
// 1. 收集所有非空、非 signature 参数
// 2. 按 key ASCII 排序
// 3. 拼接 key=value& 格式
// 4. 末尾拼接 authToken（无 & 分隔符）
// 5. MD5 取小写 hex
func generateBepusdtSignature(params map[string]string, authToken string) string {
	// 收集并排序 key
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "signature" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 拼接
	var buf strings.Builder
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteString("=")
		buf.WriteString(params[k])
		buf.WriteString("&")
	}
	// 去掉最后一个 &，然后拼接 token
	s := buf.String()
	if len(s) > 0 {
		s = s[:len(s)-1] // 去掉末尾 &
	}
	s += authToken

	hash := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", hash)
}

// verifyBepusdtNotifySignature 验证回调签名
func verifyBepusdtNotifySignature(payload *bepusdtNotifyPayload, authToken string) bool {
	params := make(map[string]string)
	if payload.TradeId != "" {
		params["trade_id"] = payload.TradeId
	}
	if payload.OrderId != "" {
		params["order_id"] = payload.OrderId
	}
	if payload.Amount != nil {
		params["amount"] = fmt.Sprintf("%v", payload.Amount)
	}
	if payload.ActualAmount != nil {
		params["actual_amount"] = fmt.Sprintf("%v", payload.ActualAmount)
	}
	if payload.Token != "" {
		params["token"] = payload.Token
	}
	if payload.BlockTransactionId != "" {
		params["block_transaction_id"] = payload.BlockTransactionId
	}
	params["status"] = strconv.Itoa(payload.Status)

	expected := generateBepusdtSignature(params, authToken)
	return strings.EqualFold(expected, payload.Signature)
}

// getBepusdtPayMoney 计算 bepusdt 支付金额（CNY）
func getBepusdtPayMoney(amount int64, group string) float64 {
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
	dUnitPrice := decimal.NewFromFloat(setting.BepusdtUnitPrice)

	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dUnitPrice).Mul(dTopupGroupRatio).Mul(dDiscount)
	return payMoney.InexactFloat64()
}

func getBepusdtPayMoneyFromUSD(amount float64) float64 {
	return decimal.NewFromFloat(amount).
		Mul(decimal.NewFromFloat(setting.BepusdtUnitPrice)).
		Round(2).
		InexactFloat64()
}

// getBepusdtMinTopup 获取最低充值额度
func getBepusdtMinTopup() int64 {
	minTopup := setting.BepusdtMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

// isValidBepusdtTradeType 校验 trade_type 是否在配置的链列表中
func isValidBepusdtTradeType(tradeType string) bool {
	chains := setting.GetBepusdtChains()
	for _, chain := range chains {
		if chain.TradeType == tradeType {
			return true
		}
	}
	return false
}

// RequestBepusdtAmount 计算 bepusdt USDT 支付金额
func RequestBepusdtAmount(c *gin.Context) {
	var req BepusdtAmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getBepusdtMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getBepusdtMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getBepusdtPayMoney(req.Amount, group)
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

// RequestBepusdtPay 创建 bepusdt USDT 支付订单
func RequestBepusdtPay(c *gin.Context) {
	var req BepusdtPayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getBepusdtMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getBepusdtMinTopup())})
		return
	}

	if req.TradeType == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "请选择支付链"})
		return
	}

	if !isValidBepusdtTradeType(req.TradeType) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付链"})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getBepusdtPayMoney(req.Amount, group)
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

	// 生成订单号
	tradeNo := fmt.Sprintf("USR%dNO%s%d", id, common.GetRandomString(6), time.Now().Unix())

	amount := normalizeTopUpAmountForStorage(req.Amount)
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodBepusdt,
		PaymentProvider: model.PaymentProviderBepusdt,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	model.ApplyPromoCodeResultToTopUp(topUp, discount)
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 创建充值订单失败 user_id=%d trade_no=%s trade_type=%s amount=%d error=%q", id, tradeNo, req.TradeType, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// 处理 0 元优惠订单
	if payMoney < 0.01 {
		completedTopUp, quotaToAdd, completedNow, err := model.CompleteFreeTopUp(tradeNo, model.PaymentProviderBepusdt)
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 0元优惠充值完成失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
			return
		}
		if completedNow {
			model.RecordTopupLog(completedTopUp.UserId, fmt.Sprintf("使用优惠码充值成功，充值金额: %v，支付金额：0.00", logger.LogQuota(quotaToAdd)), c.ClientIP(), completedTopUp.PaymentMethod, "promo")
		}
		c.JSON(http.StatusOK, freeTopUpResponse(completedTopUp, quotaToAdd, discount))
		return
	}

	// 调用 bepusdt API 创建交易
	callBackAddress := service.GetCallbackAddress()
	notifyUrl := callBackAddress + "/api/bepusdt/notify"
	redirectUrl := paymentReturnPath("/console/log")

	paymentUrl, err := createBepusdtTransaction(c, tradeNo, payMoney, req.TradeType, notifyUrl, redirectUrl)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderBepusdt, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 拉起支付失败 user_id=%d trade_no=%s trade_type=%s amount=%d money=%.2f error=%q", id, tradeNo, req.TradeType, req.Amount, payMoney, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("拉起支付失败: %s", err.Error())})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt 充值订单创建成功 user_id=%d trade_no=%s trade_type=%s amount=%d money=%.2f CNY", id, tradeNo, req.TradeType, req.Amount, payMoney))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url": paymentUrl,
			"trade_no":    tradeNo,
		},
	})
}

func SubscriptionRequestBepusdtPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionBepusdtPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.TradeType == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "请选择支付链"})
		return
	}
	if !isValidBepusdtTradeType(req.TradeType) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付链"})
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

	tradeNo := fmt.Sprintf("BEPUSDT_SUBUSR%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())
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

	bepusdtDiscount, err := convertSubscriptionDiscountToBepusdtMoney(plan, discount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var payMoney float64
	if bepusdtDiscount != nil {
		payMoney = bepusdtDiscount.PaidAmount
	} else {
		payMoney, err = getSubscriptionBepusdtPayMoney(plan, paidUSD)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodBepusdt,
		PaymentProvider: model.PaymentProviderBepusdt,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if discount == nil {
		order.AffiliateSourceQuota = subscriptionPaidQuotaFromUSD(paidUSD)
	}
	model.ApplyPromoCodeResultToSubscriptionOrder(order, bepusdtDiscount)
	if err := order.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 订阅订单创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	if payMoney < 0.01 {
		if err := model.CompleteFreeSubscriptionOrder(tradeNo, model.PaymentProviderBepusdt); err != nil {
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

	callBackAddress := service.GetCallbackAddress()
	notifyUrl := callBackAddress + "/api/bepusdt/notify"
	redirectUrl := paymentReturnPath("/console/topup")
	paymentUrl, err := createBepusdtTransaction(c, tradeNo, payMoney, req.TradeType, notifyUrl, redirectUrl, fmt.Sprintf("SUB:%s", plan.Title))
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderBepusdt)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 订阅拉起支付失败 user_id=%d plan_id=%d trade_no=%s trade_type=%s money=%.2f error=%q", userId, plan.Id, tradeNo, req.TradeType, payMoney, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("拉起支付失败: %s", err.Error())})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt 订阅订单创建成功 user_id=%d plan_id=%d trade_no=%s trade_type=%s money=%.2f CNY", userId, plan.Id, tradeNo, req.TradeType, payMoney))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url": paymentUrl,
			"trade_no":    tradeNo,
			"discount":    discount,
		},
	})
}

// createBepusdtTransaction 调用 bepusdt API 创建交易订单
func createBepusdtTransaction(c *gin.Context, orderId string, amountCNY float64, tradeType string, notifyUrl string, redirectUrl string, names ...string) (string, error) {
	apiUrl := strings.TrimRight(setting.BepusdtApiUrl, "/") + "/api/v1/order/create-transaction"
	name := fmt.Sprintf("TopUp-%s", orderId)
	if len(names) > 0 && strings.TrimSpace(names[0]) != "" {
		name = strings.TrimSpace(names[0])
	}

	// 签名用字符串 map（amount 格式必须与 JSON body 中的数字表示一致）
	amountStr := strconv.FormatFloat(amountCNY, 'f', -1, 64)
	timeoutStr := strconv.Itoa(setting.BepusdtTimeout)
	params := map[string]string{
		"order_id":     orderId,
		"amount":       amountStr,
		"fiat":         "CNY",
		"trade_type":   tradeType,
		"notify_url":   notifyUrl,
		"redirect_url": redirectUrl,
		"name":         name,
		"timeout":      timeoutStr,
	}
	params["signature"] = generateBepusdtSignature(params, setting.BepusdtAuthToken)

	// 构建 JSON body，amount 和 timeout 用数字类型
	jsonBody := map[string]interface{}{
		"order_id":     orderId,
		"amount":       amountCNY,
		"fiat":         "CNY",
		"trade_type":   tradeType,
		"notify_url":   notifyUrl,
		"redirect_url": redirectUrl,
		"name":         name,
		"timeout":      setting.BepusdtTimeout,
		"signature":    params["signature"],
	}

	jsonData, err := common.Marshal(jsonBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 bepusdt 失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt API 响应 order_id=%s status_code=%d body=%q", orderId, resp.StatusCode, string(body)))

	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("bepusdt API HTTP %d", resp.StatusCode)
	}

	var result bepusdtCreateTransactionResp
	err = common.Unmarshal(body, &result)
	if err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if result.StatusCode != 200 {
		return "", fmt.Errorf("bepusdt API 错误: %s", result.Message)
	}

	if result.Data.PaymentUrl == "" {
		return "", fmt.Errorf("bepusdt 未返回 payment_url")
	}

	return result.Data.PaymentUrl, nil
}

// BepusdtNotify 处理 bepusdt 回调通知
func BepusdtNotify(c *gin.Context) {
	if !isBepusdtWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Bepusdt webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt webhook 收到请求 path=%q client_ip=%s body=%q", c.Request.RequestURI, c.ClientIP(), string(bodyBytes)))

	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var payload bepusdtNotifyPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt webhook 解析失败 path=%q client_ip=%s error=%q body=%q", c.Request.RequestURI, c.ClientIP(), err.Error(), string(bodyBytes)))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 验证签名
	if !verifyBepusdtNotifySignature(&payload, setting.BepusdtAuthToken) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Bepusdt webhook 验签失败 path=%q client_ip=%s order_id=%s trade_id=%s signature=%q", c.Request.RequestURI, c.ClientIP(), payload.OrderId, payload.TradeId, payload.Signature))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt webhook 验签成功 order_id=%s trade_id=%s status=%d client_ip=%s", payload.OrderId, payload.TradeId, payload.Status, c.ClientIP()))

	// status: 1=等待支付, 2=支付成功, 3=已过期
	switch payload.Status {
	case 2:
		// 支付成功
		handleBepusdtPaymentSuccess(c, &payload)
	case 1:
		// 等待支付，记录日志
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt 订单等待支付 order_id=%s trade_id=%s", payload.OrderId, payload.TradeId))
		_, _ = c.Writer.Write([]byte("ok"))
	case 3:
		// 订单过期
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt 订单已过期 order_id=%s trade_id=%s", payload.OrderId, payload.TradeId))
		if payload.OrderId != "" {
			if err := model.UpdatePendingTopUpStatus(payload.OrderId, model.PaymentProviderBepusdt, common.TopUpStatusExpired); err != nil {
				_ = model.ExpireSubscriptionOrder(payload.OrderId, model.PaymentProviderBepusdt)
			}
		}
		_, _ = c.Writer.Write([]byte("ok"))
	default:
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt webhook 未知状态 order_id=%s trade_id=%s status=%d", payload.OrderId, payload.TradeId, payload.Status))
		_, _ = c.Writer.Write([]byte("ok"))
	}
}

// handleBepusdtPaymentSuccess 处理支付成功回调
func handleBepusdtPaymentSuccess(c *gin.Context, payload *bepusdtNotifyPayload) {
	tradeNo := payload.OrderId
	if tradeNo == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Bepusdt webhook 缺少 order_id trade_id=%s", payload.TradeId))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		err := model.CompleteSubscriptionOrder(tradeNo, common.GetJsonString(payload), model.PaymentProviderBepusdt, model.PaymentMethodBepusdt)
		if err == nil {
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt USDT 订阅购买成功 trade_no=%s trade_id=%s actual_amount=%v block_tx=%s client_ip=%s", tradeNo, payload.TradeId, payload.ActualAmount, payload.BlockTransactionId, c.ClientIP()))
			_, _ = c.Writer.Write([]byte("ok"))
			return
		}
		if !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 订阅处理失败 trade_no=%s trade_id=%s client_ip=%s error=%q", tradeNo, payload.TradeId, c.ClientIP(), err.Error()))
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Bepusdt 充值/订阅订单不存在 trade_no=%s trade_id=%s", tradeNo, payload.TradeId))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt 充值订单状态非 pending，忽略处理 trade_no=%s status=%s trade_id=%s", tradeNo, topUp.Status, payload.TradeId))
		_, _ = c.Writer.Write([]byte("ok"))
		return
	}

	err := model.RechargeBepusdt(tradeNo, c.ClientIP())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 充值处理失败 trade_no=%s trade_id=%s client_ip=%s error=%q", tradeNo, payload.TradeId, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Bepusdt USDT 充值成功 trade_no=%s trade_id=%s actual_amount=%v block_tx=%s client_ip=%s", tradeNo, payload.TradeId, payload.ActualAmount, payload.BlockTransactionId, c.ClientIP()))
	_, _ = c.Writer.Write([]byte("ok"))
}
