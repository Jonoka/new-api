package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const balanceCacheRepairVersion = 1

// BalanceCacheRepair is a durable instruction to invalidate a derived cache.
// It intentionally contains no balance or delta and can never replay money.
type BalanceCacheRepair struct {
	ID            string `gorm:"size:36;primaryKey"`
	Version       int    `gorm:"not null;default:1"`
	UserID        *int   `gorm:"index"`
	TokenCacheKey string `gorm:"size:128;index"`
	CreatedAt     int64  `gorm:"autoCreateTime;index"`
	RepairedAt    int64  `gorm:"not null;default:0;index"`
}

type balanceMutationCommitUnresolvedError struct {
	OperationID string
	Cause       error
}

func (err *balanceMutationCommitUnresolvedError) Error() string {
	return fmt.Sprintf("balance mutation %s commit outcome is unresolved: %v", err.OperationID, err.Cause)
}

func (err *balanceMutationCommitUnresolvedError) Unwrap() error {
	return err.Cause
}

var balanceCacheRepairInFlight = struct {
	sync.Mutex
	operations map[string]struct{}
}{operations: make(map[string]struct{})}

var invalidateBalanceUserProjection = invalidateUserCache
var invalidateBalanceTokenProjection = invalidateTokenCacheDataKey

func newUserBalanceCacheRepair(operationID string, userID int) (*BalanceCacheRepair, error) {
	if strings.TrimSpace(operationID) == "" || userID <= 0 {
		return nil, errors.New("invalid user balance cache repair target")
	}
	return &BalanceCacheRepair{
		ID:      operationID,
		Version: balanceCacheRepairVersion,
		UserID:  &userID,
	}, nil
}

func newTokenBalanceCacheRepair(operationID, tokenKey string) (*BalanceCacheRepair, error) {
	if strings.TrimSpace(operationID) == "" || strings.TrimSpace(tokenKey) == "" {
		return nil, errors.New("invalid token balance cache repair target")
	}
	return &BalanceCacheRepair{
		ID:            operationID,
		Version:       balanceCacheRepairVersion,
		TokenCacheKey: tokenCacheRedisKey(tokenKey),
	}, nil
}

func validateBalanceCacheRepair(repair *BalanceCacheRepair) error {
	if repair == nil || strings.TrimSpace(repair.ID) == "" || repair.Version != balanceCacheRepairVersion {
		return errors.New("invalid balance cache repair")
	}
	hasUser := repair.UserID != nil && *repair.UserID > 0
	hasToken := strings.TrimSpace(repair.TokenCacheKey) != ""
	if hasUser == hasToken {
		return errors.New("balance cache repair must identify exactly one projection")
	}
	if hasToken && !strings.HasPrefix(repair.TokenCacheKey, "token:") {
		return errors.New("invalid token cache projection key")
	}
	return nil
}

func markBalanceCacheRepairInFlight(operationID string) {
	balanceCacheRepairInFlight.Lock()
	balanceCacheRepairInFlight.operations[operationID] = struct{}{}
	balanceCacheRepairInFlight.Unlock()
}

func unmarkBalanceCacheRepairInFlight(operationID string) {
	balanceCacheRepairInFlight.Lock()
	delete(balanceCacheRepairInFlight.operations, operationID)
	balanceCacheRepairInFlight.Unlock()
}

func isBalanceCacheRepairInFlight(operationID string) bool {
	balanceCacheRepairInFlight.Lock()
	_, ok := balanceCacheRepairInFlight.operations[operationID]
	balanceCacheRepairInFlight.Unlock()
	return ok
}

func balanceCacheRepairExists(ctx context.Context, repair *BalanceCacheRepair) (bool, error) {
	if err := validateBalanceCacheRepair(repair); err != nil {
		return false, err
	}
	var count int64
	query := DB.WithContext(ctx).Model(&BalanceCacheRepair{}).
		Where("id = ? AND version = ?", repair.ID, repair.Version)
	if repair.UserID != nil {
		query = query.Where("user_id = ? AND token_cache_key = ?", *repair.UserID, "")
	} else {
		query = query.Where("user_id IS NULL AND token_cache_key = ?", repair.TokenCacheKey)
	}
	err := query.Count(&count).Error
	return count == 1, err
}

