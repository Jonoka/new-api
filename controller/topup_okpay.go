package controller

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
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

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// OkpayPayRequest 用户发起 OKPay 支付请求
type OkpayPayRequest struct {
	Amount    int64                `json:"amount"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

// OkpayAmountRequest 金额计算请求
type OkpayAmountRequest struct {
	Amount    int64                `json:"amount"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

type okpayPaymentAmount struct {
	FiatAmount     float64
	CoinAmount     float64
	Rate           float64
	RateSource     string
	AutoRateFailed bool
	Coin           string
}

type okpayRateCacheEntry struct {
	rate      float64
	source    string
	expiresAt time.Time
}

var (
	okpayRateCacheMu sync.Mutex
	okpayRateCache   okpayRateCacheEntry
)

const okpayRateCacheTTL = 5 * time.Minute

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

func mergeOkpayCallbackValues(dst url.Values, src url.Values) {
	for key, values := range src {
		key = strings.TrimSpace(key)
		if key == "" || len(values) == 0 {
			continue
		}
		dst[key] = append([]string(nil), values...)
	}
}

func setOkpayJSONCallbackValue(values url.Values, key string, raw json.RawMessage) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	value := strings.TrimSpace(common.JsonRawMessageToString(raw))
	if value == "" {
		return
	}
	values.Set(key, value)
}

func parseOkpayJSONCallbackValues(body []byte) (url.Values, error) {
	var payload map[string]json.RawMessage
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	values := url.Values{}
	for key, raw := range payload {
		if strings.TrimSpace(key) == "data" && common.GetJsonType(raw) == "object" {
			var data map[string]json.RawMessage
			if err := common.Unmarshal(raw, &data); err != nil {
				return nil, err
			}
			for dataKey, dataRaw := range data {
				setOkpayJSONCallbackValue(values, "data["+dataKey+"]", dataRaw)
			}
			continue
		}
		setOkpayJSONCallbackValue(values, key, raw)
	}
	return values, nil
}

func parseOkpayCallbackValues(c *gin.Context) (url.Values, []byte, error) {
	values := url.Values{}
	mergeOkpayCallbackValues(values, c.Request.URL.Query())

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.Contains(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return nil, bodyBytes, err
		}
		mergeOkpayCallbackValues(values, c.Request.PostForm)
		if c.Request.MultipartForm != nil {
			mergeOkpayCallbackValues(values, c.Request.MultipartForm.Value)
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return values, bodyBytes, nil
	}

	trimmedBody := bytes.TrimSpace(bodyBytes)
	if len(trimmedBody) == 0 {
		return values, bodyBytes, nil
	}

	if trimmedBody[0] == '{' {
		jsonValues, err := parseOkpayJSONCallbackValues(trimmedBody)
		if err != nil {
			return nil, bodyBytes, err
		}
		mergeOkpayCallbackValues(values, jsonValues)
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		return values, bodyBytes, nil
	}

	formValues, err := url.ParseQuery(string(trimmedBody))
	if err != nil {
		return nil, bodyBytes, err
	}
	mergeOkpayCallbackValues(values, formValues)
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return values, bodyBytes, nil
}

func getOkpayCallbackValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func isOkpayCallbackSuccess(requestStatus string, paymentStatus string) bool {
	requestStatus = strings.TrimSpace(requestStatus)
	paymentStatus = strings.TrimSpace(paymentStatus)
	requestOk := requestStatus == "" || strings.EqualFold(requestStatus, "success") || requestStatus == "1"
	if paymentStatus != "" {
		return requestOk && (paymentStatus == "1" || strings.EqualFold(paymentStatus, "success") || strings.EqualFold(paymentStatus, "paid"))
	}
	return strings.EqualFold(requestStatus, "success") || requestStatus == "1"
}

// getOkpayFiatPayMoney 计算站内 OKPay 标价金额（CNY）。
func getOkpayFiatPayMoney(amount int64, group string) float64 {
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

// getOkpayPayMoney 保持旧测试/调用兼容，返回站内 CNY 标价金额。
func getOkpayPayMoney(amount int64, group string) float64 {
	return getOkpayFiatPayMoney(amount, group)
}

func getOkpayCoin() string {
	coin := strings.ToUpper(strings.TrimSpace(setting.OkpayCoin))
	if coin == "" {
		return "USDT"
	}
	return coin
}

func getOkpayFallbackUsdtCnyRate() float64 {
	if setting.OkpayUsdtCnyRate > 0 && !math.IsNaN(setting.OkpayUsdtCnyRate) && !math.IsInf(setting.OkpayUsdtCnyRate, 0) {
		return setting.OkpayUsdtCnyRate
	}
	if setting.OkpayExchangeRate > 0 && !math.IsNaN(setting.OkpayExchangeRate) && !math.IsInf(setting.OkpayExchangeRate, 0) {
		return setting.OkpayExchangeRate
	}
	return 1
}

func parseOkpayRateFromBody(body []byte) (float64, error) {
	var payload map[string]interface{}
	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, err
	}
	tether, ok := payload["tether"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("missing tether price")
	}
	rawRate, ok := tether["cny"]
	if !ok {
		return 0, fmt.Errorf("missing cny price")
	}
	rate, err := strconv.ParseFloat(fmt.Sprintf("%v", rawRate), 64)
	if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, fmt.Errorf("invalid cny price")
	}
	return rate, nil
}

