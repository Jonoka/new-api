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

func TestGetAffiliateLeaderboardKeepsInviteAndCommissionMetricsSeparate(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	old := now - 90*24*3600
	require.NoError(t, DB.Create(&User{Id: 58, Username: "historical", AffCode: "aff58", Status: common.UserStatusEnabled, CreatedAt: old}).Error)
	require.NoError(t, DB.Create(&User{Id: 59, Username: "historical-invitee", AffCode: "aff59", Status: common.UserStatusEnabled, InviterId: 58, CreatedAt: old}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 58, InviteeId: 59, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "lb-month-current", RewardQuota: 1000, Status: AffiliateRecordStatusAvailable, CreatedAt: now}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 58, InviteeId: 59, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "lb-month-old", RewardQuota: 9000, Status: AffiliateRecordStatusAvailable, CreatedAt: old}).Error)

	monthlyItems, err := GetAffiliateLeaderboardByMetric("month", 10, "commission", "commission")
	require.NoError(t, err)
	require.Len(t, monthlyItems, 1)
	assert.Equal(t, 58, monthlyItems[0].UserId)
	assert.Equal(t, 0, monthlyItems[0].InviteCount)
	assert.Equal(t, 1000, monthlyItems[0].CommissionQuota)

	inviteItems, err := GetAffiliateLeaderboardByMetric("month", 10, "invites", "invites")
	require.NoError(t, err)
	assert.Empty(t, inviteItems)
}

func TestGetAffiliateLeaderboardByMetricPageKeepsGlobalRank(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	for i := 0; i < 3; i++ {
		userId := 70 + i
		require.NoError(t, DB.Create(&User{
			Id:        userId,
			Username:  fmt.Sprintf("rank-user-%d", i),
			AffCode:   fmt.Sprintf("aff%d", userId),
			Status:    common.UserStatusEnabled,
			CreatedAt: now,
		}).Error)
		require.NoError(t, DB.Create(&AffiliateRecord{
			UserId:      userId,
			InviteeId:   100 + i,
			Level:       1,
			SourceType:  AffiliateSourceTopUp,
			SourceId:    fmt.Sprintf("rank-%d", i),
			RewardQuota: (3 - i) * 1000,
			Status:      AffiliateRecordStatusAvailable,
			CreatedAt:   now,
		}).Error)
	}

	items, total, err := GetAffiliateLeaderboardByMetricPage("month", 2, 1, "commission", "commission")
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, items, 1)
	assert.Equal(t, 2, items[0].Rank)
	assert.Equal(t, 71, items[0].UserId)
}

func TestGetAffiliateLeaderboardMasksPublicUserNames(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{
		Id:          90,
		Username:    "private-alice",
		DisplayName: "Alice Wang",
		AffCode:     "aff90",
		Status:      common.UserStatusEnabled,
		CreatedAt:   now,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{
		UserId:      90,
		InviteeId:   91,
		Level:       1,
		SourceType:  AffiliateSourceTopUp,
		SourceId:    "masked-lb",
		RewardQuota: 1000,
		Status:      AffiliateRecordStatusAvailable,
		CreatedAt:   now,
	}).Error)

	items, err := GetAffiliateLeaderboard("day", 10, "commission")
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, 90, items[0].UserId)
	assert.Empty(t, items[0].Username)
	assert.Empty(t, items[0].DisplayName)
	assert.Equal(t, "Ali***ang", items[0].MaskedName)
	assert.NotContains(t, items[0].MaskedName, "Alice Wang")
}

