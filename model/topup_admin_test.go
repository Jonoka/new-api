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

func setupAdminTopUpTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "admin-topup.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}))

	oldDB := DB
	oldSQLite := common.UsingSQLite
	oldMySQL := common.UsingMySQL
	oldPostgreSQL := common.UsingPostgreSQL
	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		DB = oldDB
		common.UsingSQLite = oldSQLite
		common.UsingMySQL = oldMySQL
		common.UsingPostgreSQL = oldPostgreSQL
	})

	return db
}

func TestAdminTopUpsIncludeUsername(t *testing.T) {
	db := setupAdminTopUpTestDB(t)
	require.NoError(t, db.Create(&User{Id: 3101, Username: "billing-alice", AffCode: "billing-aff-3101"}).Error)
	require.NoError(t, db.Create(&TopUp{
		UserId:     3101,
		TradeNo:    "BILLING-USERNAME-ORDER",
		CreateTime: 100,
		Status:     common.TopUpStatusSuccess,
	}).Error)

	items, total, err := GetAdminTopUps(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "billing-alice", items[0].Username)
	assert.Equal(t, 3101, items[0].UserId)
}

func TestSearchAdminTopUpsByOrderUsernameAndUserID(t *testing.T) {
	db := setupAdminTopUpTestDB(t)
	require.NoError(t, db.Create(&[]User{
		{Id: 3201, Username: "billing-search-user", AffCode: "billing-aff-3201"},
		{Id: 3202, Username: "billing-other-user", AffCode: "billing-aff-3202"},
	}).Error)
	require.NoError(t, db.Create(&[]TopUp{
		{UserId: 3201, TradeNo: "ORDER-BY-TRADE", CreateTime: 101},
		{UserId: 3202, TradeNo: "ORDER-OTHER", CreateTime: 102},
	}).Error)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	assertSearchResult := func(keyword string, expectedTradeNo string) {
		t.Helper()
		items, total, err := SearchAdminTopUps(keyword, pageInfo)
		require.NoError(t, err)
		assert.EqualValues(t, 1, total)
		require.Len(t, items, 1)
		assert.Equal(t, expectedTradeNo, items[0].TradeNo)
	}

	assertSearchResult("ORDER-BY-TRADE", "ORDER-BY-TRADE")
	assertSearchResult("billing-search-user", "ORDER-BY-TRADE")
	assertSearchResult("3201", "ORDER-BY-TRADE")
}
