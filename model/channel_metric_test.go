package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newChannelMetricTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, MigrateChannelAnalyticsLogDB(db))
	return db
}

func validChannelMetricBucket(hashCharacter string, bucketTs int64) ChannelMetricBucket {
	return ChannelMetricBucket{
		BucketLevel:           "5m",
		BucketTs:              bucketTs,
		DimensionHash:         strings.Repeat(hashCharacter, 64),
		DimensionVersion:      ChannelMetricDimensionVersion,
		MetricScope:           "channel_attempt",
		ChannelPresent:        true,
		ChannelId:             7,
		ChannelNameSnapshot:   "测试渠道",
		ChannelNameHash:       strings.Repeat("c", 64),
		ChannelType:           1,
		RequestedModelPresent: true,
		RequestedModel:        "gpt-4.1",
		RequestedModelHash:    strings.Repeat("a", 64),
		UpstreamModelPresent:  true,
		UpstreamModel:         "gpt-4.1-2025-04-14",
		UpstreamModelHash:     strings.Repeat("b", 64),
		Group:                 "default",
		GroupHash:             strings.Repeat("d", 64),
		TrafficSource:         "relay",
		DataOrigin:            "live",
		Outcome:               "success",
		QualityEligible:       true,
		UpstreamStatusPresent: true,
		UpstreamStatusCode:    200,
		EventCount:            2,
		SuccessCount:          2,
		QualityEligibleCount:  2,
		QualitySuccessCount:   2,
		UsageSampleCount:      2,
		InputTokensTotal:      30,
		OutputTokens:          10,
		LatencySumMs:          800,
		LatencyCount:          2,
		LatencyBucket500Ms:    2,
		TtftSumMs:             400,
		TtftCount:             2,
		TtftBucket250Ms:       2,
	}
}

func TestMigrateChannelAnalyticsLogDBCreatesFactTablesAndIndexes(t *testing.T) {
	db := newChannelMetricTestDB(t)

	assert.True(t, db.Migrator().HasTable(&ChannelMetricBucket{}))
	assert.True(t, db.Migrator().HasTable(&ChannelFailureEvent{}))
	assert.True(t, db.Migrator().HasTable(&ChannelMetricFlush{}))
	assert.True(t, db.Migrator().HasTable(&ChannelMetricBackfillJob{}))
	assert.True(t, db.Migrator().HasIndex(&ChannelMetricBucket{}, "ux_cmb_identity"))
	assert.True(t, db.Migrator().HasIndex(&ChannelMetricBucket{}, "idx_cmb_scope_time"))
	assert.True(t, db.Migrator().HasIndex(&ChannelFailureEvent{}, "idx_cfe_channel_time"))

	// AutoMigrate 必须可重复执行，便于主节点重启。
	require.NoError(t, MigrateChannelAnalyticsLogDB(db))
}

func TestInitLogDBMigratesFactsOnFinalSharedHandle(t *testing.T) {
	primary, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	staleLogDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	oldDB, oldLogDB, oldMaster := DB, LOG_DB, common.IsMasterNode
	DB, LOG_DB, common.IsMasterNode = primary, staleLogDB, true
	t.Cleanup(func() {
		DB, LOG_DB, common.IsMasterNode = oldDB, oldLogDB, oldMaster
	})
	t.Setenv("LOG_SQL_DSN", "")

	require.NoError(t, InitLogDB())
	assert.Same(t, primary, LOG_DB)
	assert.True(t, primary.Migrator().HasTable(&ChannelMetricBucket{}))
	assert.True(t, primary.Migrator().HasTable(&ChannelFailureEvent{}))
	assert.False(t, staleLogDB.Migrator().HasTable(&ChannelMetricBucket{}))
}