func TestGetAffiliateInvitationsForUserAggregatesInviteePurchases(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 92, Username: "owner", DisplayName: "Owner", AffCode: "aff92", Status: common.UserStatusEnabled, CreatedAt: now - 100}).Error)
	require.NoError(t, DB.Create(&User{Id: 93, Username: "buyer-a", DisplayName: "Buyer A", AffCode: "aff93", Status: common.UserStatusEnabled, InviterId: 92, CreatedAt: now - 90}).Error)
	require.NoError(t, DB.Create(&User{Id: 94, Username: "buyer-b", DisplayName: "Buyer B", AffCode: "aff94", Status: common.UserStatusEnabled, InviterId: 92, CreatedAt: now - 80}).Error)
	require.NoError(t, DB.Create(&User{Id: 95, Username: "other-buyer", DisplayName: "Other", AffCode: "aff95", Status: common.UserStatusEnabled, InviterId: 96, CreatedAt: now - 70}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:               93,
		Amount:               7000,
		Money:                70,
		ActualMoney:          70,
		AffiliateSourceQuota: 7000,
		TradeNo:              "invite-user-topup",
		PaymentProvider:      PaymentProviderStripe,
		PaymentMethod:        PaymentMethodStripe,
		CreateTime:           now - 30,
		CompleteTime:         now - 20,
		Status:               common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 92, InviteeId: 93, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "invite-user-topup", SourceQuota: 7000, RewardQuota: 700, Ratio: 10, Status: AffiliateRecordStatusAvailable, CreatedAt: now - 10}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 96, InviteeId: 95, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "other-topup", SourceQuota: 9000, RewardQuota: 900, Ratio: 10, Status: AffiliateRecordStatusAvailable, CreatedAt: now - 9}).Error)

	items, total, err := GetAffiliateInvitations(92, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, items, 2)

	byInviteeId := map[int]*AffiliateInvitationItem{}
	for _, item := range items {
		byInviteeId[item.Invitee.Id] = item
	}
	require.Contains(t, byInviteeId, 93)
	require.Contains(t, byInviteeId, 94)
	assert.NotContains(t, byInviteeId, 95)
	assert.Equal(t, "buyer-a", byInviteeId[93].Invitee.Username)
	assert.Equal(t, "Buyer A", byInviteeId[93].Invitee.DisplayName)
	assert.Equal(t, 1, byInviteeId[93].TopUpCount)
	assert.Equal(t, 7000, byInviteeId[93].TopUpQuota)
	assert.Equal(t, now-20, byInviteeId[93].LastTopUpTime)
	assert.Equal(t, 700, byInviteeId[93].CommissionQuota)
	assert.Equal(t, 0, byInviteeId[94].TopUpCount)
	assert.Equal(t, 0, byInviteeId[94].CommissionQuota)
}

func TestGetAffiliateRecordsWithDetailsIncludesInviteeUser(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 97, Username: "seller", DisplayName: "Seller", AffCode: "aff97", Status: common.UserStatusEnabled, CreatedAt: now - 100}).Error)
	require.NoError(t, DB.Create(&User{Id: 98, Username: "purchase-user", DisplayName: "Purchase User", AffCode: "aff98", Status: common.UserStatusEnabled, InviterId: 97, CreatedAt: now - 90}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:               98,
		Amount:               6000,
		Money:                60,
		ActualMoney:          60,
		AffiliateSourceQuota: 6000,
		TradeNo:              "record-invitee-topup",
		PaymentProvider:      PaymentProviderStripe,
		PaymentMethod:        PaymentMethodStripe,
		CreateTime:           now - 30,
		CompleteTime:         now - 20,
		Status:               common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 97, InviteeId: 98, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "record-invitee-topup", SourceQuota: 6000, RewardQuota: 600, Ratio: 10, Status: AffiliateRecordStatusAvailable, CreatedAt: now - 10}).Error)

	items, total, err := GetAffiliateRecordsWithDetails(97, "", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, 98, items[0].Invitee.Id)
	assert.Equal(t, "purchase-user", items[0].Invitee.Username)
	assert.Equal(t, "Purchase User", items[0].Invitee.DisplayName)
	require.NotNil(t, items[0].Detail)
	assert.Equal(t, "余额充值", items[0].Detail.Title)
}

func TestGetAdminAffiliateInvitationsAggregatesInviteeRechargeAndCommission(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 100, Username: "inviter", DisplayName: "Inviter", Email: "inviter@example.com", AffCode: "aff100", Status: common.UserStatusEnabled, CreatedAt: now - 100}).Error)
	require.NoError(t, DB.Create(&User{Id: 101, Username: "invitee", DisplayName: "Invitee", Email: "invitee@example.com", AffCode: "aff101", Status: common.UserStatusEnabled, InviterId: 100, CreatedAt: now - 50}).Error)
	require.NoError(t, DB.Create(&User{Id: 102, Username: "other", DisplayName: "Other", Email: "other@example.com", AffCode: "aff102", Status: common.UserStatusEnabled, CreatedAt: now - 40}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:               101,
		Amount:               100,
		Money:                50,
		ActualMoney:          50,
		AffiliateSourceQuota: 5000,
		TradeNo:              "admin-invite-topup",
		PaymentProvider:      PaymentProviderEpay,
		PaymentMethod:        "alipay",
		CreateTime:           now - 20,
		CompleteTime:         now - 10,
		Status:               common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{
		UserId:      100,
		InviteeId:   101,
		Level:       1,
		SourceType:  AffiliateSourceTopUp,
		SourceId:    "admin-invite-topup",
		SourceQuota: 5000,
		RewardQuota: 500,
		Ratio:       10,
		Status:      AffiliateRecordStatusAvailable,
		CreatedAt:   now - 5,
	}).Error)

	items, total, err := GetAdminAffiliateInvitations("", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)

	assert.Equal(t, 100, items[0].InviterId)
	assert.Equal(t, "inviter", items[0].InviterUsername)
	assert.Equal(t, "aff100", items[0].InviterAffCode)
	assert.Equal(t, 101, items[0].InviteeId)
	assert.Equal(t, "invitee", items[0].InviteeUsername)
	assert.Equal(t, "invitee@example.com", items[0].InviteeEmail)
	assert.EqualValues(t, 1, items[0].TopUpCount)
	assert.Equal(t, 5000, items[0].TopUpQuota)
	assert.Equal(t, 500, items[0].CommissionQuota)
}

