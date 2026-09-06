package model

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resetBalanceBatchState(t *testing.T) {
	t.Helper()
	userQuotaBatchApplyLock.Lock()
	tokenQuotaBatchApplyLock.Lock()
	for _, typeID := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
		batchUpdateLocks[typeID].Lock()
		batchUpdateStores[typeID] = make(map[int]int)
		batchUpdateLocks[typeID].Unlock()
	}
	pendingBalanceBatches.Lock()
	for operationID := range pendingBalanceBatches.operations {
		unmarkBalanceCacheRepairInFlight(operationID)
	}
	pendingBalanceBatches.operations = make(map[string]pendingBalanceBatch)
	pendingBalanceBatches.Unlock()
	tokenQuotaBatchApplyLock.Unlock()
	userQuotaBatchApplyLock.Unlock()
	t.Cleanup(func() {
		userQuotaBatchApplyLock.Lock()
		tokenQuotaBatchApplyLock.Lock()
		for _, typeID := range []int{BatchUpdateTypeUserQuota, BatchUpdateTypeTokenQuota} {
			batchUpdateLocks[typeID].Lock()
			batchUpdateStores[typeID] = make(map[int]int)
			batchUpdateLocks[typeID].Unlock()
		}
		pendingBalanceBatches.Lock()
		for operationID := range pendingBalanceBatches.operations {
			unmarkBalanceCacheRepairInFlight(operationID)
		}
		pendingBalanceBatches.operations = make(map[string]pendingBalanceBatch)
		pendingBalanceBatches.Unlock()
		tokenQuotaBatchApplyLock.Unlock()
		userQuotaBatchApplyLock.Unlock()
	})
}

func pendingBalanceBatchCount() int {
	pendingBalanceBatches.Lock()
	defer pendingBalanceBatches.Unlock()
	return len(pendingBalanceBatches.operations)
}

func prepareBalanceRepairDatabase(t *testing.T, fixture groupReservationDatabase) *gorm.DB {
	t.Helper()
	db := useGroupReservationDatabase(t, fixture)
	require.NoError(t, db.AutoMigrate(&BalanceCacheRepair{}))
	resetBalanceBatchState(t)
	return db
}

func TestBalanceHelpersCommitDirectlyWithBatchingEnabled(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			common.BatchUpdateEnabled = true
			user, token := seedGroupReservationWallet(t, db, "direct-balance-"+fixture.name, 1000, 1000)

			require.NoError(t, DecreaseUserQuota(user.Id, 120, false))
			require.NoError(t, IncreaseUserQuota(user.Id, 20, false))
			require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 120))
			require.NoError(t, IncreaseTokenQuota(token.Id, token.Key, 20))
			require.NoError(t, IncreaseUserQuota(user.Id, 0, false))
			require.NoError(t, IncreaseTokenQuota(token.Id, token.Key, 0))

			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 900, userQuota)
			require.Equal(t, 900, tokenRemain)
			require.Equal(t, 100, tokenUsed)
			batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
			require.Zero(t, batchUpdateStores[BatchUpdateTypeUserQuota][user.Id])
			batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
			batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
			require.Zero(t, batchUpdateStores[BatchUpdateTypeTokenQuota][token.Id])
			batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()

			require.ErrorIs(t, IncreaseUserQuota(user.Id+1_000_000_000, 1, false), gorm.ErrRecordNotFound)
			require.ErrorIs(t, IncreaseTokenQuota(token.Id+1_000_000_000, "missing", 1), gorm.ErrRecordNotFound)
			require.ErrorIs(t, IncreaseUserQuota(user.Id+1_000_000_000, 0, false), gorm.ErrRecordNotFound)
			require.ErrorIs(t, IncreaseTokenQuota(token.Id+1_000_000_000, "missing", 0), gorm.ErrRecordNotFound)
			var repairCount int64
			require.NoError(t, db.Model(&BalanceCacheRepair{}).
				Where("repaired_at = ? AND (user_id = ? OR token_cache_key = ?)", 0, user.Id, tokenCacheRedisKey(token.Key)).
				Count(&repairCount).Error)
			require.Zero(t, repairCount)
		})
	}
}

