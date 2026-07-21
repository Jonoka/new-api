package channelmetrics

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const defaultBucketLevel = "5m"

// Config 控制热桶边界和刷新生命周期。建议从 DefaultConfig 开始覆盖。
type Config struct {
	Enabled                      bool
	BucketLevel                  string
	BucketSeconds                int64
	FlushInterval                time.Duration
	FinalFlushTimeout            time.Duration
	MaxActiveDimensionsPerBucket int
	MaxHotBuckets                int
	ShardCount                   int
	SnapshotLimits               SnapshotLimits
	NodeID                       string
	Now                          func() time.Time
	OnFlushError                 func(error)
}

func DefaultConfig() Config {
	return Config{
		Enabled:                      true,
		BucketLevel:                  defaultBucketLevel,
		BucketSeconds:                300,
		FlushInterval:                20 * time.Second,
		FinalFlushTimeout:            5 * time.Second,
		MaxActiveDimensionsPerBucket: 10_000,
		MaxHotBuckets:                50_000,
		ShardCount:                   32,
		SnapshotLimits:               DefaultSnapshotLimits(),
	}
}

func (c Config) normalized() Config {
	defaults := DefaultConfig()
	if c.BucketLevel == "" {
		c.BucketLevel = defaults.BucketLevel
	}
	if c.BucketSeconds <= 0 {
		c.BucketSeconds = defaults.BucketSeconds
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = defaults.FlushInterval
	}
	if c.FinalFlushTimeout <= 0 {
		c.FinalFlushTimeout = defaults.FinalFlushTimeout
	}
	if c.MaxActiveDimensionsPerBucket <= 0 {
		c.MaxActiveDimensionsPerBucket = defaults.MaxActiveDimensionsPerBucket
	}
	if c.MaxHotBuckets <= 0 {
		c.MaxHotBuckets = defaults.MaxHotBuckets
	}
	if c.ShardCount <= 0 {
		c.ShardCount = defaults.ShardCount
	}
	if c.ShardCount > 256 {
		c.ShardCount = 256
	}
	c.SnapshotLimits = c.SnapshotLimits.normalized()
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.NodeID == "" {
		c.NodeID = randomNodeID()
	}
	c.NodeID = TruncateUTF8(c.NodeID, 64)
	return c
}

type bucketKey struct {
	bucketTs      int64
	dimensionHash string
}

type bucketEntry struct {
	dimension Dimension
	counters  MetricCounters
}

type bucketShard struct {
	mu      sync.Mutex
	buckets map[bucketKey]*bucketEntry
}

// Collector 是线程安全的分片热桶采集器。
type Collector struct {
	config Config
	sink   Sink
	shards []bucketShard

	// admissionMu 只保护新桶准入、drain 和 restore；已有桶的热路径仅锁单个分片。
	admissionMu              sync.Mutex
	activeDimensionsByBucket map[int64]int
	hotBucketCount           int

	flushMu sync.Mutex
	pending *MetricBatch

	batchSequence atomic.Uint64
	quality       qualityState
}

func NewCollector(config Config, sink Sink) *Collector {
	config = config.normalized()
	collector := &Collector{
		config:                   config,
		sink:                     sink,
		shards:                   make([]bucketShard, config.ShardCount),
		activeDimensionsByBucket: make(map[int64]int),
	}
	for index := range collector.shards {
		collector.shards[index].buckets = make(map[bucketKey]*bucketEntry)
	}
	return collector
}

