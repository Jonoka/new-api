package controller

import (
	"crypto/md5"
	"errors"
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
	"github.com/tidwall/gjson"
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

type SubscriptionOkpayPayRequest struct {
	PlanId    int                  `json:"plan_id"`
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

type okpayPaymentLinkResult struct {
	ProviderOrderId string
	PaymentUrl      string
	Amount          string
	PaymentAmount   okpayPaymentAmount
}

type okpayRateCacheEntry struct {
	rate      float64
	source    string
	expiresAt time.Time
}

type okpayOrderedPair struct {
	Key   string
	Value string
}

type okpayCallbackPayload struct {
	Pairs  []okpayOrderedPair
	Values map[string]string
}

func (p *okpayCallbackPayload) Get(key string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.Values[key])
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

// verifyOkpayCallbackSignature 验证回调签名。
// OKPay 的不同版本既可能按原始字段顺序签名，也可能按字段名排序签名；
// 因此先使用原始顺序验证，再使用排序后的字段兜底。
func verifyOkpayCallbackSignature(payload *okpayCallbackPayload, merchantToken string) bool {
	if payload == nil {
		return false
	}
	actual := payload.Get("sign")
	if actual == "" {
		return false
	}

	pairs := make([]okpayOrderedPair, 0, len(payload.Pairs))
	for _, pair := range payload.Pairs {
		if strings.EqualFold(strings.TrimSpace(pair.Key), "sign") {
			continue
		}
		pairs = append(pairs, pair)
	}
	if strings.EqualFold(generateOkpayOrderedSignature(pairs, merchantToken), actual) {
		return true
	}

	sort.SliceStable(pairs, func(i, j int) bool {
		return pairs[i].Key < pairs[j].Key
	})
	return strings.EqualFold(generateOkpayOrderedSignature(pairs, merchantToken), actual)
}

func generateOkpayOrderedSignature(pairs []okpayOrderedPair, merchantToken string) string {
	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		key := strings.TrimSpace(pair.Key)
		value := strings.TrimSpace(pair.Value)
		if key == "" || value == "" || strings.EqualFold(key, "sign") {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	query := strings.Join(parts, "&") + "&token=" + strings.TrimSpace(merchantToken)
	hash := md5.Sum([]byte(query))
	return strings.ToUpper(fmt.Sprintf("%x", hash))
}

// parseOkpayCallbackPayload 同时兼容 OKPay 的 Form 与 JSON 回调，并保留字段顺序。
func parseOkpayCallbackPayload(body []byte) (*okpayCallbackPayload, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, errors.New("empty callback body")
	}

	pairs := make([]okpayOrderedPair, 0)
	if strings.HasPrefix(trimmed, "{") {
		if !gjson.Valid(trimmed) {
			return nil, errors.New("invalid callback json")
		}
		root := gjson.Parse(trimmed)
		if !root.IsObject() {
			return nil, errors.New("invalid callback json object")
		}
		flattenOkpayJSONPairs("", root, &pairs)
	} else {
		for _, item := range strings.Split(trimmed, "&") {
			if item == "" {
				continue
			}
			parts := strings.SplitN(item, "=", 2)
			key, err := url.QueryUnescape(parts[0])
			if err != nil {
				key = parts[0]
			}
			value := ""
			if len(parts) == 2 {
				value, err = url.QueryUnescape(parts[1])
				if err != nil {
					value = parts[1]
				}
			}
			pairs = append(pairs, okpayOrderedPair{Key: strings.TrimSpace(key), Value: strings.TrimSpace(value)})
		}
	}
	if len(pairs) == 0 {
		return nil, errors.New("empty callback parameters")
	}

	values := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		values[pair.Key] = pair.Value
	}
	return &okpayCallbackPayload{Pairs: pairs, Values: values}, nil
}

func flattenOkpayJSONPairs(prefix string, value gjson.Result, pairs *[]okpayOrderedPair) {
	if value.IsObject() {
		value.ForEach(func(key, child gjson.Result) bool {
			childKey := key.String()
			if prefix != "" {
				childKey = prefix + "[" + childKey + "]"
			}
			flattenOkpayJSONPairs(childKey, child, pairs)
			return true
		})
		return
	}
	if value.IsArray() {
		index := 0
		value.ForEach(func(_, child gjson.Result) bool {
			childKey := fmt.Sprintf("%s[%d]", prefix, index)
			flattenOkpayJSONPairs(childKey, child, pairs)
			index++
			return true
		})
		return
	}
	*pairs = append(*pairs, okpayOrderedPair{Key: strings.TrimSpace(prefix), Value: strings.TrimSpace(value.String())})
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

func createOkpayPaymentLink(c *gin.Context, tradeNo string, paymentAmount okpayPaymentAmount, name string, callbackUrl string, redirectUrl string) (*okpayPaymentLinkResult, error) {
	amount := decimal.NewFromFloat(paymentAmount.CoinAmount).StringFixed(8)
	payload := map[string]string{
		"unique_id":    tradeNo,
		"amount":       amount,
		"return_url":   redirectUrl,
		"callback_url": callbackUrl,
		"coin":         paymentAmount.Coin,
		"name":         name,
		"id":           setting.OkpayMerchantId,
	}
	payload["sign"] = generateOkpaySignature(payload, setting.OkpayMerchantToken)

	formValues := url.Values{}
	for key, value := range payload {
		formValues.Set(key, value)
	}
	gatewayUrl := strings.TrimRight(setting.OkpayGatewayUrl, "/") + "/payLink"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, gatewayUrl, strings.NewReader(formValues.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建 OKPay 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 OKPay 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 OKPay 响应失败: %w", err)
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay API 响应 trade_no=%s status_code=%d body=%q", tradeNo, resp.StatusCode, string(body)))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("OKPay API HTTP %d", resp.StatusCode)
	}

	var raw map[string]interface{}
	if err := common.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("解析 OKPay 响应失败: %w", err)
	}
	paymentUrl := ""
	providerOrderId := ""
	if data, ok := raw["data"].(map[string]interface{}); ok {
		if value, ok := data["pay_url"].(string); ok {
			paymentUrl = strings.TrimSpace(value)
		}
		providerOrderId = okpayResponseString(data["order_id"])
	}
	if paymentUrl == "" || providerOrderId == "" {
		if items, ok := raw["data"].([]interface{}); ok && len(items) > 0 {
			if first, ok := items[0].(map[string]interface{}); ok {
				if paymentUrl == "" {
					value, _ := first["pay_url"].(string)
					paymentUrl = strings.TrimSpace(value)
				}
				if providerOrderId == "" {
					providerOrderId = okpayResponseString(first["order_id"])
				}
			}
		}
	}
	if paymentUrl == "" {
		return nil, errors.New("OKPay 未返回 pay_url")
	}
	if providerOrderId == "" {
		return nil, errors.New("OKPay 未返回 order_id")
	}
	return &okpayPaymentLinkResult{
		ProviderOrderId: providerOrderId,
		PaymentUrl:      paymentUrl,
		Amount:          amount,
		PaymentAmount:   paymentAmount,
	}, nil
}

func okpayResponseString(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
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
	paymentAmount := okpayPaymentAmount{}
	providerAmount := ""
	providerCurrency := ""
	if totalFiatPayMoney >= 0.01 {
		paymentAmount = getOkpayPaymentAmountFromFiat(totalFiatPayMoney)
		providerAmount = decimal.NewFromFloat(paymentAmount.CoinAmount).StringFixed(8)
		providerCurrency = strings.ToUpper(strings.TrimSpace(paymentAmount.Coin))
	}

	tradeNo := fmt.Sprintf("USR%dNO%s%d", id, common.GetRandomString(6), time.Now().Unix())

	amount := normalizeTopUpAmountForStorage(req.Amount)
	topUp := &model.TopUp{
		UserId:           id,
		Amount:           amount,
		Money:            totalFiatPayMoney,
		TradeNo:          tradeNo,
		PaymentMethod:    model.PaymentMethodOkpay,
		PaymentProvider:  model.PaymentProviderOkpay,
		RequestIP:        c.ClientIP(),
		ProviderAmount:   providerAmount,
		ProviderCurrency: providerCurrency,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
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
			model.RecordTopupOrderLog(completedTopUp, fmt.Sprintf("使用优惠码充值成功，充值金额: %v，支付金额：0.00", logger.LogQuota(quotaToAdd)), "promo")
		}
		c.JSON(http.StatusOK, freeTopUpResponse(completedTopUp, quotaToAdd, discount))
		return
	}

	// 调用 OKPay API 创建支付
	callBackAddress := service.GetCallbackAddress()
	callbackUrl := callBackAddress + "/api/okpay/notify"
	redirectUrl := paymentReturnPath("/console/log")

	payment, err := createOkpayPaymentLink(c, tradeNo, paymentAmount, fmt.Sprintf("TopUp-%s", tradeNo), callbackUrl, redirectUrl)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 拉起支付失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if err := model.UpdateTopUpProviderSnapshot(tradeNo, model.PaymentProviderOkpay, payment.ProviderOrderId, payment.Amount, payment.PaymentAmount.Coin); err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 保存第三方订单号失败 user_id=%d trade_no=%s provider_order_id=%s error=%q", id, tradeNo, payment.ProviderOrderId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "保存支付订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 充值订单创建成功 user_id=%d trade_no=%s provider_order_id=%s amount=%d fiat_money=%.2f CNY coin_amount=%s coin=%s rate=%.8f rate_source=%s auto_rate_failed=%t", id, tradeNo, payment.ProviderOrderId, req.Amount, totalFiatPayMoney, payment.Amount, payment.PaymentAmount.Coin, payment.PaymentAmount.Rate, payment.PaymentAmount.RateSource, payment.PaymentAmount.AutoRateFailed))

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
		},
	})
}

