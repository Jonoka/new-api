package extension

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/shopspring/decimal"
)

const (
	OkxAlipayRateModuleID = "okx-alipay-rate"
	OkxAlipayRateSourceID = "okx-alipay-rate-module"

	OkxAlipayRateOptionRateAPIURL       = setting.OkxAlipayRateOptionRateAPIURL
	OkxAlipayRateOptionSide             = setting.OkxAlipayRateOptionSide
	OkxAlipayRateOptionTier             = setting.OkxAlipayRateOptionTier
	OkxAlipayRateOptionAdjustmentType   = setting.OkxAlipayRateOptionAdjustmentType
	OkxAlipayRateOptionAdjustmentValue  = setting.OkxAlipayRateOptionAdjustmentValue
	OkxAlipayRateAdjustmentTypeAbsolute = "absolute"
	OkxAlipayRateAdjustmentTypePercent  = "percent"
)

type OkxAlipayRateConfig struct {
	RateAPIURL      string  `json:"rate_api_url"`
	Side            string  `json:"side"`
	Tier            int     `json:"tier"`
	AdjustmentType  string  `json:"adjustment_type"`
	AdjustmentValue float64 `json:"adjustment_value"`
}

type OkxAlipayRateQuote struct {
	RawRate         float64 `json:"raw_rate"`
	AdjustedRate    float64 `json:"adjusted_rate"`
	Source          string  `json:"source"`
	Side            string  `json:"side"`
	Tier            int     `json:"tier"`
	AdjustmentType  string  `json:"adjustment_type"`
	AdjustmentValue float64 `json:"adjustment_value"`
	RateAPIURL      string  `json:"rate_api_url"`
	OrderID         string  `json:"order_id,omitempty"`
	NickName        string  `json:"nick_name,omitempty"`
}

type okxAlipayOrder struct {
	Price    float64
	ID       string
	NickName string
}

func DefaultOkxAlipayRateConfig() OkxAlipayRateConfig {
	return OkxAlipayRateConfig{
		Side:           "buy",
		Tier:           3,
		AdjustmentType: OkxAlipayRateAdjustmentTypeAbsolute,
	}
}

func NormalizeOkxAlipayRateConfig(config OkxAlipayRateConfig) OkxAlipayRateConfig {
	defaults := DefaultOkxAlipayRateConfig()
	config.RateAPIURL = strings.TrimSpace(config.RateAPIURL)

	config.Side = strings.ToLower(strings.TrimSpace(config.Side))
	if config.Side != "sell" {
		config.Side = defaults.Side
	}

	if config.Tier <= 0 {
		config.Tier = defaults.Tier
	}

	config.AdjustmentType = strings.ToLower(strings.TrimSpace(config.AdjustmentType))
	if config.AdjustmentType != OkxAlipayRateAdjustmentTypePercent {
		config.AdjustmentType = OkxAlipayRateAdjustmentTypeAbsolute
	}
	return config
}

func ValidateOkxAlipayRateConfig(config OkxAlipayRateConfig) error {
	config = NormalizeOkxAlipayRateConfig(config)
	if config.Tier <= 0 {
		return fmt.Errorf("tier must be greater than zero")
	}
	if math.IsNaN(config.AdjustmentValue) || math.IsInf(config.AdjustmentValue, 0) {
		return fmt.Errorf("adjustment value is invalid")
	}
	if config.RateAPIURL == "" {
		return nil
	}
	parsed, err := url.Parse(config.RateAPIURL)
	if err != nil || parsed == nil || parsed.Host == "" {
		return fmt.Errorf("rate api url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("rate api url only supports http or https")
	}
	return nil
}

func (config OkxAlipayRateConfig) OptionValues() map[string]string {
	config = NormalizeOkxAlipayRateConfig(config)
	return map[string]string{
		OkxAlipayRateOptionRateAPIURL:      config.RateAPIURL,
		OkxAlipayRateOptionSide:            config.Side,
		OkxAlipayRateOptionTier:            strconv.Itoa(config.Tier),
		OkxAlipayRateOptionAdjustmentType:  config.AdjustmentType,
		OkxAlipayRateOptionAdjustmentValue: strconv.FormatFloat(config.AdjustmentValue, 'f', -1, 64),
	}
}

func GetOkxAlipayRateConfig() OkxAlipayRateConfig {
	defaults := DefaultOkxAlipayRateConfig()
	config := defaults

	common.OptionMapRWMutex.RLock()
	rateAPIURL := common.OptionMap[OkxAlipayRateOptionRateAPIURL]
	side := common.OptionMap[OkxAlipayRateOptionSide]
	tierText := common.OptionMap[OkxAlipayRateOptionTier]
	adjustmentType := common.OptionMap[OkxAlipayRateOptionAdjustmentType]
	adjustmentValueText := common.OptionMap[OkxAlipayRateOptionAdjustmentValue]
	common.OptionMapRWMutex.RUnlock()

	config.RateAPIURL = rateAPIURL
	config.Side = side
	if tier, err := strconv.Atoi(strings.TrimSpace(tierText)); err == nil {
		config.Tier = tier
	}
	config.AdjustmentType = adjustmentType
	if adjustmentValue, err := strconv.ParseFloat(strings.TrimSpace(adjustmentValueText), 64); err == nil {
		config.AdjustmentValue = adjustmentValue
	}
	return NormalizeOkxAlipayRateConfig(config)
}

func OkxAlipayRateConfigCacheKey() string {
	config := GetOkxAlipayRateConfig()
	return fmt.Sprintf(
		"%s|%s|%d|%s|%.8f",
		strings.TrimSpace(config.RateAPIURL),
		config.Side,
		config.Tier,
		config.AdjustmentType,
		config.AdjustmentValue,
	)
}

func IsOkxAlipayRateModuleEnabled() bool {
	module, ok := DefaultManager.Get(OkxAlipayRateModuleID)
	return ok && module.Enabled && module.Error == ""
}

func defaultOkxAlipayRateAPIURL(side string) string {
	if side != "sell" {
		side = "buy"
	}
	return fmt.Sprintf(
		"https://www.okx.com/v3/c2c/tradingOrders/books?quoteCurrency=CNY&baseCurrency=USDT&side=%s&paymentMethod=aliPay",
		side,
	)
}

func okxAlipayRateAPIURL(config OkxAlipayRateConfig) string {
	config = NormalizeOkxAlipayRateConfig(config)
	if config.RateAPIURL != "" {
		return config.RateAPIURL
	}
	return defaultOkxAlipayRateAPIURL(config.Side)
}

func newOkxAlipayRateRequest(rateURL string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, rateURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")
	return req, nil
}

func applyOkxAlipayRateAdjustment(rate float64, config OkxAlipayRateConfig) (float64, error) {
	config = NormalizeOkxAlipayRateConfig(config)
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, fmt.Errorf("invalid raw rate")
	}
	adjusted := rate
	switch config.AdjustmentType {
	case OkxAlipayRateAdjustmentTypePercent:
		adjusted = decimal.NewFromFloat(rate).
			Mul(decimal.NewFromFloat(100).Add(decimal.NewFromFloat(config.AdjustmentValue))).
			Div(decimal.NewFromFloat(100)).
			InexactFloat64()
	default:
		adjusted = decimal.NewFromFloat(rate).Add(decimal.NewFromFloat(config.AdjustmentValue)).InexactFloat64()
	}
	if adjusted <= 0 || math.IsNaN(adjusted) || math.IsInf(adjusted, 0) {
		return 0, fmt.Errorf("adjusted rate must be greater than zero")
	}
	return adjusted, nil
}

