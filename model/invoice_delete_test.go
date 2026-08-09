package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeleteInvoiceRecordsSoftDeletesAndPreservesIssuedSourceState(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	topUp := TopUp{
		UserId:          2101,
		TradeNo:         "TOP-INVOICE-DELETE-ISSUED",
		Status:          common.TopUpStatusSuccess,
		InvoiceRequired: true,
		InvoiceStatus:   InvoiceStatusIssued,
	}
	require.NoError(t, db.Create(&topUp).Error)
	record := InvoiceRecord{
		UserId:        topUp.UserId,
		SourceType:    InvoiceSourceTopUp,
		SourceId:      topUp.TradeNo,
		PaymentStatus: InvoicePaymentStatusSuccess,
		Status:        InvoiceStatusIssued,
	}
	require.NoError(t, db.Create(&record).Error)

	deleted, err := DeleteInvoiceRecords([]int{record.Id, record.Id, record.Id + 999})
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	var visible InvoiceRecord
	require.ErrorIs(t, db.First(&visible, record.Id).Error, gorm.ErrRecordNotFound)
	var archived InvoiceRecord
	require.NoError(t, db.Unscoped().First(&archived, record.Id).Error)
	assert.True(t, archived.DeletedAt.Valid)

	var savedTopUp TopUp
	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&savedTopUp).Error)
	assert.True(t, savedTopUp.InvoiceRequired)
	assert.Equal(t, InvoiceStatusIssued, savedTopUp.InvoiceStatus)
}

func TestDeleteInvoiceRecordsCancelsPendingExternalPaymentBeforeSoftDelete(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	topUp := TopUp{
		UserId:          2102,
		TradeNo:         "TOP-INVOICE-DELETE-PAYMENT-PENDING",
		Status:          common.TopUpStatusSuccess,
		InvoiceRequired: true,
		InvoiceStatus:   InvoiceStatusPaymentPending,
	}
	require.NoError(t, db.Create(&topUp).Error)
	record := InvoiceRecord{
		UserId:        topUp.UserId,
		SourceType:    InvoiceSourceCombined,
		SourceId:      "INV-PAYMENT-PENDING-DELETE",
		PaymentStatus: InvoicePaymentStatusPending,
		Status:        InvoiceStatusPaymentPending,
	}
	require.NoError(t, db.Create(&record).Error)
	require.NoError(t, db.Create(&InvoiceOrderLink{
		InvoiceId:  record.Id,
		UserId:     topUp.UserId,
		SourceType: InvoiceSourceTopUp,
		SourceId:   topUp.TradeNo,
	}).Error)

	deleted, err := DeleteInvoiceRecords([]int{record.Id})
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	var savedTopUp TopUp
	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&savedTopUp).Error)
	assert.False(t, savedTopUp.InvoiceRequired)
	assert.Empty(t, savedTopUp.InvoiceStatus)

	var linkCount int64
	require.NoError(t, db.Model(&InvoiceOrderLink{}).Where("invoice_id = ?", record.Id).Count(&linkCount).Error)
	assert.Zero(t, linkCount)

	var archived InvoiceRecord
	require.NoError(t, db.Unscoped().First(&archived, record.Id).Error)
	assert.True(t, archived.DeletedAt.Valid)
	assert.Equal(t, InvoiceStatusClosed, archived.Status)
	assert.Equal(t, InvoicePaymentStatusCanceled, archived.PaymentStatus)
}

func TestDeleteInvoiceRecordsValidatesBatch(t *testing.T) {
	setupInvoiceOrderTestDB(t)

	_, err := DeleteInvoiceRecords(nil)
	require.ErrorContains(t, err, "至少选择")
	_, err = DeleteInvoiceRecords([]int{0})
	require.ErrorContains(t, err, "ID 无效")
	_, err = DeleteInvoiceRecords(make([]int, maxAdminInvoiceDeleteBatch+1))
	require.ErrorContains(t, err, "最多删除")
}
