package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetAffiliateSettingForTest(t *testing.T) {
	t.Helper()
	affiliateSetting := setting.GetAffiliateSetting()
	original := *affiliateSetting
	t.Cleanup(func() {
		*affiliateSetting = original
	})

	affiliateSetting.FirstLevelEnabled = true
	affiliateSetting.FirstLevelRatio = 10
	affiliateSetting.SecondLevelEnabled = true
	affiliateSetting.SecondLevelRatio = 5
	affiliateSetting.SettlementDelaySeconds = 60
	affiliateSetting.MinWithdrawalAmount = 10
	affiliateSetting.TriggerTopupEnabled = true
	affiliateSetting.TriggerSubscriptionEnabled = false
	affiliateSetting.PayoutMethods = "usdt,alipay,wechat"
	affiliateSetting.UsdtChain = "TRC20"
	affiliateSetting.PromotionTemplate = "邀请链接：{invite_link}"
}

func confirmAffiliatePaymentComplianceForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})

	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func insertAffiliateUser(t *testing.T, id int, inviterId int, quota int) {
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

func getAffiliateUserAffCodeForTest(t *testing.T, userId int) string {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("aff_code").Where("id = ?", userId).First(&user).Error)
	require.NotEmpty(t, user.AffCode)
	return user.AffCode
}

func getAffiliateBalanceForTest(t *testing.T, userId int) AffiliateBalance {
	t.Helper()
	balance, err := GetAffiliateBalance(userId)
	require.NoError(t, err)
	return *balance
}

func TestCreateAffiliateRewardsForPaymentCreatesTwoLevelsAndIsIdempotent(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	insertAffiliateUser(t, 1, 0, 0)
	insertAffiliateUser(t, 2, 1, 0)
	insertAffiliateUser(t, 3, 2, 0)

	require.NoError(t, CreateAffiliateRewardsForPayment(3, AffiliateSourceTopUp, "trade-1", 10000))
	require.NoError(t, CreateAffiliateRewardsForPayment(3, AffiliateSourceTopUp, "trade-1", 10000))

	var records []AffiliateRecord
	require.NoError(t, DB.Order("level asc").Find(&records).Error)
	require.Len(t, records, 2)

	assert.Equal(t, 2, records[0].UserId)
	assert.Equal(t, 3, records[0].InviteeId)
	assert.Equal(t, 1, records[0].Level)
	assert.Equal(t, 10000, records[0].SourceQuota)
	assert.Equal(t, 1000, records[0].RewardQuota)
	assert.Equal(t, AffiliateRecordStatusPending, records[0].Status)

	assert.Equal(t, 1, records[1].UserId)
	assert.Equal(t, 2, records[1].Level)
	assert.Equal(t, 500, records[1].RewardQuota)
	assert.Equal(t, AffiliateRecordStatusPending, records[1].Status)

	parentBalance := getAffiliateBalanceForTest(t, 2)
	assert.Equal(t, 1000, parentBalance.PendingQuota)
	assert.Equal(t, 0, parentBalance.AvailableQuota)
	assert.Equal(t, 1000, parentBalance.TotalQuota)

	grandParentBalance := getAffiliateBalanceForTest(t, 1)
	assert.Equal(t, 500, grandParentBalance.PendingQuota)
	assert.Equal(t, 500, grandParentBalance.TotalQuota)
}

func TestInvitedRegistrationKeepsInviteeRewardWithoutFixedInviterQuota(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	confirmAffiliatePaymentComplianceForTest(t)

	originalNewUserQuota := common.QuotaForNewUser
	originalInviteeQuota := common.QuotaForInvitee
	originalInviterQuota := common.QuotaForInviter
	t.Cleanup(func() {
		common.QuotaForNewUser = originalNewUserQuota
		common.QuotaForInvitee = originalInviteeQuota
		common.QuotaForInviter = originalInviterQuota
	})
	common.QuotaForNewUser = 0
	common.QuotaForInvitee = 123
	common.QuotaForInviter = 456

	insertAffiliateUser(t, 40, 0, 0)

	user := &User{
		Username:    common.GetRandomString(8),
		DisplayName: "invited",
		Status:      common.UserStatusEnabled,
		Role:        common.RoleCommonUser,
	}
	require.NoError(t, user.Insert(40))

	var invitee User
	require.NoError(t, DB.Where("username = ?", user.Username).First(&invitee).Error)
	assert.Equal(t, 40, invitee.InviterId)
	assert.Equal(t, 123, invitee.Quota)

	var inviter User
	require.NoError(t, DB.Where("id = ?", 40).First(&inviter).Error)
	assert.Equal(t, 1, inviter.AffCount)
	assert.Equal(t, 0, inviter.AffQuota)
	assert.Equal(t, 0, inviter.AffHistoryQuota)
}

