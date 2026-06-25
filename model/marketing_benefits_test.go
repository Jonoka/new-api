package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertMarketingUser(t *testing.T, id int, inviterId int, quota int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:        id,
		Username:  common.GetRandomString(8),
		AffCode:   common.GetRandomString(8),
		Status:    common.UserStatusEnabled,
		InviterId: inviterId,
		Quota:     quota,
	}).Error)
}

func TestRedeem_AllowsMultipleUsersUntilMaxRedeemCount(t *testing.T) {
	truncateTables(t)

	insertMarketingUser(t, 701, 0, 0)
	insertMarketingUser(t, 702, 0, 0)
	insertMarketingUser(t, 703, 0, 0)

	code := &Redemption{
		UserId:         1,
		Key:            "multi-redeem-code",
		Status:         common.RedemptionCodeStatusEnabled,
		Name:           "multi",
		Quota:          120,
		CreatedTime:    common.GetTimestamp(),
		MaxRedeemCount: 2,
	}
	require.NoError(t, code.Insert())

	quota, err := Redeem(code.Key, 701)
	require.NoError(t, err)
	assert.Equal(t, 120, quota)

	quota, err = Redeem(code.Key, 702)
	require.NoError(t, err)
	assert.Equal(t, 120, quota)

	_, err = Redeem(code.Key, 703)
	require.Error(t, err)
	assert.EqualError(t, err, "该兑换码已达兑换次数上限")

	var saved Redemption
	require.NoError(t, DB.Where("id = ?", code.Id).First(&saved).Error)
	assert.Equal(t, 2, saved.RedeemedCount)
	assert.Equal(t, 2, saved.MaxRedeemCount)
	assert.Equal(t, common.RedemptionCodeStatusUsed, saved.Status)
}

func TestRedeem_PreventsSameUserRedeemingSharedCodeTwice(t *testing.T) {
	truncateTables(t)

	insertMarketingUser(t, 711, 0, 0)
	code := &Redemption{
		UserId:         1,
		Key:            "same-user-once",
		Status:         common.RedemptionCodeStatusEnabled,
		Name:           "once",
		Quota:          100,
		CreatedTime:    common.GetTimestamp(),
		MaxRedeemCount: 3,
	}
	require.NoError(t, code.Insert())

	_, err := Redeem(code.Key, 711)
	require.NoError(t, err)

	_, err = Redeem(code.Key, 711)
	require.Error(t, err)
	assert.EqualError(t, err, "该兑换码已兑换过")

	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", 711).First(&user).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestPromoCodeCalculateDiscountHonorsScopeAndType(t *testing.T) {
	truncateTables(t)

	selected := &PromoCode{
		Name:                     "selected sub",
		Code:                     "SUB20",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             PromoCodeDiscountTypePercent,
		DiscountValue:            20,
		AppliesToTopup:           false,
		AppliesToAllSubscription: false,
		SubscriptionPlanIds:      "1001,1002",
		MaxRedeemCount:           10,
		CreatedTime:              common.GetTimestamp(),
	}
	require.NoError(t, selected.Insert())

	result, err := CalculatePromoCodeDiscount("SUB20", PromoCodeTargetSubscription, 1002, 50)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "SUB20", result.Code)
	assert.InDelta(t, 50, result.OriginalAmount, 0.000001)
	assert.InDelta(t, 10, result.DiscountAmount, 0.000001)
	assert.InDelta(t, 40, result.PaidAmount, 0.000001)

	_, err = CalculatePromoCodeDiscount("SUB20", PromoCodeTargetTopUp, 0, 50)
	require.Error(t, err)

	fixed := &PromoCode{
		Name:                     "topup fixed",
		Code:                     "FIXED",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             PromoCodeDiscountTypeFixed,
		DiscountValue:            int64(15 * common.QuotaPerUnit),
		AppliesToTopup:           true,
		AppliesToAllSubscription: false,
		MaxRedeemCount:           10,
		CreatedTime:              common.GetTimestamp(),
	}
	require.NoError(t, fixed.Insert())

	result, err = CalculatePromoCodeDiscount("FIXED", PromoCodeTargetTopUp, 0, 30)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.InDelta(t, 15, result.DiscountAmount, 0.000001)
	assert.InDelta(t, 15, result.PaidAmount, 0.000001)
}

