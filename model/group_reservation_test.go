package model

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type groupReservationDatabase struct {
	name     string
	open     func() (*gorm.DB, error)
	sqlite   bool
	mysql    bool
	postgres bool
}

func groupReservationDatabases(t *testing.T) []groupReservationDatabase {
	t.Helper()
	dbs := []groupReservationDatabase{{
		name:   "sqlite",
		sqlite: true,
		open: func() (*gorm.DB, error) {
			return gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "group-reservation.db")+"?_busy_timeout=5000"), &gorm.Config{})
		},
	}}
	if dsn := strings.TrimSpace(os.Getenv("NEW_API_TEST_POSTGRES_DSN")); dsn != "" {
		dbs = append(dbs, groupReservationDatabase{name: "postgres", postgres: true, open: func() (*gorm.DB, error) {
			return gorm.Open(postgres.Open(dsn), &gorm.Config{})
		}})
	}
	mysqlDSN := strings.TrimSpace(os.Getenv("NEW_API_TEST_MYSQL_DSN"))
	if mysqlDSN == "" {
		mysqlDSN = strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	}
	if mysqlDSN != "" {
		dbs = append(dbs, groupReservationDatabase{name: "mysql", mysql: true, open: func() (*gorm.DB, error) {
			return gorm.Open(mysql.Open(mysqlDSN), &gorm.Config{})
		}})
	}
	return dbs
}

func useGroupReservationDatabase(t *testing.T, fixture groupReservationDatabase) *gorm.DB {
	t.Helper()
	oldDB := DB
	oldSQLite, oldMySQL, oldPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	oldBatch, oldRedis := common.BatchUpdateEnabled, common.RedisEnabled
	db, err := fixture.open()
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = fixture.sqlite
	common.UsingMySQL = fixture.mysql
	common.UsingPostgreSQL = fixture.postgres
	common.BatchUpdateEnabled = false
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}, &BalanceCacheRepair{}))
	t.Cleanup(func() {
		DB = oldDB
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = oldSQLite, oldMySQL, oldPostgres
		common.BatchUpdateEnabled, common.RedisEnabled = oldBatch, oldRedis
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedGroupReservationWallet(t *testing.T, db *gorm.DB, suffix string, userQuota int, tokenQuota int) (*User, *Token) {
	t.Helper()
	user := &User{Username: fmt.Sprintf("gr%d", time.Now().UnixNano()%1_000_000_000_000_000), Password: "test-password", Quota: userQuota}
	user.AffCode = user.Username
	require.NoError(t, db.Create(user).Error)
	token := &Token{UserId: user.Id, Key: "group-reservation-token-" + common.GetUUID(), Name: suffix, RemainQuota: tokenQuota}
	require.NoError(t, db.Create(token).Error)
	return user, token
}

func readGroupReservationBalances(t *testing.T, db *gorm.DB, userId int, tokenId int) (int, int, int) {
	t.Helper()
	var user User
	var token Token
	require.NoError(t, db.First(&user, userId).Error)
	require.NoError(t, db.First(&token, tokenId).Error)
	return user.Quota, token.RemainQuota, token.UsedQuota
}

func TestReconcileGroupReservationWalletCrossDatabase(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			suffix := fmt.Sprintf("%s-%d", fixture.name, time.Now().UnixNano())
			user, token := seedGroupReservationWallet(t, db, suffix, 1000, 1000)
			req := GroupReservationRequest{
				Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key,
				TargetReserved: 200,
			}
			result, err := ReconcileGroupReservation(req)
			require.NoError(t, err)
			require.Equal(t, 200, result.Reserved)
			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 800, userQuota)
			require.Equal(t, 800, tokenRemain)
			require.Equal(t, 200, tokenUsed)

			req.ExpectedReserved, req.TargetReserved = 200, 500
			_, err = ReconcileGroupReservation(req)
			require.NoError(t, err)
			req.ExpectedReserved, req.TargetReserved = 500, 100
			_, err = ReconcileGroupReservation(req)
			require.NoError(t, err)
			userQuota, tokenRemain, tokenUsed = readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 900, userQuota)
			require.Equal(t, 900, tokenRemain)
			require.Equal(t, 100, tokenUsed)

			req.ExpectedReserved = 100
			_, err = ReconcileGroupReservation(req)
			require.NoError(t, err)
			userQuota2, tokenRemain2, tokenUsed2 := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, userQuota, userQuota2)
			require.Equal(t, tokenRemain, tokenRemain2)
			require.Equal(t, tokenUsed, tokenUsed2)
		})
	}
}

func TestReconcileGroupReservationRollsBackFundingWhenTokenAdmissionFails(t *testing.T) {
	fixture := groupReservationDatabases(t)[0]
	db := useGroupReservationDatabase(t, fixture)
	user, token := seedGroupReservationWallet(t, db, fmt.Sprintf("rollback-%d", time.Now().UnixNano()), 1000, 50)

	_, err := ReconcileGroupReservation(GroupReservationRequest{
		Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TargetReserved: 200,
	})
	require.ErrorIs(t, err, ErrGroupReservationTokenInsufficient)
	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 1000, userQuota)
	require.Equal(t, 50, tokenRemain)
	require.Zero(t, tokenUsed)
}