func TestBalanceHelperClassifiesCommitErrorByExactRepair(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			user, _ := seedGroupReservationWallet(t, db, "commit-receipt-"+fixture.name, 1000, 1000)
			pool := installTaskSubmissionCommitErrorPool(t, db)
			pool.failCommit = true

			require.NoError(t, DecreaseUserQuota(user.Id, 100, false))
			var refreshed User
			require.NoError(t, db.First(&refreshed, user.Id).Error)
			require.Equal(t, 900, refreshed.Quota)
			var repairCount int64
			require.NoError(t, db.Model(&BalanceCacheRepair{}).
				Where("repaired_at = ? AND user_id = ?", 0, user.Id).Count(&repairCount).Error)
			require.Zero(t, repairCount)
		})
	}
}

func TestRepairedReceiptStillClassifiesLostCommitReply(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			user, _ := seedGroupReservationWallet(t, db, "retained-receipt-"+fixture.name, 1000, 1000)
			repair, err := newUserBalanceCacheRepair(common.GetUUID(), user.Id)
			require.NoError(t, err)
			require.NoError(t, db.Create(repair).Error)
			// Simulate another node completing projection repair before the
			// writer receives and classifies its lost Commit reply.
			require.NoError(t, RecoverBalanceCacheRepairs(context.Background(), 100))

			var stored BalanceCacheRepair
			require.NoError(t, db.First(&stored, "id = ?", repair.ID).Error)
			require.NotZero(t, stored.RepairedAt)
			require.Equal(t, repair.ID, stored.ID, "repair acknowledgement must retain commit proof")
			require.NoError(t, classifyBalanceMutationCommit(
				context.Background(), repair, errInjectedTaskSubmissionCommit, false,
			))
		})
	}
}

func TestBalanceCommitClassificationRequiresExactRepairTarget(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			user, _ := seedGroupReservationWallet(t, db, "exact-receipt-target-"+fixture.name, 1000, 1000)
			otherUser := user.Id + 1_000_000_000
			operationID := common.GetUUID()
			stored, err := newUserBalanceCacheRepair(operationID, otherUser)
			require.NoError(t, err)
			require.NoError(t, db.Create(stored).Error)
			expected, err := newUserBalanceCacheRepair(operationID, user.Id)
			require.NoError(t, err)

			err = classifyBalanceMutationCommit(
				context.Background(), expected, errInjectedTaskSubmissionCommit, false,
			)
			require.ErrorIs(t, err, errInjectedTaskSubmissionCommit)
		})
	}
}

func TestRejectedBalanceWriteDoesNotInvalidateCache(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			user, token := seedGroupReservationWallet(t, db, "rejected-write-"+fixture.name, 1000, 1000)
			errRejected := errors.New("injected balance update rejection")
			failTable := "users"
			callbackName := "balance-cache-repair:reject-update"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == failTable {
					tx.AddError(errRejected)
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

			oldRedis := common.RedisEnabled
			oldUserInvalidation := invalidateBalanceUserProjection
			oldTokenInvalidation := invalidateBalanceTokenProjection
			common.RedisEnabled = true
			userInvalidations, tokenInvalidations := 0, 0
			invalidateBalanceUserProjection = func(int) error {
				userInvalidations++
				return nil
			}
			invalidateBalanceTokenProjection = func(string) error {
				tokenInvalidations++
				return nil
			}
			t.Cleanup(func() {
				common.RedisEnabled = oldRedis
				invalidateBalanceUserProjection = oldUserInvalidation
				invalidateBalanceTokenProjection = oldTokenInvalidation
			})

			require.ErrorIs(t, DecreaseUserQuota(user.Id, 100, false), errRejected)
			failTable = "tokens"
			require.ErrorIs(t, DecreaseTokenQuota(token.Id, token.Key, 100), errRejected)
			require.Zero(t, userInvalidations)
			require.Zero(t, tokenInvalidations)
			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 1000, userQuota)
			require.Equal(t, 1000, tokenRemain)
			require.Zero(t, tokenUsed)
			var repairCount int64
			require.NoError(t, db.Model(&BalanceCacheRepair{}).
				Where("repaired_at = ? AND (user_id = ? OR token_cache_key = ?)", 0, user.Id, tokenCacheRedisKey(token.Key)).
				Count(&repairCount).Error)
			require.Zero(t, repairCount)
		})
	}
}

