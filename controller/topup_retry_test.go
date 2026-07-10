package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedRetryTopUpUser(t *testing.T, id int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{
		Id:       id,
		Username: common.GetRandomString(8),
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  common.GetRandomString(8),
	}).Error)
}

func seedRetryTopUpOrder(t *testing.T, topUp *model.TopUp) {
	t.Helper()
	if topUp.CreateTime == 0 {
		topUp.CreateTime = common.GetTimestamp()
	}
	require.NoError(t, topUp.Insert())
}

func TestEnsureRetryableTopUpForUserRejectsOtherUserOrder(t *testing.T) {
	setupSubscriptionPaymentControllerTestDB(t)
	seedRetryTopUpUser(t, 901)
	seedRetryTopUpUser(t, 902)
	seedRetryTopUpOrder(t, &model.TopUp{
		UserId:          902,
		Amount:          10,
		Money:           10.30,
		TradeNo:         "retry-other-user",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	})

	ctx, recorder := newSubscriptionPaymentContext(t, RetryTopUpPaymentRequest{
		TradeNo: "retry-other-user",
	}, 901)
	topUp, ok := ensureRetryableTopUpForUser(ctx, "retry-other-user")

	assert.False(t, ok)
	assert.Nil(t, topUp)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "充值订单不存在")
}

func TestEnsureRetryableTopUpForUserRejectsNonPendingOrder(t *testing.T) {
	setupSubscriptionPaymentControllerTestDB(t)
	seedRetryTopUpUser(t, 901)
	seedRetryTopUpOrder(t, &model.TopUp{
		UserId:          901,
		Amount:          10,
		Money:           10.30,
		TradeNo:         "retry-success-order",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
	})

	ctx, recorder := newSubscriptionPaymentContext(t, RetryTopUpPaymentRequest{
		TradeNo: "retry-success-order",
	}, 901)
	topUp, ok := ensureRetryableTopUpForUser(ctx, "retry-success-order")

	assert.False(t, ok)
	assert.Nil(t, topUp)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "订单状态不是待支付")
}

func TestRetryStripeGatewayPayMoneyUsesActualMoneyAndInvoiceFee(t *testing.T) {
	originalPrice := operation_setting.Price
	operation_setting.Price = 1.03
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
	})

	topUp := &model.TopUp{
		Money:            10,
		ActualMoney:      4,
		InvoiceRequired:  true,
		InvoiceFeeAmount: 10.3,
	}

	assert.InDelta(t, 14, retryStripeGatewayPayMoney(topUp), 0.000001)
}

func TestRetryTopUpPaymentDoesNotCreateNewLocalOrder(t *testing.T) {
	setupSubscriptionPaymentControllerTestDB(t)
	seedRetryTopUpUser(t, 901)
	seedRetryTopUpOrder(t, &model.TopUp{
		UserId:          901,
		Amount:          10,
		Money:           10.30,
		TradeNo:         "retry-creem-order",
		PaymentMethod:   model.PaymentMethodCreem,
		PaymentProvider: model.PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
	})

	ctx, recorder := newSubscriptionPaymentContext(t, RetryTopUpPaymentRequest{
		TradeNo: "retry-creem-order",
	}, 901)
	RetryTopUpPayment(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var count int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Contains(t, recorder.Body.String(), "暂不支持重新支付")
}

func TestAdminCompleteTopUpAllowsExpiredOrder(t *testing.T) {
	setupSubscriptionPaymentControllerTestDB(t)
	seedRetryTopUpUser(t, 901)
	seedRetryTopUpOrder(t, &model.TopUp{
		UserId:          901,
		Amount:          10,
		Money:           10.30,
		TradeNo:         "admin-complete-expired-order",
		PaymentMethod:   model.PaymentMethodBepusdt,
		PaymentProvider: model.PaymentProviderBepusdt,
		Status:          common.TopUpStatusExpired,
	})

	ctx, recorder := newSubscriptionPaymentContext(t, AdminCompleteTopupRequest{
		TradeNo: "admin-complete-expired-order",
	}, 1)
	AdminCompleteTopUp(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)

	var savedTopUp model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", "admin-complete-expired-order").First(&savedTopUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, savedTopUp.Status)
	assert.Greater(t, savedTopUp.CompleteTime, int64(0))

	var savedUser model.User
	require.NoError(t, model.DB.First(&savedUser, 901).Error)
	assert.Equal(t, int(float64(10)*common.QuotaPerUnit), savedUser.Quota)
}
