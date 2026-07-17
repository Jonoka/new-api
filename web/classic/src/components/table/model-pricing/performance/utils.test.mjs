import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildLatencyBarHeights,
  buildPerformanceSlots,
  buildPerformanceView,
  getSuccessRateLevel,
  getUptimeAxisMin,
  normalizePerformanceSeries,
} from './utils.js';

test('状态区固定生成 24 个时间槽并在缺失小时保留占位', () => {
  const slots = buildPerformanceSlots(
    [
      { ts: 3600, avg_latency_ms: 100, success_rate: 100 },
      { ts: 10800, avg_latency_ms: 300, success_rate: 98 },
    ],
    24,
  );

  assert.equal(slots.length, 24);
  assert.equal(slots.at(-1).ts, 10800);
  assert.equal(slots.at(-1).is_placeholder, false);
  assert.equal(slots.at(-2).ts, 7200);
  assert.equal(slots.at(-2).is_placeholder, true);
  assert.equal(slots.at(-3).ts, 3600);
  assert.equal(slots.at(-3).is_placeholder, false);
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
  assert.equal(heights[2], 35);
  assert.equal(heights[3], 35);
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
  assert.equal(getUptimeAxisMin([99.9, 98]), 95);
  assert.equal(getUptimeAxisMin([94.5]), 90);
  assert.equal(getUptimeAxisMin([83]), 70);
});
