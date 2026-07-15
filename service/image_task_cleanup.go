package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	imageTaskDataCleanupInterval   = time.Minute
	imageTaskDataCleanupBatchSize  = 50
	imageTaskDataCleanupMaxBatches = 1
)

var (
	imageTaskDataCleanupOnce    sync.Once
	imageTaskDataCleanupRunning atomic.Bool
)

// StartImageTaskDataCleanupTask 启动图片异步任务响应体清理任务。
// 分布式部署仅由主节点执行，避免多个实例重复扫描和更新同一批记录。
func StartImageTaskDataCleanupTask() {
	imageTaskDataCleanupOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf(
				"image task data cleanup started: tick=%s", imageTaskDataCleanupInterval,
			))
			ticker := time.NewTicker(imageTaskDataCleanupInterval)
			defer ticker.Stop()

			runImageTaskDataCleanupOnce()
			for range ticker.C {
				runImageTaskDataCleanupOnce()
			}
		})
	})
}

func runImageTaskDataCleanupOnce() {
	retentionHours := common.GetImageTaskDataRetentionHours()
	if retentionHours <= 0 || !imageTaskDataCleanupRunning.CompareAndSwap(false, true) {
		return
	}
	defer imageTaskDataCleanupRunning.Store(false)

	cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour).Unix()
	var cleared int64
	for range imageTaskDataCleanupMaxBatches {
		count, err := model.ClearExpiredImageTaskData(cutoff, imageTaskDataCleanupBatchSize)
		if err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("image task data cleanup failed: %v", err))
			return
		}
		cleared += count
		if count < imageTaskDataCleanupBatchSize {
			break
		}
	}
	if cleared > 0 {
		logger.LogInfo(context.Background(), fmt.Sprintf("image task data cleanup cleared %d records", cleared))
	}
}