func TestSettleMatureAffiliateRecordsMovesPendingToAvailable(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	insertAffiliateUser(t, 10, 0, 0)
	insertAffiliateUser(t, 11, 10, 0)

	require.NoError(t, CreateAffiliateRewardsForPayment(11, AffiliateSourceTopUp, "trade-2", 20000))
	assert.Equal(t, 2000, getAffiliateBalanceForTest(t, 10).PendingQuota)

	require.NoError(t, DB.Model(&AffiliateRecord{}).Where("user_id = ?", 10).Update("available_time", common.GetTimestamp()-1).Error)
	require.NoError(t, SettleMatureAffiliateRecords(10))

	balance := getAffiliateBalanceForTest(t, 10)
	assert.Equal(t, 0, balance.PendingQuota)
	assert.Equal(t, 2000, balance.AvailableQuota)

	var record AffiliateRecord
	require.NoError(t, DB.Where("user_id = ?", 10).First(&record).Error)
	assert.Equal(t, AffiliateRecordStatusAvailable, record.Status)
	assert.NotZero(t, record.SettledTime)
}

func TestAffiliateWithdrawalFreezesRejectsAndPaysQuota(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	setting.GetAffiliateSetting().MinWithdrawalAmount = 0

	insertAffiliateUser(t, 20, 0, 0)
	require.NoError(t, DB.Create(&AffiliateBalance{UserId: 20, AvailableQuota: 5000, TotalQuota: 5000}).Error)
	require.NoError(t, DB.Create(&AffiliatePayoutAccount{UserId: 20, AlipayAccount: "pay@example.com"}).Error)

	withdrawal, err := CreateAffiliateWithdrawal(20, AffiliatePayoutMethodAlipay, 2000)
	require.NoError(t, err)
	assert.Equal(t, AffiliateWithdrawalStatusPending, withdrawal.Status)

	balance := getAffiliateBalanceForTest(t, 20)
	assert.Equal(t, 3000, balance.AvailableQuota)
	assert.Equal(t, 2000, balance.FrozenQuota)

	require.NoError(t, RejectAffiliateWithdrawal(withdrawal.Id, 100, "资料不完整"))
	balance = getAffiliateBalanceForTest(t, 20)
	assert.Equal(t, 5000, balance.AvailableQuota)
	assert.Equal(t, 0, balance.FrozenQuota)

	withdrawal, err = CreateAffiliateWithdrawal(20, AffiliatePayoutMethodAlipay, 2000)
	require.NoError(t, err)
	require.NoError(t, MarkAffiliateWithdrawalPaid(withdrawal.Id, 100, "已打款"))
	balance = getAffiliateBalanceForTest(t, 20)
	assert.Equal(t, 3000, balance.AvailableQuota)
	assert.Equal(t, 0, balance.FrozenQuota)
	assert.Equal(t, 2000, balance.WithdrawnQuota)
}