func fetchOkpayUsdtCnyRate() (float64, string, error) {
	rateUrl := strings.TrimSpace(setting.OkpayRateApiUrl)
	if rateUrl == "" {
		return 0, "", fmt.Errorf("rate api url is empty")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(rateUrl)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	if resp.StatusCode/100 != 2 {
		return 0, "", fmt.Errorf("rate api http %d", resp.StatusCode)
	}
	rate, err := parseOkpayRateFromBody(body)
	if err != nil {
		return 0, "", err
	}
	return rate, "coingecko", nil
}

func getOkpayUsdtCnyRate() (float64, string, bool) {
	fallbackRate := getOkpayFallbackUsdtCnyRate()
	if !setting.OkpayAutoExchangeEnabled {
		return fallbackRate, "fallback", false
	}

	now := time.Now()
	okpayRateCacheMu.Lock()
	if okpayRateCache.rate > 0 && now.Before(okpayRateCache.expiresAt) {
		cached := okpayRateCache
		okpayRateCacheMu.Unlock()
		return cached.rate, cached.source, false
	}
	okpayRateCacheMu.Unlock()

	rate, source, err := fetchOkpayUsdtCnyRate()
	if err != nil {
		common.SysLog("failed to fetch OKPay USDT/CNY rate, using fallback: " + err.Error())
		return fallbackRate, "fallback", true
	}

	okpayRateCacheMu.Lock()
	okpayRateCache = okpayRateCacheEntry{
		rate:      rate,
		source:    source,
		expiresAt: now.Add(okpayRateCacheTTL),
	}
	okpayRateCacheMu.Unlock()
	return rate, source, false
}

func getOkpayPaymentAmountFromFiat(fiatAmount float64) okpayPaymentAmount {
	coin := getOkpayCoin()
	if coin != "USDT" {
		return okpayPaymentAmount{
			FiatAmount: fiatAmount,
			CoinAmount: fiatAmount,
			Rate:       1,
			RateSource: "coin",
			Coin:       coin,
		}
	}

	rate, source, failed := getOkpayUsdtCnyRate()
	if rate <= 0 {
		rate = 1
		source = "fallback"
		failed = true
	}
	coinAmount := decimal.NewFromFloat(fiatAmount).Div(decimal.NewFromFloat(rate)).Round(8).InexactFloat64()
	return okpayPaymentAmount{
		FiatAmount:     fiatAmount,
		CoinAmount:     coinAmount,
		Rate:           rate,
		RateSource:     source,
		AutoRateFailed: failed,
		Coin:           coin,
	}
}

func calculateOkpayAffiliateSourceQuota(storedAmount int64, originalFiatAmount float64, paidFiatAmount float64) int {
	if storedAmount <= 0 || originalFiatAmount <= 0 || paidFiatAmount <= 0 {
		return 0
	}
	ratio := decimal.NewFromFloat(paidFiatAmount).Div(decimal.NewFromFloat(originalFiatAmount))
	if ratio.GreaterThan(decimal.NewFromInt(1)) {
		ratio = decimal.NewFromInt(1)
	}
	quota := decimal.NewFromInt(storedAmount).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(ratio).
		Round(0).
		IntPart()
	return int(quota)
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

	originalFiatPayMoney := getOkpayFiatPayMoney(req.Amount, group)
	fiatPayMoney := originalFiatPayMoney
	discount, err := model.CalculatePromoCodeDiscount(req.PromoCode, model.PromoCodeTargetTopUp, 0, fiatPayMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if discount != nil {
		fiatPayMoney = discount.PaidAmount
	}
	invoiceAmounts, err := buildInvoicePaymentPreviewAmounts(req.Invoice, model.PaymentProviderOkpay, fiatPayMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	totalFiatPayMoney := fiatPayMoney
	if invoiceAmounts.Required {
		totalFiatPayMoney = invoiceAmounts.TotalPayment
	}
	paymentAmount := getOkpayPaymentAmountFromFiat(totalFiatPayMoney)
	coinAmountText := decimal.NewFromFloat(paymentAmount.CoinAmount).StringFixed(8)

	response := gin.H{
		"message":          "success",
		"data":             coinAmountText,
		"amount":           coinAmountText,
		"amount_text":      fmt.Sprintf("%s %s", coinAmountText, paymentAmount.Coin),
		"coin":             paymentAmount.Coin,
		"fiat_amount":      strconv.FormatFloat(paymentAmount.FiatAmount, 'f', 2, 64),
		"fiat_currency":    "CNY",
		"rate":             strconv.FormatFloat(paymentAmount.Rate, 'f', -1, 64),
		"rate_source":      paymentAmount.RateSource,
		"auto_rate_failed": paymentAmount.AutoRateFailed,
	}
	if discount != nil {
		response["discount"] = discount
	}
	addInvoiceFieldsToResponse(response, invoiceAmounts)
	c.JSON(http.StatusOK, response)
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

	originalFiatPayMoney := getOkpayFiatPayMoney(req.Amount, group)
	fiatPayMoney := originalFiatPayMoney
	discount, err := model.CalculatePromoCodeDiscount(req.PromoCode, model.PromoCodeTargetTopUp, 0, fiatPayMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if discount != nil {
		fiatPayMoney = discount.PaidAmount
	}
	if fiatPayMoney < 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderOkpay, fiatPayMoney)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	totalFiatPayMoney := fiatPayMoney
	if invoiceAmounts.Required {
		totalFiatPayMoney = invoiceAmounts.TotalPayment
	}

	tradeNo := fmt.Sprintf("USR%dNO%s%d", id, common.GetRandomString(6), time.Now().Unix())

	amount := normalizeTopUpAmountForStorage(req.Amount)
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           totalFiatPayMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodOkpay,
		PaymentProvider: model.PaymentProviderOkpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	model.ApplyPromoCodeResultToTopUp(topUp, discount)
	if discount != nil {
		topUp.AffiliateSourceQuota = calculateOkpayAffiliateSourceQuota(amount, originalFiatPayMoney, fiatPayMoney)
	}
	applyInvoiceToTopUp(topUp, invoiceAmounts, originalFiatPayMoney, fiatPayMoney, true)
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// 处理 0 元优惠订单
	if totalFiatPayMoney < 0.01 {
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
	paymentAmount := getOkpayPaymentAmountFromFiat(totalFiatPayMoney)
	dPayMoney := decimal.NewFromFloat(paymentAmount.CoinAmount)

	payload := map[string]string{
		"unique_id":    tradeNo,
		"amount":       dPayMoney.StringFixed(8),
		"return_url":   redirectUrl,
		"callback_url": callbackUrl,
		"coin":         paymentAmount.Coin,
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

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 充值订单创建成功 user_id=%d trade_no=%s amount=%d fiat_money=%.2f CNY coin_amount=%s coin=%s rate=%.8f rate_source=%s auto_rate_failed=%t", id, tradeNo, req.Amount, totalFiatPayMoney, payload["amount"], paymentAmount.Coin, paymentAmount.Rate, paymentAmount.RateSource, paymentAmount.AutoRateFailed))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url":      payUrl,
			"trade_no":         tradeNo,
			"amount":           payload["amount"],
			"amount_text":      fmt.Sprintf("%s %s", payload["amount"], paymentAmount.Coin),
			"coin":             paymentAmount.Coin,
			"fiat_amount":      strconv.FormatFloat(paymentAmount.FiatAmount, 'f', 2, 64),
			"fiat_currency":    "CNY",
			"rate":             strconv.FormatFloat(paymentAmount.Rate, 'f', -1, 64),
			"rate_source":      paymentAmount.RateSource,
			"auto_rate_failed": paymentAmount.AutoRateFailed,
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

	formValues, bodyBytes, err := parseOkpayCallbackValues(c)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay webhook 解析请求失败 path=%q method=%s content_type=%q client_ip=%s error=%q body=%q", c.Request.RequestURI, c.Request.Method, c.GetHeader("Content-Type"), c.ClientIP(), err.Error(), string(bodyBytes)))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay webhook 收到请求 path=%q method=%s content_type=%q client_ip=%s params=%q body=%q", c.Request.RequestURI, c.Request.Method, c.GetHeader("Content-Type"), c.ClientIP(), common.GetJsonString(formValues), string(bodyBytes)))

	if len(formValues) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 参数为空 path=%q method=%s client_ip=%s", c.Request.RequestURI, c.Request.Method, c.ClientIP()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	sign := strings.TrimSpace(formValues.Get("sign"))
	if sign == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 缺少 sign path=%q method=%s client_ip=%s params=%q", c.Request.RequestURI, c.Request.Method, c.ClientIP(), common.GetJsonString(formValues)))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 验证签名
	if !verifyOkpayCallbackSignature(formValues, setting.OkpayMerchantToken) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 验签失败 path=%q method=%s client_ip=%s sign=%q params=%q", c.Request.RequestURI, c.Request.Method, c.ClientIP(), sign, common.GetJsonString(formValues)))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	requestStatus := strings.TrimSpace(formValues.Get("status"))
	paymentStatus := getOkpayCallbackValue(formValues, "data[status]", "payment_status", "trade_status", "order_status")
	uniqueID := getOkpayCallbackValue(formValues, "data[unique_id]", "unique_id", "trade_no", "out_trade_no", "order_id")
	orderID := getOkpayCallbackValue(formValues, "data[order_id]", "order_id", "trade_id")

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay webhook 验签成功 path=%q method=%s unique_id=%s order_id=%s status=%s payment_status=%s client_ip=%s", c.Request.RequestURI, c.Request.Method, uniqueID, orderID, requestStatus, paymentStatus, c.ClientIP()))

	// 兼容 OKPay 回调的嵌套状态与扁平状态字段。
	if !isOkpayCallbackSuccess(requestStatus, paymentStatus) {
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
