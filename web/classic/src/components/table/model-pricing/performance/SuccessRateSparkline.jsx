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

import React, { useMemo } from 'react';
import {
  buildStatusSegments,
  buildLatencyBarHeights,
  formatBucketTime,
  formatLatency,
  formatSuccessRate,
  getStatusRateTextClass,
  getStatusSegmentHex,
  getAvailabilityStatusHex,
  STATUS_SEGMENT_COUNT,
} from './utils';

const SuccessRateSparkline = ({
  series,
  overall,
  maxPoints = STATUS_SEGMENT_COUNT,
  showOverall = true,
  compact = false,
  latestTimestamp,
  className = '',
  availabilityTone = false,
  signalStyle = false,
}) => {
  const windowEndTs = Math.trunc(Date.now() / 1000);
  const points = useMemo(() => {
    return buildStatusSegments(series, windowEndTs, maxPoints);
  }, [maxPoints, series, windowEndTs]);

  if (!Array.isArray(series) || series.length === 0) {
    return (
      <span className={`text-xs text-semi-color-text-2 ${className}`}>—</span>
    );
  }

  const computedOverall = Number.isFinite(Number(overall))
    ? Number(overall)
    : points.reduce((sum, point) => sum + (point.success_rate ?? 0), 0) /
      Math.max(1, points.filter((point) => point.sample_count > 0).length);
  const barHeights = buildLatencyBarHeights(points);
  const barWidth = signalStyle || compact ? 'w-1' : 'w-2';
  const gap = signalStyle ? 'gap-0.5' : 'gap-1';
  const height = signalStyle ? 'h-4' : compact ? 'h-3.5' : 'h-4';

  return (
    <div className={`relative flex items-center gap-2 ${className}`}>
      {Number(latestTimestamp) > 0 && (
        <span className='absolute bottom-full left-0 mb-0.5 whitespace-nowrap text-[10px] leading-none text-semi-color-text-2'>
          {formatBucketTime(latestTimestamp)}
        </span>
      )}
      <div
        className={`flex items-end ${height} ${gap}`}
        role='img'
        aria-label={formatSuccessRate(computedOverall, 2)}
      >
        {points.map((point, index) => (
          <span
            key={`${point.ts}-${point.success_rate}`}
            className={`flex items-end ${barWidth} ${height}`}
            title={`${formatBucketTime(point.ts)} – ${formatBucketTime(
              point.end_ts,
            )} · ${
              point.sample_count > 0
                ? `${formatLatency(point.avg_latency_ms)} · ${formatSuccessRate(
                    point.success_rate,
                    2,
                  )}`
                : '—'
            }`}
          >
            <span
              className={`w-full ${signalStyle ? 'rounded-full' : 'rounded-sm'}`}
              style={{
                backgroundColor:
                  point.sample_count > 0
                    ? availabilityTone
                      ? getAvailabilityStatusHex(point.success_rate)
                      : getStatusSegmentHex(point.success_rate)
                    : 'var(--semi-color-fill-1)',
                height: signalStyle
                  ? `${8 + Math.min(index, 2) * 2}px`
                  : `${barHeights[index] || 50}%`,
              }}
            />
          </span>
        ))}
      </div>
      {showOverall && (
        <span
          className={`whitespace-nowrap font-mono font-semibold leading-none tabular-nums ${compact ? 'text-[11px]' : 'text-xs'} ${getStatusRateTextClass(
            computedOverall,
          )}`}
        >
          {formatSuccessRate(computedOverall)}
        </span>
      )}
    </div>
  );
};

export default React.memo(SuccessRateSparkline);
