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
  buildLatencyBarHeights,
  buildPerformanceSlots,
  formatBucketTime,
  formatLatency,
  formatSuccessRate,
  getSuccessRateBarClass,
  getSuccessRateTextClass,
  normalizePerformanceSeries,
} from './utils';

const SuccessRateSparkline = ({
  series,
  overall,
  maxPoints = 24,
  showOverall = true,
  compact = false,
  latestTimestamp,
  className = '',
}) => {
  const points = useMemo(() => {
    const normalized = normalizePerformanceSeries(series);
    return normalized.slice(-Math.max(1, maxPoints));
  }, [maxPoints, series]);

  if (points.length === 0) {
    return (
      <span className={`text-xs text-semi-color-text-2 ${className}`}>—</span>
    );
  }

  const computedOverall = Number.isFinite(Number(overall))
    ? Number(overall)
    : points.reduce((sum, point) => sum + point.success_rate, 0) /
      points.length;
  const barHeights = buildLatencyBarHeights(points);
  const heightByTimestamp = new Map(
    points.map((point, index) => [point.ts, barHeights[index]]),
  );
  const slots = buildPerformanceSlots(points, maxPoints);
  const barWidth = compact ? 'w-0.5' : 'w-1';
  const gap = compact ? 'gap-px' : 'gap-[2px]';
  const height = compact ? 'h-3.5' : 'h-4';

  return (
    <div className={`flex items-end gap-2 ${className}`}>
      <div className='flex min-w-0 flex-col items-center'>
        {Number(latestTimestamp) > 0 && (
          <div className='mb-1 whitespace-nowrap text-[10px] leading-none text-semi-color-text-2'>
            {formatBucketTime(latestTimestamp)}
          </div>
        )}
        <div
          className={`flex items-end ${height} ${gap}`}
          role='img'
          aria-label={formatSuccessRate(computedOverall, 2)}
        >
          {slots.map((point) => (
            <span
              key={`${point.ts}-${point.is_placeholder ? 'empty' : point.success_rate}`}
              className={`flex items-end ${barWidth} ${height}`}
              title={
                point.is_placeholder
                  ? `${formatBucketTime(point.ts)} · —`
                  : `${formatBucketTime(point.ts)} · ${formatLatency(
                      point.avg_latency_ms,
                    )} · ${formatSuccessRate(point.success_rate, 2)}`
              }
            >
              <span
                className={`w-full rounded-full ${
                  point.is_placeholder
                    ? 'bg-semi-color-fill-2'
                    : getSuccessRateBarClass(point.success_rate)
                }`}
                style={{
                  height: point.is_placeholder
                    ? '2px'
                    : `${heightByTimestamp.get(point.ts) || 35}%`,
                }}
              />
            </span>
          ))}
        </div>
      </div>
      {showOverall && (
        <span
          className={`whitespace-nowrap font-mono font-semibold leading-none tabular-nums ${compact ? 'text-[11px]' : 'text-xs'} ${getSuccessRateTextClass(
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