func acknowledgeBalanceCacheRepair(ctx context.Context, repair *BalanceCacheRepair) error {
	query := DB.WithContext(ctx).Model(&BalanceCacheRepair{}).
		Where("id = ? AND version = ?", repair.ID, repair.Version)
	if repair.UserID != nil {
		query = query.Where("user_id = ? AND token_cache_key = ?", *repair.UserID, "")
	} else {
		query = query.Where("user_id IS NULL AND token_cache_key = ?", repair.TokenCacheKey)
	}
	result := query.Where("repaired_at = ?", 0).
		Update("repaired_at", common.GetTimestamp())
	if result.Error != nil || result.RowsAffected == 1 {
		return result.Error
	}
	var stored BalanceCacheRepair
	if err := DB.WithContext(ctx).First(&stored, "id = ? AND version = ?", repair.ID, repair.Version).Error; err != nil {
		return err
	}
	if stored.RepairedAt == 0 || !sameBalanceCacheRepairTarget(&stored, repair) {
		return errors.New("balance cache repair acknowledgement did not match observed state")
	}
	return nil
}

func sameBalanceCacheRepairTarget(left, right *BalanceCacheRepair) bool {
	if left == nil || right == nil || left.TokenCacheKey != right.TokenCacheKey {
		return false
	}
	if left.UserID == nil || right.UserID == nil {
		return left.UserID == nil && right.UserID == nil
	}
	return *left.UserID == *right.UserID
}

func repairBalanceCacheProjection(ctx context.Context, repair *BalanceCacheRepair) error {
	if err := validateBalanceCacheRepair(repair); err != nil {
		return err
	}
	if common.RedisEnabled {
		var err error
		if repair.UserID != nil {
			err = invalidateBalanceUserProjection(*repair.UserID)
		} else {
			err = invalidateBalanceTokenProjection(repair.TokenCacheKey)
		}
		if err != nil {
			return err
		}
	}
	return acknowledgeBalanceCacheRepair(ctx, repair)
}

func classifyBalanceMutationCommit(
	ctx context.Context,
	repair *BalanceCacheRepair,
	transactionErr error,
	keepUnresolved bool,
) error {
	committed, resolveErr := balanceCacheRepairExists(ctx, repair)
	if resolveErr != nil {
		if keepUnresolved {
			markBalanceCacheRepairInFlight(repair.ID)
		}
		return &balanceMutationCommitUnresolvedError{OperationID: repair.ID, Cause: errors.Join(transactionErr, resolveErr)}
	}
	if !committed {
		return transactionErr
	}
	if err := repairBalanceCacheProjection(ctx, repair); err != nil {
		common.SysLog("balance cache repair pending: " + err.Error())
	}
	return nil
}

func applyUserQuotaDeltaWithOperation(id, delta int, operationID string, keepUnresolved bool) error {
	if id <= 0 {
		return errors.New("invalid user id")
	}
	if delta == 0 {
		var user User
		return DB.Select("id").First(&user, "id = ?", id).Error
	}
	repair, err := newUserBalanceCacheRepair(operationID, id)
	if err != nil {
		return err
	}
	markBalanceCacheRepairInFlight(operationID)
	transactionBodyCompleted := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&User{}).Where("id = ?", id).
			Update("quota", gorm.Expr("quota + ?", delta))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("user %d: %w", id, gorm.ErrRecordNotFound)
		}
		if err := tx.Create(repair).Error; err != nil {
			return err
		}
		transactionBodyCompleted = true
		return nil
	})
	if err != nil {
		if !transactionBodyCompleted {
			unmarkBalanceCacheRepairInFlight(operationID)
			return err
		}
		classifiedErr := classifyBalanceMutationCommit(context.Background(), repair, err, keepUnresolved)
		if unresolved := new(balanceMutationCommitUnresolvedError); keepUnresolved && errors.As(classifiedErr, &unresolved) {
			return classifiedErr
		}
		unmarkBalanceCacheRepairInFlight(operationID)
		return classifiedErr
	}
	if err := repairBalanceCacheProjection(context.Background(), repair); err != nil {
		common.SysLog("balance cache repair pending: " + err.Error())
	}
	unmarkBalanceCacheRepairInFlight(operationID)
	return nil
}

