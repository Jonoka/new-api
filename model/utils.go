package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int
var batchUpdateLocks []sync.Mutex
var userQuotaBatchApplyLock sync.Mutex
var tokenQuotaBatchApplyLock sync.Mutex
var applyTokenQuotaBatchDelta = applyTokenQuotaBatchDeltaDirect
var applyUserQuotaBatchDelta = applyUserQuotaBatchDeltaDirect

type pendingBalanceBatch struct {
	OperationID string
	Type        int
	ID          int
	Delta       int
}

var pendingBalanceBatches = struct {
	sync.Mutex
	operations map[string]pendingBalanceBatch
}{operations: make(map[string]pendingBalanceBatch)}

func init() {
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Duration(common.BatchUpdateInterval) * time.Second)
			batchUpdate()
		}
	})
}

func addNewRecord(type_ int, id int, value int) {
	batchUpdateLocks[type_].Lock()
	defer batchUpdateLocks[type_].Unlock()
	if _, ok := batchUpdateStores[type_][id]; !ok {
		batchUpdateStores[type_][id] = value
	} else {
		batchUpdateStores[type_][id] += value
	}
}

func parkPendingBalanceBatch(operation pendingBalanceBatch) {
	pendingBalanceBatches.Lock()
	pendingBalanceBatches.operations[operation.OperationID] = operation
	pendingBalanceBatches.Unlock()
	markBalanceCacheRepairInFlight(operation.OperationID)
}

func removePendingBalanceBatch(operation pendingBalanceBatch) {
	pendingBalanceBatches.Lock()
	if current, ok := pendingBalanceBatches.operations[operation.OperationID]; ok && current == operation {
		delete(pendingBalanceBatches.operations, operation.OperationID)
	}
	pendingBalanceBatches.Unlock()
	unmarkBalanceCacheRepairInFlight(operation.OperationID)
}

func snapshotPendingBalanceBatches(limit int, match func(pendingBalanceBatch) bool) []pendingBalanceBatch {
	if limit <= 0 {
		limit = 100
	}
	pendingBalanceBatches.Lock()
	defer pendingBalanceBatches.Unlock()
	operations := make([]pendingBalanceBatch, 0, limit)
	for _, operation := range pendingBalanceBatches.operations {
		if match != nil && !match(operation) {
			continue
		}
		operations = append(operations, operation)
		if len(operations) >= limit {
			break
		}
	}
	return operations
}

func restoreBalanceBatch(operation pendingBalanceBatch) {
	batchUpdateLocks[operation.Type].Lock()
	batchUpdateStores[operation.Type][operation.ID] += operation.Delta
	batchUpdateLocks[operation.Type].Unlock()
	removePendingBalanceBatch(operation)
}

func recoverPendingBalanceBatches(ctx context.Context, limit int) error {
	userQuotaBatchApplyLock.Lock()
	defer userQuotaBatchApplyLock.Unlock()
	tokenQuotaBatchApplyLock.Lock()
	defer tokenQuotaBatchApplyLock.Unlock()
	return recoverPendingBalanceBatchesLocked(ctx, limit, nil)
}

