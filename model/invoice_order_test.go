package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInvoiceOrderTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := DB
	originalLogDB := LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	originalRedis := common.RedisEnabled
	originalEnabled := InvoiceEnabled
	originalRules := InvoiceFeeRules
	originalTypes := InvoiceTypes
	originalKinds := InvoiceKinds
	originalPrice := operation_setting.Price
	originalQuotaPerUnit := common.QuotaPerUnit

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	InvoiceEnabled = true
	InvoiceFeeRules = `[{"min":0,"type":"percent","value":10}]`
	InvoiceTypes = `["personal","company"]`
	InvoiceKinds = `["normal","special"]`
	operation_setting.Price = 7
	common.QuotaPerUnit = 500000

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&User{},
		&Log{},
		&TopUp{},
		&SubscriptionOrder{},
		&InvoiceRecord{},
		&InvoiceOrderLink{},
	))

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		DB = originalDB
		LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		common.RedisEnabled = originalRedis
		InvoiceEnabled = originalEnabled
		InvoiceFeeRules = originalRules
		InvoiceTypes = originalTypes
		InvoiceKinds = originalKinds
		operation_setting.Price = originalPrice
		common.QuotaPerUnit = originalQuotaPerUnit
	})
	return db
}

func createInvoiceOrderTestUser(t *testing.T, db *gorm.DB, id int, quota int) {
	t.Helper()
	require.NoError(t, db.Create(&User{
		Id:       id,
		Username: fmt.Sprintf("invoice-user-%d", id),
		AffCode:  fmt.Sprintf("invoice-aff-%d", id),
		Quota:    quota,
	}).Error)
}

func recentInvoiceTopUp(userId int, tradeNo string, money float64) TopUp {
	now := common.GetTimestamp()
	return TopUp{
		UserId:          userId,
		TradeNo:         tradeNo,
		Money:           money,
		ActualMoney:     money,
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 60,
		CompleteTime:    now - 30,
	}
}

func validCombinedInvoiceRequest() InvoiceRequest {
	return InvoiceRequest{
		Required: true,
		Type:     InvoiceTypeCompany,
		Kind:     InvoiceKindSpecial,
		Title:    "测试公司",
		TaxNo:    "91310000TEST",
		Email:    "invoice@example.com",
	}
}

func TestGetRecentInvoiceOrdersMarksUsedOrdersAndDeduplicatesSubscriptionMirror(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createInvoiceOrderTestUser(t, db, 1001, 2_000_000)

	eligible := recentInvoiceTopUp(1001, "TOP-ELIGIBLE", 70)
	used := recentInvoiceTopUp(1001, "TOP-USED", 30)
	used.InvoiceRequired = true
	used.InvoiceStatus = InvoiceStatusIssued
	recorded := recentInvoiceTopUp(1001, "TOP-RECORDED", 20)
	old := recentInvoiceTopUp(1001, "TOP-OLD", 50)
	old.CreateTime = common.GetTimestamp() - invoiceOrderWindowSeconds - 100
	old.CompleteTime = old.CreateTime
	require.NoError(t, db.Create(&[]TopUp{eligible, used, recorded, old}).Error)
	require.NoError(t, db.Create(&InvoiceRecord{
		UserId:     1001,
		SourceType: InvoiceSourceTopUp,
		SourceId:   recorded.TradeNo,
		Status:     InvoiceStatusClosed,
	}).Error)

	now := common.GetTimestamp()
	subscription := SubscriptionOrder{
		UserId:          1001,
		TradeNo:         "SUB-RECENT",
		Money:           10,
		ActualMoney:     10,
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 50,
		CompleteTime:    now - 20,
	}
	require.NoError(t, db.Create(&subscription).Error)
	mirror := recentInvoiceTopUp(1001, subscription.TradeNo, 10)
	mirror.PaymentMethod = PaymentMethodStripe
	mirror.PaymentProvider = PaymentProviderStripe
	require.NoError(t, db.Create(&mirror).Error)

	orders, err := GetRecentInvoiceOrders(1001)
	require.NoError(t, err)
	require.Len(t, orders, 4)

	byId := make(map[string]InvoiceEligibleOrder, len(orders))
	for _, order := range orders {
		byId[order.SourceId] = order
	}
	assert.True(t, byId["TOP-ELIGIBLE"].InvoiceEligible)
	assert.Equal(t, 70.0, byId["TOP-ELIGIBLE"].InvoiceAmount)
	assert.False(t, byId["TOP-USED"].InvoiceEligible)
	assert.Equal(t, InvoiceStatusIssued, byId["TOP-USED"].InvoiceStatus)
	assert.True(t, byId["TOP-RECORDED"].Invoiced)
	assert.False(t, byId["TOP-RECORDED"].InvoiceEligible)
	assert.Equal(t, InvoiceStatusClosed, byId["TOP-RECORDED"].InvoiceStatus)
	assert.Equal(t, InvoiceSourceSubscription, byId["SUB-RECENT"].SourceType)
	assert.Equal(t, 70.0, byId["SUB-RECENT"].InvoiceAmount)
	_, containsOld := byId["TOP-OLD"]
	assert.False(t, containsOld)
}

