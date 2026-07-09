package setting

const (
	OkxAlipayRateOptionRateAPIURL      = "extension.okx_alipay_rate.rate_api_url"
	OkxAlipayRateOptionSide            = "extension.okx_alipay_rate.side"
	OkxAlipayRateOptionTier            = "extension.okx_alipay_rate.tier"
	OkxAlipayRateOptionAdjustmentType  = "extension.okx_alipay_rate.adjustment_type"
	OkxAlipayRateOptionAdjustmentValue = "extension.okx_alipay_rate.adjustment_value"
)

func DefaultOkxAlipayRateModuleOptions() map[string]string {
	return map[string]string{
		OkxAlipayRateOptionRateAPIURL:      "",
		OkxAlipayRateOptionSide:            "buy",
		OkxAlipayRateOptionTier:            "3",
		OkxAlipayRateOptionAdjustmentType:  "absolute",
		OkxAlipayRateOptionAdjustmentValue: "0",
	}
}