func TestFlushChannelMetricsIsIdempotent(t *testing.T) {
	db := newChannelMetricTestDB(t)
	bucket := validChannelMetricBucket("1", 300)
	failure := ChannelFailureEvent{
		EventId:               "failure-1",
		CreatedAt:             310,
		RequestId:             "request-1",
		AttemptSeq:            1,
		ChannelId:             7,
		ChannelNameSnapshot:   "测试渠道",
		ChannelType:           1,
		TrafficSource:         "relay",
		Outcome:               "http_error",
		FailureOwner:          "channel",
		QualityEligible:       true,
		UpstreamStatusPresent: true,
		UpstreamStatusCode:    429,
		MaskedErrorSummary:    "rate limited",
	}
	flush := &ChannelMetricFlush{
		FlushId: "flush-1", InstanceId: "node-1", BatchCreatedAt: 320,
		InvalidSampleCount: 2, DimensionOverflowCount: 3, DroppedMetricEventCount: 4,
	}

	applied, err := FlushChannelMetrics(db, flush, []ChannelMetricBucket{bucket}, []ChannelFailureEvent{failure})
	require.NoError(t, err)
	assert.True(t, applied)

	applied, err = FlushChannelMetrics(db, flush, []ChannelMetricBucket{bucket}, []ChannelFailureEvent{failure})
	require.NoError(t, err)
	assert.False(t, applied)
	conflictingFlush := *flush
	conflictingFlush.BatchCreatedAt++
	_, err = FlushChannelMetrics(db, &conflictingFlush, []ChannelMetricBucket{bucket}, nil)
	require.ErrorIs(t, err, ErrChannelMetricInvalidBatch)

	// 失败明细队列可以独立使用同一 event_id 重试。
	require.NoError(t, InsertChannelFailureEvents(db, []ChannelFailureEvent{failure}))

	var stored ChannelMetricBucket
	require.NoError(t, db.First(&stored).Error)
	assert.EqualValues(t, 2, stored.EventCount)
	assert.EqualValues(t, 30, stored.InputTokensTotal)
	assert.EqualValues(t, 2, stored.LatencyBucket500Ms)

	var failureCount int64
	require.NoError(t, db.Model(&ChannelFailureEvent{}).Count(&failureCount).Error)
	assert.EqualValues(t, 1, failureCount)

	var flushCount int64
	require.NoError(t, db.Model(&ChannelMetricFlush{}).Count(&flushCount).Error)
	assert.EqualValues(t, 1, flushCount)
	quality, err := GetChannelMetricDataQuality(db, 300, 400)
	require.NoError(t, err)
	assert.EqualValues(t, 2, quality.InvalidSampleCount)
	assert.EqualValues(t, 3, quality.DimensionOverflowCount)
	assert.EqualValues(t, 4, quality.DroppedMetricEventCount)
}

func TestChannelMetricIncrementAndOverwriteUseDifferentSemantics(t *testing.T) {
	db := newChannelMetricTestDB(t)
	bucket := validChannelMetricBucket("2", 600)

	require.NoError(t, UpsertChannelMetricBucketIncrement(db, &bucket))
	require.NoError(t, UpsertChannelMetricBucketIncrement(db, &bucket))

	var stored ChannelMetricBucket
	require.NoError(t, db.First(&stored).Error)
	assert.EqualValues(t, 4, stored.EventCount)
	assert.EqualValues(t, 60, stored.InputTokensTotal)

	overwrite := bucket
	overwrite.EventCount = 9
	overwrite.SuccessCount = 8
	overwrite.InputTokensTotal = 123
	overwrite.LatencyBucket500Ms = 7
	require.NoError(t, UpsertChannelMetricBucketOverwrite(db, &overwrite))

	require.NoError(t, db.First(&stored).Error)
	assert.EqualValues(t, 9, stored.EventCount)
	assert.EqualValues(t, 8, stored.SuccessCount)
	assert.EqualValues(t, 123, stored.InputTokensTotal)
	assert.EqualValues(t, 7, stored.LatencyBucket500Ms)
}