// recoverPendingBalanceBatchesLocked classifies parked legacy balance writes.
// Callers hold userQuotaBatchApplyLock, followed by tokenQuotaBatchApplyLock.
func recoverPendingBalanceBatchesLocked(ctx context.Context, limit int, match func(pendingBalanceBatch) bool) error {
	var firstErr error
	for _, operation := range snapshotPendingBalanceBatches(limit, match) {
		var repair BalanceCacheRepair
		query := DB.WithContext(ctx).
			Where("id = ? AND version = ?", operation.OperationID, balanceCacheRepairVersion).
			Limit(1).Find(&repair)
		if query.Error != nil {
			if firstErr == nil {
				firstErr = query.Error
			}
			continue
		}
		if query.RowsAffected == 0 {
			restoreBalanceBatch(operation)
			continue
		}
		removePendingBalanceBatch(operation)
		if err := repairBalanceCacheProjection(ctx, &repair); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func recoverPendingBalanceBatchesForReservationLocked(ctx context.Context, userID, tokenID int) error {
	match := func(operation pendingBalanceBatch) bool {
		return operation.Type == BatchUpdateTypeUserQuota && operation.ID == userID ||
			operation.Type == BatchUpdateTypeTokenQuota && operation.ID == tokenID
	}
	for {
		operations := snapshotPendingBalanceBatches(100, match)
		if len(operations) == 0 {
			return nil
		}
		for _, operation := range operations {
			operationID := operation.OperationID
			if err := recoverPendingBalanceBatchesLocked(ctx, 1, func(candidate pendingBalanceBatch) bool {
				return candidate.OperationID == operationID
			}); err != nil {
				return err
			}
		}
	}
}

func ConsumePendingUserQuotaDelta(id int, apply func(delta int) error) error {
	userQuotaBatchApplyLock.Lock()
	defer userQuotaBatchApplyLock.Unlock()
	tokenQuotaBatchApplyLock.Lock()
	defer tokenQuotaBatchApplyLock.Unlock()

	recoverCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := recoverPendingBalanceBatchesForReservationLocked(recoverCtx, id, 0)
	if err == nil {
		err = recoverPendingTaskSubmissionBatchesForReservationLocked(recoverCtx, id, 0)
	}
	cancel()
	if err != nil {
		return fmt.Errorf("reconcile pending user quota batch values: %w", err)
	}

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	delta := batchUpdateStores[BatchUpdateTypeUserQuota][id]
	if delta != 0 {
		delete(batchUpdateStores[BatchUpdateTypeUserQuota], id)
	}
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	if err := apply(delta); err != nil {
		if delta != 0 {
			batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
			batchUpdateStores[BatchUpdateTypeUserQuota][id] += delta
			batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
		}
		return err
	}
	return nil
}

func flushUserQuotaBatchUpdates() {
	userQuotaBatchApplyLock.Lock()
	defer userQuotaBatchApplyLock.Unlock()

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	store := batchUpdateStores[BatchUpdateTypeUserQuota]
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	for key, value := range store {
		err := applyUserQuotaBatchDelta(key, value)
		if unresolved := new(balanceMutationCommitUnresolvedError); errors.As(err, &unresolved) {
			parkPendingBalanceBatch(pendingBalanceBatch{
				OperationID: unresolved.OperationID,
				Type:        BatchUpdateTypeUserQuota,
				ID:          key,
				Delta:       value,
			})
		}
		if err != nil {
			if unresolved := new(balanceMutationCommitUnresolvedError); !errors.As(err, &unresolved) {
				addNewRecord(BatchUpdateTypeUserQuota, key, value)
			}
			common.SysLog("failed to batch update user quota: " + err.Error())
		}
	}
}

func flushTokenQuotaBatchUpdates() {
	tokenQuotaBatchApplyLock.Lock()
	defer tokenQuotaBatchApplyLock.Unlock()

	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	store := batchUpdateStores[BatchUpdateTypeTokenQuota]
	batchUpdateStores[BatchUpdateTypeTokenQuota] = make(map[int]int)
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()

	for key, value := range store {
		err := applyTokenQuotaBatchDelta(key, value)
		if unresolved := new(balanceMutationCommitUnresolvedError); errors.As(err, &unresolved) {
			parkPendingBalanceBatch(pendingBalanceBatch{
				OperationID: unresolved.OperationID,
				Type:        BatchUpdateTypeTokenQuota,
				ID:          key,
				Delta:       value,
			})
		}
		if err != nil {
			if unresolved := new(balanceMutationCommitUnresolvedError); !errors.As(err, &unresolved) {
				addNewRecord(BatchUpdateTypeTokenQuota, key, value)
			}
			common.SysLog("failed to batch update token quota: " + err.Error())
		}
	}
}

func batchUpdate() {
	if err := RecoverPendingTaskSubmissionBatches(context.Background(), 100); err != nil {
		common.SysLog("failed to reconcile pending task submission batch values: " + err.Error())
	}
	if err := RecoverBalanceCacheRepairs(context.Background(), 100); err != nil {
		common.SysLog("failed to reconcile balance cache repairs: " + err.Error())
	}
	// check if there's any data to update
	hasData := false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateLocks[i].Lock()
		if len(batchUpdateStores[i]) > 0 {
			hasData = true
			batchUpdateLocks[i].Unlock()
			break
		}
		batchUpdateLocks[i].Unlock()
	}

	if !hasData {
		return
	}

	common.SysLog("batch update started")
	for i := 0; i < BatchUpdateTypeCount; i++ {
		if i == BatchUpdateTypeUserQuota {
			flushUserQuotaBatchUpdates()
			continue
		}
		if i == BatchUpdateTypeTokenQuota {
			flushTokenQuotaBatchUpdates()
			continue
		}
		batchUpdateLocks[i].Lock()
		store := batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int)
		batchUpdateLocks[i].Unlock()
		// TODO: maybe we can combine updates with same key?
		for key, value := range store {
			switch i {
			case BatchUpdateTypeUserQuota:
				err := increaseUserQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update user quota: " + err.Error())
				}
			case BatchUpdateTypeUsedQuota:
				updateUserUsedQuota(key, value)
			case BatchUpdateTypeRequestCount:
				updateUserRequestCount(key, value)
			case BatchUpdateTypeChannelUsedQuota:
				updateChannelUsedQuota(key, value)
			}
		}
	}
	common.SysLog("batch update finished")
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