func TestAffiliateWithdrawalRequiresPayoutAccountAndUsesDisplayMinimum(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalExchangeRate := operation_setting.USDExchangeRate
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.USDExchangeRate = originalExchangeRate
	})
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.USDExchangeRate = 7.3

	insertAffiliateUser(t, 21, 0, 0)
	minQuota := affiliateDisplayAmountToQuota(setting.GetAffiliateSetting().MinWithdrawalAmount)
	require.Greater(t, minQuota, 0)
	require.NoError(t, DB.Create(&AffiliateBalance{UserId: 21, AvailableQuota: minQuota, TotalQuota: minQuota}).Error)

	_, err := CreateAffiliateWithdrawal(21, AffiliatePayoutMethodAlipay, minQuota-1)
	require.Error(t, err)

	_, err = CreateAffiliateWithdrawal(21, AffiliatePayoutMethodAlipay, minQuota)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "支付宝")

	require.NoError(t, DB.Create(&AffiliatePayoutAccount{UserId: 21, AlipayQrPath: "/upload/affiliate_qr/test.png"}).Error)
	withdrawal, err := CreateAffiliateWithdrawal(21, AffiliatePayoutMethodAlipay, minQuota)
	require.NoError(t, err)
	assert.Equal(t, minQuota, withdrawal.Quota)
}

func TestAffiliateWithdrawalHonorsConfiguredPayoutMethods(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.MinWithdrawalAmount = 0
	affiliateSetting.PayoutMethods = "usdt"

	insertAffiliateUser(t, 22, 0, 0)
	require.NoError(t, DB.Create(&AffiliateBalance{UserId: 22, AvailableQuota: 5000, TotalQuota: 5000}).Error)
	require.NoError(t, DB.Create(&AffiliatePayoutAccount{
		UserId:        22,
		UsdtAddress:   "TExampleAddress",
		AlipayAccount: "pay@example.com",
	}).Error)

	_, err := CreateAffiliateWithdrawal(22, AffiliatePayoutMethodAlipay, 1000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未开放")

	withdrawal, err := CreateAffiliateWithdrawal(22, AffiliatePayoutMethodUSDT, 1000)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePayoutMethodUSDT, withdrawal.Method)
}

func TestAffiliateWithdrawalKeepsDefaultPayoutMethodsWhenConfigEmpty(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)
	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.MinWithdrawalAmount = 0
	affiliateSetting.PayoutMethods = ""

	insertAffiliateUser(t, 23, 0, 0)
	require.NoError(t, DB.Create(&AffiliateBalance{UserId: 23, AvailableQuota: 5000, TotalQuota: 5000}).Error)
	require.NoError(t, DB.Create(&AffiliatePayoutAccount{UserId: 23, AlipayAccount: "pay@example.com"}).Error)

	withdrawal, err := CreateAffiliateWithdrawal(23, AffiliatePayoutMethodAlipay, 1000)
	require.NoError(t, err)
	assert.Equal(t, AffiliatePayoutMethodAlipay, withdrawal.Method)
}

func TestTransferAffiliateQuotaToBalance(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	insertAffiliateUser(t, 30, 0, 100)
	require.NoError(t, DB.Create(&AffiliateBalance{UserId: 30, AvailableQuota: 3000, TotalQuota: 3000}).Error)

	require.NoError(t, TransferAffiliateQuotaToBalance(30, 1000))

	balance := getAffiliateBalanceForTest(t, 30)
	assert.Equal(t, 2000, balance.AvailableQuota)
	assert.Equal(t, 1000, balance.TransferredQuota)

	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", 30).First(&user).Error)
	assert.Equal(t, 1100, user.Quota)
}