func TestFlushChannelMetricsRejectsDimensionHashCollisionAndRollsBack(t *testing.T) {
	db := newChannelMetricTestDB(t)
	first := validChannelMetricBucket("3", 900)
	applied, err := FlushChannelMetrics(db,
		&ChannelMetricFlush{FlushId: "flush-collision-1", InstanceId: "node-1", BatchCreatedAt: 910},
		[]ChannelMetricBucket{first}, nil)
	require.NoError(t, err)
	assert.True(t, applied)

	conflict := first
	conflict.BucketTs = 1200
	conflict.Outcome = "http_error"
	_, err = FlushChannelMetrics(db,
		&ChannelMetricFlush{FlushId: "flush-collision-2", InstanceId: "node-1", BatchCreatedAt: 1210},
		[]ChannelMetricBucket{conflict}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrChannelMetricHashCollision))

	var bucketCount int64
	require.NoError(t, db.Model(&ChannelMetricBucket{}).Count(&bucketCount).Error)
	assert.EqualValues(t, 1, bucketCount)
	var flushCount int64
	require.NoError(t, db.Model(&ChannelMetricFlush{}).Count(&flushCount).Error)
	assert.EqualValues(t, 1, flushCount, "碰撞批次的 flush 记录必须与指标一起回滚")
}