func TestReconcileGroupReservationRejectsQuotaOverflowWithoutMutation(t *testing.T) {
	fixture := groupReservationDatabases(t)[0]
	db := useGroupReservationDatabase(t, fixture)
	user, token := seedGroupReservationWallet(t, db, fmt.Sprintf("overflow-%d", time.Now().UnixNano()), math.MaxInt, math.MaxInt)

	_, err := ReconcileGroupReservation(GroupReservationRequest{
		Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key,
		ExpectedReserved: 1, TargetReserved: 0,
	})
	require.Error(t, err)
	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, math.MaxInt, userQuota)
	require.Equal(t, math.MaxInt, tokenRemain)
	require.Zero(t, tokenUsed)
}

func TestReconcileGroupReservationFoldsPendingBatchDeltasOnce(t *testing.T) {
	fixture := groupReservationDatabases(t)[0]
	db := useGroupReservationDatabase(t, fixture)
	user, token := seedGroupReservationWallet(t, db, fmt.Sprintf("batch-%d", time.Now().UnixNano()), 1000, 1000)
	common.BatchUpdateEnabled = true
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -100)
	addNewRecord(BatchUpdateTypeTokenQuota, token.Id, -100)

	_, err := ReconcileGroupReservation(GroupReservationRequest{
		Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, TargetReserved: 200,
	})
	require.NoError(t, err)
	userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 700, userQuota)
	require.Equal(t, 700, tokenRemain)
	require.Equal(t, 300, tokenUsed)
	require.Zero(t, batchUpdateStores[BatchUpdateTypeUserQuota][user.Id])
	require.Zero(t, batchUpdateStores[BatchUpdateTypeTokenQuota][token.Id])
}

func TestReconcileGroupReservationSubscriptionCreateResizeAndRollback(t *testing.T) {
	fixture := groupReservationDatabases(t)[0]
	db := useGroupReservationDatabase(t, fixture)
	user, token := seedGroupReservationWallet(t, db, fmt.Sprintf("subscription-%d", time.Now().UnixNano()), 1000, 1000)
	plan := &SubscriptionPlan{Title: "group reservation", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1000, QuotaResetPeriod: SubscriptionResetNever}
	require.NoError(t, db.Create(plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	sub := &UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active"}
	require.NoError(t, db.Create(sub).Error)

	req := GroupReservationRequest{
		Source: GroupReservationSubscription, RequestId: fmt.Sprintf("request-%d", time.Now().UnixNano()),
		UserId: user.Id, ModelName: "test-model", TokenId: token.Id, TokenKey: token.Key, TargetReserved: 200,
	}
	result, err := ReconcileGroupReservation(req)
	require.NoError(t, err)
	require.Equal(t, sub.Id, result.SubscriptionId)
	req.SubscriptionId = sub.Id
	req.ExpectedReserved, req.TargetReserved = 200, 500
	_, err = ReconcileGroupReservation(req)
	require.NoError(t, err)
	req.ExpectedReserved, req.TargetReserved = 500, 0
	_, err = ReconcileGroupReservation(req)
	require.NoError(t, err)

	var gotSub UserSubscription
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.First(&gotSub, sub.Id).Error)
	require.NoError(t, db.Where("request_id = ?", req.RequestId).First(&record).Error)
	require.Zero(t, gotSub.AmountUsed)
	require.Zero(t, record.PreConsumed)
	_, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
	require.Equal(t, 1000, tokenRemain)
	require.Zero(t, tokenUsed)

	req.ExpectedReserved, req.TargetReserved = 0, 200
	require.NoError(t, db.Model(&Token{}).Where("id = ?", token.Id).Update("remain_quota", 50).Error)
	_, err = ReconcileGroupReservation(req)
	require.ErrorIs(t, err, ErrGroupReservationTokenInsufficient)
	require.NoError(t, db.First(&gotSub, sub.Id).Error)
	require.NoError(t, db.Where("request_id = ?", req.RequestId).First(&record).Error)
	require.Zero(t, gotSub.AmountUsed)
	require.Zero(t, record.PreConsumed)
}

func TestReconcileGroupReservationSubscriptionCrossDatabase(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := useGroupReservationDatabase(t, fixture)
			suffix := fmt.Sprintf("subscription-matrix-%s-%d", fixture.name, time.Now().UnixNano())
			user, token := seedGroupReservationWallet(t, db, suffix, 1000, 1000)
			plan := &SubscriptionPlan{Title: suffix, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1000, QuotaResetPeriod: SubscriptionResetNever}
			require.NoError(t, db.Create(plan).Error)
			InvalidateSubscriptionPlanCache(plan.Id)
			sub := &UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: time.Now().Add(-time.Hour).Unix(), EndTime: time.Now().Add(time.Hour).Unix(), Status: "active"}
			require.NoError(t, db.Create(sub).Error)
			req := GroupReservationRequest{
				Source: GroupReservationSubscription, RequestId: "request-" + suffix, UserId: user.Id,
				ModelName: "test-model", TokenId: token.Id, TokenKey: token.Key, TargetReserved: 250,
			}
			result, err := ReconcileGroupReservation(req)
			require.NoError(t, err)
			req.SubscriptionId = result.SubscriptionId
			req.ExpectedReserved, req.TargetReserved = 250, 100
			_, err = ReconcileGroupReservation(req)
			require.NoError(t, err)

			var gotSub UserSubscription
			var record SubscriptionPreConsumeRecord
			require.NoError(t, db.First(&gotSub, sub.Id).Error)
			require.NoError(t, db.Where("request_id = ?", req.RequestId).First(&record).Error)
			require.EqualValues(t, 100, gotSub.AmountUsed)
			require.EqualValues(t, 100, record.PreConsumed)
			_, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 900, tokenRemain)
			require.Equal(t, 100, tokenUsed)
		})
	}
}

