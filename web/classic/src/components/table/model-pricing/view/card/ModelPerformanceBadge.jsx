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
  formatBucketTime,
  formatLatency,
  formatThroughput,
  normalizePerformanceSeries,
} from '../../performance/utils';

const ModelPerformanceBadge = ({ performance, t }) => {
  if (!performance) return null;

  const { avg_latency_ms, avg_tps, success_rate } = performance;
  const series = normalizePerformanceSeries(performance.series);
  const latestPoint = series[series.length - 1];

  return (
    <div className='hidden sm:flex shrink-0 items-end gap-2 text-right tabular-nums'>
      <div className='flex items-center gap-1.5 whitespace-nowrap pb-px'>
        <span
          title={t('吞吐量')}
          className='text-[10px] text-semi-color-text-2'
        >
          TPS&nbsp;
          <span className='font-mono text-semi-color-text-1'>
            {formatThroughput(avg_tps)}
          </span>
        </span>
        <span
          title={t('平均延迟')}
          className='text-[10px] text-semi-color-text-2'
        >
          {t('延迟')}&nbsp;
          <span className='font-mono text-semi-color-text-1'>
            {formatLatency(avg_latency_ms)}
          </span>
        </span>
      </div>
      <div className='flex min-w-0 flex-col items-end'>
        {latestPoint && (
          <div className='mb-0.5 text-[9px] leading-none text-semi-color-text-2'>
            {formatBucketTime(latestPoint.ts)}
          </div>
        )}
        <div title={t('成功率')}>
          <SuccessRateSparkline
            series={series}
            overall={success_rate}
            maxPoints={24}
            compact
          />
        </div>
      </div>
    </div>
  );
};

export default React.memo(ModelPerformanceBadge);