func TestGetAffiliateLeaderboardAggregatesInvitesAndCommissionByPeriod(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	dayStart := now
	old := now - 40*24*3600

	require.NoError(t, DB.Create(&User{Id: 50, Username: "alice", AffCode: "aff50", Status: common.UserStatusEnabled, CreatedAt: old}).Error)
	require.NoError(t, DB.Create(&User{Id: 51, Username: "bob", AffCode: "aff51", Status: common.UserStatusEnabled, CreatedAt: old}).Error)
	require.NoError(t, DB.Create(&User{Id: 52, Username: "a1", AffCode: "aff52", Status: common.UserStatusEnabled, InviterId: 50, CreatedAt: dayStart}).Error)
	require.NoError(t, DB.Create(&User{Id: 53, Username: "a2", AffCode: "aff53", Status: common.UserStatusEnabled, InviterId: 50, CreatedAt: dayStart}).Error)
	require.NoError(t, DB.Create(&User{Id: 54, Username: "b1", AffCode: "aff54", Status: common.UserStatusEnabled, InviterId: 51, CreatedAt: dayStart}).Error)
	require.NoError(t, DB.Create(&User{Id: 55, Username: "old", AffCode: "aff55", Status: common.UserStatusEnabled, InviterId: 51, CreatedAt: old}).Error)

	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 50, InviteeId: 52, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "lb-1", RewardQuota: 1000, Status: AffiliateRecordStatusAvailable, CreatedAt: dayStart}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 51, InviteeId: 54, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "lb-2", RewardQuota: 2000, Status: AffiliateRecordStatusAvailable, CreatedAt: dayStart}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 50, InviteeId: 55, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "lb-old", RewardQuota: 9999, Status: AffiliateRecordStatusAvailable, CreatedAt: old}).Error)

	items, err := GetAffiliateLeaderboard("day", 10, "commission")
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, 1, items[0].Rank)
	assert.Equal(t, 51, items[0].UserId)
	assert.Equal(t, 1, items[0].InviteCount)
	assert.Equal(t, 2000, items[0].CommissionQuota)

	assert.Equal(t, 2, items[1].Rank)
	assert.Equal(t, 50, items[1].UserId)
	assert.Equal(t, 2, items[1].InviteCount)
	assert.Equal(t, 1000, items[1].CommissionQuota)

	items, err = GetAffiliateLeaderboard("day", 10, "invites")
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, 1, items[0].Rank)
	assert.Equal(t, 50, items[0].UserId)
	assert.Equal(t, 2, items[0].InviteCount)
	assert.Equal(t, 1000, items[0].CommissionQuota)

	assert.Equal(t, 2, items[1].Rank)
	assert.Equal(t, 51, items[1].UserId)
	assert.Equal(t, 1, items[1].InviteCount)
	assert.Equal(t, 2000, items[1].CommissionQuota)
}

func TestSaveAffiliatePayoutAccountPreservesQrPaths(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	require.NoError(t, DB.Create(&AffiliatePayoutAccount{
		UserId:        60,
		AlipayQrPath:  "/upload/affiliate_qr/alipay-old.png",
		WechatQrPath:  "/upload/affiliate_qr/wechat-old.png",
		AlipayAccount: "old@example.com",
	}).Error)

	require.NoError(t, SaveAffiliatePayoutAccount(&AffiliatePayoutAccount{
		UserId:        60,
		UsdtAddress:   "TExample",
		AlipayAccount: "new@example.com",
		WechatAccount: "wechat-id",
	}))

	account, err := GetAffiliatePayoutAccount(60)
	require.NoError(t, err)
	assert.Equal(t, "new@example.com", account.AlipayAccount)
	assert.Equal(t, "wechat-id", account.WechatAccount)
	assert.Equal(t, "/upload/affiliate_qr/alipay-old.png", account.AlipayQrPath)
	assert.Equal(t, "/upload/affiliate_qr/wechat-old.png", account.WechatQrPath)
}

func TestBindUserInviterByAffCodeUpdatesInviterAndCounts(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	insertAffiliateUser(t, 70, 0, 0)
	insertAffiliateUser(t, 71, 0, 0)
	insertAffiliateUser(t, 72, 0, 0)

	result, err := BindUserInviterByAffCode(72, "", getAffiliateUserAffCodeForTest(t, 70), false)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Updated)
	assert.Equal(t, 72, result.UserId)
	assert.Equal(t, 70, result.InviterId)
	assert.Equal(t, 0, result.PreviousInviterId)

	var invitee User
	require.NoError(t, DB.Select("id", "inviter_id").Where("id = ?", 72).First(&invitee).Error)
	assert.Equal(t, 70, invitee.InviterId)

	var inviter User
	require.NoError(t, DB.Select("aff_count").Where("id = ?", 70).First(&inviter).Error)
	assert.Equal(t, 1, inviter.AffCount)

	_, err = BindUserInviterByAffCode(72, "", getAffiliateUserAffCodeForTest(t, 71), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已有邀请人")

	result, err = BindUserInviterByAffCode(72, "", getAffiliateUserAffCodeForTest(t, 71), true)
	require.NoError(t, err)
	assert.True(t, result.Updated)
	assert.Equal(t, 70, result.PreviousInviterId)
	assert.Equal(t, 71, result.InviterId)

	require.NoError(t, DB.Select("inviter_id").Where("id = ?", 72).First(&invitee).Error)
	assert.Equal(t, 71, invitee.InviterId)

	require.NoError(t, DB.Select("aff_count").Where("id = ?", 70).First(&inviter).Error)
	assert.Equal(t, 0, inviter.AffCount)
	require.NoError(t, DB.Select("aff_count").Where("id = ?", 71).First(&inviter).Error)
	assert.Equal(t, 1, inviter.AffCount)
}

