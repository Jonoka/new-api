package setting

import "github.com/QuantumNous/new-api/setting/config"

const DefaultAffiliatePayoutMethods = "usdt,alipay,wechat"

type AffiliateSetting struct {
	FirstLevelEnabled            bool   `json:"first_level_enabled"`
	FirstLevelRatio              int    `json:"first_level_ratio"`
	SecondLevelEnabled           bool   `json:"second_level_enabled"`
	SecondLevelRatio             int    `json:"second_level_ratio"`
	SettlementDelaySeconds       int64  `json:"settlement_delay_seconds"`
	MinWithdrawalAmount          int    `json:"min_withdrawal_amount"`
	TriggerTopupEnabled          bool   `json:"trigger_topup_enabled"`
	TriggerSubscriptionEnabled   bool   `json:"trigger_subscription_enabled"`
	FilterRedemptionTopupEnabled bool   `json:"filter_redemption_topup_enabled"`
	PayoutMethods                string `json:"payout_methods"`
	UsdtChain                    string `json:"usdt_chain"`
	PromotionTemplate            string `json:"promotion_template"`
}

var affiliateSetting = AffiliateSetting{
	FirstLevelEnabled:            false,
	FirstLevelRatio:              0,
	SecondLevelEnabled:           false,
	SecondLevelRatio:             0,
	SettlementDelaySeconds:       0,
	MinWithdrawalAmount:          10,
	TriggerTopupEnabled:          true,
	TriggerSubscriptionEnabled:   false,
	FilterRedemptionTopupEnabled: false,
	PayoutMethods:                DefaultAffiliatePayoutMethods,
	UsdtChain:                    "TRC20",
	PromotionTemplate:            "邀请链接：{invite_link}",
}

func init() {
	config.GlobalConfig.Register("affiliate_setting", &affiliateSetting)
}

func GetAffiliateSetting() *AffiliateSetting {
	if affiliateSetting.UsdtChain == "" {
		affiliateSetting.UsdtChain = "TRC20"
	}
	if affiliateSetting.PayoutMethods == "" {
		affiliateSetting.PayoutMethods = DefaultAffiliatePayoutMethods
	}
	if affiliateSetting.PromotionTemplate == "" {
		affiliateSetting.PromotionTemplate = "邀请链接：{invite_link}"
	}
	return &affiliateSetting
}