func TestPromoCodeUpdateAllowsSameCodeForExistingRecord(t *testing.T) {
	truncateTables(t)

	promo := &PromoCode{
		Name:           "original",
		Code:           "KEEP_CODE",
		Status:         common.RedemptionCodeStatusEnabled,
		DiscountType:   PromoCodeDiscountTypePercent,
		DiscountValue:  10,
		AppliesToTopup: true,
		MaxRedeemCount: 10,
		CreatedTime:    common.GetTimestamp(),
	}
	require.NoError(t, promo.Insert())

	promo.Name = "updated"
	promo.Code = "keep_code"
	promo.DiscountValue = 20
	require.NoError(t, promo.Update())

	var saved PromoCode
	require.NoError(t, DB.Where("id = ?", promo.Id).First(&saved).Error)
	assert.Equal(t, "updated", saved.Name)
	assert.Equal(t, "KEEP_CODE", saved.Code)
	assert.EqualValues(t, 20, saved.DiscountValue)

	var count int64
	require.NoError(t, DB.Model(&PromoCode{}).Where("code = ?", "KEEP_CODE").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestPromoCodeUsageAndAffiliateRewardsUseActualPaidAmount(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.TriggerSubscriptionEnabled = true
	affiliateSetting.FirstLevelRatio = 10
	affiliateSetting.SecondLevelEnabled = false

	insertMarketingUser(t, 801, 0, 0)
	insertMarketingUser(t, 802, 801, 0)

	plan := &SubscriptionPlan{
		Id:            8801,
		Title:         "Promo Plan",
		PriceAmount:   100,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   999999,
	}
	require.NoError(t, DB.Create(plan).Error)

	promo := &PromoCode{
		Name:                     "half",
		Code:                     "HALF",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             PromoCodeDiscountTypePercent,
		DiscountValue:            50,
		AppliesToTopup:           false,
		AppliesToAllSubscription: true,
		MaxRedeemCount:           10,
		CreatedTime:              common.GetTimestamp(),
	}
	require.NoError(t, promo.Insert())

	discount, err := CalculatePromoCodeDiscount("HALF", PromoCodeTargetSubscription, plan.Id, plan.PriceAmount)
	require.NoError(t, err)
	order := &SubscriptionOrder{
		UserId:          802,
		PlanId:          plan.Id,
		Money:           discount.PaidAmount,
		TradeNo:         "sub-promo-paid",
		PaymentMethod:   PaymentProviderEpay,
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	ApplyPromoCodeResultToSubscriptionOrder(order, discount)
	require.NoError(t, order.Insert())

	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, `{}`, PaymentProviderEpay, "alipay"))

	var record AffiliateRecord
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceSubscription, order.TradeNo).First(&record).Error)
	expectedSourceQuota := int(discount.PaidAmount * common.QuotaPerUnit)
	assert.Equal(t, expectedSourceQuota, record.SourceQuota)
	assert.Equal(t, expectedSourceQuota/10, record.RewardQuota)

	var savedPromo PromoCode
	require.NoError(t, DB.Where("id = ?", promo.Id).First(&savedPromo).Error)
	assert.Equal(t, 1, savedPromo.RedeemedCount)

	var usage PromoCodeUsage
	require.NoError(t, DB.Where("promo_code_id = ? AND order_no = ?", promo.Id, order.TradeNo).First(&usage).Error)
	assert.InDelta(t, 100, usage.OriginalAmount, 0.000001)
	assert.InDelta(t, 50, usage.DiscountAmount, 0.000001)
	assert.InDelta(t, 50, usage.PaidAmount, 0.000001)
}

