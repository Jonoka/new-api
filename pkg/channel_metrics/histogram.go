package channelmetrics

import "math"

const (
	LatencyHistogramBucketCount = 13
	// InfiniteHistogramUpperBoundMs 表示直方图最后一个 +Inf 分箱。
	InfiniteHistogramUpperBoundMs int64 = -1
)

var latencyHistogramFiniteBounds = [...]int64{
	100,
	250,
	500,
	1_000,
	2_000,
	4_000,
	8_000,
	15_000,
	30_000,
	60_000,
	120_000,
	300_000,
}

// LatencyHistogram 使用固定、非累计分箱，兼容 SQLite、MySQL 和 PostgreSQL。
type LatencyHistogram struct {
	Counts [LatencyHistogramBucketCount]int64 `json:"counts"`
}

// LatencyHistogramBounds 返回 12 个有限上界；第 13 个分箱固定为 +Inf。
func LatencyHistogramBounds() []int64 {
	bounds := make([]int64, len(latencyHistogramFiniteBounds))
	copy(bounds, latencyHistogramFiniteBounds[:])
	return bounds
}

// Observe 把一个非负毫秒样本增加到恰好一个非累计分箱，并返回分箱下标。
func (h *LatencyHistogram) Observe(milliseconds int64) int {
	if milliseconds < 0 {
		return -1
	}
	index := len(latencyHistogramFiniteBounds)
	for i, upperBound := range latencyHistogramFiniteBounds {
		if milliseconds <= upperBound {
			index = i
			break
		}
	}
	h.Counts[index]++
	return index
}

func (h *LatencyHistogram) Merge(other LatencyHistogram) {
	for i, count := range other.Counts {
		h.Counts[i] += count
	}
}

func (h LatencyHistogram) Total() int64 {
	var total int64
	for _, count := range h.Counts {
		total += count
	}
	return total
}

// ApproxQuantile 返回命中分箱的上界。没有样本或 q 不在 (0,1] 时 ok=false；
// 命中最后一个分箱时返回 InfiniteHistogramUpperBoundMs。
func (h LatencyHistogram) ApproxQuantile(q float64) (upperBoundMs int64, ok bool) {
	total := h.Total()
	if total <= 0 || q <= 0 || q > 1 || math.IsNaN(q) {
		return 0, false
	}
	rank := int64(math.Ceil(q * float64(total)))
	var cumulative int64
	for index, count := range h.Counts {
		cumulative += count
		if cumulative < rank {
			continue
		}
		if index == len(latencyHistogramFiniteBounds) {
			return InfiniteHistogramUpperBoundMs, true
		}
		return latencyHistogramFiniteBounds[index], true
	}
	return 0, false
}
