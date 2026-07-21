package channelmetrics

import "testing"

func TestLatencyHistogramUsesFixedNonCumulativeBuckets(t *testing.T) {
	var histogram LatencyHistogram
	values := []int64{0, 100, 101, 250, 300_000, 300_001}
	wantIndexes := []int{0, 0, 1, 1, 11, 12}
	for index, value := range values {
		if got := histogram.Observe(value); got != wantIndexes[index] {
			t.Fatalf("%dms 的分箱下标 = %d，期望 %d", value, got, wantIndexes[index])
		}
	}
	if got := histogram.Observe(-1); got != -1 {
		t.Fatalf("负延迟分箱下标 = %d，期望 -1", got)
	}
	if histogram.Total() != int64(len(values)) {
		t.Fatalf("直方图样本数 = %d，期望 %d", histogram.Total(), len(values))
	}
	if histogram.Counts[0] != 2 || histogram.Counts[1] != 2 || histogram.Counts[11] != 1 || histogram.Counts[12] != 1 {
		t.Fatalf("直方图不是非累计固定分箱：%v", histogram.Counts)
	}

	if value, ok := histogram.ApproxQuantile(0.5); !ok || value != 250 {
		t.Fatalf("P50 = (%d, %v)，期望 (250, true)", value, ok)
	}
	if value, ok := histogram.ApproxQuantile(1); !ok || value != InfiniteHistogramUpperBoundMs {
		t.Fatalf("P100 = (%d, %v)，期望 +Inf 标记", value, ok)
	}
	if _, ok := (LatencyHistogram{}).ApproxQuantile(0.95); ok {
		t.Fatal("空直方图不应返回百分位")
	}
}

func TestLatencyHistogramBoundsReturnsCopy(t *testing.T) {
	bounds := LatencyHistogramBounds()
	if len(bounds) != LatencyHistogramBucketCount-1 || bounds[0] != 100 || bounds[len(bounds)-1] != 300_000 {
		t.Fatalf("固定分箱边界异常：%v", bounds)
	}
	bounds[0] = 1
	if LatencyHistogramBounds()[0] != 100 {
		t.Fatal("调用方不应能修改全局固定分箱")
	}
}
