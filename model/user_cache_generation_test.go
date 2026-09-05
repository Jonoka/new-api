package model

import (
	"context"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestUserCacheGenerationRejectsLateQuotaFill(t *testing.T) {
	address := os.Getenv("NEW_API_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("requires GitHub Actions Redis fixture")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	previousClient, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() {
		common.RDB, common.RedisEnabled = previousClient, previousEnabled
		_ = client.Close()
	})
	require.NoError(t, client.Ping(context.Background()).Err())
	user := User{Id: 987654321, Username: "cache-fixture", Quota: 100}
	generation, err := common.RedisGetGeneration(userCacheGenerationRedisKey)
	require.NoError(t, err)
	require.NoError(t, invalidateUserCache(user.Id))
	filled, err := fillUserCacheIfGeneration(user, generation)
	require.NoError(t, err)
	require.False(t, filled)
	require.Zero(t, client.Exists(context.Background(), getUserCacheKey(user.Id)).Val())

	generation, err = common.RedisGetGeneration(userCacheGenerationRedisKey)
	require.NoError(t, err)
	user.Quota = 50
	filled, err = fillUserCacheIfGeneration(user, generation)
	require.NoError(t, err)
	require.True(t, filled)
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	require.Equal(t, 50, cached.Quota)
	require.NoError(t, invalidateUserCache(user.Id))
}
