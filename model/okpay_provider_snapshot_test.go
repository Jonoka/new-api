package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkpayProviderSnapshotMigrationAndLookup(t *testing.T) {
	truncateTables(t)

	for _, modelValue := range []interface{}{&TopUp{}, &SubscriptionOrder{}} {
		assert.True(t, DB.Migrator().HasColumn(modelValue, "provider_order_id"))
		assert.True(t, DB.Migrator().HasColumn(modelValue, "provider_amount"))
		assert.True(t, DB.Migrator().HasColumn(modelValue, "provider_currency"))
	}

	topUp := &TopUp{
		UserId:           1701,
		Amount:           10,
		Money:            72,
		TradeNo:          "OKPAY_TOPUP_SNAPSHOT",
		PaymentMethod:    PaymentMethodOkpay,
		PaymentProvider:  PaymentProviderOkpay,
		ProviderAmount:   "10.00000000",
		ProviderCurrency: "USDT",
		Status:           common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	require.NoError(t, UpdateTopUpProviderSnapshot(
		topUp.TradeNo,
		PaymentProviderOkpay,
		"provider-topup-1701",
		"10.00000000",
		"USDT",
	))

	savedTopUp := GetTopUpByProviderOrderId(PaymentProviderOkpay, "provider-topup-1701")
	require.NotNil(t, savedTopUp)
	assert.Equal(t, topUp.TradeNo, savedTopUp.TradeNo)
	assert.Equal(t, "10.00000000", savedTopUp.ProviderAmount)
	assert.Equal(t, "USDT", savedTopUp.ProviderCurrency)
	assert.Nil(t, GetTopUpByProviderOrderId(PaymentProviderBepusdt, "provider-topup-1701"))

	order := &SubscriptionOrder{
		UserId:           1702,
		PlanId:           99,
		Money:            72,
		TradeNo:          "OKPAY_SUBSCRIPTION_SNAPSHOT",
		PaymentMethod:    PaymentMethodOkpay,
		PaymentProvider:  PaymentProviderOkpay,
		ProviderAmount:   "10.00000000",
		ProviderCurrency: "USDT",
		Status:           common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())
	require.NoError(t, UpdateSubscriptionOrderProviderSnapshot(
		order.TradeNo,
		PaymentProviderOkpay,
		"provider-subscription-1702",
		"10.00000000",
		"USDT",
	))

	savedOrder := GetSubscriptionOrderByProviderOrderId(PaymentProviderOkpay, "provider-subscription-1702")
	require.NotNil(t, savedOrder)
	assert.Equal(t, order.TradeNo, savedOrder.TradeNo)
	assert.Equal(t, "10.00000000", savedOrder.ProviderAmount)
	assert.Equal(t, "USDT", savedOrder.ProviderCurrency)
	assert.Nil(t, GetSubscriptionOrderByProviderOrderId(PaymentProviderBepusdt, "provider-subscription-1702"))
}

func TestOkpayProviderSnapshotUpdateRejectsIncompleteOrWrongProvider(t *testing.T) {
	truncateTables(t)

	topUp := &TopUp{
		UserId:          1710,
		Amount:          10,
		Money:           72,
		TradeNo:         "OKPAY_SNAPSHOT_GUARD",
		PaymentMethod:   PaymentMethodOkpay,
		PaymentProvider: PaymentProviderOkpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	require.Error(t, UpdateTopUpProviderSnapshot(topUp.TradeNo, PaymentProviderOkpay, "", "10.00000000", "USDT"))
	require.Error(t, UpdateTopUpProviderSnapshot(topUp.TradeNo, PaymentProviderBepusdt, "provider-wrong", "10.00000000", "USDT"))

	saved := GetTopUpByTradeNo(topUp.TradeNo)
	require.NotNil(t, saved)
	assert.Empty(t, saved.ProviderOrderId)
	assert.Empty(t, saved.ProviderAmount)
	assert.Empty(t, saved.ProviderCurrency)
}
