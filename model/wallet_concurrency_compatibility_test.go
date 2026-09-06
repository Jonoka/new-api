package model

import (
	"context"
	"fmt"
	"os"
	"testing"

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
	oldClient, oldEnabled, oldCrypto := common.RDB, common.RedisEnabled, common.CryptoSecret
	common.RDB, common.RedisEnabled, common.CryptoSecret = client, true, "d-ci-crypto-only"
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled, common.CryptoSecret = oldClient, oldEnabled, oldCrypto
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