func TestReconcileGroupReservationSubscriptionReestablishesTargetAfterReset(t *testing.T) {
	fixture := groupReservationDatabases(t)[0]
	db := useGroupReservationDatabase(t, fixture)
	suffix := fmt.Sprintf("reset-%d", time.Now().UnixNano())
	user, token := seedGroupReservationWallet(t, db, suffix, 1000, 1000)
	plan := &SubscriptionPlan{Title: suffix, DurationUnit: SubscriptionDurationMonth, DurationValue: 1, Enabled: true, TotalAmount: 1000, QuotaResetPeriod: SubscriptionResetDaily}
	require.NoError(t, db.Create(plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	now := time.Now()
	sub := &UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 1000, StartTime: now.Add(-time.Hour).Unix(), EndTime: now.Add(48 * time.Hour).Unix(), Status: "active"}
	require.NoError(t, db.Create(sub).Error)
	req := GroupReservationRequest{
		Source: GroupReservationSubscription, RequestId: "request-" + suffix, UserId: user.Id,
		ModelName: "test-model", TokenId: token.Id, TokenKey: token.Key, TargetReserved: 300,
	}
	result, err := ReconcileGroupReservation(req)
	require.NoError(t, err)
	req.SubscriptionId = result.SubscriptionId

	require.NoError(t, db.Model(&UserSubscription{}).Where("id = ?", sub.Id).Updates(map[string]interface{}{
		"start_time": now.Add(-72 * time.Hour).Unix(), "last_reset_time": now.Add(-48 * time.Hour).Unix(),
		"next_reset_time": now.Add(-24 * time.Hour).Unix(), "amount_used": 300,
	}).Error)
	req.ExpectedReserved, req.TargetReserved = 300, 500
	_, err = ReconcileGroupReservation(req)
	require.NoError(t, err)
	var gotSub UserSubscription
	var record SubscriptionPreConsumeRecord
	require.NoError(t, db.First(&gotSub, sub.Id).Error)
	require.NoError(t, db.Where("request_id = ?", req.RequestId).First(&record).Error)
	require.EqualValues(t, 500, gotSub.AmountUsed)
	require.EqualValues(t, 500, record.PreConsumed)

	req.ExpectedReserved, req.TargetReserved = 500, 0
	_, err = ReconcileGroupReservation(req)
	require.NoError(t, err)
	require.NoError(t, RefundSubscriptionPreConsume(req.RequestId))
	require.NoError(t, db.First(&gotSub, sub.Id).Error)
	require.NoError(t, db.Where("request_id = ?", req.RequestId).First(&record).Error)
	require.Zero(t, gotSub.AmountUsed)
	require.Equal(t, "refunded", record.Status)
}

func TestReconcileGroupReservationSkipTokenQuotaStillTracksWallet(t *testing.T) {
	fixture := groupReservationDatabases(t)[0]
	db := useGroupReservationDatabase(t, fixture)
	user := &User{Username: fmt.Sprintf("skip-token-%d", time.Now().UnixNano()), Password: "test-password", Quota: 500}
	user.AffCode = common.GetRandomString(12)
	require.NoError(t, db.Create(user).Error)
	req := GroupReservationRequest{Source: GroupReservationWallet, UserId: user.Id, SkipTokenQuota: true, TargetReserved: 300}
	_, err := ReconcileGroupReservation(req)
	require.NoError(t, err)
	req.ExpectedReserved, req.TargetReserved = 300, 0
	_, err = ReconcileGroupReservation(req)
	require.NoError(t, err)
	var got User
	require.NoError(t, db.First(&got, user.Id).Error)
	require.Equal(t, 500, got.Quota)
}