func ParseOkxAlipayTierRateFromBody(body []byte, side string, tier int) (okxAlipayOrder, error) {
	if tier <= 0 {
		tier = 3
	}
	side = strings.ToLower(strings.TrimSpace(side))
	if side != "sell" {
		side = "buy"
	}

	var payload map[string]interface{}
	if err := common.Unmarshal(body, &payload); err != nil {
		return okxAlipayOrder{}, err
	}
	rawData, ok := payload["data"].(map[string]interface{})
	if !ok {
		return okxAlipayOrder{}, fmt.Errorf("missing okx data")
	}
	rawOrders, ok := rawData[side].([]interface{})
	if !ok || len(rawOrders) < tier {
		return okxAlipayOrder{}, fmt.Errorf("missing okx %s tier %d", side, tier)
	}
	order, ok := rawOrders[tier-1].(map[string]interface{})
	if !ok {
		return okxAlipayOrder{}, fmt.Errorf("invalid okx tier %d", tier)
	}
	rawPrice, ok := order["price"]
	if !ok {
		return okxAlipayOrder{}, fmt.Errorf("missing okx price")
	}
	rate, err := strconv.ParseFloat(fmt.Sprintf("%v", rawPrice), 64)
	if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return okxAlipayOrder{}, fmt.Errorf("invalid okx price")
	}
	return okxAlipayOrder{
		Price:    rate,
		ID:       strings.TrimSpace(fmt.Sprintf("%v", order["id"])),
		NickName: strings.TrimSpace(fmt.Sprintf("%v", order["nickName"])),
	}, nil
}

func FetchOkxAlipayRateQuote(config OkxAlipayRateConfig) (OkxAlipayRateQuote, error) {
	config = NormalizeOkxAlipayRateConfig(config)
	if err := ValidateOkxAlipayRateConfig(config); err != nil {
		return OkxAlipayRateQuote{}, err
	}

	rateAPIURL := okxAlipayRateAPIURL(config)
	req, err := newOkxAlipayRateRequest(rateAPIURL)
	if err != nil {
		return OkxAlipayRateQuote{}, err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return OkxAlipayRateQuote{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OkxAlipayRateQuote{}, err
	}
	if resp.StatusCode/100 != 2 {
		return OkxAlipayRateQuote{}, fmt.Errorf("okx rate api http %d", resp.StatusCode)
	}

	order, err := ParseOkxAlipayTierRateFromBody(body, config.Side, config.Tier)
	if err != nil {
		return OkxAlipayRateQuote{}, err
	}
	adjustedRate, err := applyOkxAlipayRateAdjustment(order.Price, config)
	if err != nil {
		return OkxAlipayRateQuote{}, err
	}
	return OkxAlipayRateQuote{
		RawRate:         order.Price,
		AdjustedRate:    adjustedRate,
		Source:          OkxAlipayRateSourceID,
		Side:            config.Side,
		Tier:            config.Tier,
		AdjustmentType:  config.AdjustmentType,
		AdjustmentValue: config.AdjustmentValue,
		RateAPIURL:      rateAPIURL,
		OrderID:         order.ID,
		NickName:        order.NickName,
	}, nil
}

func FetchEnabledOkxAlipayRateQuote() (OkxAlipayRateQuote, error) {
	if !IsOkxAlipayRateModuleEnabled() {
		return OkxAlipayRateQuote{}, fmt.Errorf("%s module is not enabled", OkxAlipayRateModuleID)
	}
	return FetchOkxAlipayRateQuote(GetOkxAlipayRateConfig())
}