func TestLegacyBalanceBatchFlushRestoresOrDiscardsAfterCommitClassification(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			user, token := seedGroupReservationWallet(t, db, "legacy-flush-"+fixture.name, 1000, 1000)
			pool := installTaskSubmissionCommitErrorPool(t, db)

			addNewRecord(BatchUpdateTypeUserQuota, user.Id, -100)
			pool.failCommitBefore = true
			pool.queryErrorsAfterCommit = 1
			flushUserQuotaBatchUpdates()
			require.Equal(t, 1, pendingBalanceBatchCount())
			require.NoError(t, RecoverBalanceCacheRepairs(context.Background(), 100))
			require.Zero(t, pendingBalanceBatchCount())
			batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
			require.Equal(t, -100, batchUpdateStores[BatchUpdateTypeUserQuota][user.Id])
			batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
			flushUserQuotaBatchUpdates()

			addNewRecord(BatchUpdateTypeTokenQuota, token.Id, -100)
			pool.failCommit = true
			pool.queryErrorsAfterCommit = 1
			flushTokenQuotaBatchUpdates()
			require.Equal(t, 1, pendingBalanceBatchCount())
			require.NoError(t, RecoverBalanceCacheRepairs(context.Background(), 100))
			require.Zero(t, pendingBalanceBatchCount())
			flushTokenQuotaBatchUpdates()

			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 900, userQuota)
			require.Equal(t, 900, tokenRemain)
			require.Equal(t, 100, tokenUsed)
		})
	}
}

func TestLegacyBalanceBatchDefiniteRollbackDoesNotQueryOrPark(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			user, _ := seedGroupReservationWallet(t, db, "definite-rollback-"+fixture.name, 1000, 1000)
			pool := installTaskSubmissionCommitErrorPool(t, db)
			pool.queryErrorsAfterRollback = 1
			errRejected := errors.New("injected balance callback rollback")
			callbackName := "balance-cache-repair:definite-rollback"
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == "users" {
					tx.AddError(errRejected)
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })

			addNewRecord(BatchUpdateTypeUserQuota, user.Id, -100)
			flushUserQuotaBatchUpdates()
			require.Zero(t, pendingBalanceBatchCount(), "a callback failure is a definite rollback")
			batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
			require.Equal(t, -100, batchUpdateStores[BatchUpdateTypeUserQuota][user.Id])
			batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

			var repairCount int64
			err := db.Model(&BalanceCacheRepair{}).Count(&repairCount).Error
			require.ErrorIs(t, err, errInjectedTaskSubmissionQuery,
				"the unavailable read must remain unused after the definite rollback")
			pool.setQueryErrors(0)
			var refreshed User
			require.NoError(t, db.First(&refreshed, user.Id).Error)
			require.Equal(t, 1000, refreshed.Quota)
		})
	}
}