func TestGetAffiliateInvitationsUsesRecordSourceQuotaForLegacyTopUpAmount(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	sourceQuota := int(10 * common.QuotaPerUnit)
	rewardQuota := int(1 * common.QuotaPerUnit)
	require.NoError(t, DB.Create(&User{Id: 103, Username: "legacy-inviter", DisplayName: "Legacy Inviter", Email: "legacy-inviter@example.com", AffCode: "aff103", Status: common.UserStatusEnabled, CreatedAt: now - 100}).Error)
	require.NoError(t, DB.Create(&User{Id: 104, Username: "legacy-invitee", DisplayName: "Legacy Invitee", Email: "legacy-invitee@example.com", AffCode: "aff104", Status: common.UserStatusEnabled, InviterId: 103, CreatedAt: now - 80}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:               104,
		Amount:               10,
		Money:                10,
		ActualMoney:          10,
		AffiliateSourceQuota: 0,
		TradeNo:              "legacy-epay-topup",
		PaymentProvider:      PaymentProviderEpay,
		PaymentMethod:        "alipay",
		CreateTime:           now - 40,
		CompleteTime:         now - 30,
		Status:               common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{
		UserId:      103,
		InviteeId:   104,
		Level:       1,
		SourceType:  AffiliateSourceTopUp,
		SourceId:    "legacy-epay-topup",
		SourceQuota: sourceQuota,
		RewardQuota: rewardQuota,
		Ratio:       10,
		Status:      AffiliateRecordStatusAvailable,
		CreatedAt:   now - 20,
	}).Error)

	userItems, userTotal, err := GetAffiliateInvitations(103, &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, userTotal)
	require.Len(t, userItems, 1)
	assert.Equal(t, 1, userItems[0].TopUpCount)
	assert.Equal(t, sourceQuota, userItems[0].TopUpQuota)
	assert.Equal(t, rewardQuota, userItems[0].CommissionQuota)

	adminItems, adminTotal, err := GetAdminAffiliateInvitations("", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, adminTotal)
	require.Len(t, adminItems, 1)
	assert.EqualValues(t, 1, adminItems[0].TopUpCount)
	assert.Equal(t, sourceQuota, adminItems[0].TopUpQuota)
	assert.Equal(t, rewardQuota, adminItems[0].CommissionQuota)
}

func TestGetAdminAffiliateRecordsWithDetailsIncludesUsersAndSourceFilter(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 110, Username: "parent", DisplayName: "Parent", Email: "parent@example.com", AffCode: "aff110", Status: common.UserStatusEnabled, CreatedAt: now - 100}).Error)
	require.NoError(t, DB.Create(&User{Id: 111, Username: "child", DisplayName: "Child", Email: "child@example.com", AffCode: "aff111", Status: common.UserStatusEnabled, InviterId: 110, CreatedAt: now - 90}).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:               111,
		Amount:               8000,
		Money:                80,
		OriginalMoney:        100,
		DiscountMoney:        20,
		ActualMoney:          80,
		AffiliateSourceQuota: 8000,
		TradeNo:              "admin-record-topup",
		PaymentProvider:      PaymentProviderStripe,
		PaymentMethod:        PaymentMethodStripe,
		CreateTime:           now - 20,
		CompleteTime:         now - 10,
		Status:               common.TopUpStatusSuccess,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{
		UserId:      110,
		InviteeId:   111,
		Level:       1,
		SourceType:  AffiliateSourceTopUp,
		SourceId:    "admin-record-topup",
		SourceQuota: 8000,
		RewardQuota: 800,
		Ratio:       10,
		Status:      AffiliateRecordStatusAvailable,
		CreatedAt:   now - 5,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{
		UserId:      110,
		InviteeId:   111,
		Level:       1,
		SourceType:  AffiliateSourceSubscription,
		SourceId:    "admin-record-subscription",
		SourceQuota: 9000,
		RewardQuota: 900,
		Ratio:       10,
		Status:      AffiliateRecordStatusAvailable,
		CreatedAt:   now - 4,
	}).Error)

	items, total, err := GetAdminAffiliateRecordsWithDetails(AffiliateSourceTopUp, "", "", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)

	assert.Equal(t, AffiliateSourceTopUp, items[0].SourceType)
	assert.Equal(t, 110, items[0].Inviter.Id)
	assert.Equal(t, "parent", items[0].Inviter.Username)
	assert.Equal(t, "parent@example.com", items[0].Inviter.Email)
	assert.Equal(t, 111, items[0].Invitee.Id)
	assert.Equal(t, "child", items[0].Invitee.Username)
	assert.Equal(t, "child@example.com", items[0].Invitee.Email)
	require.NotNil(t, items[0].Detail)
	assert.Equal(t, "余额充值", items[0].Detail.Title)
	assert.InDelta(t, 100, items[0].Detail.OriginalAmount, 0.000001)
	assert.InDelta(t, 20, items[0].Detail.DiscountAmount, 0.000001)
	assert.InDelta(t, 80, items[0].Detail.PaidAmount, 0.000001)
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