func writeOkpayCallbackStatus(c *gin.Context, success bool) {
	status := "fail"
	if success {
		status = "success"
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(fmt.Sprintf(`{"status":"%s"}`, status)))
}

func validateOkpayCallbackSnapshot(callback *okpayCallbackPayload, providerOrderId string, providerAmount string, providerCurrency string) (bool, error) {
	providerOrderId = strings.TrimSpace(providerOrderId)
	providerAmount = strings.TrimSpace(providerAmount)
	providerCurrency = strings.ToUpper(strings.TrimSpace(providerCurrency))
	if providerOrderId == "" && providerAmount == "" && providerCurrency == "" {
		return true, nil
	}
	if providerOrderId == "" || providerAmount == "" || providerCurrency == "" {
		return false, errors.New("订单网关金额快照不完整")
	}

	callbackAmount := strings.TrimSpace(callback.Get("data[amount]"))
	if callbackAmount == "" {
		return false, errors.New("回调缺少支付金额")
	}
	expectedAmount, err := decimal.NewFromString(providerAmount)
	if err != nil {
		return false, fmt.Errorf("订单网关金额快照无效: %w", err)
	}
	actualAmount, err := decimal.NewFromString(callbackAmount)
	if err != nil {
		return false, fmt.Errorf("回调支付金额无效: %w", err)
	}
	if !actualAmount.Equal(expectedAmount) {
		return false, fmt.Errorf("回调支付金额不匹配: expected=%s actual=%s", expectedAmount.String(), actualAmount.String())
	}

	callbackCurrency := strings.ToUpper(strings.TrimSpace(callback.Get("data[coin]")))
	if callbackCurrency == "" {
		return false, errors.New("回调缺少支付币种")
	}
	if callbackCurrency != providerCurrency {
		return false, fmt.Errorf("回调支付币种不匹配: expected=%s actual=%s", providerCurrency, callbackCurrency)
	}

	callbackOrderId := strings.TrimSpace(callback.Get("data[order_id]"))
	if callbackOrderId == "" {
		return false, errors.New("回调缺少第三方订单号")
	}
	if callbackOrderId != providerOrderId {
		return false, fmt.Errorf("回调第三方订单号不匹配: expected=%s actual=%s", providerOrderId, callbackOrderId)
	}
	return false, nil
}

