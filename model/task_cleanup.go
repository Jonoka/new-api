package model

import (
	"github.com/QuantumNous/new-api/constant"
)

// ClearExpiredImageTaskData 清空已过保留期的图片异步任务响应体。
// 任务、计费和状态等审计字段会继续保留；先选取主键再更新，以兼容
// SQLite、MySQL 和 PostgreSQL，并避免依赖各数据库不同的 UPDATE LIMIT 语法。
func ClearExpiredImageTaskData(cutoffUnix int64, limit int) (int64, error) {
	if cutoffUnix <= 0 || limit <= 0 {
		return 0, nil
	}

	var ids []int64
	err := DB.Model(&Task{}).
		Where("platform IN ?", constant.ImageTaskPlatforms()).
		Where("status IN ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).
		Where("finish_time > 0 AND finish_time <= ?", cutoffUnix).
		Where("data IS NOT NULL").
		// 从刚过期的数据开始向前清理，避免稳态运行时反复跨过大量
		// 已清空的历史记录后才找到新到期数据。
		Order("finish_time DESC, id DESC").
		Limit(limit).
		Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return 0, err
	}

	result := DB.Model(&Task{}).
		Where("id IN ?", ids).
		Where("platform IN ?", constant.ImageTaskPlatforms()).
		Where("status IN ?", []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).
		Where("finish_time > 0 AND finish_time <= ?", cutoffUnix).
		Where("data IS NOT NULL").
		UpdateColumn("data", nil)
	return result.RowsAffected, result.Error
}
