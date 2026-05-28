package model

import (
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

	items, err := GetAffiliateLeaderboard("day", 10)
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
}