func TestAffiliateFraudDetectionFiltersIPv6AndTopupLogs(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 201, Username: "fraud-parent", Email: "parent@example.com", AffCode: "aff201", Status: common.UserStatusEnabled, AffCount: 2}).Error)
	require.NoError(t, DB.Create(&User{Id: 202, Username: "ipv6-child", Email: "ipv6@example.com", AffCode: "aff202", Status: common.UserStatusEnabled, InviterId: 201}).Error)
	require.NoError(t, DB.Create(&User{Id: 203, Username: "topup-child", Email: "topup@example.com", AffCode: "aff203", Status: common.UserStatusEnabled, InviterId: 201}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 201, Ip: "2a0a:4cc0:2000:2ae1:a487:f9ff:fe89:6ccf", Action: "login", CreatedAt: now}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 202, Ip: "2a0a:4cc0:2000:2ae1:a487:f9ff:fe89:6ccf", Action: "login", CreatedAt: now}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 201, Username: "fraud-parent", Type: LogTypeTopup, Ip: "45.205.31.18", CreatedAt: now}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 203, Username: "topup-child", Type: LogTypeTopup, Ip: "45.205.31.18", CreatedAt: now}).Error)

	newAlerts, err := DetectFraudDeep(30)
	require.NoError(t, err)
	assert.Equal(t, 0, newAlerts)

	var count int64
	require.NoError(t, DB.Model(&AffiliateFraudAlert{}).Where("status = ?", FraudAlertStatusDetected).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestAffiliateFraudDeepDetectsIPv4AndRescansExistingAlerts(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 211, Username: "deep-parent", Email: "deep-parent@example.com", AffCode: "aff211", Status: common.UserStatusEnabled, AffCount: 1}).Error)
	require.NoError(t, DB.Create(&User{Id: 212, Username: "deep-child", Email: "deep-child@example.com", AffCode: "aff212", Status: common.UserStatusEnabled, InviterId: 211}).Error)
	require.NoError(t, DB.Create(&AffiliateFraudAlert{
		InviterId:     211,
		InviteeId:     212,
		SharedIps:     `["2a0a:4cc0:2000:2ae1:a487:f9ff:fe89:6ccf","203.0.113.10"]`,
		SharedIpCount: 2,
		Status:        FraudAlertStatusDetected,
		DetectedAt:    now - 10,
	}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 211, Username: "deep-parent", Type: LogTypeConsume, Ip: "203.0.113.10", CreatedAt: now}).Error)
	require.NoError(t, LOG_DB.Create(&Log{UserId: 212, Username: "deep-child", Type: LogTypeError, Ip: "203.0.113.10", CreatedAt: now}).Error)

	newAlerts, err := DetectFraudDeep(30)
	require.NoError(t, err)
	assert.Equal(t, 0, newAlerts)

	var alert AffiliateFraudAlert
	require.NoError(t, DB.Where("inviter_id = ? AND invitee_id = ?", 211, 212).First(&alert).Error)
	assert.Equal(t, FraudAlertStatusDetected, alert.Status)
	assert.Equal(t, 1, alert.SharedIpCount)
	assert.Equal(t, `["203.0.113.10"]`, alert.SharedIps)
}

