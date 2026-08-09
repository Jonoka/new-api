package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createExternalInvoicePaymentForTest(t *testing.T, userId int, tradeNo string, provider string, method string) (*InvoiceRecord, *TopUp) {
	t.Helper()
	db := DB
	createInvoiceOrderTestUser(t, db, userId, 2_000_000)
	topUp := recentInvoiceTopUp(userId, tradeNo, 70)
	require.NoError(t, db.Create(&topUp).Error)
	record, err := CreateCombinedInvoiceExternalPayment(
		userId,
		[]InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: tradeNo}},
		validCombinedInvoiceRequest(),
		InvoiceExternalPaymentOptions{
			PaymentProvider:    provider,
			PaymentMethod:      method,
			ProviderMerchantId: "merchant-test",
			RequestIP:          "127.0.0.1",
		},
	)
	require.NoError(t, err)
	return record, &topUp
}

func TestCreateCombinedInvoiceExternalPaymentDoesNotChargeBalanceAndFreezesOrders(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	record, topUp := createExternalInvoicePaymentForTest(t, 1301, "TOP-EXTERNAL-CREATE", PaymentProviderEpay, "alipay")

	assert.Equal(t, InvoiceStatusPaymentPending, record.Status)
	assert.Equal(t, InvoicePaymentStatusPending, record.PaymentStatus)
	assert.Equal(t, int64(700), record.PaymentAmountMinor)
	assert.Equal(t, "7.00", record.ProviderAmount)
	assert.Equal(t, "CNY", record.ProviderCurrency)
	assert.Len(t, record.Orders, 1)
	var eventCount int64
	require.NoError(t, db.Model(&NotificationEvent{}).Where("event_type = ?", NotificationEventTypeInvoicePending).Count(&eventCount).Error)
	assert.Zero(t, eventCount, "服务费尚未支付时不得创建待开票通知")

	var user User
	require.NoError(t, db.First(&user, 1301).Error)
	assert.Equal(t, 2_000_000, user.Quota)
	var topUpCount int64
	require.NoError(t, db.Model(&TopUp{}).Count(&topUpCount).Error)
	assert.EqualValues(t, 1, topUpCount, "外部开票支付不得创建充值单")

	require.NoError(t, db.Where("id = ?", topUp.Id).First(topUp).Error)
	assert.True(t, topUp.InvoiceRequired)
	assert.Equal(t, InvoiceStatusPaymentPending, topUp.InvoiceStatus)
	_, err := CreateCombinedInvoiceExternalPayment(
		1301,
		[]InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo}},
		validCombinedInvoiceRequest(),
		InvoiceExternalPaymentOptions{PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay"},
	)
	require.ErrorContains(t, err, "已经申请过发票")
}

func TestCreateCombinedInvoiceExternalPaymentEnqueuesZeroFeeInvoiceImmediately(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createNotificationFixture(t)
	InvoiceFeeRules = `[{"min":0,"type":"fixed","value":0}]`
	createInvoiceOrderTestUser(t, db, 1307, 2_000_000)
	topUp := recentInvoiceTopUp(1307, "TOP-EXTERNAL-ZERO-FEE", 70)
	require.NoError(t, db.Create(&topUp).Error)

	record, err := CreateCombinedInvoiceExternalPayment(
		1307,
		[]InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo}},
		validCombinedInvoiceRequest(),
		InvoiceExternalPaymentOptions{
			PaymentProvider: PaymentProviderEpay,
			PaymentMethod:   "alipay",
			RequestIP:       "127.0.0.1",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusPending, record.Status)
	assert.Equal(t, InvoicePaymentStatusSuccess, record.PaymentStatus)
	assert.Zero(t, record.PaymentAmountMinor)

	var eventCount int64
	require.NoError(t, db.Model(&NotificationEvent{}).Where("event_type = ? AND event_key = ?", NotificationEventTypeInvoicePending, fmt.Sprintf("invoice:%d", record.Id)).Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount)
}

func TestCompleteInvoiceExternalPaymentTransitionsAndIsIdempotent(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createNotificationFixture(t)
	record, topUp := createExternalInvoicePaymentForTest(t, 1302, "TOP-EXTERNAL-COMPLETE", PaymentProviderEpay, "alipay")
	require.NoError(t, UpdateInvoiceExternalPaymentStatus(record.SourceId, PaymentProviderEpay, InvoicePaymentStatusExpired, "expired"))

	callback := InvoicePaymentCallback{
		PaymentProvider:    PaymentProviderEpay,
		PaymentMethod:      "alipay",
		ProviderOrderId:    "EPAY-ORDER-1302",
		ProviderMerchantId: "merchant-test",
		ProviderAmount:     "7.00",
		ProviderCurrency:   "CNY",
		ProviderPayload:    `{"trade_status":"TRADE_SUCCESS"}`,
	}
	completed, completedNow, err := CompleteInvoiceExternalPayment(record.SourceId, callback)
	require.NoError(t, err)
	assert.True(t, completedNow)
	assert.Equal(t, InvoicePaymentStatusSuccess, completed.PaymentStatus)
	assert.Equal(t, InvoiceStatusPending, completed.Status)
	assert.NotZero(t, completed.PaidTime)
	var eventCount int64
	require.NoError(t, db.Model(&NotificationEvent{}).Where("event_type = ?", NotificationEventTypeInvoicePending).Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount)

	completed, completedNow, err = CompleteInvoiceExternalPayment(record.SourceId, callback)
	require.NoError(t, err)
	assert.False(t, completedNow)
	assert.Equal(t, InvoicePaymentStatusSuccess, completed.PaymentStatus)
	require.NoError(t, db.Model(&NotificationEvent{}).Where("event_type = ?", NotificationEventTypeInvoicePending).Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount, "重复支付回调不得重复入队")

	require.NoError(t, db.Where("id = ?", topUp.Id).First(topUp).Error)
	assert.Equal(t, InvoiceStatusPending, topUp.InvoiceStatus)
	var user User
	require.NoError(t, db.First(&user, 1302).Error)
	assert.Equal(t, 2_000_000, user.Quota)
}