// Record 按值复制并聚合样本。配置关闭时是无副作用的空操作。
func (c *Collector) Record(sample Sample) error {
	if !c.config.Enabled {
		return nil
	}
	sample = normalizeSample(sample)
	c.quality.receivedMetricEvents.Add(1)
	dimension, err := DimensionFromSample(sample, c.config.SnapshotLimits)
	if err != nil {
		c.quality.invalidSamples.add(1)
		return err
	}
	if sample.OccurredAt.IsZero() {
		sample.OccurredAt = c.config.Now()
	}
	bucketTs := floorBucket(sample.OccurredAt.Unix(), c.config.BucketSeconds)
	key := bucketKey{bucketTs: bucketTs, dimensionHash: DimensionHash(dimension)}

	if found, addErr := c.addToExisting(key, dimension, sample, false); found || addErr != nil {
		return c.finishRecord(found, addErr)
	}

	c.admissionMu.Lock()
	defer c.admissionMu.Unlock()

	// 在等待准入锁时可能已有并发请求创建了同一桶。
	if found, addErr := c.addToExisting(key, dimension, sample, false); found || addErr != nil {
		return c.finishRecord(found, addErr)
	}

	needsOverflow := c.activeDimensionsByBucket[bucketTs] >= c.config.MaxActiveDimensionsPerBucket ||
		c.hotBucketCount >= c.config.MaxHotBuckets
	if !needsOverflow {
		if err := c.addNewBucket(key, dimension, sample, false); err != nil {
			return c.finishRecord(false, err)
		}
		c.activeDimensionsByBucket[bucketTs]++
		c.hotBucketCount++
		c.quality.hotBuckets.Store(int64(c.hotBucketCount))
		return c.finishRecord(true, nil)
	}

	overflow := overflowDimension(dimension)
	overflowKey := bucketKey{bucketTs: bucketTs, dimensionHash: DimensionHash(overflow)}
	if found, addErr := c.addToExisting(overflowKey, overflow, sample, true); found || addErr != nil {
		if found {
			c.quality.dimensionOverflows.add(1)
		}
		return c.finishRecord(found, addErr)
	}
	if c.hotBucketCount >= c.config.MaxHotBuckets {
		c.quality.droppedMetricEvents.add(1)
		return ErrCollectorCapacity
	}
	if err := c.addNewBucket(overflowKey, overflow, sample, true); err != nil {
		return c.finishRecord(false, err)
	}
	c.hotBucketCount++
	c.quality.hotBuckets.Store(int64(c.hotBucketCount))
	c.quality.dimensionOverflows.add(1)
	return c.finishRecord(true, nil)
}

func (c *Collector) finishRecord(recorded bool, err error) error {
	if err != nil {
		if errors.Is(err, ErrDimensionHashCollision) {
			c.quality.hashCollisions.add(1)
		}
		return err
	}
	if recorded {
		c.quality.recordedMetricEvents.Add(1)
	}
	return nil
}

func (c *Collector) addToExisting(key bucketKey, dimension Dimension, sample Sample, overflowed bool) (bool, error) {
	shard := c.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	entry, exists := shard.buckets[key]
	if !exists {
		return false, nil
	}
	if entry.dimension != dimension {
		return false, fmt.Errorf("%w：hash=%s", ErrDimensionHashCollision, key.dimensionHash)
	}
	entry.counters.addSample(sample, overflowed)
	return true, nil
}

// 调用方必须持有 admissionMu。
func (c *Collector) addNewBucket(key bucketKey, dimension Dimension, sample Sample, overflowed bool) error {
	shard := c.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if existing, ok := shard.buckets[key]; ok {
		if existing.dimension != dimension {
			return fmt.Errorf("%w：hash=%s", ErrDimensionHashCollision, key.dimensionHash)
		}
		existing.counters.addSample(sample, overflowed)
		return nil
	}
	entry := &bucketEntry{dimension: dimension}
	entry.counters.addSample(sample, overflowed)
	shard.buckets[key] = entry
	return nil
}

func (c *Collector) shardFor(key bucketKey) *bucketShard {
	var value uint64 = uint64(key.bucketTs)
	for index := 0; index < len(key.dimensionHash) && index < 16; index++ {
		value = value*33 + uint64(key.dimensionHash[index])
	}
	return &c.shards[value%uint64(len(c.shards))]
}

