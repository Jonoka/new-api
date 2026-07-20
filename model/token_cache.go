package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

const tokenCacheReadTimeout = 10 * time.Second
const tokenCacheGenerationRedisKey = "token-cache-generation"

type tokenCacheBackend interface {
	generation() (int64, error)
	setTokenIfGeneration(token Token, generation int64, preserveCachedQuota bool) (bool, error)
	invalidateTokens(keys []string) error
	invalidateTokenIfGeneration(key string, generation int64) (bool, error)
}

type redisTokenCacheBackend struct{}

func tokenCacheRedisKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

func uniqueTokenCacheKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	unique := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	sort.Slice(unique, func(i, j int) bool {
		return tokenCacheRedisKey(unique[i]) < tokenCacheRedisKey(unique[j])
	})
	return unique
}

func (redisTokenCacheBackend) generation() (int64, error) {
	return common.RedisGetGeneration(tokenCacheGenerationRedisKey)
}

func (redisTokenCacheBackend) setTokenIfGeneration(
	token Token,
	generation int64,
	preserveCachedQuota bool,
) (bool, error) {
	rawKey := token.Key
	token.Clean()
	expiration := time.Duration(common.RedisKeyCacheSeconds()) * time.Second
	preserveFields := []string(nil)
	if preserveCachedQuota {
		preserveFields = []string{constant.TokenFiledRemainQuota}
	}
	return common.RedisHSetObjIfGeneration(
		tokenCacheGenerationRedisKey,
		tokenCacheRedisKey(rawKey),
		generation,
		&token,
		expiration,
		preserveFields...,
	)
}

func (redisTokenCacheBackend) invalidateTokens(keys []string) error {
	keys = uniqueTokenCacheKeys(keys)
	dataKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		dataKeys = append(dataKeys, tokenCacheRedisKey(key))
	}
	return common.RedisBumpGenerationAndDeleteKeys(tokenCacheGenerationRedisKey, dataKeys)
}

func (redisTokenCacheBackend) invalidateTokenIfGeneration(key string, generation int64) (bool, error) {
	return common.RedisBumpGenerationAndDeleteIfCurrent(
		tokenCacheGenerationRedisKey,
		tokenCacheRedisKey(key),
		generation,
	)
}

// cacheRefreshToken 在读数据库前记录 generation，只允许未经失效的快照回填缓存。
func cacheRefreshTokenWithBackend(
	backend tokenCacheBackend,
	tokenID int,
	fallbackKey string,
	preserveCachedQuota bool,
) error {
	if tokenID <= 0 {
		return nil
	}
	if DB == nil {
		return errors.New("database is nil")
	}
	if fallbackKey == "" {
		return errors.New("刷新令牌缓存时令牌键为空")
	}
	generation, err := backend.generation()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), tokenCacheReadTimeout)
	defer cancel()
	db := DB.WithContext(ctx)
	var token Token
	if err := db.First(&token, "id = ?", tokenID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_, invalidateErr := backend.invalidateTokenIfGeneration(fallbackKey, generation)
			return invalidateErr
		}
		return err
	}
	if token.Key != fallbackKey {
		if _, err := backend.invalidateTokenIfGeneration(fallbackKey, generation); err != nil {
			return err
		}
		return fmt.Errorf("令牌 %d 的键在缓存刷新期间发生变化", tokenID)
	}
	if err := HydrateTokenGroupBindings(db, []*Token{&token}); err != nil {
		return err
	}
	_, err = backend.setTokenIfGeneration(token, generation, preserveCachedQuota)
	return err
}

func cacheRefreshToken(tokenID int, fallbackKey string, preserveCachedQuota bool) error {
	return cacheRefreshTokenWithBackend(
		redisTokenCacheBackend{},
		tokenID,
		fallbackKey,
		preserveCachedQuota,
	)
}

func cacheDeleteTokenWithBackend(backend tokenCacheBackend, key string) error {
	return cacheDeleteTokensWithBackend(backend, []string{key})
}

func cacheDeleteToken(key string) error {
	return cacheDeleteTokenWithBackend(redisTokenCacheBackend{}, key)
}

func cacheDeleteTokensWithBackend(backend tokenCacheBackend, keys []string) error {
	keys = uniqueTokenCacheKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	return backend.invalidateTokens(keys)
}

func cacheDeleteTokens(keys []string) error {
	return cacheDeleteTokensWithBackend(redisTokenCacheBackend{}, keys)
}

func cacheIncrTokenQuota(key string, increment int64) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHIncrBy(fmt.Sprintf("token:%s", key), constant.TokenFiledRemainQuota, increment)
	if err != nil {
		return err
	}
	return nil
}

func cacheDecrTokenQuota(key string, decrement int64) error {
	return cacheIncrTokenQuota(key, -decrement)
}

func cacheSetTokenField(key string, field string, value string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHSetField(fmt.Sprintf("token:%s", key), field, value)
	if err != nil {
		return err
	}
	return nil
}

// CacheGetTokenByKey 从缓存中获取 token，如果缓存中不存在，则从数据库中获取
func cacheGetTokenByKey(key string) (*Token, error) {
	hmacKey := common.GenerateHMAC(key)
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	err := common.RedisHGetObj(fmt.Sprintf("token:%s", hmacKey), &token)
	if err != nil {
		return nil, err
	}
	token.Key = key
	return &token, nil
}
