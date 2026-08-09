package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPaymentRequestIPTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "payment-request-ip.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}, &SubscriptionOrder{}))

	oldDB := DB
	oldLogDB := LOG_DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgreSQL := common.UsingPostgreSQL
	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	t.Cleanup(func() {
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
		DB = oldDB
		LOG_DB = oldLogDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgreSQL
	})

	return db
}

func TestPaymentOrdersPersistRequestIP(t *testing.T) {
	db := setupPaymentRequestIPTestDB(t)

	topUp := TopUp{TradeNo: "request-ip-topup", RequestIP: "203.0.113.10"}
	subscriptionOrder := SubscriptionOrder{TradeNo: "request-ip-subscription", RequestIP: "2001:db8::10"}
	require.NoError(t, db.Create(&topUp).Error)
	require.NoError(t, db.Create(&subscriptionOrder).Error)

	var savedTopUp TopUp
	var savedSubscriptionOrder SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&savedTopUp).Error)
	require.NoError(t, db.Where("trade_no = ?", subscriptionOrder.TradeNo).First(&savedSubscriptionOrder).Error)
	assert.Equal(t, "203.0.113.10", savedTopUp.RequestIP)
	assert.Equal(t, "2001:db8::10", savedSubscriptionOrder.RequestIP)
}

func TestRecordTopupLogSeparatesRequestAndCallbackIP(t *testing.T) {
	db := setupPaymentRequestIPTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1001, Username: "payment-ip-user"}).Error)

	RecordTopupLog(
		1001,
		"充值成功",
		"203.0.113.10",
		"alipay",
		"epay",
		"152.53.54.47",
	)

	var log Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", 1001, LogTypeTopup).First(&log).Error)
	assert.Equal(t, "203.0.113.10", log.Ip)

	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "203.0.113.10", adminInfo["request_ip"])
	assert.Equal(t, "152.53.54.47", adminInfo["callback_ip"])
}

func TestRecordTopupLogDoesNotUseCallbackIPForLegacyOrder(t *testing.T) {
	db := setupPaymentRequestIPTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1002, Username: "legacy-payment-ip-user"}).Error)

	RecordTopupLog(
		1002,
		"充值成功",
		"",
		"alipay",
		"epay",
		"152.53.54.47",
	)

	var log Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", 1002, LogTypeTopup).First(&log).Error)
	assert.Empty(t, log.Ip)

	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, adminInfo["request_ip"])
	assert.Equal(t, "152.53.54.47", adminInfo["callback_ip"])
}

func TestRecordTopupOrderLogIncludesOrderAndBalanceSnapshot(t *testing.T) {
	db := setupPaymentRequestIPTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1003, Username: "topup-audit-user", Quota: 1200}).Error)

	topUp := &TopUp{
		UserId:          1003,
		TradeNo:         "TOPUP-AUDIT-ORDER",
		PaymentMethod:   "wxpay",
		PaymentProvider: PaymentProviderEpay,
		RequestIP:       "203.0.113.30",
		PaidAmountCNY:   19.98,
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		before, after, err := creditTopUpUserQuotaTx(tx, topUp.UserId, 5000, nil)
		if err != nil {
			return err
		}
		topUp.BalanceBefore = before
		topUp.BalanceAfter = after
		topUp.CreditedQuota = after - before
		return nil
	}))

	RecordTopupOrderLog(topUp, "充值成功", PaymentProviderEpay, "152.53.54.47")

	var log Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", topUp.UserId, LogTypeTopup).First(&log).Error)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, topUp.TradeNo, adminInfo["trade_no"])
	assert.EqualValues(t, 1200, adminInfo["balance_before"])
	assert.EqualValues(t, 5000, adminInfo["credited_quota"])
	assert.EqualValues(t, 6200, adminInfo["balance_after"])
	assert.InDelta(t, 19.98, adminInfo["paid_amount_cny"], 0.000001)
}