func TestCompleteSubscriptionOrder_DoesNotFailPaidExternalOrderWhenPromoLimitReachedAfterCreation(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.TriggerSubscriptionEnabled = true
	affiliateSetting.FirstLevelRatio = 10
	affiliateSetting.SecondLevelEnabled = false

	insertMarketingUser(t, 821, 0, 0)
	insertMarketingUser(t, 822, 821, 0)

	plan := &SubscriptionPlan{
		Id:            8821,
		Title:         "External Promo Plan",
		PriceAmount:   100,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   999999,
	}
	require.NoError(t, DB.Create(plan).Error)

	promo := &PromoCode{
		Name:                     "single external",
		Code:                     "EXTONLY",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             PromoCodeDiscountTypePercent,
		DiscountValue:            50,
		AppliesToTopup:           false,
		AppliesToAllSubscription: true,
		MaxRedeemCount:           1,
		CreatedTime:              common.GetTimestamp(),
	}
	require.NoError(t, promo.Insert())

	discount, err := CalculatePromoCodeDiscount("EXTONLY", PromoCodeTargetSubscription, plan.Id, plan.PriceAmount)
	require.NoError(t, err)
	order := &SubscriptionOrder{
		UserId:          822,
		PlanId:          plan.Id,
		Money:           discount.PaidAmount,
		TradeNo:         "sub-external-already-paid",
		PaymentMethod:   PaymentProviderEpay,
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	ApplyPromoCodeResultToSubscriptionOrder(order, discount)
	require.NoError(t, order.Insert())

	require.NoError(t, DB.Model(&PromoCode{}).Where("id = ?", promo.Id).Updates(map[string]interface{}{
		"redeemed_count": 1,
		"status":         common.RedemptionCodeStatusUsed,
	}).Error)

	require.NoError(t, CompleteSubscriptionOrder(order.TradeNo, `{}`, PaymentProviderEpay, "alipay"))

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 822, plan.Id).First(&sub).Error)
	assert.Equal(t, "active", sub.Status)

	var usage PromoCodeUsage
	require.NoError(t, DB.Where("promo_code_id = ? AND order_no = ?", promo.Id, order.TradeNo).First(&usage).Error)
	assert.InDelta(t, 50, usage.PaidAmount, 0.000001)

	var savedPromo PromoCode
	require.NoError(t, DB.Where("id = ?", promo.Id).First(&savedPromo).Error)
	assert.Equal(t, 2, savedPromo.RedeemedCount)
	assert.Equal(t, common.RedemptionCodeStatusUsed, savedPromo.Status)
}

func TestPurchaseSubscriptionWithBalance_AffiliateRewardUsesDiscountedPaidAmount(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.TriggerSubscriptionEnabled = true
	affiliateSetting.FirstLevelRatio = 10
	affiliateSetting.SecondLevelEnabled = false

	insertMarketingUser(t, 811, 0, 0)
	insertMarketingUser(t, 812, 811, int(100*common.QuotaPerUnit))

	plan := &SubscriptionPlan{
		Id:            8811,
		Title:         "Balance Promo Plan",
		PriceAmount:   100,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   999999,
	}
	require.NoError(t, DB.Create(plan).Error)

	promo := &PromoCode{
		Name:                     "balance half",
		Code:                     "BALHALF",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             PromoCodeDiscountTypePercent,
		DiscountValue:            50,
		AppliesToTopup:           false,
		AppliesToAllSubscription: true,
		MaxRedeemCount:           10,
		CreatedTime:              common.GetTimestamp(),
	}
	require.NoError(t, promo.Insert())

	require.NoError(t, PurchaseSubscriptionWithBalance(812, plan.Id, "BALHALF"))

	var order SubscriptionOrder
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 812, plan.Id).First(&order).Error)
	assert.InDelta(t, 50, order.Money, 0.000001)
	assert.Equal(t, int(50*common.QuotaPerUnit), order.AffiliateSourceQuota)

	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", 812).First(&user).Error)
	assert.Equal(t, int(50*common.QuotaPerUnit), user.Quota)

	var record AffiliateRecord
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceSubscription, order.TradeNo).First(&record).Error)
	expectedSourceQuota := int(50 * common.QuotaPerUnit)
	assert.Equal(t, expectedSourceQuota, record.SourceQuota)
	assert.Equal(t, expectedSourceQuota/10, record.RewardQuota)
}

