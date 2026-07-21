package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ChannelMetricBackfillStatusPending   = "pending"
	ChannelMetricBackfillStatusRunning   = "running"
	ChannelMetricBackfillStatusCompleted = "completed"
	ChannelMetricBackfillStatusFailed    = "failed"

	// 违规扣费会为失败请求额外写入一条消费日志，但它只是账单调整，
	// 不能作为成功调用参与指标聚合或最终请求判定。
	channelMetricBackfillViolationFeeContent = "Violation fee charged"
)

var ErrChannelMetricBackfillStaleCursor = errors.New("channel metric backfill cursor changed")

// EnsureChannelMetricBackfillJob 只创建不存在的任务，避免多实例启动时覆盖已有游标。
func EnsureChannelMetricBackfillJob(db *gorm.DB, job *ChannelMetricBackfillJob) error {
	if db == nil || job == nil || strings.TrimSpace(job.JobId) == "" {
		return ErrChannelMetricInvalidBatch
	}
	copyJob := *job
	now := time.Now().Unix()
	if copyJob.CreatedAt <= 0 {
		copyJob.CreatedAt = now
	}
	if copyJob.UpdatedAt <= 0 {
		copyJob.UpdatedAt = now
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "job_id"}},
		DoNothing: true,
	}).Create(&copyJob).Error
}

// ResumeChannelMetricBackfillJob 只恢复游标未变化的失败/待执行任务。
func ResumeChannelMetricBackfillJob(db *gorm.DB, jobId string, expectedCursor int64) (bool, error) {
	if db == nil || strings.TrimSpace(jobId) == "" || expectedCursor < 0 {
		return false, ErrChannelMetricInvalidBatch
	}
	result := db.Model(&ChannelMetricBackfillJob{}).
		Where("job_id = ? AND current_cursor = ? AND status IN ?", jobId, expectedCursor, []string{
			ChannelMetricBackfillStatusFailed, ChannelMetricBackfillStatusPending,
		}).
		Updates(map[string]interface{}{
			"status":     ChannelMetricBackfillStatusRunning,
			"last_error": "",
			"updated_at": time.Now().Unix(),
		})
	return result.RowsAffected == 1, result.Error
}

// GetEarliestLiveChannelMetricBucketTs 返回实时真实转发渠道尝试的最早桶起点。
// 历史回填只处理该时间点之前的日志，避免和实时聚合重复累计。
func GetEarliestLiveChannelMetricBucketTs(db *gorm.DB, bucketLevel string) (int64, error) {
	if db == nil || strings.TrimSpace(bucketLevel) == "" {
		return 0, ErrChannelMetricInvalidBatch
	}
	var timestamp int64
	err := db.Model(&ChannelMetricBucket{}).
		Where("bucket_level = ? AND metric_scope = ? AND traffic_source = ? AND data_origin = ?", bucketLevel, "channel_attempt", "relay", "live").
		Select("COALESCE(MIN(bucket_ts), 0)").Scan(&timestamp).Error
	return timestamp, err
}

// GetChannelMetricBackfillMaxLogId 冻结本轮可见日志上界。
func GetChannelMetricBackfillMaxLogId(db *gorm.DB, startTs int64, cutoverTs int64) (int64, error) {
	if db == nil || startTs <= 0 || cutoverTs <= startTs {
		return 0, ErrChannelMetricInvalidBatch
	}
	var maxId int64
	err := channelMetricBackfillLogQuery(db, startTs, cutoverTs).
		Select("COALESCE(MAX(id), 0)").Scan(&maxId).Error
	return maxId, err
}

func CountChannelMetricBackfillLogs(db *gorm.DB, startTs int64, cutoverTs int64, maxId int64) (int64, error) {
	if db == nil || startTs <= 0 || cutoverTs <= startTs || maxId < 0 {
		return 0, ErrChannelMetricInvalidBatch
	}
	var count int64
	err := channelMetricBackfillLogQuery(db, startTs, cutoverTs).
		Where("id <= ?", maxId).
		Count(&count).Error
	return count, err
}

// ListChannelMetricBackfillLogs 按主键游标读取固定快照内的日志。
func ListChannelMetricBackfillLogs(db *gorm.DB, startTs int64, cutoverTs int64, afterId int64, maxId int64, limit int) ([]Log, error) {
	if db == nil || startTs <= 0 || cutoverTs <= startTs || afterId < 0 || maxId < afterId || limit <= 0 {
		return nil, ErrChannelMetricInvalidBatch
	}
	var logs []Log
	err := channelMetricBackfillLogQuery(db, startTs, cutoverTs).
		Where("id > ? AND id <= ?", afterId, maxId).
		Order("id ASC").Limit(limit).Find(&logs).Error
	return logs, err
}

