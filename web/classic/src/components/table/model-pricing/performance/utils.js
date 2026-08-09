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

export const STATUS_SEGMENT_COUNT = 4;
export const STATUS_WINDOW_HOURS = 24;

const STATUS_WINDOW_SECONDS = STATUS_WINDOW_HOURS * 60 * 60;

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

export const getSuccessRateTextClass = (value) => {
  const level = getSuccessRateLevel(value);
  if (level === 'healthy') return 'text-semi-color-success';
  if (level === 'warning') return 'text-semi-color-warning';
  if (level === 'critical') return 'text-semi-color-danger';
  return 'text-semi-color-text-2';
};

export const getSuccessRateHex = (value) => {
  if (!Number.isFinite(Number(value))) return '#9ca3af';
  const rate = clampSuccessRate(value);
  if (rate >= 99.9) return '#10b981';
  if (rate >= 99) return '#34d399';
  if (rate >= 95) return '#f59e0b';
  if (rate >= 90) return '#d97706';
  return '#f43f5e';
};

export const getStatusSegmentHex = (value) => {
  if (!Number.isFinite(Number(value))) return '#9ca3af';
  const rate = clampSuccessRate(value);
  if (rate >= 99.9) return '#10b981';
  if (rate >= 99) return '#f59e0b';
  return '#f43f5e';
};

export const getAvailabilityStatusLevel = (value) => {
  if (!Number.isFinite(Number(value))) return 'unavailable';
  const rate = clampSuccessRate(value);
  if (rate >= 95) return 'healthy';
  if (rate > 0) return 'degraded';
  return 'unavailable';
};

export const getAvailabilityStatusHex = (value) => {
  const level = getAvailabilityStatusLevel(value);
  if (level === 'healthy') return '#10b981';
  if (level === 'degraded') return '#f59e0b';
  return '#f43f5e';
};

export const getStatusRateTextClass = (value) => {
  if (!Number.isFinite(Number(value))) return 'text-semi-color-text-2';
  const rate = clampSuccessRate(value);
  if (rate >= 99.9) return 'text-semi-color-success';
  if (rate >= 99) return 'text-semi-color-warning';
  return 'text-semi-color-danger';
};

export const buildLatencyBarHeights = (series, minimumHeight = 50) => {
  const points = Array.isArray(series) ? series : [];
  const floor = Math.min(80, Math.max(20, finiteNumber(minimumHeight, 50)));
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
      status_rate: Number.isFinite(Number(point?.status_rate))
        ? clampSuccessRate(point.status_rate)
        : undefined,
      avg_tps: finiteNumber(point?.avg_tps),
    }))
    .filter((point) => point.ts > 0)
    .sort((left, right) => left.ts - right.ts);
};

/**
 * 将最近 24 小时的性能桶压缩为固定状态段，并保留无数据时段。
 */
export const buildStatusSegments = (
  series,
  endTs,
  segmentCount = STATUS_SEGMENT_COUNT,
) => {
  const normalizedEndTs = Number.isFinite(Number(endTs))
    ? Math.trunc(Number(endTs))
    : Math.trunc(Date.now() / 1000);
  const safeSegmentCount = Math.min(
    STATUS_SEGMENT_COUNT,
    Math.max(1, Math.trunc(finiteNumber(segmentCount, STATUS_SEGMENT_COUNT))),
  );
  const startTs = normalizedEndTs - STATUS_WINDOW_SECONDS;
  const segmentSeconds = STATUS_WINDOW_SECONDS / safeSegmentCount;
  const accumulators = Array.from({ length: safeSegmentCount }, () => ({
    totalRate: 0,
    rateCount: 0,
    totalLatency: 0,
    latencyCount: 0,
  }));

  const validSeries = Array.isArray(series)
    ? series.filter((point) => {
        const timestamp = Number(point?.ts);
        const successRate = Number(point?.success_rate);
        return (
          Number.isFinite(timestamp) &&
          Number.isFinite(successRate) &&
          successRate >= 0 &&
          successRate <= 100
        );
      })
    : [];

  normalizePerformanceSeries(validSeries).forEach((point) => {
    if (point.ts < startTs || point.ts > normalizedEndTs) return;
    const rawIndex = Math.floor((point.ts - startTs) / segmentSeconds);
    const index = Math.min(rawIndex, safeSegmentCount - 1);
    const accumulator = accumulators[index];
    accumulator.totalRate += point.success_rate;
    accumulator.rateCount += 1;
    if (point.avg_latency_ms > 0) {
      accumulator.totalLatency += point.avg_latency_ms;
      accumulator.latencyCount += 1;
    }
  });

  return accumulators.map((accumulator, index) => {
    const segmentStartTs = startTs + index * segmentSeconds;
    return {
      ts: segmentStartTs,
      end_ts: segmentStartTs + segmentSeconds,
      success_rate:
        accumulator.rateCount > 0
          ? Math.round((accumulator.totalRate / accumulator.rateCount) * 100) /
            100
          : null,
      avg_latency_ms:
        accumulator.latencyCount > 0
          ? Math.round(accumulator.totalLatency / accumulator.latencyCount)
          : 0,
      sample_count: accumulator.rateCount,
    };
  });
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
