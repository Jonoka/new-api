package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func resetOkpayRateCacheForTest() {
	okpayRateCacheMu.Lock()
	defer okpayRateCacheMu.Unlock()
	okpayRateCache = okpayRateCacheEntry{}
}

func TestOkpayPaymentAmountUsesFallbackUsdtCnyRate(t *testing.T) {
	originalExchangeRate := setting.OkpayExchangeRate
	originalAutoExchangeEnabled := setting.OkpayAutoExchangeEnabled
	originalUsdtCnyRate := setting.OkpayUsdtCnyRate
	originalCoin := setting.OkpayCoin
	t.Cleanup(func() {
		setting.OkpayExchangeRate = originalExchangeRate
		setting.OkpayAutoExchangeEnabled = originalAutoExchangeEnabled
		setting.OkpayUsdtCnyRate = originalUsdtCnyRate
		setting.OkpayCoin = originalCoin
		resetOkpayRateCacheForTest()
	})

	setting.OkpayExchangeRate = 7.2
	setting.OkpayAutoExchangeEnabled = false
	setting.OkpayUsdtCnyRate = 6.8
	setting.OkpayCoin = "USDT"

	amount := getOkpayPaymentAmountFromFiat(72)

	require.Equal(t, "USDT", amount.Coin)
	require.Equal(t, "fallback", amount.RateSource)
	require.False(t, amount.AutoRateFailed)
	require.InDelta(t, 6.8, amount.Rate, 0.000001)
	require.InDelta(t, 10.58823529, amount.CoinAmount, 0.00000001)
}

func TestOkpayPaymentAmountUsesCachedAutoRate(t *testing.T) {
	originalAutoExchangeEnabled := setting.OkpayAutoExchangeEnabled
	originalUsdtCnyRate := setting.OkpayUsdtCnyRate
	originalCoin := setting.OkpayCoin
	originalAdjustmentType := setting.OkpayRateAdjustmentType
	originalAdjustmentValue := setting.OkpayRateAdjustmentValue
	t.Cleanup(func() {
		setting.OkpayAutoExchangeEnabled = originalAutoExchangeEnabled
		setting.OkpayUsdtCnyRate = originalUsdtCnyRate
		setting.OkpayCoin = originalCoin
		setting.OkpayRateAdjustmentType = originalAdjustmentType
		setting.OkpayRateAdjustmentValue = originalAdjustmentValue
		resetOkpayRateCacheForTest()
	})

	setting.OkpayAutoExchangeEnabled = true
	setting.OkpayUsdtCnyRate = 7.2
	setting.OkpayCoin = "USDT"
	setting.OkpayRateAdjustmentType = "absolute"
	setting.OkpayRateAdjustmentValue = 0
	okpayRateCacheMu.Lock()
	okpayRateCache = okpayRateCacheEntry{
		rate:      6.75,
		source:    "test",
		configKey: okpayRateCacheKey(),
		expiresAt: time.Now().Add(time.Minute),
	}
	okpayRateCacheMu.Unlock()

	amount := getOkpayPaymentAmountFromFiat(67.5)

	require.Equal(t, "test", amount.RateSource)
	require.False(t, amount.AutoRateFailed)
	require.InDelta(t, 6.75, amount.Rate, 0.000001)
	require.InDelta(t, 10, amount.CoinAmount, 0.00000001)
}

func TestOkpayPaymentAmountLeavesNonUsdtCoinAsConfiguredAmount(t *testing.T) {
	originalAutoExchangeEnabled := setting.OkpayAutoExchangeEnabled
	originalCoin := setting.OkpayCoin
	t.Cleanup(func() {
		setting.OkpayAutoExchangeEnabled = originalAutoExchangeEnabled
		setting.OkpayCoin = originalCoin
		resetOkpayRateCacheForTest()
	})

	setting.OkpayAutoExchangeEnabled = true
	setting.OkpayCoin = "TRX"

	amount := getOkpayPaymentAmountFromFiat(12.34)

	require.Equal(t, "TRX", amount.Coin)
	require.Equal(t, "coin", amount.RateSource)
	require.InDelta(t, 1, amount.Rate, 0.000001)
	require.InDelta(t, 12.34, amount.CoinAmount, 0.00000001)
}

