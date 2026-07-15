package controller

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
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
	"github.com/QuantumNous/new-api/extension"
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
	configKey string
	expiresAt time.Time
}

type okpayRateQuote struct {
	RawRate        float64 `json:"raw_rate"`
	AdjustedRate   float64 `json:"adjusted_rate"`
	Source         string  `json:"source"`
	Tier           int     `json:"tier,omitempty"`
	Side           string  `json:"side,omitempty"`
	Adjustment     float64 `json:"adjustment"`
	AdjustmentType string  `json:"adjustment_type"`
}

type okpaySignPair struct {
	Key   string
	Value string
}

var (
	okpayRateCacheMu sync.Mutex
	okpayRateCache   okpayRateCacheEntry
)

const okpayRateCacheTTL = 5 * time.Minute

const (
	okpayRateSourceCoinGecko     = "coingecko"
	okpayRateSourceOkxAlipayTier = "okx-alipay-tier"
	okpayRateSourceOkxModule     = extension.OkxAlipayRateSourceID
	okpayAdjustmentTypeAbsolute  = "absolute"
	okpayAdjustmentTypePercent   = "percent"
	okpayDefaultCoinGeckoRateUrl = "https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true"
)

var okpayCallbackSignatureOrder = []string{
	"code",
	"data[order_id]",
	"data[unique_id]",
	"data[pay_user_id]",
	"data[amount]",
	"data[coin]",
	"data[status]",
	"data[type]",
	"id",
	"status",
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

	pairs := make([]okpaySignPair, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, okpaySignPair{Key: k, Value: params[k]})
	}
	return generateOkpaySignatureFromPairs(pairs, merchantToken)
}

func generateOkpaySignatureFromPairs(pairs []okpaySignPair, merchantToken string) string {
	signParts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		key := strings.TrimSpace(pair.Key)
		value := strings.TrimSpace(pair.Value)
		if key == "" || value == "" || strings.EqualFold(key, "sign") {
			continue
		}
		signParts = append(signParts, key+"="+value)
	}
	if len(signParts) == 0 {
		return ""
	}
	query := strings.Join(signParts, "&")
	query += "&token=" + strings.TrimSpace(merchantToken)

	hash := md5.Sum([]byte(query))
	return strings.ToUpper(fmt.Sprintf("%x", hash))
}

func generateOkpayCallbackOrderedSignature(formValues url.Values, merchantToken string) string {
	pairs := make([]okpaySignPair, 0, len(okpayCallbackSignatureOrder))
	for _, key := range okpayCallbackSignatureOrder {
		value := strings.TrimSpace(formValues.Get(key))
		if value == "" {
			continue
		}
		pairs = append(pairs, okpaySignPair{Key: key, Value: value})
	}
	return generateOkpaySignatureFromPairs(pairs, merchantToken)
}

// verifyOkpayCallbackSignature 验证回调签名。
// 依次兼容官方字段顺序、上游实际字段顺序及字典序。
func verifyOkpayCallbackSignature(formValues url.Values, merchantToken string, orderedPairs ...[]okpaySignPair) bool {
	actual := strings.TrimSpace(formValues.Get("sign"))
	if actual == "" {
		return false
	}

	// OKPay 回调签名按官方回调字段顺序生成；独角数卡/Dujiao-Next
	// 也是先按该顺序验签，再用字典序作为兼容兜底。
	if expected := generateOkpayCallbackOrderedSignature(formValues, merchantToken); expected != "" && strings.EqualFold(expected, actual) {
		return true
	}
	if len(orderedPairs) > 0 && okpayOrderedPairsCoverValues(orderedPairs[0], formValues) {
		if expected := generateOkpaySignatureFromPairs(orderedPairs[0], merchantToken); expected != "" && strings.EqualFold(expected, actual) {
			return true
		}
	}

	params := make(map[string]string)
	for key := range formValues {
		value := strings.TrimSpace(formValues.Get(key))
		if strings.EqualFold(key, "sign") || value == "" {
			continue
		}
		params[key] = value
	}
	expected := generateOkpaySignature(params, merchantToken)
	return strings.EqualFold(expected, actual)
}