func applyTokenQuotaDeltaWithOperation(id, delta int, operationID string, keepUnresolved bool) error {
	if id <= 0 {
		return errors.New("invalid token id")
	}
	var repair *BalanceCacheRepair
	markBalanceCacheRepairInFlight(operationID)
	transactionBodyCompleted := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var token Token
		if err := lockForUpdate(tx).Select([]string{"id", "key"}).First(&token, "id = ?", id).Error; err != nil {
			return err
		}
		if delta == 0 {
			transactionBodyCompleted = true
			return nil
		}
		var err error
		repair, err = newTokenBalanceCacheRepair(operationID, token.Key)
		if err != nil {
			return err
		}
		result := tx.Model(&Token{}).Where("id = ?", id).Updates(map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", delta),
			"used_quota":    gorm.Expr("used_quota - ?", delta),
			"accessed_time": common.GetTimestamp(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("token %d: %w", id, gorm.ErrRecordNotFound)
		}
		if err := tx.Create(repair).Error; err != nil {
			return err
		}
		transactionBodyCompleted = true
		return nil
	})
	if err != nil {
		if !transactionBodyCompleted || repair == nil {
			unmarkBalanceCacheRepairInFlight(operationID)
			return err
		}
		classifiedErr := classifyBalanceMutationCommit(context.Background(), repair, err, keepUnresolved)
		if unresolved := new(balanceMutationCommitUnresolvedError); keepUnresolved && errors.As(classifiedErr, &unresolved) {
			return classifiedErr
		}
		unmarkBalanceCacheRepairInFlight(operationID)
		return classifiedErr
	}
	if repair != nil {
		if err := repairBalanceCacheProjection(context.Background(), repair); err != nil {
			common.SysLog("balance cache repair pending: " + err.Error())
		}
	}
	unmarkBalanceCacheRepairInFlight(operationID)
	return nil
}

func applyUserQuotaDelta(id, delta int) error {
	return applyUserQuotaDeltaWithOperation(id, delta, uuid.NewString(), false)
}

func applyTokenQuotaDelta(id, delta int) error {
	return applyTokenQuotaDeltaWithOperation(id, delta, uuid.NewString(), false)
}

func applyUserQuotaBatchDeltaDirect(id, delta int) error {
	return applyUserQuotaDeltaWithOperation(id, delta, uuid.NewString(), true)
}

func applyTokenQuotaBatchDeltaDirect(id, delta int) error {
	return applyTokenQuotaDeltaWithOperation(id, delta, uuid.NewString(), true)
}

// RecoverBalanceCacheRepairs retries derived projection invalidation. It never
// reapplies a quota delta.
func RecoverBalanceCacheRepairs(ctx context.Context, limit int) error {
	if err := recoverPendingBalanceBatches(ctx, limit); err != nil {
		return err
	}
	if limit <= 0 {
		limit = 100
	}
	var repairs []BalanceCacheRepair
	if err := DB.WithContext(ctx).Where("repaired_at = ?", 0).
		Order("created_at, id").Limit(limit).Find(&repairs).Error; err != nil {
		return err
	}
	var firstErr error
	for i := range repairs {
		repair := &repairs[i]
		if isBalanceCacheRepairInFlight(repair.ID) {
			continue
		}
		if err := repairBalanceCacheProjection(ctx, repair); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