func TestParseOkpayRateFromBody(t *testing.T) {
	rate, err := parseOkpayRateFromBody([]byte(`{"tether":{"cny":6.76,"last_updated_at":1782038217}}`))

	require.NoError(t, err)
	require.InDelta(t, 6.76, rate, 0.000001)
}

func TestParseOkpayOkxAlipayTierRateFromBody(t *testing.T) {
	body := []byte(`{"code":0,"data":{"buy":[{"price":"6.71"},{"price":"6.72"},{"price":"6.73"}],"sell":[{"price":"6.81"}]}}`)

	rate, err := parseOkpayOkxAlipayTierRateFromBody(body, "buy", 3)

	require.NoError(t, err)
	require.InDelta(t, 6.73, rate, 0.000001)
}

func TestFetchOkpayUsdtCnyRateQuoteUsesOkxTierAndAbsoluteAdjustment(t *testing.T) {
	originalRateSource := setting.OkpayRateSource
	originalRateApiUrl := setting.OkpayRateApiUrl
	originalOkxSide := setting.OkpayOkxSide
	originalOkxTier := setting.OkpayOkxTier
	originalAdjustmentType := setting.OkpayRateAdjustmentType
	originalAdjustmentValue := setting.OkpayRateAdjustmentValue
	t.Cleanup(func() {
		setting.OkpayRateSource = originalRateSource
		setting.OkpayRateApiUrl = originalRateApiUrl
		setting.OkpayOkxSide = originalOkxSide
		setting.OkpayOkxTier = originalOkxTier
		setting.OkpayRateAdjustmentType = originalAdjustmentType
		setting.OkpayRateAdjustmentValue = originalAdjustmentValue
		resetOkpayRateCacheForTest()
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.Header.Get("User-Agent"), "Mozilla")
		_, _ = w.Write([]byte(`{"code":0,"data":{"buy":[{"price":"6.70"},{"price":"6.80"},{"price":"6.90"}]}}`))
	}))
	defer server.Close()

	setting.OkpayRateSource = "okx-alipay-tier"
	setting.OkpayRateApiUrl = server.URL
	setting.OkpayOkxSide = "buy"
	setting.OkpayOkxTier = 2
	setting.OkpayRateAdjustmentType = "absolute"
	setting.OkpayRateAdjustmentValue = -0.2

	quote, err := fetchOkpayUsdtCnyRateQuote()

	require.NoError(t, err)
	require.Equal(t, "okx-alipay-tier", quote.Source)
	require.Equal(t, "buy", quote.Side)
	require.Equal(t, 2, quote.Tier)
	require.InDelta(t, 6.8, quote.RawRate, 0.000001)
	require.InDelta(t, 6.6, quote.AdjustedRate, 0.000001)
}