func okpayOrderedPairsCoverValues(pairs []okpaySignPair, values url.Values) bool {
	orderedValues := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key := strings.TrimSpace(pair.Key)
		value := strings.TrimSpace(pair.Value)
		if key == "" || value == "" || strings.EqualFold(key, "sign") {
			continue
		}
		orderedValues[key] = value
	}
	for key := range values {
		value := strings.TrimSpace(values.Get(key))
		if value == "" || strings.EqualFold(key, "sign") {
			continue
		}
		if orderedValues[key] != value {
			return false
		}
	}
	return true
}

func parseOkpayCallbackOrderedPairs(body []byte) []okpaySignPair {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil
	}

	pairs := make([]okpaySignPair, 0)
	if strings.HasPrefix(trimmed, "{") {
		if !gjson.Valid(trimmed) {
			return nil
		}
		root := gjson.Parse(trimmed)
		if !root.IsObject() {
			return nil
		}
		flattenOkpayJSONSignPairs("", root, &pairs)
		return pairs
	}

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
		pairs = append(pairs, okpaySignPair{Key: key, Value: value})
	}
	return pairs
}

func flattenOkpayJSONSignPairs(prefix string, value gjson.Result, pairs *[]okpaySignPair) {
	if value.IsObject() {
		value.ForEach(func(key, child gjson.Result) bool {
			childKey := key.String()
			if prefix != "" {
				childKey = prefix + "[" + childKey + "]"
			}
			flattenOkpayJSONSignPairs(childKey, child, pairs)
			return true
		})
		return
	}
	if value.IsArray() {
		index := 0
		value.ForEach(func(_, child gjson.Result) bool {
			flattenOkpayJSONSignPairs(fmt.Sprintf("%s[%d]", prefix, index), child, pairs)
			index++
			return true
		})
		return
	}
	*pairs = append(*pairs, okpaySignPair{Key: prefix, Value: value.String()})
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

func normalizeOkpayRateSource() string {
	source := strings.ToLower(strings.TrimSpace(setting.OkpayRateSource))
	switch source {
	case okpayRateSourceOkxModule:
		return okpayRateSourceOkxModule
	case okpayRateSourceOkxAlipayTier:
		return okpayRateSourceOkxAlipayTier
	default:
		return okpayRateSourceCoinGecko
	}
}

func normalizeOkpayOkxSide() string {
	side := strings.ToLower(strings.TrimSpace(setting.OkpayOkxSide))
	if side == "sell" {
		return "sell"
	}
	return "buy"
}

func getOkpayOkxTier() int {
	if setting.OkpayOkxTier <= 0 {
		return 3
	}
	return setting.OkpayOkxTier
}

func normalizeOkpayAdjustmentType() string {
	adjustmentType := strings.ToLower(strings.TrimSpace(setting.OkpayRateAdjustmentType))
	if adjustmentType == okpayAdjustmentTypePercent {
		return okpayAdjustmentTypePercent
	}
	return okpayAdjustmentTypeAbsolute
}

func okpayRateCacheKey() string {
	source := normalizeOkpayRateSource()
	if source == okpayRateSourceOkxModule {
		return source + "|" + extension.OkxAlipayRateConfigCacheKey()
	}
	return fmt.Sprintf(
		"%s|%s|%s|%d|%s|%.8f",
		source,
		strings.TrimSpace(setting.OkpayRateApiUrl),
		normalizeOkpayOkxSide(),
		getOkpayOkxTier(),
		normalizeOkpayAdjustmentType(),
		setting.OkpayRateAdjustmentValue,
	)
}

func applyOkpayRateAdjustment(rate float64) (float64, error) {
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, fmt.Errorf("invalid raw rate")
	}
	adjusted := rate
	value := setting.OkpayRateAdjustmentValue
	switch normalizeOkpayAdjustmentType() {
	case okpayAdjustmentTypePercent:
		adjusted = decimal.NewFromFloat(rate).
			Mul(decimal.NewFromFloat(100).Add(decimal.NewFromFloat(value))).
			Div(decimal.NewFromFloat(100)).
			InexactFloat64()
	default:
		adjusted = decimal.NewFromFloat(rate).Add(decimal.NewFromFloat(value)).InexactFloat64()
	}
	if adjusted <= 0 || math.IsNaN(adjusted) || math.IsInf(adjusted, 0) {
		return 0, fmt.Errorf("adjusted rate must be greater than zero")
	}
	return adjusted, nil
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

func parseOkpayOkxAlipayTierRateFromBody(body []byte, side string, tier int) (float64, error) {
	if tier <= 0 {
		tier = 3
	}
	var payload map[string]interface{}
	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, err
	}
	rawData, ok := payload["data"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("missing okx data")
	}
	rawOrders, ok := rawData[side].([]interface{})
	if !ok || len(rawOrders) < tier {
		return 0, fmt.Errorf("missing okx %s tier %d", side, tier)
	}
	order, ok := rawOrders[tier-1].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid okx tier %d", tier)
	}
	rawPrice, ok := order["price"]
	if !ok {
		return 0, fmt.Errorf("missing okx price")
	}
	rate, err := strconv.ParseFloat(fmt.Sprintf("%v", rawPrice), 64)
	if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, fmt.Errorf("invalid okx price")
	}
	return rate, nil
}