func channelMetricBackfillLogQuery(db *gorm.DB, startTs int64, cutoverTs int64) *gorm.DB {
	return db.Model(&Log{}).
		Where("created_at >= ? AND created_at < ?", startTs, cutoverTs).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
		Where("(content IS NULL OR content <> ?)", channelMetricBackfillViolationFeeContent).
		Where("channel_id > 0")
}

// GetChannelMetricBackfillLastLogIds 返回请求在固定快照内最后一条消费/错误日志，
// 用于从旧日志保守推导最终请求结果。空 request_id 不参与跨行合并。
func GetChannelMetricBackfillLastLogIds(db *gorm.DB, requestIds []string, maxId int64, cutoverTs int64) (map[string]int64, error) {
	result := make(map[string]int64)
	if db == nil || maxId < 0 || cutoverTs <= 0 {
		return nil, ErrChannelMetricInvalidBatch
	}
	unique := make([]string, 0, len(requestIds))
	seen := make(map[string]struct{}, len(requestIds))
	for _, requestId := range requestIds {
		requestId = strings.TrimSpace(requestId)
		if requestId == "" {
			continue
		}
		if _, exists := seen[requestId]; exists {
			continue
		}
		seen[requestId] = struct{}{}
		unique = append(unique, requestId)
	}
	if len(unique) == 0 {
		return result, nil
	}
	type requestLastLog struct {
		RequestId string `gorm:"column:request_id"`
		MaxId     int64  `gorm:"column:max_id"`
	}
	var rows []requestLastLog
	err := db.Model(&Log{}).
		Select("request_id, MAX(id) AS max_id").
		Where("request_id IN ? AND request_id <> ''", unique).
		Where("id <= ? AND created_at < ?", maxId, cutoverTs).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
		Where("(content IS NULL OR content <> ?)", channelMetricBackfillViolationFeeContent).
		Group("request_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.RequestId] = row.MaxId
	}
	return result, nil
}

// ApplyChannelMetricBackfillBatch 在同一事务中完成游标 CAS、指标累加和失败明细写入。
// applied=false 表示另一实例已经推进游标，调用方应重新读取任务而不能重复写入。
func ApplyChannelMetricBackfillBatch(db *gorm.DB, expectedCursor int64, job *ChannelMetricBackfillJob, buckets []ChannelMetricBucket, failures []ChannelFailureEvent) (applied bool, err error) {
	if db == nil || job == nil || strings.TrimSpace(job.JobId) == "" || expectedCursor < 0 || job.CurrentCursor <= expectedCursor || job.CurrentCursor > job.MaxLogId {
		return false, ErrChannelMetricInvalidBatch
	}
	for i := range failures {
		if strings.TrimSpace(failures[i].EventId) == "" || failures[i].CreatedAt <= 0 {
			return false, fmt.Errorf("%w: invalid failure event at index %d", ErrChannelMetricInvalidBatch, i)
		}
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"status":              job.Status,
			"current_cursor":      job.CurrentCursor,
			"scanned_rows":        job.ScannedRows,
			"converted_rows":      job.ConvertedRows,
			"skipped_rows":        job.SkippedRows,
			"metric_bucket_count": job.MetricBucketCount,
			"failure_event_count": job.FailureEventCount,
			"last_error":          job.LastError,
			"updated_at":          job.UpdatedAt,
			"completed_at":        job.CompletedAt,
		}
		claim := tx.Model(&ChannelMetricBackfillJob{}).
			Where("job_id = ? AND current_cursor = ?", job.JobId, expectedCursor).
			Updates(updates)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return ErrChannelMetricBackfillStaleCursor
		}

		for i := range buckets {
			bucket := buckets[i]
			if err := normalizeAndVerifyChannelMetricBucket(tx, &bucket); err != nil {
				return fmt.Errorf("channel metric backfill bucket %d: %w", i, err)
			}
			if err := upsertChannelMetricBucketIncrement(tx, &bucket); err != nil {
				return err
			}
		}
		return InsertChannelFailureEvents(tx, failures)
	})
	if errors.Is(err, ErrChannelMetricBackfillStaleCursor) {
		return false, nil
	}
	return err == nil, err
}

// MarkChannelMetricBackfillFailed 仅在游标未被其他实例推进时记录错误。
func MarkChannelMetricBackfillFailed(db *gorm.DB, jobId string, expectedCursor int64, message string) error {
	if db == nil || strings.TrimSpace(jobId) == "" || expectedCursor < 0 {
		return ErrChannelMetricInvalidBatch
	}
	message = strings.TrimSpace(message)
	return db.Model(&ChannelMetricBackfillJob{}).
		Where("job_id = ? AND current_cursor = ?", jobId, expectedCursor).
		Updates(map[string]interface{}{
			"status":     ChannelMetricBackfillStatusFailed,
			"last_error": message,
			"updated_at": time.Now().Unix(),
		}).Error
}