func TestReservationBlocksForUnclassifiedBalanceBatchesThenRecoversOnce(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			user, token := seedGroupReservationWallet(t, db, "parked-admission-"+fixture.name, 1000, 1000)
			pool := installTaskSubmissionCommitErrorPool(t, db)
			parkPendingBalanceBatch(pendingBalanceBatch{
				OperationID: common.GetUUID(), Type: BatchUpdateTypeUserQuota, ID: user.Id, Delta: -900,
			})
			parkPendingBalanceBatch(pendingBalanceBatch{
				OperationID: common.GetUUID(), Type: BatchUpdateTypeTokenQuota, ID: token.Id, Delta: -900,
			})
			pool.setQueryErrors(1)

			req := GroupReservationRequest{
				Source: GroupReservationWallet, UserId: user.Id, TokenId: token.Id,
				TokenKey: token.Key, TargetReserved: 50,
			}
			_, err := ReconcileGroupReservation(req)
			require.ErrorIs(t, err, errInjectedTaskSubmissionQuery)
			userQuota, tokenRemain, tokenUsed := readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 1000, userQuota)
			require.Equal(t, 1000, tokenRemain)
			require.Zero(t, tokenUsed)

			result, err := ReconcileGroupReservation(req)
			require.NoError(t, err)
			require.Equal(t, 50, result.Reserved)
			require.Zero(t, pendingBalanceBatchCount())
			batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
			require.Zero(t, batchUpdateStores[BatchUpdateTypeUserQuota][user.Id])
			batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
			batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
			require.Zero(t, batchUpdateStores[BatchUpdateTypeTokenQuota][token.Id])
			batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
			userQuota, tokenRemain, tokenUsed = readGroupReservationBalances(t, db, user.Id, token.Id)
			require.Equal(t, 50, userQuota)
			require.Equal(t, 50, tokenRemain)
			require.Equal(t, 950, tokenUsed)
		})
	}
}

func TestConsumePendingUserQuotaDeltaBlocksForUnclassifiedBalanceBatch(t *testing.T) {
	for _, fixture := range groupReservationDatabases(t) {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			db := prepareBalanceRepairDatabase(t, fixture)
			user, _ := seedGroupReservationWallet(t, db, "parked-user-consumer-"+fixture.name, 1000, 1000)
			pool := installTaskSubmissionCommitErrorPool(t, db)
			parkPendingBalanceBatch(pendingBalanceBatch{
				OperationID: common.GetUUID(), Type: BatchUpdateTypeUserQuota, ID: user.Id, Delta: -100,
			})
			pool.setQueryErrors(1)
			applyCalls := 0

			err := ConsumePendingUserQuotaDelta(user.Id, func(delta int) error {
				applyCalls++
				return nil
			})
			require.ErrorIs(t, err, errInjectedTaskSubmissionQuery)
			require.Zero(t, applyCalls)
			var refreshed User
			require.NoError(t, db.First(&refreshed, user.Id).Error)
			require.Equal(t, 1000, refreshed.Quota)

			require.NoError(t, ConsumePendingUserQuotaDelta(user.Id, func(delta int) error {
				applyCalls++
				result := db.Model(&User{}).Where("id = ?", user.Id).
					Update("quota", gorm.Expr("quota + ?", delta))
				require.EqualValues(t, 1, result.RowsAffected)
				return result.Error
			}))
			require.Equal(t, 1, applyCalls)
			require.Zero(t, pendingBalanceBatchCount())
			require.NoError(t, db.First(&refreshed, user.Id).Error)
			require.Equal(t, 900, refreshed.Quota)
		})
	}
}