func defaultOkpayOkxRateApiUrl() string {
	return fmt.Sprintf(
		"https://www.okx.com/v3/c2c/tradingOrders/books?quoteCurrency=CNY&baseCurrency=USDT&side=%s&paymentMethod=aliPay",
		normalizeOkpayOkxSide(),
	)
}

func newOkpayRateRequest(rateUrl string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, rateUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")
	return req, nil
}

func fetchOkpayUsdtCnyRateQuote() (okpayRateQuote, error) {
	rateUrl := strings.TrimSpace(setting.OkpayRateApiUrl)
	source := normalizeOkpayRateSource()
	if source == okpayRateSourceOkxModule {
		quote, err := extension.FetchEnabledOkxAlipayRateQuote()
		if err != nil {
			return okpayRateQuote{}, err
		}
		return okpayRateQuote{
			RawRate:        quote.RawRate,
			AdjustedRate:   quote.AdjustedRate,
			Source:         quote.Source,
			Tier:           quote.Tier,
			Side:           quote.Side,
			Adjustment:     quote.AdjustmentValue,
			AdjustmentType: quote.AdjustmentType,
		}, nil
	}
	if source == okpayRateSourceOkxAlipayTier {
		if rateUrl == "" || strings.EqualFold(rateUrl, okpayDefaultCoinGeckoRateUrl) {
			rateUrl = defaultOkpayOkxRateApiUrl()
		}
	} else if rateUrl == "" {
		return okpayRateQuote{}, fmt.Errorf("rate api url is empty")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := newOkpayRateRequest(rateUrl)
	if err != nil {
		return okpayRateQuote{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return okpayRateQuote{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return okpayRateQuote{}, err
	}
	if resp.StatusCode/100 != 2 {
		return okpayRateQuote{}, fmt.Errorf("rate api http %d", resp.StatusCode)
	}

	side := ""
	tier := 0
	var rate float64
	switch source {
	case okpayRateSourceOkxAlipayTier:
		side = normalizeOkpayOkxSide()
		tier = getOkpayOkxTier()
		rate, err = parseOkpayOkxAlipayTierRateFromBody(body, side, tier)
	default:
		source = okpayRateSourceCoinGecko
		rate, err = parseOkpayRateFromBody(body)
	}
	if err != nil {
		return okpayRateQuote{}, err
	}
	adjustedRate, err := applyOkpayRateAdjustment(rate)
	if err != nil {
		return okpayRateQuote{}, err
	}
	return okpayRateQuote{
		RawRate:        rate,
		AdjustedRate:   adjustedRate,
		Source:         source,
		Tier:           tier,
		Side:           side,
		Adjustment:     setting.OkpayRateAdjustmentValue,
		AdjustmentType: normalizeOkpayAdjustmentType(),
	}, nil
}

func fetchOkpayUsdtCnyRate() (float64, string, error) {
	quote, err := fetchOkpayUsdtCnyRateQuote()
	if err != nil {
		return 0, "", err
	}
	return quote.AdjustedRate, quote.Source, nil
}

// PreviewOkpayRate 获取当前 OKPay 汇率配置的实时预览，不写入任何状态。
func PreviewOkpayRate(c *gin.Context) {
	quote, err := fetchOkpayUsdtCnyRateQuote()
	if err != nil {
		common.ApiErrorMsg(c, "获取 OKPay 汇率失败: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"raw_rate":        strconv.FormatFloat(quote.RawRate, 'f', -1, 64),
			"adjusted_rate":   strconv.FormatFloat(quote.AdjustedRate, 'f', -1, 64),
			"source":          quote.Source,
			"side":            quote.Side,
			"tier":            quote.Tier,
			"adjustment_type": quote.AdjustmentType,
			"adjustment":      strconv.FormatFloat(quote.Adjustment, 'f', -1, 64),
		},
	})
}

func getOkpayUsdtCnyRate() (float64, string, bool) {
	fallbackRate := getOkpayFallbackUsdtCnyRate()
	if !setting.OkpayAutoExchangeEnabled {
		return fallbackRate, "fallback", false
	}

	now := time.Now()
	cacheKey := okpayRateCacheKey()
	okpayRateCacheMu.Lock()
	if okpayRateCache.rate > 0 && okpayRateCache.configKey == cacheKey && now.Before(okpayRateCache.expiresAt) {
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
		configKey: cacheKey,
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
			model.RecordTopupLog(completedTopUp.UserId, fmt.Sprintf("使用优惠码充值成功，充值金额: %v，支付金额：0.00", logger.LogQuota(quotaToAdd)), c.ClientIP(), completedTopUp.PaymentMethod, "promo")
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

func validateOkpayCallbackSnapshot(callback url.Values, providerOrderId string, providerAmount string, providerCurrency string) (bool, error) {
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
	orderedSource := bodyBytes
	if len(bytes.TrimSpace(orderedSource)) == 0 {
		orderedSource = []byte(c.Request.URL.RawQuery)
	}
	orderedPairs := parseOkpayCallbackOrderedPairs(orderedSource)
	if !verifyOkpayCallbackSignature(formValues, setting.OkpayMerchantToken, orderedPairs) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 验签失败 path=%q method=%s client_ip=%s sign=%q params=%q", c.Request.RequestURI, c.Request.Method, c.ClientIP(), sign, common.GetJsonString(formValues)))
		writeOkpayCallbackStatus(c, false)
		return
	}

	merchantId := strings.TrimSpace(formValues.Get("id"))
	if configuredMerchantId := strings.TrimSpace(setting.OkpayMerchantId); merchantId != "" && configuredMerchantId != "" && merchantId != configuredMerchantId {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay webhook 商户 ID 不匹配 client_ip=%s merchant_id=%q", c.ClientIP(), merchantId))
		writeOkpayCallbackStatus(c, false)
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
		legacySnapshot, err := validateOkpayCallbackSnapshot(formValues, order.ProviderOrderId, order.ProviderAmount, order.ProviderCurrency)
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 订阅回调快照校验失败 trade_no=%s order_id=%s error=%q", tradeNo, orderID, err.Error()))
			writeOkpayCallbackStatus(c, false)
			return
		}
		if legacySnapshot {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("OKPay 订阅订单缺少网关快照，按存量订单兼容处理 trade_no=%s order_id=%s", tradeNo, orderID))
		}
		if err := model.CompleteSubscriptionOrder(tradeNo, common.GetJsonString(formValues), model.PaymentProviderOkpay, model.PaymentMethodOkpay); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 订阅处理失败 trade_no=%s order_id=%s client_ip=%s error=%q", tradeNo, orderID, c.ClientIP(), err.Error()))
			writeOkpayCallbackStatus(c, false)
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("OKPay 订阅购买成功 trade_no=%s order_id=%s amount=%s coin=%s client_ip=%s", tradeNo, orderID, formValues.Get("data[amount]"), formValues.Get("data[coin]"), c.ClientIP()))
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
	legacySnapshot, err := validateOkpayCallbackSnapshot(formValues, topUp.ProviderOrderId, topUp.ProviderAmount, topUp.ProviderCurrency)
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
		formValues.Get("data[amount]"),
		formValues.Get("data[coin]"),
		c.ClientIP()))
	writeOkpayCallbackStatus(c, true)
}
