package model

import (
	"context"
	"errors"
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
var applyTokenQuotaBatchDelta = increaseTokenQuota

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

func ConsumePendingUserQuotaDelta(id int, apply func(delta int) error) error {
	userQuotaBatchApplyLock.Lock()
	defer userQuotaBatchApplyLock.Unlock()

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
		err := increaseUserQuota(key, value)
		if err != nil {
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
		if err := applyTokenQuotaBatchDelta(key, value); err != nil {
			common.SysLog("failed to batch update token quota: " + err.Error())
		}
	}
}

func batchUpdate() {
	if err := RecoverPendingTaskSubmissionBatches(context.Background(), 100); err != nil {
		common.SysLog("failed to reconcile pending task submission batch values: " + err.Error())
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
