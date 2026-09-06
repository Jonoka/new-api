package model

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

func TestWalletConcurrencyCompatibilitySeed(t *testing.T) {
	db := taskAccountingCompatibilityDatabase(t)
	prices := make(map[string]float64)
	for _, fixture := range []struct {
		name          string
		wallet, token int
	}{
		{"wallet", 6000, 60000}, {"token", 60000, 6000}, {"stale", 100, 100},
		{"trust", 6000000, 6000000}, {"refund", 12000, 12000}, {"restart", 12000, 12000},
		{"cache", 12000, 12000},
	} {
		name := "d-wallet-" + fixture.name
		user := &User{Username: name, Password: "ci-only", Quota: fixture.wallet,
			Status: common.UserStatusEnabled, Group: "default", AffCode: name}
		user.SetSetting(dto.UserSetting{BillingPreference: "wallet_only", QuotaWarningThreshold: 1})
		require.NoError(t, db.Create(user).Error)
		token := &Token{UserId: user.Id, Key: "dwallet" + fixture.name, Name: name,
			Status: common.TokenStatusEnabled, RemainQuota: fixture.token, Group: "default",
			ModelLimitsEnabled: true, ModelLimits: name}
		require.NoError(t, token.Insert())
		channel := &Channel{Name: name, Type: constant.ChannelTypeOpenAI,
			Key: "d-ci-gateway", Status: common.ChannelStatusEnabled,
			BaseURL: common.GetPointer("http://127.0.0.2:38080"), Models: name,
			Group: "default", Weight: common.GetPointer(uint(1)), Priority: common.GetPointer(int64(100)),
			AutoBan: common.GetPointer(0)}
		require.NoError(t, channel.Insert())
		prices[name] = 0.004
	}
	priceJSON, err := common.Marshal(prices)
	require.NoError(t, err)
	for key, value := range map[string]string{"ModelPrice": string(priceJSON), "RetryTimes": "0", "LogConsumeEnabled": "true"} {
		require.NoError(t, db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Option{Key: key, Value: value}).Error)
	}
}

func TestWalletConcurrencyCompatibilityStaleCache(t *testing.T) {
	db := taskAccountingCompatibilityDatabase(t)
	address := os.Getenv("NEW_API_TEST_REDIS_ADDR")
	require.NotEmpty(t, address)
	client := redis.NewClient(&redis.Options{Addr: address, DB: 13})
	oldClient, oldEnabled, oldCrypto, oldFrequency := common.RDB, common.RedisEnabled, common.CryptoSecret, common.SyncFrequency
	common.RDB, common.RedisEnabled, common.CryptoSecret = client, true, "d-ci-crypto-only"
	common.SyncFrequency = 3600
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled, common.CryptoSecret = oldClient, oldEnabled, oldCrypto
		common.SyncFrequency = oldFrequency
		_ = client.Close()
	})
	require.NoError(t, client.Ping(context.Background()).Err())
	var user User
	var token Token
	require.NoError(t, db.Where("username = ?", "d-wallet-stale").First(&user).Error)
	require.NoError(t, db.Where("name = ?", "d-wallet-stale").First(&token).Error)
	user.Quota, token.RemainQuota = 6000000, 6000000
	generation, err := common.RedisGetGeneration(userCacheGenerationRedisKey)
	require.NoError(t, err)
	filled, err := fillUserCacheIfGeneration(user, generation)
	require.NoError(t, err)
	require.True(t, filled)
	backend := redisTokenCacheBackend{}
	generation, err = backend.generation()
	require.NoError(t, err)
	filled, err = backend.setTokenIfGeneration(token, generation, false)
	require.NoError(t, err)
	require.True(t, filled)
	fmt.Println("Seeded stale-high synthetic cache for authoritative trust rejection fixture.")
}

func TestWalletConcurrencyCompatibilityCacheOutage(t *testing.T) {
	db := taskAccountingCompatibilityDatabase(t)
	address := os.Getenv("NEW_API_TEST_REDIS_ADDR")
	require.NotEmpty(t, address)
	client := redis.NewClient(&redis.Options{Addr: address, DB: 13})
	broken := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 20 * time.Millisecond, ReadTimeout: 20 * time.Millisecond, WriteTimeout: 20 * time.Millisecond})
	oldClient, oldEnabled, oldCrypto, oldFrequency := common.RDB, common.RedisEnabled, common.CryptoSecret, common.SyncFrequency
	common.RDB, common.RedisEnabled, common.CryptoSecret = client, true, "d-ci-crypto-only"
	common.SyncFrequency = 3600
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled, common.CryptoSecret = oldClient, oldEnabled, oldCrypto
		common.SyncFrequency = oldFrequency
		_ = client.Close()
		_ = broken.Close()
	})
	var user User
	var token Token
	require.NoError(t, db.Where("username = ?", "d-wallet-cache").First(&user).Error)
	require.NoError(t, db.Where("name = ?", "d-wallet-cache").First(&token).Error)
	generation, err := common.RedisGetGeneration(userCacheGenerationRedisKey)
	require.NoError(t, err)
	filled, err := fillUserCacheIfGeneration(user, generation)
	require.NoError(t, err)
	require.True(t, filled)
	backend := redisTokenCacheBackend{}
	generation, err = backend.generation()
	require.NoError(t, err)
	filled, err = backend.setTokenIfGeneration(token, generation, false)
	require.NoError(t, err)
	require.True(t, filled)
	common.RDB = broken
	require.NoError(t, DecreaseUserQuota(user.Id, 1000, false))
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 1000))
	var pending int64
	require.NoError(t, db.Model(&BalanceCacheRepair{}).Where("repaired_at = 0 AND (user_id = ? OR token_cache_key = ?)", user.Id, tokenCacheRedisKey(token.Key)).Count(&pending).Error)
	require.EqualValues(t, 2, pending)
	require.Equal(t, "12000", client.HGet(context.Background(), getUserCacheKey(user.Id), "Quota").Val())
	require.Equal(t, "12000", client.HGet(context.Background(), tokenCacheRedisKey(token.Key), "RemainQuota").Val())
	fmt.Println("PostgreSQL balance mutations committed with two durable cache repairs pending after Redis failure.")
}