func TestChannelMetricCollisionCheckIgnoresDisplaySnapshotChanges(t *testing.T) {
	db := newChannelMetricTestDB(t)
	first := validChannelMetricBucket("9", 900)
	require.NoError(t, UpsertChannelMetricBucketOverwrite(db, &first))

	// 展示快照不参与维度哈希；调整快照长度后，同一真实维度仍应继续写入。
	second := first
	second.BucketTs = 1200
	second.ChannelNameSnapshot = "测试渠道~short-hash"
	second.RequestedModel = "gpt-4.1~short-hash"
	second.UpstreamModel = "gpt-4.1~short-hash"
	second.Group = "default~short-hash"
	require.NoError(t, UpsertChannelMetricBucketOverwrite(db, &second))

	var count int64
	require.NoError(t, db.Model(&ChannelMetricBucket{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

func TestChannelMetricQueriesUseHalfOpenRangeAndStatusPresence(t *testing.T) {
	db := newChannelMetricTestDB(t)
	first := validChannelMetricBucket("4", 300)
	first.UpstreamStatusPresent = false
	first.UpstreamStatusCode = 0
	second := validChannelMetricBucket("5", 600)
	second.UpstreamStatusCode = 0
	require.NoError(t, UpsertChannelMetricBucketOverwrite(db, &first))
	require.NoError(t, UpsertChannelMetricBucketOverwrite(db, &second))

	rows, err := QueryChannelMetricBuckets(db, ChannelMetricBucketFilter{
		StartTs:      300,
		EndTs:        600,
		BucketLevel:  "5m",
		MetricScopes: []string{"channel_attempt"},
		Groups:       []string{"default"},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 300, rows[0].BucketTs)

	rows, err = QueryChannelMetricBuckets(db, ChannelMetricBucketFilter{
		StartTs:             1,
		EndTs:               901,
		UpstreamStatusCodes: []int{0},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 600, rows[0].BucketTs)
}

func TestAggregateChannelMetricStatusCodesGroupsPortableColumns(t *testing.T) {
	db := newChannelMetricTestDB(t)
	first := validChannelMetricBucket("a", 300)
	first.MetricScope = "upstream_call"
	first.EventCount = 2
	first.UpstreamStatusCode = 200
	second := validChannelMetricBucket("b", 600)
	second.MetricScope = "upstream_call"
	second.EventCount = 3
	second.UpstreamStatusCode = 200
	require.NoError(t, UpsertChannelMetricBucketOverwrite(db, &first))
	require.NoError(t, UpsertChannelMetricBucketOverwrite(db, &second))

	rows, err := AggregateChannelMetricStatusCodes(db, ChannelMetricBucketFilter{
		StartTs:        300,
		EndTs:          900,
		BucketLevel:    "5m",
		MetricScopes:   []string{"upstream_call"},
		TrafficSources: []string{"relay"},
		DataOrigins:    []string{"live"},
	}, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].StatusPresent)
	assert.Equal(t, 200, rows[0].StatusCode)
	assert.EqualValues(t, 5, rows[0].EventCount)
	assert.EqualValues(t, 4, rows[0].TtftCount)
	assert.EqualValues(t, 800, rows[0].TtftSumMs)
	assert.EqualValues(t, 4, rows[0].TtftHistogram()[1])

	rows, err = AggregateChannelMetricStatusCodesByChannel(db, ChannelMetricBucketFilter{
		StartTs:        300,
		EndTs:          900,
		BucketLevel:    "5m",
		MetricScopes:   []string{"upstream_call"},
		TrafficSources: []string{"relay"},
		DataOrigins:    []string{"live"},
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 7, rows[0].ChannelId)
	assert.Equal(t, 200, rows[0].StatusCode)
	assert.EqualValues(t, 5, rows[0].EventCount)

	rows, err = AggregateChannelMetricStatusCodesByModel(db, ChannelMetricBucketFilter{
		StartTs:        300,
		EndTs:          900,
		BucketLevel:    "5m",
		MetricScopes:   []string{"upstream_call"},
		TrafficSources: []string{"relay"},
		DataOrigins:    []string{"live"},
	}, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, strings.Repeat("a", 64), rows[0].RequestedModelHash)
	assert.Equal(t, 200, rows[0].StatusCode)
	assert.EqualValues(t, 5, rows[0].EventCount)
}

func TestChannelMetricBoundedCleanup(t *testing.T) {
	db := newChannelMetricTestDB(t)
	for i, hash := range []string{"6", "7", "8"} {
		bucket := validChannelMetricBucket(hash, int64((i+1)*300))
		if i == 1 {
			bucket.BucketLevel = "1h"
		}
		require.NoError(t, UpsertChannelMetricBucketOverwrite(db, &bucket))
	}

	deleted, err := DeleteChannelMetricBucketsBefore(db, 1000, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)
	var count int64
	require.NoError(t, db.Model(&ChannelMetricBucket{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestChannelMetricBackfillCheckpointIsUpserted(t *testing.T) {
	db := newChannelMetricTestDB(t)
	job := &ChannelMetricBackfillJob{
		JobId: "backfill-1", Status: "running", LiveCutoverTs: 1000,
		MaxLogId: 500, CurrentCursor: 100, CreatedAt: 10, UpdatedAt: 20,
	}
	require.NoError(t, SaveChannelMetricBackfillJob(db, job))
	job.CurrentCursor = 200
	job.UpdatedAt = 30
	require.NoError(t, SaveChannelMetricBackfillJob(db, job))

	stored, err := GetChannelMetricBackfillJob(db, job.JobId)
	require.NoError(t, err)
	assert.EqualValues(t, 200, stored.CurrentCursor)
	assert.EqualValues(t, 10, stored.CreatedAt)
	assert.EqualValues(t, 30, stored.UpdatedAt)
}

func TestChannelMetricHistogramHelpers(t *testing.T) {
	assert.Equal(t, -1, ChannelMetricHistogramBucketIndex(-1))
	assert.Equal(t, 0, ChannelMetricHistogramBucketIndex(100))
	assert.Equal(t, 1, ChannelMetricHistogramBucketIndex(101))
	assert.Equal(t, 12, ChannelMetricHistogramBucketIndex(300001))

	values := [ChannelMetricHistogramBuckets]int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	var bucket ChannelMetricBucket
	bucket.SetLatencyHistogram(values)
	bucket.SetTtftHistogram(values)
	assert.Equal(t, values, bucket.LatencyHistogram())
	assert.Equal(t, values, bucket.TtftHistogram())
}
