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

const formatLatency = (value) => {
  if (!Number.isFinite(value) || value <= 0) return '—';
  if (value >= 1000) return `${(value / 1000).toFixed(2)}s`;
  return `${Math.round(value)}ms`;
};

const formatThroughput = (value) => {
  if (!Number.isFinite(value) || value <= 0) return '—';
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
  return `${value.toFixed(value < 10 ? 2 : 1)}`;
};

const getStatusColor = (successRate) => {
  if (!Number.isFinite(successRate) || successRate < 99) {
    return 'bg-red-500';
  }
  if (successRate < 99.9) return 'bg-amber-500';
  return 'bg-emerald-500';
};

const ModelPerformanceBadge = ({ performance, t }) => {
  if (!performance) return null;

  const { avg_latency_ms, avg_tps, success_rate } = performance;
  const successLabel = Number.isFinite(success_rate)
    ? `${success_rate.toFixed(1)}%`
    : '—';

  return (
    <div className='flex shrink-0 items-end gap-2 text-right tabular-nums'>
      <div title={t('平均延迟')}>
        <div className='text-[10px] leading-4 text-gray-400'>{t('延迟')}</div>
        <div className='font-mono text-xs leading-4 text-gray-600'>
          {formatLatency(avg_latency_ms)}
        </div>
      </div>
      <div title={t('吞吐量')}>
        <div className='text-[10px] leading-4 text-gray-400'>TPS</div>
        <div className='font-mono text-xs leading-4 text-gray-600'>
          {formatThroughput(avg_tps)}
        </div>
      </div>
      <div title={`${t('成功率')}: ${successLabel}`}>
        <div className='text-[10px] leading-4 text-gray-400'>{t('状态')}</div>
        <div className='flex h-4 items-center justify-end gap-0.5'>
          <span className='h-2 w-1 rounded-full bg-gray-200' />
          <span className='h-2.5 w-1 rounded-full bg-gray-300' />
          <span
            className={`h-3 w-1 rounded-full ${getStatusColor(success_rate)}`}
          />
        </div>
      </div>
    </div>
  );
};

export default React.memo(ModelPerformanceBadge);
