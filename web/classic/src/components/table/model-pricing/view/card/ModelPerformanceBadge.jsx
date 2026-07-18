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

import React from 'react';
import SuccessRateSparkline from '../../performance/SuccessRateSparkline';
import {
  formatLatency,
  formatThroughput,
  normalizePerformanceSeries,
} from '../../performance/utils';

const ModelPerformanceBadge = ({ performance, t, isMobile = false }) => {
  if (!performance) return null;

  const { avg_latency_ms, avg_tps, success_rate } = performance;
  const series = normalizePerformanceSeries(performance.series);
  const latestPoint = series[series.length - 1];

  return (
    <div
      className={
        isMobile
          ? 'flex w-full min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-[11px] tabular-nums'
          : 'flex min-w-0 items-center gap-3 text-xs tabular-nums'
      }
    >
      <div
        className={
          isMobile
            ? 'flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1'
            : 'flex shrink-0 items-center gap-3 whitespace-nowrap'
        }
      >
        <span
          title={t('吞吐量')}
          className='inline-flex items-baseline gap-1 text-semi-color-text-2'
        >
          <span>{t('吞吐量')}</span>
          <span className='font-mono font-medium text-semi-color-text-0'>
            {formatThroughput(avg_tps)}
          </span>
        </span>
        <span
          title={t('平均延迟')}
          className='inline-flex items-baseline gap-1 text-semi-color-text-2'
        >
          <span>{t('延迟')}</span>
          <span className='font-mono font-medium text-semi-color-text-0'>
            {formatLatency(avg_latency_ms)}
          </span>
        </span>
      </div>
      <div
        className={isMobile ? 'relative ml-auto shrink-0' : 'relative'}
        title={t('成功率')}
      >
        <SuccessRateSparkline
          series={series}
          overall={success_rate}
          maxPoints={isMobile ? 12 : 24}
          compact={isMobile}
          latestTimestamp={isMobile ? undefined : latestPoint?.ts}
        />
      </div>
    </div>
  );
};

export default React.memo(ModelPerformanceBadge);