func TestPurchaseSubscriptionWithBalance_EnforcesPromoUseLimit(t *testing.T) {
	truncateTables(t)

	insertMarketingUser(t, 831, 0, int(100*common.QuotaPerUnit))

	plan := &SubscriptionPlan{
		Id:            8831,
		Title:         "Limited Balance Promo Plan",
		PriceAmount:   100,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   999999,
	}
	require.NoError(t, DB.Create(plan).Error)

	promo := &PromoCode{
		Name:                     "used up",
		Code:                     "USEDUP",
		Status:                   common.RedemptionCodeStatusUsed,
		DiscountType:             PromoCodeDiscountTypePercent,
		DiscountValue:            50,
		AppliesToTopup:           false,
		AppliesToAllSubscription: true,
		MaxRedeemCount:           1,
		RedeemedCount:            1,
		CreatedTime:              common.GetTimestamp(),
	}
	require.NoError(t, promo.Insert())

	err := PurchaseSubscriptionWithBalance(831, plan.Id, "USEDUP")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "优惠码不可用")

	var count int64
	require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("user_id = ? AND plan_id = ?", 831, plan.Id).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestApplyPromoCodeResultToStripeTopUpKeepsRechargeCreditMoney(t *testing.T) {
	discount := &PromoCodeDiscountResult{
		PromoCodeId:     1,
		Code:            "STRIPEHALF",
		OriginalAmount:  100,
		DiscountAmount:  50,
		PaidAmount:      50,
		ActualPaidQuota: int(50 * common.QuotaPerUnit),
	}
	topUp := &TopUp{Money: 100}

	ApplyPromoCodeResultToStripeTopUp(topUp, discount, 100)

	assert.InDelta(t, 100, topUp.Money, 0.000001)
	assert.InDelta(t, 100, topUp.OriginalMoney, 0.000001)
	assert.InDelta(t, 50, topUp.DiscountMoney, 0.000001)
	assert.InDelta(t, 50, topUp.ActualMoney, 0.000001)
	assert.Equal(t, int(50*common.QuotaPerUnit), topUp.AffiliateSourceQuota)
}

func TestGetUserTotalRechargeAmountUsesActualPaidAmount(t *testing.T) {
	truncateTables(t)

	insertMarketingUser(t, 839, 0, 0)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          839,
		Money:           30,
		ActualMoney:     0,
		TradeNo:         "legacy-paid-topup",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          839,
		Money:           100,
		OriginalMoney:   100,
		DiscountMoney:   50,
		ActualMoney:     50,
		PromoCodeId:     1,
		PromoCode:       "HALF",
		TradeNo:         "half-paid-topup",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          839,
		Money:           100,
		OriginalMoney:   100,
		DiscountMoney:   100,
		ActualMoney:     0,
		PromoCodeId:     2,
		PromoCode:       "FREE",
		TradeNo:         "free-stripe-topup",
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusSuccess,
	}).Error)

	total, err := GetUserTotalRechargeAmount(839)
	require.NoError(t, err)
	assert.InDelta(t, 80, total, 0.000001)
}

func TestCompleteEpayTopUp_RecordsPromoAndAffiliateInOneCompletion(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.TriggerTopupEnabled = true
	affiliateSetting.FirstLevelRatio = 10
	affiliateSetting.SecondLevelEnabled = false

	insertMarketingUser(t, 841, 0, 0)
	insertMarketingUser(t, 842, 841, 0)

	promo := &PromoCode{
		Name:           "epay half",
		Code:           "EPAYHALF",
		Status:         common.RedemptionCodeStatusEnabled,
		DiscountType:   PromoCodeDiscountTypePercent,
		DiscountValue:  50,
		AppliesToTopup: true,
		MaxRedeemCount: 10,
		CreatedTime:    common.GetTimestamp(),
	}
	require.NoError(t, promo.Insert())

	discount, err := CalculatePromoCodeDiscount("EPAYHALF", PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	topUp := &TopUp{
		UserId:          842,
		Amount:          100,
		Money:           discount.PaidAmount,
		TradeNo:         "epay-promo-complete",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	ApplyPromoCodeResultToTopUp(topUp, discount)
	require.NoError(t, topUp.Insert())

	completedTopUp, quotaToAdd, completedNow, err := CompleteEpayTopUp(topUp.TradeNo, "wxpay")
	require.NoError(t, err)
	require.NotNil(t, completedTopUp)
	assert.True(t, completedNow)
	assert.Equal(t, int(100*common.QuotaPerUnit), quotaToAdd)
	assert.Equal(t, "wxpay", completedTopUp.PaymentMethod)

	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", 842).First(&user).Error)
	assert.Equal(t, int(100*common.QuotaPerUnit), user.Quota)

	var usage PromoCodeUsage
	require.NoError(t, DB.Where("promo_code_id = ? AND order_no = ?", promo.Id, topUp.TradeNo).First(&usage).Error)
	assert.InDelta(t, 100, usage.OriginalAmount, 0.000001)
	assert.InDelta(t, 50, usage.DiscountAmount, 0.000001)
	assert.InDelta(t, 50, usage.PaidAmount, 0.000001)

	var record AffiliateRecord
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, topUp.TradeNo).First(&record).Error)
	expectedSourceQuota := int(50 * common.QuotaPerUnit)
	assert.Equal(t, expectedSourceQuota, record.SourceQuota)
	assert.Equal(t, expectedSourceQuota/10, record.RewardQuota)

	_, quotaToAdd, completedNow, err = CompleteEpayTopUp(topUp.TradeNo, "wxpay")
	require.NoError(t, err)
	assert.False(t, completedNow)
	assert.Equal(t, 0, quotaToAdd)

	require.NoError(t, DB.Select("quota").Where("id = ?", 842).First(&user).Error)
	assert.Equal(t, int(100*common.QuotaPerUnit), user.Quota)

	var usageCount int64
	require.NoError(t, DB.Model(&PromoCodeUsage{}).Where("promo_code_id = ?", promo.Id).Count(&usageCount).Error)
	assert.EqualValues(t, 1, usageCount)

	var recordCount int64
	require.NoError(t, DB.Model(&AffiliateRecord{}).Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, topUp.TradeNo).Count(&recordCount).Error)
	assert.EqualValues(t, 1, recordCount)
}