// Drain 原子切走当前全部热桶，同时生成稳定 flush_id。
func (c *Collector) Drain() MetricBatch {
	if !c.config.Enabled {
		return MetricBatch{}
	}
	c.admissionMu.Lock()
	buckets := make([]Bucket, 0, c.hotBucketCount)
	for index := range c.shards {
		shard := &c.shards[index]
		shard.mu.Lock()
		for key, entry := range shard.buckets {
			buckets = append(buckets, Bucket{
				BucketLevel:   c.config.BucketLevel,
				BucketTs:      key.bucketTs,
				DimensionHash: key.dimensionHash,
				Dimension:     entry.dimension,
				Counters:      entry.counters,
			})
		}
		shard.buckets = make(map[bucketKey]*bucketEntry)
		shard.mu.Unlock()
	}
	c.hotBucketCount = 0
	c.activeDimensionsByBucket = make(map[int64]int)
	c.quality.hotBuckets.Store(0)
	c.admissionMu.Unlock()

	qualityDelta := c.quality.drainDelta()
	if len(buckets) == 0 && qualityDelta.Empty() {
		return MetricBatch{}
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].BucketTs == buckets[j].BucketTs {
			return buckets[i].DimensionHash < buckets[j].DimensionHash
		}
		return buckets[i].BucketTs < buckets[j].BucketTs
	})
	now := c.config.Now()
	return MetricBatch{
		ID:              c.nextBatchID(now),
		NodeID:          c.config.NodeID,
		CreatedAtUnixMs: now.UnixMilli(),
		Buckets:         buckets,
		Quality:         qualityDelta,
	}
}

// Restore 把确认未提交的批次合并回热桶。普通 Flush 失败会保留原批次重试，
// 不调用 Restore，从而避免“提交成功但响应超时”场景重复累计。
func (c *Collector) Restore(batch MetricBatch) error {
	if batch.Empty() {
		return nil
	}
	seen := make(map[bucketKey]Dimension, len(batch.Buckets))
	for _, bucket := range batch.Buckets {
		expectedHash := DimensionHash(bucket.Dimension)
		if bucket.DimensionHash != expectedHash {
			c.quality.hashCollisions.add(1)
			return fmt.Errorf("%w：批次中的维度哈希不匹配", ErrDimensionHashCollision)
		}
		key := bucketKey{bucketTs: bucket.BucketTs, dimensionHash: bucket.DimensionHash}
		if previous, ok := seen[key]; ok && previous != bucket.Dimension {
			c.quality.hashCollisions.add(1)
			return fmt.Errorf("%w：批次内存在相同哈希的不同维度", ErrDimensionHashCollision)
		}
		seen[key] = bucket.Dimension
	}

	c.admissionMu.Lock()
	defer c.admissionMu.Unlock()
	// 先完整检查，再执行合并，避免碰撞时只恢复半个批次。
	for key, dimension := range seen {
		shard := c.shardFor(key)
		shard.mu.Lock()
		entry, exists := shard.buckets[key]
		collision := exists && entry.dimension != dimension
		shard.mu.Unlock()
		if collision {
			c.quality.hashCollisions.add(1)
			return fmt.Errorf("%w：热桶中已有不同维度", ErrDimensionHashCollision)
		}
	}
	for _, bucket := range batch.Buckets {
		key := bucketKey{bucketTs: bucket.BucketTs, dimensionHash: bucket.DimensionHash}
		shard := c.shardFor(key)
		shard.mu.Lock()
		entry, exists := shard.buckets[key]
		if exists {
			entry.counters.Merge(bucket.Counters)
		} else {
			copyEntry := &bucketEntry{dimension: bucket.Dimension, counters: bucket.Counters}
			shard.buckets[key] = copyEntry
			c.hotBucketCount++
			if !bucket.Dimension.Overflowed {
				c.activeDimensionsByBucket[bucket.BucketTs]++
			}
		}
		shard.mu.Unlock()
	}
	c.quality.restoreDelta(batch.Quality)
	c.quality.hotBuckets.Store(int64(c.hotBucketCount))
	c.quality.restoredBatches.Add(1)
	return nil
}