func TestCompleteInvoiceExternalPaymentRejectsGatewayAndAmountMismatch(t *testing.T) {
	setupInvoiceOrderTestDB(t)
	record, _ := createExternalInvoicePaymentForTest(t, 1303, "TOP-EXTERNAL-MISMATCH", PaymentProviderEpay, "alipay")

	_, _, err := CompleteInvoiceExternalPayment(record.SourceId, InvoicePaymentCallback{
		PaymentProvider: PaymentProviderBepusdt, PaymentMethod: PaymentMethodBepusdt,
		ProviderOrderId: "BEP-WRONG", ProviderAmount: "7.00", ProviderCurrency: "CNY",
	})
	require.ErrorIs(t, err, ErrInvoicePaymentSnapshotMismatch)

	_, _, err = CompleteInvoiceExternalPayment(record.SourceId, InvoicePaymentCallback{
		PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay",
		ProviderOrderId: "EPAY-WRONG-AMOUNT", ProviderMerchantId: "merchant-test",
		ProviderAmount: "7.01", ProviderCurrency: "CNY",
	})
	require.ErrorIs(t, err, ErrInvoicePaymentSnapshotMismatch)

	saved, err := GetInvoicePaymentByTradeNo(record.SourceId)
	require.NoError(t, err)
	assert.Equal(t, InvoicePaymentStatusPending, saved.PaymentStatus)
	assert.Equal(t, InvoiceStatusPaymentPending, saved.Status)
}

func TestCompleteInvoiceExternalPaymentRejectsEmptyOkpaySnapshot(t *testing.T) {
	setupInvoiceOrderTestDB(t)
	record, _ := createExternalInvoicePaymentForTest(t, 1304, "TOP-OKPAY-EMPTY-SNAPSHOT", PaymentProviderOkpay, PaymentMethodOkpay)

	_, _, err := CompleteInvoiceExternalPayment(record.SourceId, InvoicePaymentCallback{
		PaymentProvider: PaymentProviderOkpay, PaymentMethod: PaymentMethodOkpay,
		ProviderOrderId: "OKPAY-ORDER", ProviderMerchantId: "merchant-test",
		ProviderAmount: "1.00000000", ProviderCurrency: "USDT",
	})
	require.ErrorIs(t, err, ErrInvoicePaymentSnapshotMismatch)

	var saved InvoiceRecord
	require.NoError(t, DB.First(&saved, record.Id).Error)
	assert.Equal(t, InvoicePaymentStatusPending, saved.PaymentStatus)
	assert.Equal(t, int64(700), saved.PaymentAmountMinor)
}

func TestAdminCannotIssuePaymentPendingInvoice(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	record, topUp := createExternalInvoicePaymentForTest(t, 1305, "TOP-PENDING-ADMIN", PaymentProviderEpay, "alipay")
	require.ErrorContains(t, UpdateInvoiceRecord(record.Id, "", InvoiceStatusPending, ""), "尚未支付")
	require.ErrorContains(t, UpdateInvoiceRecord(record.Id, "https://example.com/invoice.pdf", InvoiceStatusIssued, ""), "尚未支付")
	require.NoError(t, UpdateInvoiceRecord(record.Id, "", InvoiceStatusClosed, "取消未支付申请"))
	require.NoError(t, db.First(topUp, topUp.Id).Error)
	assert.False(t, topUp.InvoiceRequired)
	assert.Empty(t, topUp.InvoiceStatus)
	var linkCount int64
	require.NoError(t, db.Model(&InvoiceOrderLink{}).Where("invoice_id = ?", record.Id).Count(&linkCount).Error)
	assert.Zero(t, linkCount)
}

func TestUserCanCancelPendingInvoicePaymentAndLateCallbackIsRejected(t *testing.T) {
	setupInvoiceOrderTestDB(t)
	record, topUp := createExternalInvoicePaymentForTest(t, 1306, "TOP-PENDING-CANCEL", PaymentProviderEpay, "alipay")
	canceled, err := CancelInvoiceExternalPayment(1306, record.SourceId)
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusClosed, canceled.Status)
	assert.Equal(t, InvoicePaymentStatusCanceled, canceled.PaymentStatus)

	_, _, err = CompleteInvoiceExternalPayment(record.SourceId, InvoicePaymentCallback{
		PaymentProvider: PaymentProviderEpay, PaymentMethod: "alipay",
		ProviderOrderId: "EPAY-LATE", ProviderMerchantId: "merchant-test",
		ProviderAmount: "7.00", ProviderCurrency: "CNY",
	})
	require.ErrorIs(t, err, ErrInvoicePaymentStatusInvalid)

	_, err = CreateCombinedInvoiceWithBalance(
		1306,
		[]InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo}},
		validCombinedInvoiceRequest(),
	)
	require.NoError(t, err)
}