func TestCreateCombinedInvoiceWithBalanceChargesFeeAndPreventsReuse(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createInvoiceOrderTestUser(t, db, 1002, 2_000_000)

	topUp := recentInvoiceTopUp(1002, "TOP-COMBINE", 70)
	require.NoError(t, db.Create(&topUp).Error)
	now := common.GetTimestamp()
	subscription := SubscriptionOrder{
		UserId:          1002,
		TradeNo:         "SUB-COMBINE",
		Money:           10,
		ActualMoney:     10,
		PaymentMethod:   PaymentMethodBalance,
		PaymentProvider: PaymentProviderBalance,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 100,
		CompleteTime:    now - 80,
	}
	require.NoError(t, db.Create(&subscription).Error)
	mirror := recentInvoiceTopUp(1002, subscription.TradeNo, 10)
	mirror.PaymentMethod = PaymentMethodBalance
	mirror.PaymentProvider = PaymentProviderBalance
	require.NoError(t, db.Create(&mirror).Error)

	references := []InvoiceOrderReference{
		{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo},
		{SourceType: InvoiceSourceSubscription, SourceId: subscription.TradeNo},
	}
	preview, err := PreviewInvoiceOrders(1002, references)
	require.NoError(t, err)
	assert.Equal(t, 140.0, preview.BaseAmount)
	assert.Equal(t, 14.0, preview.FeeAmount)
	assert.Equal(t, 154.0, preview.TotalAmount)
	assert.Equal(t, 1_000_000, preview.FeeQuota)

	record, err := CreateCombinedInvoiceWithBalance(1002, references, validCombinedInvoiceRequest())
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, InvoiceSourceCombined, record.SourceType)
	assert.Equal(t, InvoiceKindSpecial, record.InvoiceKind)
	assert.Equal(t, 140.0, record.BaseAmount)
	assert.Equal(t, 14.0, record.FeeAmount)
	assert.Equal(t, 154.0, record.TotalAmount)
	assert.Len(t, record.Orders, 2)

	var user User
	require.NoError(t, db.First(&user, 1002).Error)
	assert.Equal(t, 1_000_000, user.Quota)

	var links []InvoiceOrderLink
	require.NoError(t, db.Where("invoice_id = ?", record.Id).Find(&links).Error)
	assert.Len(t, links, 2)

	var savedTopUp TopUp
	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&savedTopUp).Error)
	assert.True(t, savedTopUp.InvoiceRequired)
	assert.Equal(t, InvoiceKindSpecial, savedTopUp.InvoiceKind)
	assert.Equal(t, InvoiceStatusPending, savedTopUp.InvoiceStatus)
	var savedSubscription SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", subscription.TradeNo).First(&savedSubscription).Error)
	assert.True(t, savedSubscription.InvoiceRequired)
	assert.Equal(t, InvoiceKindSpecial, savedSubscription.InvoiceKind)
	var savedMirror TopUp
	require.NoError(t, db.Where("trade_no = ?", subscription.TradeNo).First(&savedMirror).Error)
	assert.True(t, savedMirror.InvoiceRequired)
	assert.Equal(t, InvoiceKindSpecial, savedMirror.InvoiceKind)

	_, err = CreateCombinedInvoiceWithBalance(1002, references, validCombinedInvoiceRequest())
	require.ErrorContains(t, err, "已经申请过发票")
	require.NoError(t, db.First(&user, 1002).Error)
	assert.Equal(t, 1_000_000, user.Quota)
	var recordCount int64
	require.NoError(t, db.Model(&InvoiceRecord{}).Count(&recordCount).Error)
	assert.EqualValues(t, 1, recordCount)
}

func TestCreateCombinedInvoiceWithBalanceRollsBackWhenBalanceInsufficient(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createInvoiceOrderTestUser(t, db, 1003, 999_999)
	topUp := recentInvoiceTopUp(1003, "TOP-NO-BALANCE", 140)
	require.NoError(t, db.Create(&topUp).Error)

	references := []InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo}}
	_, err := CreateCombinedInvoiceWithBalance(1003, references, validCombinedInvoiceRequest())
	require.ErrorContains(t, err, "余额不足")

	var recordCount int64
	require.NoError(t, db.Model(&InvoiceRecord{}).Count(&recordCount).Error)
	assert.Zero(t, recordCount)
	var linkCount int64
	require.NoError(t, db.Model(&InvoiceOrderLink{}).Count(&linkCount).Error)
	assert.Zero(t, linkCount)
	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&topUp).Error)
	assert.False(t, topUp.InvoiceRequired)
}

