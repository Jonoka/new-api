package common

import "sync/atomic"

const (
	DefaultImageTaskDataRetentionHours = 1
	MaxImageTaskDataRetentionHours     = 24 * 365
)

var imageTaskDataRetentionHours atomic.Int64

func init() {
	imageTaskDataRetentionHours.Store(DefaultImageTaskDataRetentionHours)
}

// GetImageTaskDataRetentionHours 返回图片异步任务结果在数据库中的保留时长。
// 0 表示关闭自动清理。
func GetImageTaskDataRetentionHours() int {
	return int(imageTaskDataRetentionHours.Load())
}

// SetImageTaskDataRetentionHours 更新图片异步任务结果的保留时长。
// 参数应在配置层完成范围校验。
func SetImageTaskDataRetentionHours(hours int) {
	imageTaskDataRetentionHours.Store(int64(hours))
}
