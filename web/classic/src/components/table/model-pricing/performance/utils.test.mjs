import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildPerformanceView,
  getSuccessRateLevel,
  getUptimeAxisMin,
  normalizePerformanceSeries,
} from './utils.js';

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
