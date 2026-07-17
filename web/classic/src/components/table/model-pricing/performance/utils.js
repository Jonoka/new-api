/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

const finiteNumber = (value, fallback = 0) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
};

const average = (values, positiveOnly = false) => {
  const filtered = values
    .map((value) => Number(value))
    .filter(
      (value) => Number.isFinite(value) && (!positiveOnly || Number(value) > 0),
    );
  if (filtered.length === 0) return 0;
  return filtered.reduce((sum, value) => sum + value, 0) / filtered.length;
};

export const clampSuccessRate = (value) => {
  return Math.min(100, Math.max(0, finiteNumber(value)));
};

export const formatLatency = (value) => {
  const latency = finiteNumber(value);
  if (latency <= 0) return '—';
  if (latency >= 1000) return `${(latency / 1000).toFixed(2)}s`;
  return `${Math.round(latency)}ms`;
};

export const formatThroughput = (value) => {
  const throughput = finiteNumber(value);
  if (throughput <= 0) return '—';
  if (throughput >= 1000) return `${(throughput / 1000).toFixed(1)}K t/s`;
  return `${throughput.toFixed(throughput < 10 ? 2 : 1)} t/s`;
};

export const formatSuccessRate = (value, digits = 1) => {
  if (!Number.isFinite(Number(value))) return '—';
  return `${clampSuccessRate(value).toFixed(digits)}%`;
};

export const formatBucketTime = (timestamp, includeDate = true) => {
  const date = new Date(finiteNumber(timestamp) * 1000);
  if (Number.isNaN(date.getTime())) return '';
  const pad = (value) => String(value).padStart(2, '0');
  const time = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  if (!includeDate) return time;
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(
    date.getDate(),
  )} ${time}`;
};

export const getSuccessRateLevel = (value) => {
  if (!Number.isFinite(Number(value))) return 'unknown';
  const rate = clampSuccessRate(value);
  if (rate >= 99) return 'healthy';
  if (rate >= 95) return 'warning';
  return 'critical';
};

export const getSuccessRateBarClass = (value) => {
  const level = getSuccessRateLevel(value);
  if (level === 'healthy') return 'bg-semi-color-success';
  if (level === 'warning') return 'bg-semi-color-warning';
  if (level === 'critical') return 'bg-semi-color-danger';
  return 'bg-semi-color-fill-2';
};

export const getSuccessRateTextClass = (value) => {
  const level = getSuccessRateLevel(value);
  if (level === 'healthy') return 'text-semi-color-success';
  if (level === 'warning') return 'text-semi-color-warning';
  if (level === 'critical') return 'text-semi-color-danger';
  return 'text-semi-color-text-2';
};

export const getSuccessRateHex = (value) => {
  const level = getSuccessRateLevel(value);
  if (level === 'healthy') return '#10b981';
  if (level === 'warning') return '#f59e0b';
  if (level === 'critical') return '#ef4444';
  return '#9ca3af';
};

export const buildLatencyBarHeights = (series, minimumHeight = 35) => {
  const points = Array.isArray(series) ? series : [];
  const floor = Math.min(80, Math.max(20, finiteNumber(minimumHeight, 35)));
  const latencies = points
    .map((point) => finiteNumber(point?.avg_latency_ms))
    .filter((latency) => latency > 0);

  if (latencies.length === 0) {
    return points.map(() => floor);
  }

  // 对数缩放可避免单个极慢请求把其他时间桶的高度差全部压平。
  const minimum = Math.log1p(Math.min(...latencies));
  const maximum = Math.log1p(Math.max(...latencies));
  const range = maximum - minimum;

  return points.map((point) => {
    const latency = finiteNumber(point?.avg_latency_ms);
    if (latency <= 0) return floor;
    if (range <= Number.EPSILON) return 100;

    const ratio = (Math.log1p(latency) - minimum) / range;
    return Math.round((100 - ratio * (100 - floor)) * 10) / 10;
  });
};

export const normalizePerformanceSeries = (series) => {
  if (!Array.isArray(series)) return [];
  return series
    .map((point) => ({
      ts: finiteNumber(point?.ts),
      avg_ttft_ms: finiteNumber(point?.avg_ttft_ms),
      avg_latency_ms: finiteNumber(point?.avg_latency_ms),
      success_rate: clampSuccessRate(point?.success_rate),
      avg_tps: finiteNumber(point?.avg_tps),
    }))
    .filter((point) => point.ts > 0)
    .sort((left, right) => left.ts - right.ts);
};

export const getUptimeAxisMin = (values) => {
  const finiteValues = values
    .map((value) => Number(value))
    .filter(Number.isFinite);
  if (finiteValues.length === 0) return 95;
  const minimum = Math.max(0, Math.min(...finiteValues));
  if (minimum >= 95) return 95;
  if (minimum >= 90) return 90;
  return Math.max(0, Math.floor((minimum - 5) / 10) * 10);
};

export const buildPerformanceView = (groups) => {
  const rows = (Array.isArray(groups) ? groups : [])
    .filter((group) => group?.group)
    .map((group) => ({
      group: group.group,
      avg_ttft_ms: finiteNumber(group.avg_ttft_ms),
      avg_latency_ms: finiteNumber(group.avg_latency_ms),
      success_rate: clampSuccessRate(group.success_rate),
      avg_tps: finiteNumber(group.avg_tps),
      series: normalizePerformanceSeries(group.series),
    }));

  const latencyBuckets = new Map();
  const uptimeBuckets = new Map();
  const uptimeByGroup = {};

  rows.forEach((row) => {
    uptimeByGroup[row.group] = row.series;
    row.series.forEach((point) => {
      if (point.avg_ttft_ms > 0) {
        const latencyValues = latencyBuckets.get(point.ts) || [];
        latencyValues.push(point.avg_ttft_ms);
        latencyBuckets.set(point.ts, latencyValues);
      }

      const uptime = uptimeBuckets.get(point.ts) || {
        rates: [],
        incidents: 0,
      };
      uptime.rates.push(point.success_rate);
      if (point.success_rate < 100) uptime.incidents += 1;
      uptimeBuckets.set(point.ts, uptime);
    });
  });

  const latencySeries = Array.from(latencyBuckets.entries())
    .sort(([left], [right]) => left - right)
    .map(([ts, values]) => ({
      ts,
      avg_ttft_ms: Math.round(average(values, true)),
    }));

  const uptimeSeries = Array.from(uptimeBuckets.entries())
    .sort(([left], [right]) => left - right)
    .map(([ts, bucket]) => ({
      ts,
      success_rate: Math.round(average(bucket.rates) * 100) / 100,
      incidents: bucket.incidents,
    }));

  return {
    rows,
    avgTps: average(
      rows.map((row) => row.avg_tps),
      true,
    ),
    avgLatency: average(
      rows.map((row) => row.avg_latency_ms),
      true,
    ),
    successRate: average(rows.map((row) => row.success_rate)),
    incidentCount: uptimeSeries.reduce(
      (sum, point) => sum + point.incidents,
      0,
    ),
    latencySeries,
    uptimeSeries,
    uptimeByGroup,
  };
};