func TestBalanceCacheRepairSurvivesRedisOutageAndRecovery(t *testing.T) {
	address := os.Getenv("NEW_API_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("requires GitHub Actions Redis fixture")
	}
	db := prepareBalanceRepairDatabase(t, groupReservationDatabases(t)[0])
	user := &User{Id: 987654322, Username: "balance-cache-repair", Password: "test-password", Quota: 1000, AffCode: "balance-cache-repair"}
	require.NoError(t, db.Create(user).Error)
	token := &Token{UserId: user.Id, Key: "balance-cache-repair-" + common.GetUUID(), Name: "redis-repair", RemainQuota: 1000}
	require.NoError(t, db.Create(token).Error)

	realClient := redis.NewClient(&redis.Options{Addr: address})
	brokenClient := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  20 * time.Millisecond,
		ReadTimeout:  20 * time.Millisecond,
		WriteTimeout: 20 * time.Millisecond,
		MaxRetries:   -1,
	})
	previousClient, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = realClient, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousClient, previousEnabled
		_ = brokenClient.Close()
		_ = realClient.Close()
	})
	require.NoError(t, realClient.Ping(context.Background()).Err())
	require.NoError(t, invalidateUserCache(user.Id))
	require.NoError(t, cacheDeleteToken(token.Key))
	userGeneration, err := common.RedisGetGeneration(userCacheGenerationRedisKey)
	require.NoError(t, err)
	written, err := fillUserCacheIfGeneration(*user, userGeneration)
	require.NoError(t, err)
	require.True(t, written)
	staleTokenGeneration, err := common.RedisGetGeneration(tokenCacheGenerationRedisKey)
	require.NoError(t, err)
	require.NoError(t, cacheDeleteToken(token.Key))
	written, err = (redisTokenCacheBackend{}).setTokenIfGeneration(*token, staleTokenGeneration, false)
	require.NoError(t, err)
	require.False(t, written)
	require.Zero(t, realClient.Exists(context.Background(), tokenCacheRedisKey(token.Key)).Val())
	tokenGeneration, err := common.RedisGetGeneration(tokenCacheGenerationRedisKey)
	require.NoError(t, err)
	written, err = (redisTokenCacheBackend{}).setTokenIfGeneration(*token, tokenGeneration, false)
	require.NoError(t, err)
	require.True(t, written)

	common.RDB = brokenClient
	require.NoError(t, DecreaseUserQuota(user.Id, 100, false))
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 100))
	var repairCount int64
	require.NoError(t, db.Model(&BalanceCacheRepair{}).
		Where("repaired_at = ? AND (user_id = ? OR token_cache_key = ?)", 0, user.Id, tokenCacheRedisKey(token.Key)).
		Count(&repairCount).Error)
	require.EqualValues(t, 2, repairCount)
	// Recovery must depend only on durable rows, not process-local operation state.
	balanceCacheRepairInFlight.Lock()
	balanceCacheRepairInFlight.operations = make(map[string]struct{})
	balanceCacheRepairInFlight.Unlock()

	common.RDB = realClient
	staleUser, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	require.Equal(t, 1000, staleUser.Quota)
	staleToken, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	require.Equal(t, 1000, staleToken.RemainQuota)
	require.NoError(t, RecoverBalanceCacheRepairs(context.Background(), 100))
	require.NoError(t, db.Model(&BalanceCacheRepair{}).
		Where("repaired_at = ? AND (user_id = ? OR token_cache_key = ?)", 0, user.Id, tokenCacheRedisKey(token.Key)).
		Count(&repairCount).Error)
	require.Zero(t, repairCount)
	require.Zero(t, realClient.Exists(context.Background(), getUserCacheKey(user.Id)).Val())
	require.Zero(t, realClient.Exists(context.Background(), tokenCacheRedisKey(token.Key)).Val())

	quota, err := GetUserQuota(user.Id, false)
	require.NoError(t, err)
	require.Equal(t, 900, quota)
	require.Zero(t, realClient.Exists(context.Background(), getUserCacheKey(user.Id)).Val(),
		"quota-only fallback must not create an unfenced full user hash")
}

func TestBalanceCacheRepairRejectsMalformedProjection(t *testing.T) {
	repair := &BalanceCacheRepair{ID: "bad", Version: balanceCacheRepairVersion}
	require.Error(t, validateBalanceCacheRepair(repair))
	userID := 1
	repair.UserID = &userID
	repair.TokenCacheKey = "token:not-a-single-target"
	require.Error(t, validateBalanceCacheRepair(repair))
	repair.UserID = nil
	repair.TokenCacheKey = "raw-token-secret"
	require.Error(t, validateBalanceCacheRepair(repair))
}