func TestCompleteFreeTopUp_FullDiscountCompletesServerSideWithoutAffiliateReward(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.TriggerTopupEnabled = true
	affiliateSetting.FirstLevelRatio = 10
	affiliateSetting.SecondLevelEnabled = false

	insertMarketingUser(t, 851, 0, 0)
	insertMarketingUser(t, 852, 851, 0)

	promo := &PromoCode{
		Name:           "free topup",
		Code:           "TOPFREE",
		Status:         common.RedemptionCodeStatusEnabled,
		DiscountType:   PromoCodeDiscountTypePercent,
		DiscountValue:  100,
		AppliesToTopup: true,
		MaxRedeemCount: 1,
		CreatedTime:    common.GetTimestamp(),
	}
	require.NoError(t, promo.Insert())

	discount, err := CalculatePromoCodeDiscount("TOPFREE", PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	topUp := &TopUp{
		UserId:          852,
		Amount:          100,
		Money:           discount.PaidAmount,
		TradeNo:         "topup-free-complete",
		PaymentMethod:   PaymentProviderEpay,
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	ApplyPromoCodeResultToTopUp(topUp, discount)
	require.NoError(t, topUp.Insert())

	completedTopUp, quotaToAdd, completedNow, err := CompleteFreeTopUp(topUp.TradeNo, PaymentProviderEpay)
	require.NoError(t, err)
	require.NotNil(t, completedTopUp)
	assert.True(t, completedNow)
	assert.Equal(t, int(100*common.QuotaPerUnit), quotaToAdd)
	assert.InDelta(t, 0, completedTopUp.Money, 0.000001)
	assert.InDelta(t, 0, completedTopUp.ActualMoney, 0.000001)
	assert.Equal(t, 0, completedTopUp.AffiliateSourceQuota)

	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", 852).First(&user).Error)
	assert.Equal(t, int(100*common.QuotaPerUnit), user.Quota)

	var usage PromoCodeUsage
	require.NoError(t, DB.Where("promo_code_id = ? AND order_no = ?", promo.Id, topUp.TradeNo).First(&usage).Error)
	assert.InDelta(t, 100, usage.OriginalAmount, 0.000001)
	assert.InDelta(t, 100, usage.DiscountAmount, 0.000001)
	assert.InDelta(t, 0, usage.PaidAmount, 0.000001)

	var savedPromo PromoCode
	require.NoError(t, DB.Where("id = ?", promo.Id).First(&savedPromo).Error)
	assert.Equal(t, 1, savedPromo.RedeemedCount)
	assert.Equal(t, common.RedemptionCodeStatusUsed, savedPromo.Status)

	var recordCount int64
	require.NoError(t, DB.Model(&AffiliateRecord{}).Where("source_type = ? AND source_id = ?", AffiliateSourceTopUp, topUp.TradeNo).Count(&recordCount).Error)
	assert.EqualValues(t, 0, recordCount)

	_, quotaToAdd, completedNow, err = CompleteFreeTopUp(topUp.TradeNo, PaymentProviderEpay)
	require.NoError(t, err)
	assert.False(t, completedNow)
	assert.Equal(t, 0, quotaToAdd)
}

func TestCompleteFreeSubscriptionOrder_FullDiscountCompletesWithoutAffiliateReward(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.TriggerSubscriptionEnabled = true
	affiliateSetting.FirstLevelRatio = 10
	affiliateSetting.SecondLevelEnabled = false

	insertMarketingUser(t, 861, 0, 0)
	insertMarketingUser(t, 862, 861, 0)

	plan := &SubscriptionPlan{
		Id:            8861,
		Title:         "Free Promo Plan",
		PriceAmount:   100,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   999999,
	}
	require.NoError(t, DB.Create(plan).Error)

	promo := &PromoCode{
		Name:                     "free subscription",
		Code:                     "SUBFREE",
		Status:                   common.RedemptionCodeStatusEnabled,
		DiscountType:             PromoCodeDiscountTypePercent,
		DiscountValue:            100,
		AppliesToAllSubscription: true,
		MaxRedeemCount:           1,
		CreatedTime:              common.GetTimestamp(),
	}
	require.NoError(t, promo.Insert())

	discount, err := CalculatePromoCodeDiscount("SUBFREE", PromoCodeTargetSubscription, plan.Id, plan.PriceAmount)
	require.NoError(t, err)
	order := &SubscriptionOrder{
		UserId:          862,
		PlanId:          plan.Id,
		Money:           discount.PaidAmount,
		TradeNo:         "sub-free-complete",
		PaymentMethod:   PaymentProviderEpay,
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	ApplyPromoCodeResultToSubscriptionOrder(order, discount)
	require.NoError(t, order.Insert())

	require.NoError(t, CompleteFreeSubscriptionOrder(order.TradeNo, PaymentProviderEpay))

	var savedOrder SubscriptionOrder
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&savedOrder).Error)
	assert.Equal(t, common.TopUpStatusSuccess, savedOrder.Status)
	assert.InDelta(t, 0, savedOrder.Money, 0.000001)
	assert.InDelta(t, 0, savedOrder.ActualMoney, 0.000001)
	assert.Equal(t, 0, savedOrder.AffiliateSourceQuota)

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 862, plan.Id).First(&sub).Error)
	assert.Equal(t, "active", sub.Status)

	var usage PromoCodeUsage
	require.NoError(t, DB.Where("promo_code_id = ? AND order_no = ?", promo.Id, order.TradeNo).First(&usage).Error)
	assert.InDelta(t, 100, usage.OriginalAmount, 0.000001)
	assert.InDelta(t, 100, usage.DiscountAmount, 0.000001)
	assert.InDelta(t, 0, usage.PaidAmount, 0.000001)

	var recordCount int64
	require.NoError(t, DB.Model(&AffiliateRecord{}).Where("source_type = ? AND source_id = ?", AffiliateSourceSubscription, order.TradeNo).Count(&recordCount).Error)
	assert.EqualValues(t, 0, recordCount)
}