func TestOkpayPaymentAmountUsesOkxAlipayRateModule(t *testing.T) {
	originalRateSource := setting.OkpayRateSource
	originalAutoExchangeEnabled := setting.OkpayAutoExchangeEnabled
	originalUsdtCnyRate := setting.OkpayUsdtCnyRate
	originalCoin := setting.OkpayCoin
	originalManager := extension.DefaultManager
	t.Cleanup(func() {
		setting.OkpayRateSource = originalRateSource
		setting.OkpayAutoExchangeEnabled = originalAutoExchangeEnabled
		setting.OkpayUsdtCnyRate = originalUsdtCnyRate
		setting.OkpayCoin = originalCoin
		extension.DefaultManager = originalManager
		resetOkpayRateCacheForTest()
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"buy":[{"price":"6.70"},{"price":"6.80"}]}}`))
	}))
	defer server.Close()

	rootDir := t.TempDir()
	moduleDir := filepath.Join(rootDir, extension.OkxAlipayRateModuleID)
	require.NoError(t, os.MkdirAll(filepath.Join(moduleDir, "public"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "manifest.json"), []byte(`{
		"id":"okx-alipay-rate",
		"name":"OKX 支付宝汇率",
		"version":"0.2.0",
		"runtime":{"type":"static","static_dir":"public"},
		"permissions":{"roles":["root"]}
	}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "public", "index.html"), []byte("ok"), 0644))
	manager := extension.NewManager(rootDir)
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled(extension.OkxAlipayRateModuleID, true)
	require.NoError(t, err)
	extension.DefaultManager = manager

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	originalOptions := map[string]string{
		extension.OkxAlipayRateOptionRateAPIURL:      common.OptionMap[extension.OkxAlipayRateOptionRateAPIURL],
		extension.OkxAlipayRateOptionSide:            common.OptionMap[extension.OkxAlipayRateOptionSide],
		extension.OkxAlipayRateOptionTier:            common.OptionMap[extension.OkxAlipayRateOptionTier],
		extension.OkxAlipayRateOptionAdjustmentType:  common.OptionMap[extension.OkxAlipayRateOptionAdjustmentType],
		extension.OkxAlipayRateOptionAdjustmentValue: common.OptionMap[extension.OkxAlipayRateOptionAdjustmentValue],
	}
	common.OptionMap[extension.OkxAlipayRateOptionRateAPIURL] = server.URL
	common.OptionMap[extension.OkxAlipayRateOptionSide] = "buy"
	common.OptionMap[extension.OkxAlipayRateOptionTier] = "2"
	common.OptionMap[extension.OkxAlipayRateOptionAdjustmentType] = "absolute"
	common.OptionMap[extension.OkxAlipayRateOptionAdjustmentValue] = "-0.2"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		for key, value := range originalOptions {
			if value == "" {
				delete(common.OptionMap, key)
				continue
			}
			common.OptionMap[key] = value
		}
	})

	setting.OkpayRateSource = extension.OkxAlipayRateSourceID
	setting.OkpayAutoExchangeEnabled = true
	setting.OkpayUsdtCnyRate = 7.2
	setting.OkpayCoin = "USDT"

	amount := getOkpayPaymentAmountFromFiat(66)

	require.Equal(t, extension.OkxAlipayRateSourceID, amount.RateSource)
	require.False(t, amount.AutoRateFailed)
	require.InDelta(t, 6.6, amount.Rate, 0.000001)
	require.InDelta(t, 10, amount.CoinAmount, 0.00000001)
}

func TestApplyOkpayRateAdjustmentPercent(t *testing.T) {
	originalAdjustmentType := setting.OkpayRateAdjustmentType
	originalAdjustmentValue := setting.OkpayRateAdjustmentValue
	t.Cleanup(func() {
		setting.OkpayRateAdjustmentType = originalAdjustmentType
		setting.OkpayRateAdjustmentValue = originalAdjustmentValue
	})

	setting.OkpayRateAdjustmentType = "percent"
	setting.OkpayRateAdjustmentValue = -10

	adjusted, err := applyOkpayRateAdjustment(6.8)

	require.NoError(t, err)
	require.InDelta(t, 6.12, adjusted, 0.000001)
}

func TestGetOkpayFiatPayMoneyKeepsCnyUnitPriceSemantics(t *testing.T) {
	originalExchangeRate := setting.OkpayExchangeRate
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := make(map[int]float64, len(operation_setting.GetPaymentSetting().AmountDiscount))
	for k, v := range operation_setting.GetPaymentSetting().AmountDiscount {
		originalDiscounts[k] = v
	}
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		setting.OkpayExchangeRate = originalExchangeRate
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	setting.OkpayExchangeRate = 7.2
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{10: 0.5}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":1.2}`))

	require.InDelta(t, 43.2, getOkpayFiatPayMoney(10, "vip"), 0.000001)
}

func TestCalculateOkpayAffiliateSourceQuotaUsesPurchasedCreditRatio(t *testing.T) {
	quota := calculateOkpayAffiliateSourceQuota(10, 72, 36)

	require.Equal(t, int(5*common.QuotaPerUnit), quota)
}

func signedOkpayCallbackValues(params map[string]string, merchantToken string) url.Values {
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	values.Set("sign", generateOkpaySignature(params, merchantToken))
	return values
}

func newOkpayCallbackContext(method string, target string, body string, contentType string) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		ctx.Request.Header.Set("Content-Type", contentType)
	}
	return ctx
}

func TestParseOkpayCallbackValuesSupportsGetQuery(t *testing.T) {
	merchantToken := "okpay-token"
	values := signedOkpayCallbackValues(map[string]string{
		"status":          "success",
		"data[status]":    "1",
		"data[unique_id]": "USR1NOabc",
		"data[order_id]":  "OK123",
	}, merchantToken)
	ctx := newOkpayCallbackContext(http.MethodGet, "/api/okpay/notify?"+values.Encode(), "", "")

	parsed, bodyBytes, err := parseOkpayCallbackValues(ctx)

	require.NoError(t, err)
	require.Empty(t, bodyBytes)
	require.Equal(t, "USR1NOabc", parsed.Get("data[unique_id]"))
	require.True(t, verifyOkpayCallbackSignature(parsed, merchantToken))
	require.True(t, isOkpayCallbackSuccess(parsed.Get("status"), parsed.Get("data[status]")))
}

