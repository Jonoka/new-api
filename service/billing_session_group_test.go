package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func groupBillingContext(t *testing.T) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("token_quota", 1000)
	return c
}

func seedGroupBillingWallet(t *testing.T, quota int, tokenQuota int) (*model.User, *model.Token) {
	t.Helper()
	suffix := time.Now().UnixNano()
	user := &model.User{Username: fmt.Sprintf("bg%d", suffix%1_000_000_000_000_000), Password: "test-password", Quota: quota}
	require.NoError(t, model.DB.Create(user).Error)
	token := &model.Token{UserId: user.Id, Key: fmt.Sprintf("billing-group-token-%d", suffix), Name: "group", RemainQuota: tokenQuota}
	require.NoError(t, model.DB.Create(token).Error)
	return user, token
}

func groupBillingInfo(user *model.User, token *model.Token) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		UserId: user.Id, UserQuota: user.Quota, TokenId: token.Id, TokenKey: token.Key,
		OriginModelName: "billing-group-model", UsingGroup: "free", UserGroup: "default",
		RequestId: fmt.Sprintf("billing-group-request-%d", time.Now().UnixNano()),
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"},
		PriceData: types.PriceData{FreeModel: true, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0}},
	}
}

func readGroupBillingWallet(t *testing.T, userId int, tokenId int) (int, int, int) {
	t.Helper()
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, userId).Error)
	require.NoError(t, model.DB.First(&token, tokenId).Error)
	return user.Quota, token.RemainQuota, token.UsedQuota
}

func TestBillingSessionFreePaidFreeAndSameGroupReservation(t *testing.T) {
	user, token := seedGroupBillingWallet(t, 1000, 1000)
	c := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.ForcePreConsume = true

	require.Nil(t, ReconcileBillingReservation(c, 0, info))
	require.Nil(t, info.Billing)

	info.UsingGroup = "paid"
	info.PriceData = types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	require.NotNil(t, info.Billing)
	require.Equal(t, 300, info.FinalPreConsumedQuota)
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 700, userQuota)
	require.Equal(t, 700, tokenRemain)
	require.Equal(t, 300, tokenUsed)

	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	userQuota2, tokenRemain2, tokenUsed2 := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, userQuota, userQuota2)
	require.Equal(t, tokenRemain, tokenRemain2)
	require.Equal(t, tokenUsed, tokenUsed2)
	require.Nil(t, ReconcileBillingReservation(c, 500, info))
	userQuota, tokenRemain, tokenUsed = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 500, userQuota)
	require.Equal(t, 500, tokenRemain)
	require.Equal(t, 500, tokenUsed)
	require.Nil(t, ReconcileBillingReservation(c, 150, info))
	userQuota, tokenRemain, tokenUsed = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 850, userQuota)
	require.Equal(t, 850, tokenRemain)
	require.Equal(t, 150, tokenUsed)

	info.UsingGroup = "free"
	info.PriceData = types.PriceData{FreeModel: true, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0}}
	require.Nil(t, ReconcileBillingReservation(c, 0, info))
	require.NotNil(t, info.Billing)
	require.Zero(t, info.FinalPreConsumedQuota)
	userQuota, tokenRemain, tokenUsed = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 1000, userQuota)
	require.Equal(t, 1000, tokenRemain)
	require.Zero(t, tokenUsed)

	info.UsingGroup = "paid-2"
	info.PriceData = types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}}
	require.Nil(t, ReconcileBillingReservation(c, 500, info))
	userQuota, tokenRemain, tokenUsed = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 500, userQuota)
	require.Equal(t, 500, tokenRemain)
	require.Equal(t, 500, tokenUsed)
}

