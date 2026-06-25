package setting

import "github.com/QuantumNous/new-api/setting/config"

const DefaultAffiliatePayoutMethods = "usdt,alipay,wechat"
const DefaultAffiliateAgreementText = "我已知悉并同意：利用小号邀请自身账号或关联账号参与返佣属于违规行为。一经发现，将永久封禁相关账号并冻结所有返佣收益，不予退款。"

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
	// Anti-fraud: Inviter review
	ReviewEnabled        bool `json:"review_enabled"`
	AutoApproveAfterDays int  `json:"auto_approve_after_days"`
	// Anti-fraud: Agreement
	AgreementEnabled bool   `json:"agreement_enabled"`
	AgreementText    string `json:"agreement_text"`
	// Anti-fraud: Inviter eligibility
	InviterMinAccountAgeDays int `json:"inviter_min_account_age_days"`
	InviterMinRechargeAmount int `json:"inviter_min_recharge_amount"`
	// Anti-fraud: Invitee eligibility
	InviteeMinAccountAgeDays int `json:"invitee_min_account_age_days"`
	InviteeMinRechargeAmount int `json:"invitee_min_recharge_amount"`
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
	ReviewEnabled:                false,
	AutoApproveAfterDays:         0,
	AgreementEnabled:             false,
	AgreementText:                DefaultAffiliateAgreementText,
	InviterMinAccountAgeDays:     0,
	InviterMinRechargeAmount:     0,
	InviteeMinAccountAgeDays:     0,
	InviteeMinRechargeAmount:     0,
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
	if affiliateSetting.AgreementText == "" {
		affiliateSetting.AgreementText = DefaultAffiliateAgreementText
	}
	return &affiliateSetting
}