func TestCreateCombinedInvoiceWithBalanceRejectsAnotherUsersOrder(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createInvoiceOrderTestUser(t, db, 1005, 2_000_000)
	createInvoiceOrderTestUser(t, db, 1006, 2_000_000)
	topUp := recentInvoiceTopUp(1005, "TOP-OTHER-USER", 70)
	require.NoError(t, db.Create(&topUp).Error)

	_, err := CreateCombinedInvoiceWithBalance(
		1006,
		[]InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo}},
		validCombinedInvoiceRequest(),
	)
	require.ErrorContains(t, err, "不存在或尚未支付")

	var user User
	require.NoError(t, db.First(&user, 1006).Error)
	assert.Equal(t, 2_000_000, user.Quota)
	var recordCount int64
	require.NoError(t, db.Model(&InvoiceRecord{}).Count(&recordCount).Error)
	assert.Zero(t, recordCount)
}

func TestFullDiscountOrderIsNotInvoiceable(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createInvoiceOrderTestUser(t, db, 1007, 2_000_000)
	topUp := recentInvoiceTopUp(1007, "TOP-FULL-DISCOUNT", 70)
	topUp.PromoCodeId = 1
	topUp.ActualMoney = 0
	require.NoError(t, db.Create(&topUp).Error)

	orders, err := GetRecentInvoiceOrders(1007)
	require.NoError(t, err)
	assert.Empty(t, orders)

	_, err = CreateCombinedInvoiceWithBalance(
		1007,
		[]InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo}},
		validCombinedInvoiceRequest(),
	)
	require.ErrorContains(t, err, "零金额订单不能申请发票")

	var user User
	require.NoError(t, db.First(&user, 1007).Error)
	assert.Equal(t, 2_000_000, user.Quota)
}

func TestUSDOrderIsDisabledWhenCNYExchangeRateIsInvalid(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createInvoiceOrderTestUser(t, db, 1008, 2_000_000)
	operation_setting.Price = 0
	topUp := recentInvoiceTopUp(1008, "TOP-INVALID-RATE", 10)
	topUp.PaymentMethod = PaymentMethodStripe
	topUp.PaymentProvider = PaymentProviderStripe
	require.NoError(t, db.Create(&topUp).Error)

	orders, err := GetRecentInvoiceOrders(1008)
	require.NoError(t, err)
	require.Len(t, orders, 1)
	assert.False(t, orders[0].Invoiced)
	assert.False(t, orders[0].InvoiceEligible)
	assert.Zero(t, orders[0].PaidAmount)

	_, err = PreviewInvoiceOrders(
		1008,
		[]InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo}},
	)
	require.ErrorContains(t, err, "人民币汇率配置")
}

func TestInvoiceOrderUsesPaymentTimeCNYSnapshotAfterRateChanges(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createInvoiceOrderTestUser(t, db, 1007, 3_000_000)
	now := common.GetTimestamp()

	topUp := TopUp{
		UserId:          1007,
		TradeNo:         "TOP-CNY-SNAPSHOT",
		Money:           10,
		ActualMoney:     10,
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 60,
		CompleteTime:    now - 30,
	}
	require.NoError(t, topUp.Insert())
	assert.Equal(t, 70.0, topUp.PaidAmountCNY)

	subscription := SubscriptionOrder{
		UserId:          1007,
		TradeNo:         "SUB-CNY-SNAPSHOT",
		Money:           10,
		ActualMoney:     10,
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 50,
		CompleteTime:    now - 20,
	}
	require.NoError(t, subscription.Insert())
	assert.Equal(t, 70.0, subscription.PaidAmountCNY)

	operation_setting.Price = 9
	orders, err := GetRecentInvoiceOrders(1007)
	require.NoError(t, err)
	require.Len(t, orders, 2)
	for _, order := range orders {
		assert.Equal(t, 70.0, order.PaidAmount)
	}

	preview, err := PreviewInvoiceOrders(1007, []InvoiceOrderReference{
		{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo},
		{SourceType: InvoiceSourceSubscription, SourceId: subscription.TradeNo},
	})
	require.NoError(t, err)
	assert.Equal(t, 140.0, preview.BaseAmount)
}

func TestCombinedInvoiceStatusSyncsAllSourceOrders(t *testing.T) {
	db := setupInvoiceOrderTestDB(t)
	createInvoiceOrderTestUser(t, db, 1004, 2_000_000)
	topUp := recentInvoiceTopUp(1004, "TOP-SYNC", 70)
	require.NoError(t, db.Create(&topUp).Error)

	record, err := CreateCombinedInvoiceWithBalance(
		1004,
		[]InvoiceOrderReference{{SourceType: InvoiceSourceTopUp, SourceId: topUp.TradeNo}},
		validCombinedInvoiceRequest(),
	)
	require.NoError(t, err)
	require.NoError(t, UpdateInvoiceRecord(record.Id, "https://example.com/invoice.pdf", InvoiceStatusIssued, "ok"))

	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&topUp).Error)
	assert.Equal(t, InvoiceStatusIssued, topUp.InvoiceStatus)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	records, total, err := GetUserInvoiceRecords(1004, pageInfo)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Len(t, records[0].Orders, 1)
}
