package extension

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestParseOkxAlipayTierRateFromBody(t *testing.T) {
	body := []byte(`{"code":0,"data":{"buy":[{"id":"a","nickName":"one","price":"6.70"},{"id":"b","nickName":"two","price":"6.80"},{"id":"c","nickName":"three","price":"6.90"}]}}`)

	order, err := ParseOkxAlipayTierRateFromBody(body, "buy", 3)

	require.NoError(t, err)
	require.InDelta(t, 6.9, order.Price, 0.000001)
	require.Equal(t, "c", order.ID)
	require.Equal(t, "three", order.NickName)
}

func TestFetchOkxAlipayRateQuoteAppliesAbsoluteAdjustment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.Header.Get("User-Agent"), "Mozilla")
		_, _ = w.Write([]byte(`{"code":0,"data":{"buy":[{"price":"6.70"},{"price":"6.80"},{"price":"6.90"}]}}`))
	}))
	defer server.Close()

	quote, err := FetchOkxAlipayRateQuote(OkxAlipayRateConfig{
		RateAPIURL:      server.URL,
		Side:            "buy",
		Tier:            2,
		AdjustmentType:  OkxAlipayRateAdjustmentTypeAbsolute,
		AdjustmentValue: -0.2,
	})

	require.NoError(t, err)
	require.Equal(t, OkxAlipayRateSourceID, quote.Source)
	require.InDelta(t, 6.8, quote.RawRate, 0.000001)
	require.InDelta(t, 6.6, quote.AdjustedRate, 0.000001)
}

func TestGetOkxAlipayRateConfigUsesOptionMap(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	original := map[string]string{}
	for _, key := range []string{
		OkxAlipayRateOptionRateAPIURL,
		OkxAlipayRateOptionSide,
		OkxAlipayRateOptionTier,
		OkxAlipayRateOptionAdjustmentType,
		OkxAlipayRateOptionAdjustmentValue,
	} {
		original[key] = common.OptionMap[key]
	}
	common.OptionMap[OkxAlipayRateOptionRateAPIURL] = "https://example.test/okx"
	common.OptionMap[OkxAlipayRateOptionSide] = "sell"
	common.OptionMap[OkxAlipayRateOptionTier] = "4"
	common.OptionMap[OkxAlipayRateOptionAdjustmentType] = "percent"
	common.OptionMap[OkxAlipayRateOptionAdjustmentValue] = "-10"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		for key, value := range original {
			if value == "" {
				delete(common.OptionMap, key)
				continue
			}
			common.OptionMap[key] = value
		}
	})

	config := GetOkxAlipayRateConfig()

	require.Equal(t, "https://example.test/okx", config.RateAPIURL)
	require.Equal(t, "sell", config.Side)
	require.Equal(t, 4, config.Tier)
	require.Equal(t, OkxAlipayRateAdjustmentTypePercent, config.AdjustmentType)
	require.InDelta(t, -10, config.AdjustmentValue, 0.000001)
}