func TestParseOkpayCallbackValuesSupportsPostForm(t *testing.T) {
	merchantToken := "okpay-token"
	values := signedOkpayCallbackValues(map[string]string{
		"status":          "success",
		"data[status]":    "1",
		"data[unique_id]": "USR2NOabc",
		"data[amount]":    "10.00000000",
	}, merchantToken)
	ctx := newOkpayCallbackContext(http.MethodPost, "/api/okpay/notify", values.Encode(), "application/x-www-form-urlencoded")

	parsed, bodyBytes, err := parseOkpayCallbackValues(ctx)

	require.NoError(t, err)
	require.NotEmpty(t, bodyBytes)
	require.Equal(t, "10.00000000", parsed.Get("data[amount]"))
	require.True(t, verifyOkpayCallbackSignature(parsed, merchantToken))
}

func TestParseOkpayCallbackValuesSupportsNestedJSON(t *testing.T) {
	merchantToken := "okpay-token"
	params := map[string]string{
		"status":          "success",
		"data[status]":    "1",
		"data[unique_id]": "USR3NOabc",
		"data[order_id]":  "OK789",
	}
	sign := generateOkpaySignature(params, merchantToken)
	body := `{"status":"success","data":{"status":1,"unique_id":"USR3NOabc","order_id":"OK789"},"sign":"` + sign + `"}`
	ctx := newOkpayCallbackContext(http.MethodPost, "/api/okpay/notify", body, "application/json")

	parsed, _, err := parseOkpayCallbackValues(ctx)

	require.NoError(t, err)
	require.Equal(t, "1", parsed.Get("data[status]"))
	require.Equal(t, "USR3NOabc", parsed.Get("data[unique_id]"))
	require.True(t, verifyOkpayCallbackSignature(parsed, merchantToken))
	require.True(t, isOkpayCallbackSuccess(parsed.Get("status"), parsed.Get("data[status]")))
}

func TestVerifyOkpayCallbackSignatureMatchesDocumentOrder(t *testing.T) {
	body := "code=200&data[order_id]=ac7b86615fdb137576ae35879f7ed844&data[unique_id]=BWIN-20250922152023LDVNSyxLQko&data[pay_user_id]=7238234930&data[amount]=6.00000000&data[coin]=USDT&data[status]=1&data[type]=deposit&id=1&status=success&sign=95BE540FB7D1996770E2B4CDBC6F184D"
	ctx := newOkpayCallbackContext(http.MethodPost, "/api/okpay/notify", body, "application/x-www-form-urlencoded")

	parsed, _, err := parseOkpayCallbackValues(ctx)

	require.NoError(t, err)
	require.Equal(t, "BWIN-20250922152023LDVNSyxLQko", parsed.Get("data[unique_id]"))
	require.True(t, verifyOkpayCallbackSignature(parsed, "123456"))
	require.True(t, isOkpayCallbackSuccess(parsed.Get("status"), parsed.Get("data[status]")))
}

func TestVerifyOkpayCallbackSignatureMatchesDocumentOrderJSON(t *testing.T) {
	body := `{"code":200,"data":{"order_id":"ac7b86615fdb137576ae35879f7ed844","unique_id":"BWIN-20250922152023LDVNSyxLQko","pay_user_id":7238234930,"amount":"6.00000000","coin":"USDT","status":1,"type":"deposit"},"id":1,"status":"success","sign":"95BE540FB7D1996770E2B4CDBC6F184D"}`
	ctx := newOkpayCallbackContext(http.MethodPost, "/api/okpay/notify", body, "application/json")

	parsed, _, err := parseOkpayCallbackValues(ctx)

	require.NoError(t, err)
	require.Equal(t, "7238234930", parsed.Get("data[pay_user_id]"))
	require.True(t, verifyOkpayCallbackSignature(parsed, "123456"))
	require.True(t, isOkpayCallbackSuccess(parsed.Get("status"), parsed.Get("data[status]")))
}
