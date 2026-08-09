package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "payment_guard_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestCryptoTopUpLateSuccessUpdatesQuotaAndCacheOnce(t *testing.T) {
	testCases := []struct {
		name     string
		provider string
		complete func(string, string) error
	}{
		{name: "bepusdt", provider: PaymentProviderBepusdt, complete: RechargeBepusdt},
		{name: "okpay", provider: PaymentProviderOkpay, complete: RechargeOkpay},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			userID := 1200 + index
			tradeNo := testCase.name + "-late-success"
			insertUserForPaymentGuardTest(t, userID, 0)
			insertTopUpForPaymentGuardTest(t, tradeNo, userID, testCase.provider)
			require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", tradeNo).Update("status", common.TopUpStatusExpired).Error)

			originalCacheUpdater := increaseCryptoTopUpCache
			cacheCalls := 0
			var cacheDelta int64
			increaseCryptoTopUpCache = func(gotUserID int, delta int64) error {
				assert.Equal(t, userID, gotUserID)
				cacheCalls++
				cacheDelta += delta
				return nil
			}
			t.Cleanup(func() { increaseCryptoTopUpCache = originalCacheUpdater })

			require.NoError(t, testCase.complete(tradeNo, "127.0.0.1"))
			require.NoError(t, testCase.complete(tradeNo, "127.0.0.1"))

			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, tradeNo))
			expectedQuota := int(2 * common.QuotaPerUnit)
			assert.Equal(t, expectedQuota, getUserQuotaForPaymentGuardTest(t, userID))
			assert.Equal(t, 1, cacheCalls)
			assert.Equal(t, int64(expectedQuota), cacheDelta)
		})
	}
}

func TestCompleteSubscriptionOrderAllowsLateVerifiedSuccess(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 1210, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 1210)
	insertSubscriptionOrderForPaymentGuardTest(t, "subscription-late-success", 1210, plan.Id, PaymentProviderOkpay)
	require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("trade_no = ?", "subscription-late-success").Update("status", common.TopUpStatusExpired).Error)

	require.NoError(t, CompleteSubscriptionOrder("subscription-late-success", `{}`, PaymentProviderOkpay, PaymentMethodOkpay))

	order := GetSubscriptionOrderByTradeNo("subscription-late-success")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.EqualValues(t, 1, countUserSubscriptionsForPaymentGuardTest(t, 1210))
}

func TestUpdateOkpayProviderSnapshots(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 1220, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 1220)
	insertTopUpForPaymentGuardTest(t, "okpay-topup-snapshot", 1220, PaymentProviderOkpay)
	insertSubscriptionOrderForPaymentGuardTest(t, "okpay-subscription-snapshot", 1220, plan.Id, PaymentProviderOkpay)

	require.NoError(t, UpdateTopUpProviderSnapshot(
		"okpay-topup-snapshot",
		PaymentProviderOkpay,
		"provider-topup-1220",
		"10.12345678",
		"USDT",
	))
	require.NoError(t, UpdateSubscriptionOrderProviderSnapshot(
		"okpay-subscription-snapshot",
		PaymentProviderOkpay,
		"provider-subscription-1220",
		"20.00000000",
		"USDT",
	))

	topUp := GetTopUpByProviderOrderId(PaymentProviderOkpay, "provider-topup-1220")
	require.NotNil(t, topUp)
	assert.Equal(t, "10.12345678", topUp.ProviderAmount)
	assert.Equal(t, "USDT", topUp.ProviderCurrency)

	order := GetSubscriptionOrderByProviderOrderId(PaymentProviderOkpay, "provider-subscription-1220")
	require.NotNil(t, order)
	assert.Equal(t, "20.00000000", order.ProviderAmount)
	assert.Equal(t, "USDT", order.ProviderCurrency)
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	err := CompleteSubscriptionOrder("sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}