// Flush 刷新一个批次。失败批次留在内存并使用同一 ID 重试。
func (c *Collector) Flush(ctx context.Context) error {
	if !c.config.Enabled {
		return nil
	}
	if c.sink == nil {
		return ErrSinkNotConfigured
	}
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	_, err := c.flushLocked(ctx)
	return err
}

// FlushAll 持续刷新，直到失败重试批次和当前热桶都已提交。
// 它主要用于进程关停；调用方应传入有超时上限的 context，避免持续写入时无限等待。
func (c *Collector) FlushAll(ctx context.Context) error {
	if !c.config.Enabled {
		return nil
	}
	if c.sink == nil {
		return ErrSinkNotConfigured
	}
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	for {
		flushed, err := c.flushLocked(ctx)
		if err != nil {
			return err
		}
		if !flushed {
			return nil
		}
	}
}

// flushLocked 刷新至多一个批次；调用方必须持有 flushMu。
// flushed=false 表示当前没有待提交数据。
func (c *Collector) flushLocked(ctx context.Context) (flushed bool, err error) {
	var batch MetricBatch
	if c.pending != nil {
		batch = c.pending.Clone()
	} else {
		batch = c.Drain()
		if batch.Empty() {
			return false, nil
		}
	}
	c.quality.flushAttempts.Add(1)
	if err := c.sink.Flush(ctx, batch.Clone()); err != nil {
		if c.pending == nil {
			pending := batch.Clone()
			c.pending = &pending
			c.quality.pendingBatches.Store(1)
		}
		c.quality.flushFailures.Add(1)
		c.quality.setFlushError(c.config.Now().Unix(), err)
		return true, err
	}
	c.pending = nil
	c.quality.pendingBatches.Store(0)
	c.quality.flushSuccesses.Add(1)
	c.quality.lastFlushedAt.Store(c.config.Now().Unix())
	return true, nil
}

// Run 启动定时刷新，并在上下文结束时执行一次有超时上限的最终刷新。
func (c *Collector) Run(ctx context.Context) error {
	if !c.config.Enabled {
		return nil
	}
	ticker := time.NewTicker(c.config.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := c.Flush(ctx); err != nil && c.config.OnFlushError != nil {
				c.config.OnFlushError(err)
			}
		case <-ctx.Done():
			finalContext, cancel := context.WithTimeout(context.Background(), c.config.FinalFlushTimeout)
			err := c.FlushAll(finalContext)
			cancel()
			return err
		}
	}
}

func (c *Collector) Quality() QualitySnapshot {
	return c.quality.snapshot()
}

// RecordDroppedFailureEvents 由有界失败明细队列在丢弃事件时调用。
func (c *Collector) RecordDroppedFailureEvents(count int64) {
	c.quality.droppedFailureEvents.add(count)
}

func floorBucket(timestamp int64, bucketSeconds int64) int64 {
	if bucketSeconds <= 0 {
		return timestamp
	}
	remainder := timestamp % bucketSeconds
	if remainder < 0 {
		remainder += bucketSeconds
	}
	return timestamp - remainder
}

func (c *Collector) nextBatchID(now time.Time) string {
	sequence := c.batchSequence.Add(1)
	// 持久层 flush_id 在三种数据库中统一为 64 字符；哈希原文仍包含
	// 实例 ID、纳秒时间和进程内序号，既稳定又不会因长 NodeID 超出字段上限。
	return SHA256String(fmt.Sprintf("%s\x00%d\x00%d", c.config.NodeID, now.UnixNano(), sequence))
}

func randomNodeID() string {
	var random [12]byte
	if _, err := cryptorand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("node-%d", time.Now().UnixNano())
}
