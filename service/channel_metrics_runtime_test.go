package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPersistChannelFailureEventsCancelsBlockedWrite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelFailureEvent{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:block_until_cancel", func(tx *gorm.DB) {
		cancel()
		select {
		case <-tx.Statement.Context.Done():
			tx.AddError(tx.Statement.Context.Err())
		case <-time.After(time.Second):
			tx.AddError(context.DeadlineExceeded)
		}
	}))

	startedAt := time.Now()
	err = persistChannelFailureEvents(ctx, db, []model.ChannelFailureEvent{{
		EventId:   "cancelled-write",
		CreatedAt: time.Now().Unix(),
	}})
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
}

func TestDeleteChannelMetricBatchesSignalsCatchUpAtLimit(t *testing.T) {
	calls := 0
	needsCatchUp := deleteChannelMetricBatches("test backlog", func() (int64, error) {
		calls++
		return model.ChannelMetricMaxDeleteBatch, nil
	})

	require.True(t, needsCatchUp)
	require.Equal(t, channelMetricCleanupMaxBatches, calls)
}

func TestDeleteChannelMetricBatchesStopsAfterPartialBatch(t *testing.T) {
	calls := 0
	needsCatchUp := deleteChannelMetricBatches("test drained", func() (int64, error) {
		calls++
		if calls < 3 {
			return model.ChannelMetricMaxDeleteBatch, nil
		}
		return model.ChannelMetricMaxDeleteBatch - 1, nil
	})

	require.False(t, needsCatchUp)
	require.Equal(t, 3, calls)
}
