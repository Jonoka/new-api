import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildLatencyBarHeights,
  buildPerformanceView,
  buildStatusSegments,
  getSuccessRateHex,
  getSuccessRateLevel,
  getStatusRateTextClass,
  getStatusSegmentHex,
  getAvailabilityStatusHex,
  getAvailabilityStatusLevel,
  getUptimeAxisMin,
  normalizePerformanceSeries,
} from './utils.js';

test('最近 24 小时状态压缩为四段并保留空段', () => {
  const endTs = 24 * 60 * 60;
  const segments = buildStatusSegments(
    [
      { ts: 60 * 60, success_rate: 100, avg_latency_ms: 100 },
      { ts: 7 * 60 * 60, success_rate: 100, avg_latency_ms: 200 },
      { ts: 8 * 60 * 60, success_rate: 98, avg_latency_ms: 400 },
      { ts: 19 * 60 * 60, success_rate: 90, avg_latency_ms: 800 },
      { ts: 20 * 60 * 60, success_rate: 101, avg_latency_ms: 100 },
    ],
    endTs,
  );

  assert.equal(segments.length, 4);
  assert.deepEqual(
    segments.map((segment) => segment.success_rate),
    [100, 99, null, 90],
  );
  assert.equal(segments[1].avg_latency_ms, 300);
  assert.equal(segments[2].sample_count, 0);
});

test('状态柱高度随延迟升高而降低，并忽略无效延迟的缩放影响', () => {
  const heights = buildLatencyBarHeights([
    { avg_latency_ms: 100 },
    { avg_latency_ms: 1000 },
    { avg_latency_ms: 10000 },
    { avg_latency_ms: 0 },
  ]);

  assert.equal(heights[0], 100);
  assert.ok(heights[0] > heights[1]);
  assert.ok(heights[1] > heights[2]);
  assert.equal(heights[2], 50);
  assert.equal(heights[3], 50);
  assert.deepEqual(
    buildLatencyBarHeights([{ avg_latency_ms: 500 }, { avg_latency_ms: 500 }]),
    [100, 100],
  );
});

test('性能序列按时间排序并过滤无效时间桶', () => {
  const series = normalizePerformanceSeries([
    { ts: 200, success_rate: 98 },
    { ts: 0, success_rate: 100 },
    { ts: 100, success_rate: 101 },
  ]);

  assert.deepEqual(
    series.map((point) => [point.ts, point.success_rate]),
    [
      [100, 100],
      [200, 98],
    ],
  );
});

test('详情指标按分组等权聚合并生成趋势', () => {
  const view = buildPerformanceView([
    {
      group: 'group-a',
      avg_ttft_ms: 100,
      avg_latency_ms: 1000,
      success_rate: 100,
      avg_tps: 20,
      series: [
        { ts: 100, avg_ttft_ms: 100, success_rate: 100 },
        { ts: 200, avg_ttft_ms: 200, success_rate: 98 },
      ],
    },
    {
      group: 'group-b',
      avg_ttft_ms: 300,
      avg_latency_ms: 3000,
      success_rate: 98,
      avg_tps: 40,
      series: [
        { ts: 100, avg_ttft_ms: 300, success_rate: 98 },
        { ts: 200, avg_ttft_ms: 0, success_rate: 100 },
      ],
    },
  ]);

  assert.equal(view.avgTps, 30);
  assert.equal(view.avgLatency, 2000);
  assert.equal(view.successRate, 99);
  assert.equal(view.incidentCount, 2);
  assert.deepEqual(view.latencySeries, [
    { ts: 100, avg_ttft_ms: 200 },
    { ts: 200, avg_ttft_ms: 200 },
  ]);
  assert.deepEqual(view.uptimeSeries, [
    { ts: 100, success_rate: 99, incidents: 1 },
    { ts: 200, success_rate: 99, incidents: 1 },
  ]);
});

test('PackyAPI 风格成功率颜色和可用率轴下限保持稳定', () => {
  assert.equal(getSuccessRateLevel(99), 'healthy');
  assert.equal(getSuccessRateLevel(98.6), 'warning');
  assert.equal(getSuccessRateLevel(89.5), 'critical');
  assert.equal(getSuccessRateHex(100), '#10b981');
  assert.equal(getSuccessRateHex(99.3), '#34d399');
  assert.equal(getSuccessRateHex(98.6), '#f59e0b');
  assert.equal(getSuccessRateHex(93.7), '#d97706');
  assert.equal(getSuccessRateHex(89.5), '#f43f5e');
  assert.equal(getUptimeAxisMin([99.9, 98]), 95);
  assert.equal(getUptimeAxisMin([94.5]), 90);
  assert.equal(getUptimeAxisMin([83]), 70);
});

test('四段式状态条使用统一的成功、提醒和异常阈值', () => {
  assert.equal(getStatusSegmentHex(99.9), '#10b981');
  assert.equal(getStatusSegmentHex(99), '#f59e0b');
  assert.equal(getStatusSegmentHex(98.99), '#f43f5e');
  assert.equal(getStatusRateTextClass(99.9), 'text-semi-color-success');
  assert.equal(getStatusRateTextClass(99), 'text-semi-color-warning');
  assert.equal(getStatusRateTextClass(98.99), 'text-semi-color-danger');
});

test('模型广场状态只在所有分组都不可用时显示红色', () => {
  assert.equal(getAvailabilityStatusLevel(100), 'healthy');
  assert.equal(getAvailabilityStatusLevel(95), 'healthy');
  assert.equal(getAvailabilityStatusLevel(94.99), 'degraded');
  assert.equal(getAvailabilityStatusLevel(0.01), 'degraded');
  assert.equal(getAvailabilityStatusLevel(0), 'unavailable');
  assert.equal(getAvailabilityStatusHex(100), '#10b981');
  assert.equal(getAvailabilityStatusHex(80), '#f59e0b');
  assert.equal(getAvailabilityStatusHex(0), '#f43f5e');
});
