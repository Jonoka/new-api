package setting

var (
	OkpayGatewayUrl          string = "https://api.okaypay.me/shop"
	OkpayMerchantId          string
	OkpayMerchantToken       string
	OkpayExchangeRate        float64 = 7.2
	OkpayAutoExchangeEnabled bool    = true
	OkpayUsdtCnyRate         float64 = 7.2
	OkpayRateApiUrl          string  = "https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true"
	OkpayRateSource          string  = "coingecko"
	OkpayOkxSide             string  = "buy"
	OkpayOkxTier             int     = 3
	OkpayRateAdjustmentType  string  = "absolute"
	OkpayRateAdjustmentValue float64 = 0
	OkpayMinTopUp            int     = 1
	OkpayCoin                string  = "USDT"
)