func TestAffiliateFraudRescanDismissesStaleAlerts(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 221, Username: "stale-parent", AffCode: "aff221", Status: common.UserStatusEnabled, AffCount: 1}).Error)
	require.NoError(t, DB.Create(&User{Id: 222, Username: "stale-child", AffCode: "aff222", Status: common.UserStatusEnabled, InviterId: 221}).Error)
	require.NoError(t, DB.Create(&AffiliateFraudAlert{
		InviterId:     221,
		InviteeId:     222,
		SharedIps:     `["2a0a:4cc0:2000:2ae1:a487:f9ff:fe89:6ccf"]`,
		SharedIpCount: 1,
		Status:        FraudAlertStatusDetected,
		DetectedAt:    now - 10,
	}).Error)

	newAlerts, err := DetectFraudDeep(30)
	require.NoError(t, err)
	assert.Equal(t, 0, newAlerts)

	var count int64
	require.NoError(t, DB.Model(&AffiliateFraudAlert{}).Where("inviter_id = ? AND invitee_id = ?", 221, 222).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestAffiliateFraudBulkRescanDismissesStaleAlerts(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 225, Username: "bulk-stale-parent", AffCode: "aff225", Status: common.UserStatusEnabled, AffCount: 1}).Error)
	require.NoError(t, DB.Create(&User{Id: 226, Username: "bulk-stale-child", AffCode: "aff226", Status: common.UserStatusEnabled, InviterId: 225}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 225, Ip: "2a0a:4cc0:2000:2ae1:a487:f9ff:fe89:6ccf", Action: "login", CreatedAt: now}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 226, Ip: "2a0a:4cc0:2000:2ae1:a487:f9ff:fe89:6ccf", Action: "login", CreatedAt: now}).Error)
	require.NoError(t, DB.Create(&AffiliateFraudAlert{
		InviterId:     225,
		InviteeId:     226,
		SharedIps:     `["2a0a:4cc0:2000:2ae1:a487:f9ff:fe89:6ccf"]`,
		SharedIpCount: 1,
		Status:        FraudAlertStatusDetected,
		DetectedAt:    now - 10,
	}).Error)

	newAlerts, err := DetectFraudBulk(30)
	require.NoError(t, err)
	assert.Equal(t, 0, newAlerts)

	var count int64
	require.NoError(t, DB.Model(&AffiliateFraudAlert{}).Where("inviter_id = ? AND invitee_id = ?", 225, 226).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestAffiliateFraudDetectionRespectsRecentDaysWindow(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	old := now - 40*86400
	require.NoError(t, DB.Create(&User{Id: 227, Username: "window-parent", AffCode: "aff227", Status: common.UserStatusEnabled, AffCount: 1}).Error)
	require.NoError(t, DB.Create(&User{Id: 228, Username: "window-child", AffCode: "aff228", Status: common.UserStatusEnabled, InviterId: 227}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 227, Ip: "198.51.100.88", Action: "login", CreatedAt: old}).Error)
	require.NoError(t, DB.Create(&UserIPRecord{UserId: 228, Ip: "198.51.100.88", Action: "login", CreatedAt: old}).Error)

	newAlerts, err := DetectFraudBulk(30)
	require.NoError(t, err)
	assert.Equal(t, 0, newAlerts)

	var count int64
	require.NoError(t, DB.Model(&AffiliateFraudAlert{}).Where("inviter_id = ? AND invitee_id = ?", 227, 228).Count(&count).Error)
	assert.EqualValues(t, 0, count)

	newAlerts, err = DetectFraudBulk(0)
	require.NoError(t, err)
	assert.Equal(t, 1, newAlerts)
	require.NoError(t, DB.Model(&AffiliateFraudAlert{}).Where("inviter_id = ? AND invitee_id = ?", 227, 228).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestSearchFraudAlertsFiltersByIPAndUserKeyword(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 231, Username: "needle-parent", DisplayName: "Needle Parent", Email: "needle-parent@example.com", AffCode: "needle-aff", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 232, Username: "needle-child", DisplayName: "Needle Child", Email: "needle-child@example.com", AffCode: "child-aff", Status: common.UserStatusEnabled, InviterId: 231}).Error)
	require.NoError(t, DB.Create(&User{Id: 233, Username: "other-parent", Email: "other-parent@example.com", AffCode: "other-aff", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 234, Username: "other-child", Email: "other-child@example.com", AffCode: "other-child-aff", Status: common.UserStatusEnabled, InviterId: 233}).Error)
	require.NoError(t, DB.Create(&AffiliateFraudAlert{InviterId: 231, InviteeId: 232, SharedIps: `["203.0.113.99"]`, SharedIpCount: 1, Status: FraudAlertStatusDetected, DetectedAt: now}).Error)
	require.NoError(t, DB.Create(&AffiliateFraudAlert{InviterId: 233, InviteeId: 234, SharedIps: `["198.51.100.20"]`, SharedIpCount: 1, Status: FraudAlertStatusDetected, DetectedAt: now - 1}).Error)

	items, total, err := SearchFraudAlerts(FraudAlertQuery{Status: FraudAlertStatusDetected, IP: "203.0.113", Keyword: "needle", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, 231, items[0].InviterId)
	assert.Equal(t, "needle-parent", items[0].InviterUsername)
	assert.Equal(t, "needle-child", items[0].InviteeUsername)

	items, total, err = SearchFraudAlerts(FraudAlertQuery{Keyword: "missing-user", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 0, total)
	assert.Empty(t, items)
}

func TestSearchFraudAlertGroupsAggregatesInviteesByInviter(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{Id: 235, Username: "group-parent", DisplayName: "Group Parent", Email: "group-parent@example.com", AffCode: "group-aff", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 236, Username: "group-child-a", Email: "child-a@example.com", AffCode: "group-child-a", Status: common.UserStatusEnabled, InviterId: 235}).Error)
	require.NoError(t, DB.Create(&User{Id: 237, Username: "group-child-b", Email: "child-b@example.com", AffCode: "group-child-b", Status: common.UserStatusEnabled, InviterId: 235}).Error)
	require.NoError(t, DB.Create(&AffiliateFraudAlert{InviterId: 235, InviteeId: 236, SharedIps: `["203.0.113.10"]`, SharedIpCount: 1, Status: FraudAlertStatusDetected, DetectedAt: now}).Error)
	require.NoError(t, DB.Create(&AffiliateFraudAlert{InviterId: 235, InviteeId: 237, SharedIps: `["203.0.113.10","203.0.113.11"]`, SharedIpCount: 2, Status: FraudAlertStatusDetected, DetectedAt: now - 1}).Error)

	groups, total, err := SearchFraudAlertGroups(FraudAlertQuery{Status: FraudAlertStatusDetected, Keyword: "group-parent", Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, groups, 1)
	assert.Equal(t, 235, groups[0].InviterId)
	assert.Equal(t, 2, groups[0].InviteeCount)
	assert.Equal(t, 2, groups[0].AlertCount)
	assert.ElementsMatch(t, []string{"203.0.113.10", "203.0.113.11"}, groups[0].SharedIps)
	require.Len(t, groups[0].Alerts, 2)
}