func TestRedeem_AffiliateRewardCanFilterRedemptionTopup(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.TriggerTopupEnabled = true
	affiliateSetting.FilterRedemptionTopupEnabled = false
	affiliateSetting.FirstLevelRatio = 10
	affiliateSetting.SecondLevelEnabled = false

	insertMarketingUser(t, 871, 0, 0)
	insertMarketingUser(t, 872, 871, 0)
	code := &Redemption{
		UserId:         1,
		Key:            "affiliate-redemption-code",
		Status:         common.RedemptionCodeStatusEnabled,
		Name:           "affiliate redemption",
		Quota:          1000,
		CreatedTime:    common.GetTimestamp(),
		MaxRedeemCount: 2,
	}
	require.NoError(t, code.Insert())

	quota, err := Redeem(code.Key, 872)
	require.NoError(t, err)
	assert.Equal(t, 1000, quota)

	var reward AffiliateRecord
	require.NoError(t, DB.Where("source_type = ? AND source_id = ?", AffiliateSourceRedemption, "redemption-1-user-872").First(&reward).Error)
	assert.Equal(t, 1000, reward.SourceQuota)
	assert.Equal(t, 100, reward.RewardQuota)

	affiliateSetting.FilterRedemptionTopupEnabled = true
	insertMarketingUser(t, 873, 871, 0)
	quota, err = Redeem(code.Key, 873)
	require.NoError(t, err)
	assert.Equal(t, 1000, quota)

	var filteredCount int64
	require.NoError(t, DB.Model(&AffiliateRecord{}).Where("source_type = ? AND source_id = ?", AffiliateSourceRedemption, "redemption-1-user-873").Count(&filteredCount).Error)
	assert.EqualValues(t, 0, filteredCount)
}