func TestBillingSessionInsufficientIncreaseLeavesPriorReservationAndRefundsAllFailed(t *testing.T) {
	user, token := seedGroupBillingWallet(t, 1000, 1000)
	c := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.ForcePreConsume = true
	info.PriceData.FreeModel = false
	require.Nil(t, ReconcileBillingReservation(c, 200, info))

	apiErr := ReconcileBillingReservation(c, 1200, info)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	require.Equal(t, 200, info.FinalPreConsumedQuota)
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 800, userQuota)
	require.Equal(t, 800, tokenRemain)
	require.Equal(t, 200, tokenUsed)

	info.Billing.Refund(c)
	require.Eventually(t, func() bool {
		userQuota, tokenRemain, tokenUsed = readGroupBillingWallet(t, user.Id, token.Id)
		return userQuota == 1000 && tokenRemain == 1000 && tokenUsed == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBillingSessionSettlementFailureRollsBackAndCanRetryOnce(t *testing.T) {
	user, token := seedGroupBillingWallet(t, 1000, 1000)
	c := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.ForcePreConsume = true
	info.PriceData.FreeModel = false
	require.Nil(t, ReconcileBillingReservation(c, 200, info))
	require.NoError(t, model.DB.Unscoped().Delete(&model.Token{}, token.Id).Error)

	err := SettleBilling(c, info, 400)
	require.Error(t, err)
	require.True(t, info.Billing.NeedsRefund())
	var gotUser model.User
	require.NoError(t, model.DB.First(&gotUser, user.Id).Error)
	require.Equal(t, 800, gotUser.Quota)

	restored := &model.Token{Id: token.Id, UserId: user.Id, Key: token.Key, Name: token.Name, RemainQuota: 800, UsedQuota: 200}
	require.NoError(t, model.DB.Create(restored).Error)
	require.NoError(t, SettleBilling(c, info, 400))
	require.NoError(t, SettleBilling(c, info, 400))
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 600, userQuota)
	require.Equal(t, 600, tokenRemain)
	require.Equal(t, 400, tokenUsed)
}

func TestBillingSessionRefundFailureKeepsRetryableReservation(t *testing.T) {
	user, token := seedGroupBillingWallet(t, 1000, 1000)
	c := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.ForcePreConsume = true
	info.PriceData.FreeModel = false
	require.Nil(t, ReconcileBillingReservation(c, 200, info))
	require.NoError(t, model.DB.Unscoped().Delete(&model.Token{}, token.Id).Error)

	info.Billing.Refund(c)
	require.True(t, info.Billing.NeedsRefund())
	var gotUser model.User
	require.NoError(t, model.DB.First(&gotUser, user.Id).Error)
	require.Equal(t, 800, gotUser.Quota)

	restored := &model.Token{Id: token.Id, UserId: user.Id, Key: token.Key, Name: token.Name, RemainQuota: 800, UsedQuota: 200}
	require.NoError(t, model.DB.Create(restored).Error)
	info.Billing.Refund(c)
	info.Billing.Refund(c)
	require.False(t, info.Billing.NeedsRefund())
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 1000, userQuota)
	require.Equal(t, 1000, tokenRemain)
	require.Zero(t, tokenUsed)
}

func TestBillingSessionSkipTokenWalletReservationStillRefunds(t *testing.T) {
	user, token := seedGroupBillingWallet(t, 500, 500)
	c := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.SkipTokenQuota = true
	info.ForcePreConsume = true
	info.PriceData.FreeModel = false
	require.Nil(t, ReconcileBillingReservation(c, 200, info))
	require.True(t, info.Billing.NeedsRefund())
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 300, userQuota)
	require.Equal(t, 500, tokenRemain)
	require.Zero(t, tokenUsed)

	info.Billing.Refund(c)
	require.Eventually(t, func() bool {
		userQuota, _, _ = readGroupBillingWallet(t, user.Id, token.Id)
		return userQuota == 500
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBillingSessionTrustIsNotInferredFromFreeAndForceDisablesIt(t *testing.T) {
	trustQuota := common.GetTrustQuota()
	user, token := seedGroupBillingWallet(t, trustQuota+1000, trustQuota+1000)
	c := groupBillingContext(t)
	c.Set("token_quota", trustQuota+1000)
	info := groupBillingInfo(user, token)

	require.Nil(t, ReconcileBillingReservation(c, 0, info))
	require.Nil(t, info.Billing)
	info.PriceData.FreeModel = false
	info.UsingGroup = "paid"
	require.Nil(t, ReconcileBillingReservation(c, 400, info))
	require.NotNil(t, info.Billing)
	require.Zero(t, info.FinalPreConsumedQuota)
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, trustQuota+1000, userQuota)
	require.Equal(t, trustQuota+1000, tokenRemain)
	require.Zero(t, tokenUsed)

	require.NoError(t, SettleBilling(c, info, 400))
	userQuota, tokenRemain, tokenUsed = readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, trustQuota+600, userQuota)
	require.Equal(t, trustQuota+600, tokenRemain)
	require.Equal(t, 400, tokenUsed)

	forcedUser, forcedToken := seedGroupBillingWallet(t, trustQuota+1000, trustQuota+1000)
	forced := groupBillingInfo(forcedUser, forcedToken)
	forced.PriceData.FreeModel = false
	forced.ForcePreConsume = true
	require.Nil(t, ReconcileBillingReservation(c, 400, forced))
	require.Equal(t, 400, forced.FinalPreConsumedQuota)
}