func TestAffiliateAgreementRequiredWithoutReviewApprovesConsent(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.AgreementEnabled = true
	affiliateSetting.AgreementText = "agree terms"
	affiliateSetting.ReviewEnabled = false

	require.NoError(t, DB.Create(&User{Id: 238, Username: "agreement-parent", AffCode: "aff238", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 239, Username: "agreement-child", AffCode: "aff239", Status: common.UserStatusEnabled, InviterId: 238}).Error)

	assert.False(t, AffiliateUserCanInvite(238, affiliateSetting))
	require.NoError(t, CreateAffiliateRewardsForPayment(239, AffiliateSourceTopUp, "agreement-order-before", 10000))
	var count int64
	require.NoError(t, DB.Model(&AffiliateRecord{}).Where("user_id = ?", 238).Count(&count).Error)
	assert.EqualValues(t, 0, count)

	require.NoError(t, CreateAffiliateApplication(238, affiliateSetting.AgreementText))
	assert.True(t, AffiliateUserCanInvite(238, affiliateSetting))
	var app AffiliateApplication
	require.NoError(t, DB.Where("user_id = ?", 238).First(&app).Error)
	assert.Equal(t, AffiliateAppStatusApproved, app.Status)

	affiliateSetting.FirstLevelEnabled = true
	affiliateSetting.FirstLevelRatio = 10
	require.NoError(t, CreateAffiliateRewardsForPayment(239, AffiliateSourceTopUp, "agreement-order-after", 10000))
	require.NoError(t, DB.Model(&AffiliateRecord{}).Where("user_id = ?", 238).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestRevokeAffiliateApplicationAllowsReapply(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	affiliateSetting := setting.GetAffiliateSetting()
	affiliateSetting.AgreementEnabled = true
	affiliateSetting.AgreementText = "review terms"
	affiliateSetting.ReviewEnabled = true

	require.NoError(t, DB.Create(&User{Id: 240, Username: "reapply-parent", AffCode: "aff240", Status: common.UserStatusEnabled}).Error)

	require.NoError(t, CreateAffiliateApplication(240, affiliateSetting.AgreementText))
	var app AffiliateApplication
	require.NoError(t, DB.Where("user_id = ?", 240).First(&app).Error)
	require.Equal(t, AffiliateAppStatusPending, app.Status)
	require.NoError(t, ApproveAffiliateApplication(app.Id, 1, "approved for test"))
	assert.True(t, AffiliateUserCanInvite(240, affiliateSetting))

	require.NoError(t, RevokeAffiliateApplication(app.Id))
	current, err := GetAffiliateApplicationByUserId(240)
	require.NoError(t, err)
	assert.Nil(t, current)
	assert.False(t, AffiliateUserCanInvite(240, affiliateSetting))

	require.NoError(t, CreateAffiliateApplication(240, affiliateSetting.AgreementText))
	var reapplied AffiliateApplication
	require.NoError(t, DB.Where("user_id = ?", 240).First(&reapplied).Error)
	assert.Equal(t, AffiliateAppStatusPending, reapplied.Status)
	assert.Equal(t, 240, reapplied.UserId)
}

func TestUnbindUserInviterClearsRelationshipAndDecrementsCount(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	require.NoError(t, DB.Create(&User{Id: 241, Username: "unbind-parent", AffCode: "aff241", Status: common.UserStatusEnabled, AffCount: 1}).Error)
	require.NoError(t, DB.Create(&User{Id: 242, Username: "unbind-child", AffCode: "aff242", Status: common.UserStatusEnabled, InviterId: 241}).Error)

	result, err := UnbindUserInviter(242, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Updated)
	assert.Equal(t, 241, result.PreviousInviterId)

	var invitee User
	require.NoError(t, DB.Select("inviter_id").Where("id = ?", 242).First(&invitee).Error)
	assert.Equal(t, 0, invitee.InviterId)
	var inviter User
	require.NoError(t, DB.Select("aff_count").Where("id = ?", 241).First(&inviter).Error)
	assert.Equal(t, 0, inviter.AffCount)

	result, err = UnbindUserInviter(242, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Updated)
	assert.Equal(t, 0, result.PreviousInviterId)
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
	assert.Equal(t, int(50*common.QuotaPerUnit), topupDetail.Quota)

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

func TestAffiliateRiskFreezeBlocksWithdrawTransferAndSettlesToRiskFrozen(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	insertAffiliateUser(t, 300, 0, 0)
	insertAffiliateUser(t, 301, 300, 0)
	require.NoError(t, DB.Create(&AffiliateBalance{UserId: 300, AvailableQuota: 2000, TotalQuota: 2000}).Error)

	result, err := ApplyAffiliateRiskAction(300, 1, AffiliateRiskApplyRequest{
		FreezeAssets: true,
		Reason:       "suspicious",
	})
	require.NoError(t, err)
	assert.Equal(t, 2000, result.FrozenQuota)

	balance := getAffiliateBalanceForTest(t, 300)
	assert.Equal(t, 0, balance.AvailableQuota)
	assert.Equal(t, 2000, balance.RiskFrozenQuota)

	_, err = CreateAffiliateWithdrawal(300, AffiliatePayoutMethodAlipay, 1000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "被冻结")
	err = TransferAffiliateQuotaToBalance(300, 1000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "被冻结")

	require.NoError(t, CreateAffiliateRewardsForPayment(301, AffiliateSourceTopUp, "risk-freeze-order", 10000))
	require.NoError(t, DB.Model(&AffiliateRecord{}).Where("user_id = ?", 300).Update("available_time", common.GetTimestamp()-1).Error)
	require.NoError(t, SettleMatureAffiliateRecords(300))

	balance = getAffiliateBalanceForTest(t, 300)
	assert.Equal(t, 0, balance.PendingQuota)
	assert.Equal(t, 0, balance.AvailableQuota)
	assert.Equal(t, 3000, balance.RiskFrozenQuota)

	remove, err := RemoveAffiliateRiskAction(300, 1, AffiliateRiskRemoveRequest{})
	require.NoError(t, err)
	assert.Equal(t, 3000, remove.UnfrozenQuota)
	balance = getAffiliateBalanceForTest(t, 300)
	assert.Equal(t, 3000, balance.AvailableQuota)
	assert.Equal(t, 0, balance.RiskFrozenQuota)
}

func TestAffiliateRiskBlockInviteCodeRejectsRegistrationAndManualBind(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	insertAffiliateUser(t, 310, 0, 0)
	insertAffiliateUser(t, 311, 0, 0)
	affCode := getAffiliateUserAffCodeForTest(t, 310)

	_, err := ApplyAffiliateRiskAction(310, 1, AffiliateRiskApplyRequest{
		BlockInviteCode: true,
		Reason:          "abuse",
	})
	require.NoError(t, err)

	_, err = GetUserIdByAffCode(affCode)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "失效")

	_, err = BindUserInviterByAffCode(311, "", affCode, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "废除")
}

func TestAffiliateRiskDetachInviteesCanBeAppliedAloneAndRestored(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	require.NoError(t, DB.Create(&User{Id: 320, Username: "risk-parent", AffCode: "risk320", Status: common.UserStatusEnabled, AffCount: 2}).Error)
	require.NoError(t, DB.Create(&User{Id: 321, Username: "risk-child-a", AffCode: "risk321", Status: common.UserStatusEnabled, InviterId: 320}).Error)
	require.NoError(t, DB.Create(&User{Id: 322, Username: "risk-child-b", AffCode: "risk322", Status: common.UserStatusEnabled, InviterId: 320}).Error)

	result, err := ApplyAffiliateRiskAction(320, 1, AffiliateRiskApplyRequest{
		DetachInvitees: true,
		Reason:         "manual detach",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.DetachedCount)

	var inviter User
	require.NoError(t, DB.Select("aff_count").Where("id = ?", 320).First(&inviter).Error)
	assert.Equal(t, 0, inviter.AffCount)
	var children []User
	require.NoError(t, DB.Select("id", "inviter_id").Where("id IN ?", []int{321, 322}).Order("id").Find(&children).Error)
	require.Len(t, children, 2)
	assert.Equal(t, 0, children[0].InviterId)
	assert.Equal(t, 0, children[1].InviterId)

	remove, err := RemoveAffiliateRiskAction(320, 1, AffiliateRiskRemoveRequest{RestoreDetachedInvitees: true})
	require.NoError(t, err)
	assert.Equal(t, 2, remove.RestoredInvitees)
	require.NoError(t, DB.Select("aff_count").Where("id = ?", 320).First(&inviter).Error)
	assert.Equal(t, 2, inviter.AffCount)
}

func TestAffiliateRiskClearAssetsConfiscatesBalancesWithdrawalsAndPendingRecords(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	require.NoError(t, DB.Create(&User{Id: 330, Username: "risk-clear", AffCode: "risk330", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&AffiliateBalance{
		UserId:          330,
		PendingQuota:    1000,
		AvailableQuota:  2000,
		FrozenQuota:     3000,
		RiskFrozenQuota: 4000,
		TotalQuota:      10000,
	}).Error)
	require.NoError(t, DB.Create(&AffiliateWithdrawal{UserId: 330, Quota: 3000, Method: AffiliatePayoutMethodAlipay, Status: AffiliateWithdrawalStatusPending}).Error)
	require.NoError(t, DB.Create(&AffiliateRecord{UserId: 330, InviteeId: 331, Level: 1, SourceType: AffiliateSourceTopUp, SourceId: "risk-clear-order", RewardQuota: 1000, Status: AffiliateRecordStatusPending}).Error)

	result, err := ApplyAffiliateRiskAction(330, 1, AffiliateRiskApplyRequest{
		ClearAssets: true,
		Reason:      "confirmed abuse",
	})
	require.NoError(t, err)
	assert.Equal(t, 10000, result.ClearedQuota)
	assert.Equal(t, 1, result.RejectedWithdrawals)

	balance := getAffiliateBalanceForTest(t, 330)
	assert.Equal(t, 0, balance.PendingQuota)
	assert.Equal(t, 0, balance.AvailableQuota)
	assert.Equal(t, 0, balance.FrozenQuota)
	assert.Equal(t, 0, balance.RiskFrozenQuota)
	assert.Equal(t, 10000, balance.ConfiscatedQuota)
	assert.Equal(t, 0, balance.TotalQuota)

	var withdrawal AffiliateWithdrawal
	require.NoError(t, DB.Where("user_id = ?", 330).First(&withdrawal).Error)
	assert.Equal(t, AffiliateWithdrawalStatusRejected, withdrawal.Status)

	var record AffiliateRecord
	require.NoError(t, DB.Where("user_id = ?", 330).First(&record).Error)
	assert.Equal(t, AffiliateRecordStatusConfiscated, record.Status)
}

func TestAdminAffiliateRecordsSearchByUserKeywordAndBalanceSnapshot(t *testing.T) {
	truncateTables(t)
	resetAffiliateSettingForTest(t)

	require.NoError(t, DB.Create(&User{Id: 340, Username: "search-inviter", Email: "inviter@example.com", AffCode: "risk340", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, DB.Create(&User{Id: 341, Username: "search-invitee", Email: "invitee@example.com", AffCode: "risk341", Status: common.UserStatusEnabled, InviterId: 340}).Error)
	require.NoError(t, CreateAffiliateRewardsForPayment(341, AffiliateSourceTopUp, "risk-search-order", 10000))

	items, total, err := GetAdminAffiliateRecordsWithDetails("", "", "invitee@example.com", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, 340, items[0].UserId)
	assert.Equal(t, 1000, items[0].BalanceAfterQuota)

	items, total, err = GetAdminAffiliateRecordsWithDetails("", "", "not-found-user", &common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 0, total)
	assert.Len(t, items, 0)
}
