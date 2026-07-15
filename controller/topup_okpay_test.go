package controller

import (
	"crypto/md5"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func okpaySignatureForTest(raw string, token string) string {
	hash := md5.Sum([]byte(raw + "&token=" + token))
	return strings.ToUpper(fmt.Sprintf("%x", hash))
}

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
	t.Cleanup(func() {
		setting.OkpayAutoExchangeEnabled = originalAutoExchangeEnabled
		setting.OkpayUsdtCnyRate = originalUsdtCnyRate
		setting.OkpayCoin = originalCoin
		resetOkpayRateCacheForTest()
	})

	setting.OkpayAutoExchangeEnabled = true
	setting.OkpayUsdtCnyRate = 7.2
	setting.OkpayCoin = "USDT"
	okpayRateCacheMu.Lock()
	okpayRateCache = okpayRateCacheEntry{
		rate:      6.75,
		source:    "test",
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

func TestVerifyOkpayCallbackSignatureMatchesDocumentOrder(t *testing.T) {
	const token = "123456"
	const bodyWithoutSign = "code=200&data[order_id]=ac7b86615fdb137576ae35879f7ed844&data[unique_id]=BWIN-20250922152023LDVNSyxLQko&data[pay_user_id]=7238234930&data[amount]=6.00000000&data[coin]=USDT&data[status]=1&data[type]=deposit&id=1&status=success"
	const documentedSignature = "95BE540FB7D1996770E2B4CDBC6F184D"

	payload, err := parseOkpayCallbackPayload([]byte(bodyWithoutSign + "&sign=" + documentedSignature))
	require.NoError(t, err)
	require.Equal(t, "BWIN-20250922152023LDVNSyxLQko", payload.Get("data[unique_id]"))
	require.True(t, verifyOkpayCallbackSignature(payload, token))
}

func TestVerifyOkpayJSONCallbackSignature(t *testing.T) {
	const token = "123456"
	const signature = "95BE540FB7D1996770E2B4CDBC6F184D"
	body := `{"code":200,"data":{"order_id":"ac7b86615fdb137576ae35879f7ed844","unique_id":"BWIN-20250922152023LDVNSyxLQko","pay_user_id":7238234930,"amount":"6.00000000","coin":"USDT","status":1,"type":"deposit"},"id":1,"status":"success","sign":"` + signature + `"}`

	payload, err := parseOkpayCallbackPayload([]byte(body))
	require.NoError(t, err)
	require.Equal(t, "ac7b86615fdb137576ae35879f7ed844", payload.Get("data[order_id]"))
	require.Equal(t, "1", payload.Get("data[status]"))
	require.True(t, verifyOkpayCallbackSignature(payload, token))
}

func TestVerifyOkpayCallbackSignatureFallsBackToSortedKeys(t *testing.T) {
	const token = "123456"
	const raw = "id=1&status=success&code=200&data[amount]=6.00000000&data[coin]=USDT&data[order_id]=gateway-order&data[status]=1&data[unique_id]=trade-no"
	pairs := strings.Split(raw, "&")
	sort.Strings(pairs)
	signature := okpaySignatureForTest(strings.Join(pairs, "&"), token)

	payload, err := parseOkpayCallbackPayload([]byte(raw + "&sign=" + signature))
	require.NoError(t, err)
	require.True(t, verifyOkpayCallbackSignature(payload, token))
}

func TestVerifyOkpayCallbackRejectsInvalidSignature(t *testing.T) {
	payload, err := parseOkpayCallbackPayload([]byte("status=success&data[unique_id]=trade-no&sign=invalid"))
	require.NoError(t, err)
	require.False(t, verifyOkpayCallbackSignature(payload, "123456"))
}