func TestBillingSessionSubscriptionReservationTracksLiveTarget(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	user, token := seedGroupBillingWallet(t, 1000, 1000)
	plan := &model.SubscriptionPlan{Title: "billing group", DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1000, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, model.DB.Create(plan).Error)
	sub := &model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active"}
	require.NoError(t, model.DB.Create(sub).Error)

	c := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.UserSetting.BillingPreference = "subscription_only"
	info.PriceData.FreeModel = false
	info.ForcePreConsume = true
	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	require.Equal(t, BillingSourceSubscription, info.BillingSource)
	require.Equal(t, sub.Id, info.SubscriptionId)
	require.EqualValues(t, 300, info.SubscriptionPreConsumed)
	require.Nil(t, ReconcileBillingReservation(c, 300, info))

	require.Nil(t, ReconcileBillingReservation(c, 0, info))
	require.Nil(t, ReconcileBillingReservation(c, 400, info))
	var gotSub model.UserSubscription
	var record model.SubscriptionPreConsumeRecord
	require.NoError(t, model.DB.First(&gotSub, sub.Id).Error)
	require.NoError(t, model.DB.Where("request_id = ?", info.RequestId).First(&record).Error)
	require.EqualValues(t, 400, gotSub.AmountUsed)
	require.EqualValues(t, 400, record.PreConsumed)
	require.EqualValues(t, 400, info.SubscriptionAmountUsedAfterPreConsume)

	info.Billing.Refund(c)
	require.Eventually(t, func() bool {
		if err := model.DB.First(&gotSub, sub.Id).Error; err != nil {
			return false
		}
		if err := model.DB.Where("request_id = ?", info.RequestId).First(&record).Error; err != nil {
			return false
		}
		var gotToken model.Token
		if err := model.DB.First(&gotToken, token.Id).Error; err != nil {
			return false
		}
		tokenRemain, tokenUsed := gotToken.RemainQuota, gotToken.UsedQuota
		return gotSub.AmountUsed == 0 && record.Status == "refunded" && tokenRemain == 1000 && tokenUsed == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBillingSessionWalletFirstFallsBackWithoutDuplicateTokenPreconsume(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	user, token := seedGroupBillingWallet(t, 100, 1000)
	plan := &model.SubscriptionPlan{Title: fmt.Sprintf("wallet fallback %d", time.Now().UnixNano()), DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1000, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, model.DB.Create(plan).Error)
	sub := &model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active"}
	require.NoError(t, model.DB.Create(sub).Error)

	c := groupBillingContext(t)
	info := groupBillingInfo(user, token)
	info.UserSetting.BillingPreference = "wallet_first"
	info.PriceData.FreeModel = false
	info.ForcePreConsume = true
	require.Nil(t, ReconcileBillingReservation(c, 300, info))
	require.Equal(t, BillingSourceSubscription, info.BillingSource)
	userQuota, tokenRemain, tokenUsed := readGroupBillingWallet(t, user.Id, token.Id)
	require.Equal(t, 100, userQuota)
	require.Equal(t, 700, tokenRemain)
	require.Equal(t, 300, tokenUsed)
	var gotSub model.UserSubscription
	require.NoError(t, model.DB.First(&gotSub, sub.Id).Error)
	require.EqualValues(t, 300, gotSub.AmountUsed)
}

func TestFinalGroupTieredSnapshotAndLogFactsAgree(t *testing.T) {
	requestId := fmt.Sprintf("final-group-log-%d", time.Now().UnixNano())
	c := groupBillingContext(t)
	c.Set(common.RequestIdKey, requestId)
	c.Set("token_name", "final-token")
	info := &relaycommon.RelayInfo{
		OriginModelName: "final-model", UsingGroup: "paid-final", UserId: 0,
		PriceData: types.PriceData{
			ModelRatio: 1.5, Quota: 321,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2, GroupSpecialRatio: 2, HasSpecialRatio: true},
		},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr", ModelName: "final-model", ExprString: `tier("final-tier", p * 2)`, Group: "paid-final", GroupRatio: 2,
			GroupSpecialRatio: 2, HasGroupSpecialRatio: true,
		},
	}
	other := map[string]interface{}{}
	InjectTieredBillingInfo(other, info, &billingexpr.TieredResult{MatchedTier: "final-tier"})
	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "final-tier", other["matched_tier"])
	require.Equal(t, "paid-final", info.TieredBillingSnapshot.Group)
	require.Equal(t, info.PriceData.GroupRatioInfo.GroupRatio, info.TieredBillingSnapshot.GroupRatio)

	LogTaskConsumption(c, info)
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestId).First(&log).Error)
	require.Equal(t, "final-model", log.ModelName)
	require.Equal(t, "paid-final", log.Group)
	require.Equal(t, 321, log.Quota)
	var storedOther map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &storedOther))
	require.Equal(t, float64(2), storedOther["group_ratio"])
}
