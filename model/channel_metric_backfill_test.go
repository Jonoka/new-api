package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelMetricBackfillLogSnapshotQueries(t *testing.T) {
	db := newChannelMetricTestDB(t)
	require.NoError(t, db.AutoMigrate(&Log{}))
	logs := []Log{
		{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 1, RequestId: "request-a"},
		{CreatedAt: 110, Type: LogTypeError, ChannelId: 2, RequestId: "request-a"},
		{CreatedAt: 120, Type: LogTypeManage, ChannelId: 2, RequestId: "request-a"},
		{CreatedAt: 130, Type: LogTypeConsume, ChannelId: 0, RequestId: "request-b"},
		{CreatedAt: 200, Type: LogTypeConsume, ChannelId: 3, RequestId: "request-c"},
	}
	require.NoError(t, db.Create(&logs).Error)

	maxId, err := GetChannelMetricBackfillMaxLogId(db, 90, 150)
	require.NoError(t, err)
	assert.EqualValues(t, logs[1].Id, maxId)
	// 固定上界之后即使补写了旧时间戳日志，进度总数也必须保持同一快照。
	require.NoError(t, db.Create(&Log{CreatedAt: 115, Type: LogTypeConsume, ChannelId: 4, RequestId: "late-request"}).Error)
	totalRows, err := CountChannelMetricBackfillLogs(db, 90, 150, maxId)
	require.NoError(t, err)
	assert.EqualValues(t, 2, totalRows)

	rows, err := ListChannelMetricBackfillLogs(db, 90, 150, 0, maxId, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, LogTypeConsume, rows[0].Type)
	assert.Equal(t, LogTypeError, rows[1].Type)

	lastIds, err := GetChannelMetricBackfillLastLogIds(db, []string{"request-a", "request-a", ""}, int64(logs[4].Id), 190)
	require.NoError(t, err)
	assert.EqualValues(t, logs[1].Id, lastIds["request-a"])
}

func TestChannelMetricBackfillIgnoresViolationFeeAdjustment(t *testing.T) {
	db := newChannelMetricTestDB(t)
	require.NoError(t, db.AutoMigrate(&Log{}))
	logs := []Log{
		{CreatedAt: 100, Type: LogTypeError, ChannelId: 1, RequestId: "request-a", Content: "upstream rejected request"},
		{CreatedAt: 101, Type: LogTypeConsume, ChannelId: 1, RequestId: "request-a", Content: channelMetricBackfillViolationFeeContent},
	}
	require.NoError(t, db.Create(&logs).Error)

	maxId, err := GetChannelMetricBackfillMaxLogId(db, 90, 150)
	require.NoError(t, err)
	assert.EqualValues(t, logs[0].Id, maxId)

	rows, err := ListChannelMetricBackfillLogs(db, 90, 150, 0, maxId, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, logs[0].Id, rows[0].Id)

	lastIds, err := GetChannelMetricBackfillLastLogIds(db, []string{"request-a"}, int64(logs[1].Id), 150)
	require.NoError(t, err)
	assert.EqualValues(t, logs[0].Id, lastIds["request-a"])
}

func TestApplyChannelMetricBackfillBatchAdvancesCursorExactlyOnce(t *testing.T) {
	db := newChannelMetricTestDB(t)
	job := &ChannelMetricBackfillJob{
		JobId: channelMetricLegacyBackfillJobIdForTest, Status: ChannelMetricBackfillStatusRunning,
		BackfillStartTs: 100, LiveCutoverTs: 1000, MaxLogId: 2,
		CreatedAt: 10, UpdatedAt: 10,
	}
	require.NoError(t, EnsureChannelMetricBackfillJob(db, job))

	bucket := validChannelMetricBucket("9", 300)
	bucket.DataOrigin = "legacy"
	failure := ChannelFailureEvent{EventId: "legacy-failure-1", CreatedAt: 310, ChannelId: 7, TrafficSource: "relay", DataOrigin: "legacy", Outcome: "http_error"}
	job.CurrentCursor = 2
	job.ScannedRows = 2
	job.ConvertedRows = 2
	job.MetricBucketCount = 1
	job.FailureEventCount = 1
	job.Status = ChannelMetricBackfillStatusCompleted
	job.CompletedAt = 20
	job.UpdatedAt = 20

	applied, err := ApplyChannelMetricBackfillBatch(db, 0, job, []ChannelMetricBucket{bucket}, []ChannelFailureEvent{failure})
	require.NoError(t, err)
	assert.True(t, applied)

	applied, err = ApplyChannelMetricBackfillBatch(db, 0, job, []ChannelMetricBucket{bucket}, []ChannelFailureEvent{failure})
	require.NoError(t, err)
	assert.False(t, applied)

	var storedBucket ChannelMetricBucket
	require.NoError(t, db.Where("dimension_hash = ?", bucket.DimensionHash).Take(&storedBucket).Error)
	assert.EqualValues(t, bucket.EventCount, storedBucket.EventCount)
	var failureCount int64
	require.NoError(t, db.Model(&ChannelFailureEvent{}).Where("event_id = ?", failure.EventId).Count(&failureCount).Error)
	assert.EqualValues(t, 1, failureCount)
	failureRows, total, queryErr := QueryChannelFailureEvents(db, ChannelFailureEventFilter{DataOrigins: []string{"legacy"}})
	require.NoError(t, queryErr)
	assert.EqualValues(t, 1, total)
	require.Len(t, failureRows, 1)
	assert.Equal(t, "legacy", failureRows[0].DataOrigin)
	storedJob, err := GetChannelMetricBackfillJob(db, job.JobId)
	require.NoError(t, err)
	assert.EqualValues(t, 2, storedJob.CurrentCursor)
	assert.Equal(t, ChannelMetricBackfillStatusCompleted, storedJob.Status)
}

func TestApplyChannelMetricBackfillBatchRollsBackCursorOnInvalidBucket(t *testing.T) {
	db := newChannelMetricTestDB(t)
	job := &ChannelMetricBackfillJob{
		JobId: "rollback-job", Status: ChannelMetricBackfillStatusRunning,
		BackfillStartTs: 100, LiveCutoverTs: 1000, MaxLogId: 2,
		CreatedAt: 10, UpdatedAt: 10,
	}
	require.NoError(t, EnsureChannelMetricBackfillJob(db, job))
	job.CurrentCursor = 2
	invalid := validChannelMetricBucket("8", 300)
	invalid.DimensionHash = "invalid"

	applied, err := ApplyChannelMetricBackfillBatch(db, 0, job, []ChannelMetricBucket{invalid}, nil)
	assert.False(t, applied)
	require.Error(t, err)
	stored, getErr := GetChannelMetricBackfillJob(db, job.JobId)
	require.NoError(t, getErr)
	assert.Zero(t, stored.CurrentCursor)
}

const channelMetricLegacyBackfillJobIdForTest = "legacy-relay-logs-v1-test"
