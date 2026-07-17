package model

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHardDeleteUserPurgesAuthenticationDataWhenRedisFails(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(
		&TwoFA{},
		&TwoFABackupCode{},
		&PasskeyCredential{},
		&UserOAuthBinding{},
	))

	user := User{Username: "hard-delete-user", Password: "password"}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&Token{UserId: user.Id, Key: "hard-delete-token"}).Error)
	require.NoError(t, DB.Create(&TwoFA{UserId: user.Id, Secret: "secret", IsEnabled: true}).Error)
	require.NoError(t, DB.Create(&TwoFABackupCode{UserId: user.Id, CodeHash: "hash"}).Error)
	require.NoError(t, DB.Create(&PasskeyCredential{UserID: user.Id, CredentialID: "credential", PublicKey: "public-key"}).Error)
	require.NoError(t, DB.Create(&UserOAuthBinding{UserId: user.Id, ProviderId: 1, ProviderUserId: "provider-user"}).Error)

	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	var cacheInvalidatedAfterCommit atomic.Bool
	common.RDB = redis.NewClient(&redis.Options{
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			var count int64
			if err := DB.Unscoped().Model(&User{}).Where("id = ?", user.Id).Count(&count).Error; err == nil && count == 0 {
				cacheInvalidatedAfterCommit.Store(true)
			}
			return nil, errors.New("forced redis failure")
		},
		MaxRetries: -1,
	})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = oldRedisEnabled, oldRDB
		for _, record := range []any{
			&UserOAuthBinding{},
			&PasskeyCredential{},
			&TwoFABackupCode{},
			&TwoFA{},
			&Token{},
		} {
			DB.Unscoped().Where("user_id = ?", user.Id).Delete(record)
		}
		DB.Unscoped().Delete(&User{}, user.Id)
	})

	require.NoError(t, HardDeleteUserById(user.Id))
	assert.True(t, cacheInvalidatedAfterCommit.Load())

	var count int64
	require.NoError(t, DB.Unscoped().Model(&User{}).Where("id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count)
	for _, record := range []any{
		&Token{},
		&TwoFA{},
		&TwoFABackupCode{},
		&PasskeyCredential{},
		&UserOAuthBinding{},
	} {
		require.NoError(t, DB.Unscoped().Model(record).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Zero(t, count)
	}
}

func TestUserQueriesOmitAccessTokenUnlessExplicitlyRequested(t *testing.T) {
	truncateTables(t)

	accessToken := "sensitive-access-token"
	user := User{
		Username:    "access-token-query-user",
		Password:    "password",
		AccessToken: &accessToken,
	}
	require.NoError(t, DB.Create(&user).Error)

	users, total, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	assert.Nil(t, users[0].AccessToken)

	searchResults, searchTotal, err := SearchUsers(user.Username, "", nil, nil, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), searchTotal)
	require.Len(t, searchResults, 1)
	assert.Nil(t, searchResults[0].AccessToken)

	publicUser, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Nil(t, publicUser.AccessToken)

	fullUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.NotNil(t, fullUser.AccessToken)
	assert.Equal(t, accessToken, *fullUser.AccessToken)
}

func TestIncrementFailedAttemptsCountsConcurrentFailures(t *testing.T) {
	truncateTables(t)

	user := User{Username: "twofa-cas-user", Password: "password"}
	require.NoError(t, DB.Create(&user).Error)
	twoFA := TwoFA{UserId: user.Id, Secret: "secret", IsEnabled: true}
	require.NoError(t, DB.Create(&twoFA).Error)

	const attempts = 4
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- (&TwoFA{Id: twoFA.Id}).IncrementFailedAttempts()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var reloaded TwoFA
	require.NoError(t, DB.First(&reloaded, twoFA.Id).Error)
	assert.Equal(t, attempts, reloaded.FailedAttempts)
}

func TestValidateBackupCodeCanOnlySucceedOnce(t *testing.T) {
	truncateTables(t)

	const code = "ABCD-1234"
	require.NoError(t, CreateBackupCodes(123, []string{code}))

	const attempts = 2
	results := make(chan bool, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			valid, err := ValidateBackupCode(123, code)
			results <- valid
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	wins := 0
	for valid := range results {
		if valid {
			wins++
		}
	}
	assert.Equal(t, 1, wins)

	remaining, err := GetUnusedBackupCodeCount(123)
	require.NoError(t, err)
	assert.Zero(t, remaining)
}