func findOkpayTradeNoByBusinessReference(reference string) (string, bool) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", false
	}
	if topUp := model.GetTopUpByTradeNo(reference); topUp != nil && topUp.PaymentProvider == model.PaymentProviderOkpay {
		return topUp.TradeNo, true
	}
	if order := model.GetSubscriptionOrderByTradeNo(reference); order != nil && order.PaymentProvider == model.PaymentProviderOkpay {
		return order.TradeNo, true
	}
	return "", false
}

func resolveOkpayTradeNo(uniqueId string, providerOrderId string) (string, error) {
	if tradeNo, ok := findOkpayTradeNoByBusinessReference(uniqueId); ok {
		return tradeNo, nil
	}

	providerOrderId = strings.TrimSpace(providerOrderId)
	if providerOrderId != "" {
		topUp := model.GetTopUpByProviderOrderId(model.PaymentProviderOkpay, providerOrderId)
		order := model.GetSubscriptionOrderByProviderOrderId(model.PaymentProviderOkpay, providerOrderId)
		if topUp != nil && order != nil {
			return "", errors.New("第三方订单号匹配到多个本地订单")
		}
		if topUp != nil {
			return topUp.TradeNo, nil
		}
		if order != nil {
			return order.TradeNo, nil
		}
		// 兼容历史网关把商户订单号放在 data[order_id] 的行为。
		if tradeNo, ok := findOkpayTradeNoByBusinessReference(providerOrderId); ok {
			return tradeNo, nil
		}
	}
	return "", errors.New("OKPay 充值/订阅订单不存在")
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

	callback, err := parseOkpayCallbackPayload(bodyBytes)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay webhook 解析失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	sign := callback.Get("sign")
	if sign == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 缺少 sign client_ip=%s", c.ClientIP()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 验证签名
	if !verifyOkpayCallbackSignature(callback, setting.OkpayMerchantToken) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 验签失败 client_ip=%s sign=%q", c.ClientIP(), sign))
		writeOkpayCallbackStatus(c, false)
		return
	}
	merchantId := callback.Get("id")
	if configuredMerchantId := strings.TrimSpace(setting.OkpayMerchantId); merchantId != "" && configuredMerchantId != "" && merchantId != configuredMerchantId {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 商户 ID 不匹配 client_ip=%s merchant_id=%q", c.ClientIP(), merchantId))
		writeOkpayCallbackStatus(c, false)
		return
	}

	requestStatus := callback.Get("status")
	paymentStatus := callback.Get("data[status]")
	uniqueID := callback.Get("data[unique_id]")
	orderID := callback.Get("data[order_id]")

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay webhook 验签成功 unique_id=%s order_id=%s status=%s payment_status=%s client_ip=%s", uniqueID, orderID, requestStatus, paymentStatus, c.ClientIP()))

	// 判断支付状态: request status 必须为 "success"，data[status] 为 "1" 表示成功
	if !strings.EqualFold(requestStatus, "success") || paymentStatus != "1" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 订单非成功状态 unique_id=%s status=%s payment_status=%s", uniqueID, requestStatus, paymentStatus))
		writeOkpayCallbackStatus(c, true)
		return
	}

	tradeNo, err := resolveOkpayTradeNo(uniqueID, orderID)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 查单失败 unique_id=%s order_id=%s error=%q", uniqueID, orderID, err.Error()))
		writeOkpayCallbackStatus(c, false)
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		order := model.GetSubscriptionOrderByTradeNo(tradeNo)
		if order == nil || order.PaymentProvider != model.PaymentProviderOkpay {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 充值/订阅订单不存在 trade_no=%s order_id=%s", tradeNo, orderID))
			writeOkpayCallbackStatus(c, false)
			return
		}
		if order.Status == common.TopUpStatusSuccess {
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 订阅订单已完成，幂等返回 trade_no=%s", tradeNo))
			writeOkpayCallbackStatus(c, true)
			return
		}
		legacySnapshot, err := validateOkpayCallbackSnapshot(callback, order.ProviderOrderId, order.ProviderAmount, order.ProviderCurrency)
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 订阅回调快照校验失败 trade_no=%s order_id=%s error=%q", tradeNo, orderID, err.Error()))
			writeOkpayCallbackStatus(c, false)
			return
		}
		if legacySnapshot {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 订阅订单缺少网关快照，按存量订单兼容处理 trade_no=%s order_id=%s", tradeNo, orderID))
		}
		if err := model.CompleteSubscriptionOrder(tradeNo, common.GetJsonString(callback.Values), model.PaymentProviderOkpay, model.PaymentMethodOkpay); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 订阅处理失败 trade_no=%s order_id=%s client_ip=%s error=%q", tradeNo, orderID, c.ClientIP(), err.Error()))
			writeOkpayCallbackStatus(c, false)
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 订阅购买成功 trade_no=%s order_id=%s amount=%s coin=%s client_ip=%s", tradeNo, orderID, callback.Get("data[amount]"), callback.Get("data[coin]"), c.ClientIP()))
		writeOkpayCallbackStatus(c, true)
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderOkpay {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 充值订单支付提供方不匹配 trade_no=%s provider=%s", tradeNo, topUp.PaymentProvider))
		writeOkpayCallbackStatus(c, false)
		return
	}
	if topUp.Status == common.TopUpStatusSuccess {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 充值订单已完成，幂等返回 trade_no=%s", tradeNo))
		writeOkpayCallbackStatus(c, true)
		return
	}
	legacySnapshot, err := validateOkpayCallbackSnapshot(callback, topUp.ProviderOrderId, topUp.ProviderAmount, topUp.ProviderCurrency)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 充值回调快照校验失败 trade_no=%s order_id=%s error=%q", tradeNo, orderID, err.Error()))
		writeOkpayCallbackStatus(c, false)
		return
	}
	if legacySnapshot {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 充值订单缺少网关快照，按存量订单兼容处理 trade_no=%s order_id=%s", tradeNo, orderID))
	}

	err = model.RechargeOkpay(tradeNo, c.ClientIP())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 充值处理失败 trade_no=%s order_id=%s client_ip=%s error=%q", tradeNo, orderID, c.ClientIP(), err.Error()))
		writeOkpayCallbackStatus(c, false)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 充值成功 trade_no=%s order_id=%s amount=%s coin=%s client_ip=%s",
		tradeNo, orderID,
		callback.Get("data[amount]"),
		callback.Get("data[coin]"),
		c.ClientIP()))
	writeOkpayCallbackStatus(c, true)
}