func TestBindUserInviterByAffCodeRejectsSelfAndCycles(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	insertAffiliateUser(t, 73, 0, 0)
	insertAffiliateUser(t, 74, 73, 0)

	_, err := BindUserInviterByAffCode(73, "", getAffiliateUserAffCodeForTest(t, 73), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不能绑定自己")

	_, err = BindUserInviterByAffCode(73, "", getAffiliateUserAffCodeForTest(t, 74), true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "循环邀请")
}

func TestBindUserInviterByAffCodeRejectsAmbiguousUserIdentifier(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	require.NoError(t, DB.Create(&User{Id: 75, Username: "target", DisplayName: "same-keyword", Email: "target@example.com", AffCode: "aff75", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 76, Username: "other", DisplayName: "same-keyword", Email: "other@example.com", AffCode: "aff76", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 77, Username: "inviter", AffCode: "aff77", Status: common.UserStatusEnabled}).Error)

	_, err := BindUserInviterByAffCode(0, "same-keyword", "aff77", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "匹配到多个用户")
}

func TestBindUserInviterByAffCodeRejectsDuplicateAffCode(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	require.NoError(t, DB.Exec("DROP INDEX IF EXISTS idx_users_aff_code").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM users").Error)
		require.NoError(t, DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_aff_code ON users(aff_code)").Error)
	})

	require.NoError(t, DB.Create(&User{Id: 78, Username: "target", AffCode: "aff78", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 79, Username: "inviter-a", AffCode: "dup-aff", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 80, Username: "inviter-b", AffCode: "dup-aff", Status: common.UserStatusEnabled}).Error)

	_, err := BindUserInviterByAffCode(78, "", "dup-aff", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "邀请代码存在冲突")
}

func TestGetAffiliateRecordsWithDetailsBuildsSourceDetails(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	insertAffiliateUser(t, 80, 0, 0)
	insertAffiliateUser(t, 81, 80, 0)
	now := common.GetTimestamp()

	require.NoError(t, DB.Create(&TopUp{
		UserId:               81,
		Amount:               100,
		Money:                50,
		OriginalMoney:        100,
		DiscountMoney:        50,
		ActualMoney:          50,
		PromoCode:            "TOPHALF",
		AffiliateSourceQuota: int(50 * common.QuotaPerUnit),
		TradeNo:              "aff-topup-detail",
		PaymentProvider:      PaymentProviderEpay,
		PaymentMethod:        "alipay",
		CreateTime:           now,
		CompleteTime:         now,
		Status:               common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{
		UserId:      80,
		InviteeId:   81,
		Level:       1,
		SourceType:  AffiliateSourceTopUp,
		SourceId:    "aff-topup-detail",
		SourceQuota: int(50 * common.QuotaPerUnit),
		RewardQuota: int(5 * common.QuotaPerUnit),
		Ratio:       10,
		Status:      AffiliateRecordStatusPending,
	}).Error)

	plan := &SubscriptionPlan{
		Id:            9080,
		Title:         "Pro Monthly",
		PriceAmount:   120,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   999999,
	}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		UserId:               81,
		PlanId:               plan.Id,
		Money:                90,
		OriginalMoney:        120,
		DiscountMoney:        30,
		ActualMoney:          90,
		PromoCode:            "SUB25",
		AffiliateSourceQuota: int(90 * common.QuotaPerUnit),
		TradeNo:              "aff-sub-detail",
		PaymentProvider:      PaymentProviderEpay,
		PaymentMethod:        "alipay",
		Status:               common.TopUpStatusSuccess,
		CreateTime:           now,
		CompleteTime:         now,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{
		UserId:      80,
		InviteeId:   81,
		Level:       1,
		SourceType:  AffiliateSourceSubscription,
		SourceId:    "aff-sub-detail",
		SourceQuota: int(90 * common.QuotaPerUnit),
		RewardQuota: int(9 * common.QuotaPerUnit),
		Ratio:       10,
		Status:      AffiliateRecordStatusPending,
	}).Error)

	redemption := &Redemption{
		UserId:         1,
		Key:            "detail-redemption",
		Status:         common.RedemptionCodeStatusEnabled,
		Name:           "VIP Gift",
		Quota:          1000,
		CreatedTime:    now,
		MaxRedeemCount: 1,
	}
	require.NoError(t, redemption.Insert())
	redemptionSourceId := fmt.Sprintf("redemption-%d-user-%d", redemption.Id, 81)
	require.NoError(t, DB.Create(&AffiliateRecord{
		UserId:      80,
		InviteeId:   81,
		Level:       1,
		SourceType:  AffiliateSourceRedemption,
		SourceId:    redemptionSourceId,
		SourceQuota: 1000,
		RewardQuota: 100,
		Ratio:       10,
		Status:      AffiliateRecordStatusPending,
	}).Error)

	records, total, err := GetAffiliateRecordsWithDetails(80, "", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, records, 3)

	detailsByType := map[string]*AffiliateSourceDetail{}
	for _, record := range records {
		detailsByType[record.SourceType] = record.Detail
	}

	topupDetail := detailsByType[AffiliateSourceTopUp]
	require.NotNil(t, topupDetail)
	assert.Equal(t, "余额充值", topupDetail.Title)
	assert.Equal(t, "TOPHALF", topupDetail.PromoCode)
	assert.InDelta(t, 100, topupDetail.OriginalAmount, 0.000001)
	assert.InDelta(t, 50, topupDetail.DiscountAmount, 0.000001)
	assert.InDelta(t, 50, topupDetail.PaidAmount, 0.000001)

	subscriptionDetail := detailsByType[AffiliateSourceSubscription]
	require.NotNil(t, subscriptionDetail)
	assert.Equal(t, "订阅：Pro Monthly", subscriptionDetail.Title)
	assert.Equal(t, "Pro Monthly", subscriptionDetail.PlanTitle)
	assert.Equal(t, "SUB25", subscriptionDetail.PromoCode)
	assert.InDelta(t, 120, subscriptionDetail.OriginalAmount, 0.000001)
	assert.InDelta(t, 30, subscriptionDetail.DiscountAmount, 0.000001)
	assert.InDelta(t, 90, subscriptionDetail.PaidAmount, 0.000001)

	redemptionDetail := detailsByType[AffiliateSourceRedemption]
	require.NotNil(t, redemptionDetail)
	assert.Equal(t, "兑换码兑换：VIP Gift", redemptionDetail.Title)
	assert.Equal(t, "VIP Gift", redemptionDetail.RedemptionName)
	assert.Equal(t, 1000, redemptionDetail.Quota)
}

func TestSetAffiliatePayoutQrPathReplacesAndClearsQrPath(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	require.NoError(t, DB.Create(&AffiliatePayoutAccount{
		UserId:        61,
		AlipayQrPath:  "/upload/affiliate_qr/alipay-old.png",
		WechatQrPath:  "/upload/affiliate_qr/wechat-old.png",
		AlipayAccount: "pay@example.com",
	}).Error)

	account, err := SetAffiliatePayoutQrPath(61, AffiliatePayoutMethodAlipay, "/upload/affiliate_qr/alipay-new.png")
	require.NoError(t, err)
	assert.Equal(t, "/upload/affiliate_qr/alipay-new.png", account.AlipayQrPath)
	assert.Equal(t, "/upload/affiliate_qr/wechat-old.png", account.WechatQrPath)

	account, err = SetAffiliatePayoutQrPath(61, AffiliatePayoutMethodAlipay, "")
	require.NoError(t, err)
	assert.Equal(t, "", account.AlipayQrPath)
	assert.Equal(t, "/upload/affiliate_qr/wechat-old.png", account.WechatQrPath)
}
