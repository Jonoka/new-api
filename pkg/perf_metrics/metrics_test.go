package perfmetrics

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestBuildModelSummariesMergesBucketsAndKeepsWeightedTotals(t *testing.T) {
	modelBuckets := map[string]map[int64]counters{}

	// 模拟同一模型、同一时间桶中来自持久层和热桶的数据。
	mergeModelBucket(modelBuckets, "model-a", 200, counters{
		requestCount:   2,
		successCount:   1,
		totalLatencyMs: 200,
		ttftSumMs:      50,
		ttftCount:      1,
		outputTokens:   20,
		generationMs:   1000,
	})
	mergeModelBucket(modelBuckets, "model-a", 200, counters{
		requestCount:   2,
		successCount:   2,
		totalLatencyMs: 600,
		ttftSumMs:      150,
		ttftCount:      1,
		outputTokens:   30,
		generationMs:   1000,
	})
	mergeModelBucket(modelBuckets, "model-a", 100, counters{
		requestCount:   1,
		successCount:   1,
		totalLatencyMs: 1000,
		ttftSumMs:      300,
		ttftCount:      1,
		outputTokens:   10,
		generationMs:   500,
	})
	mergeModelBucket(modelBuckets, "model-b", 100, counters{
		requestCount:   6,
		successCount:   3,
		totalLatencyMs: 600,
		outputTokens:   60,
		generationMs:   3000,
	})

	models := buildModelSummaries(modelBuckets)
	if len(models) != 2 {
		t.Fatalf("模型数量 = %d，期望 2", len(models))
	}
	if models[0].ModelName != "model-b" || models[0].RequestCount != 6 {
		t.Fatalf("首个模型 = %+v，期望按请求量降序排列 model-b", models[0])
	}

	got := models[1]
	if got.ModelName != "model-a" {
		t.Fatalf("第二个模型 = %q，期望 model-a", got.ModelName)
	}
	if got.RequestCount != 5 || got.AvgLatencyMs != 360 || got.SuccessRate != 80 || got.AvgTps != 24 {
		t.Fatalf("model-a 顶层汇总 = %+v，未按原始请求计数加权", got)
	}
	if len(got.Series) != 2 {
		t.Fatalf("model-a 序列点数量 = %d，期望同桶合并后为 2", len(got.Series))
	}

	first := got.Series[0]
	if first.Ts != 100 || first.AvgTtftMs != 0 || first.AvgLatencyMs != 0 || first.SuccessRate != 100 || first.AvgTps != 0 {
		t.Fatalf("首个序列点 = %+v，期望按 bucket_ts 升序", first)
	}
	second := got.Series[1]
	if second.Ts != 200 || second.AvgTtftMs != 0 || second.AvgLatencyMs != 0 || second.SuccessRate != 75 || second.AvgTps != 0 {
		t.Fatalf("合并后的第二个序列点 = %+v", second)
	}
}

func TestSummaryBucketPointRoundsRateAndHidesDetailedMetrics(t *testing.T) {
	point := summaryBucketPoint(300, counters{
		requestCount:   3,
		successCount:   2,
		totalLatencyMs: 900,
		ttftSumMs:      300,
		ttftCount:      3,
		outputTokens:   60,
		generationMs:   1000,
	})

	if point.Ts != 300 || point.SuccessRate != 66.67 {
		t.Fatalf("摘要序列点 = %+v，期望成功率保留两位小数", point)
	}
	if point.AvgTtftMs != 0 || point.AvgLatencyMs != 0 || point.AvgTps != 0 {
		t.Fatalf("摘要序列不应公开分时性能明细：%+v", point)
	}

	payload, err := common.Marshal(ModelSummary{
		ModelName:    "model-a",
		AvgLatencyMs: 300,
		SuccessRate:  66.67,
		AvgTps:       60,
		Series:       []BucketPoint{point},
		RequestCount: 3,
	})
	if err != nil {
		t.Fatalf("序列化摘要失败：%v", err)
	}
	jsonText := string(payload)
	for _, expected := range []string{
		`"series":[{"ts":300`,
		`"avg_ttft_ms":0`,
		`"avg_latency_ms":0`,
		`"success_rate":66.67`,
		`"avg_tps":0`,
	} {
		if !strings.Contains(jsonText, expected) {
			t.Fatalf("摘要 JSON 缺少 %s：%s", expected, jsonText)
		}
	}
	if strings.Contains(jsonText, "request_count") || strings.Contains(jsonText, "recent_success_rates") {
		t.Fatalf("摘要 JSON 泄露内部字段：%s", jsonText)
	}
}

func TestBuildModelSummariesSkipsEmptyBucketsAndSortsTies(t *testing.T) {
	modelBuckets := map[string]map[int64]counters{
		"model-b": {
			100: {requestCount: 1, successCount: 1},
			200: {},
		},
		"model-a": {
			100: {requestCount: 1, successCount: 1},
		},
		"model-empty": {
			100: {},
		},
	}

	models := buildModelSummaries(modelBuckets)
	if len(models) != 2 {
		t.Fatalf("模型数量 = %d，期望忽略空模型后为 2", len(models))
	}
	if models[0].ModelName != "model-a" || models[1].ModelName != "model-b" {
		t.Fatalf("同请求量模型顺序 = %q, %q，期望名称升序", models[0].ModelName, models[1].ModelName)
	}
	if len(models[1].Series) != 1 || models[1].Series[0].Ts != 100 {
		t.Fatalf("空桶不应进入序列：%+v", models[1].Series)
	}
}
