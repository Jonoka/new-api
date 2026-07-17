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

import React, { useEffect, useMemo, useState } from 'react';
import { Spin, Tag } from '@douyinfe/semi-ui';
import { IconAlertTriangle, IconClock, IconPulse } from '@douyinfe/semi-icons';

import { API } from '../../../../helpers';
import { LatencyTrendChart, UptimeTrendChart } from './PerformanceCharts';
import SuccessRateSparkline from './SuccessRateSparkline';
import {
  buildPerformanceView,
  formatLatency,
  formatSuccessRate,
  formatThroughput,
  getSuccessRateTextClass,
} from './utils';

const StatCard = ({ icon, label, value, hint, valueClassName = '' }) => (
  <div
    className='flex min-h-28 flex-col gap-1 rounded-xl border p-4'
    style={{
      borderColor: 'var(--semi-color-border)',
      background: 'var(--semi-color-bg-0)',
    }}
  >
    <span
      className='inline-flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider'
      style={{ color: 'var(--semi-color-text-2)' }}
    >
      {icon}
      {label}
    </span>
    <span
      className={`font-mono text-xl font-semibold tabular-nums ${valueClassName}`}
      style={valueClassName ? undefined : { color: 'var(--semi-color-text-0)' }}
    >
      {value}
    </span>
    {hint && (
      <span
        className='text-[11px]'
        style={{ color: 'var(--semi-color-text-2)' }}
      >
        {hint}
      </span>
    )}
  </div>
);

const SectionHeader = ({ icon, title, description, accent }) => (
  <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
    <div className='flex min-w-0 items-center gap-2'>
      <span style={{ color: 'var(--semi-color-text-2)' }}>{icon}</span>
      <div className='min-w-0'>
        <div
          className='text-sm font-semibold'
          style={{ color: 'var(--semi-color-text-0)' }}
        >
          {title}
        </div>
        {description && (
          <div
            className='text-xs'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {description}
          </div>
        )}
      </div>
    </div>
    {accent}
  </div>
);

const ModelPerformancePanel = ({ modelName, t }) => {
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    let alive = true;

    const loadPerformance = async () => {
      setLoading(true);
      setFailed(false);
      setGroups([]);
      try {
        const response = await API.get('/api/perf-metrics', {
          params: { model: modelName, hours: 24 },
          signal: controller.signal,
          skipErrorHandler: true,
        });
        if (!alive) return;
        const nextGroups = response.data?.success
          ? response.data?.data?.groups
          : [];
        setGroups(Array.isArray(nextGroups) ? nextGroups : []);
      } catch (_error) {
        if (!alive || controller.signal.aborted) return;
        setFailed(true);
      } finally {
        if (alive) setLoading(false);
      }
    };

    if (modelName) loadPerformance();

    return () => {
      alive = false;
      controller.abort();
    };
  }, [modelName]);

  const view = useMemo(() => buildPerformanceView(groups), [groups]);

  if (loading) {
    return (
      <div className='flex min-h-48 items-center justify-center'>
        <Spin size='large' />
      </div>
    );
  }

  if (failed || view.rows.length === 0) {
    return (
      <div
        className='rounded-xl border px-4 py-12 text-center text-sm'
        style={{
          borderColor: 'var(--semi-color-border)',
          color: 'var(--semi-color-text-2)',
        }}
      >
        {t('暂无性能数据')}
      </div>
    );
  }

  const successHint =
    view.incidentCount > 0
      ? t('最近 24 小时发生 {{count}} 个异常时段', {
          count: view.incidentCount,
        })
      : t('最近 24 小时无异常');

  return (
    <div className='flex flex-col gap-6 pb-6'>
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
        <StatCard
          icon={<IconClock size='small' />}
          label='TPS'
          value={formatThroughput(view.avgTps)}
          hint={t('持续每秒 Token 数')}
        />
        <StatCard
          icon={<IconClock size='small' />}
          label={t('平均延迟')}
          value={formatLatency(view.avgLatency)}
        />
        <StatCard
          icon={<IconPulse size='small' />}
          label={t('成功率')}
          value={formatSuccessRate(view.successRate, 2)}
          hint={successHint}
          valueClassName={getSuccessRateTextClass(view.successRate)}
        />
      </div>

      <section>
        <SectionHeader
          icon={<IconPulse size='small' />}
          title={t('各分组性能')}
          description={t('平均延迟、首 Token 延迟、TPS 和成功率')}
        />
        <div
          className='overflow-x-auto rounded-xl border'
          style={{ borderColor: 'var(--semi-color-border)' }}
        >
          <table className='w-full min-w-[720px] border-collapse text-sm'>
            <thead>
              <tr style={{ background: 'var(--semi-color-fill-0)' }}>
                {[t('分组'), 'TPS', t('首 Token 延迟'), t('平均延迟')].map(
                  (label) => (
                    <th
                      key={label}
                      className='border-b px-3 py-2 text-left text-[10px] font-medium uppercase tracking-wider'
                      style={{
                        borderColor: 'var(--semi-color-border)',
                        color: 'var(--semi-color-text-2)',
                      }}
                    >
                      {label}
                    </th>
                  ),
                )}
                <th
                  className='min-w-[190px] border-b px-3 py-2 text-left text-[10px] font-medium uppercase tracking-wider'
                  style={{
                    borderColor: 'var(--semi-color-border)',
                    color: 'var(--semi-color-text-2)',
                  }}
                >
                  {t('成功率')}
                </th>
              </tr>
            </thead>
            <tbody>
              {view.rows.map((row) => (
                <tr key={row.group}>
                  <td
                    className='border-b px-3 py-3'
                    style={{ borderColor: 'var(--semi-color-border)' }}
                  >
                    <Tag color='blue' size='small'>
                      {row.group}
                    </Tag>
                  </td>
                  <td
                    className='border-b px-3 py-3 font-mono tabular-nums'
                    style={{ borderColor: 'var(--semi-color-border)' }}
                  >
                    {formatThroughput(row.avg_tps)}
                  </td>
                  <td
                    className='border-b px-3 py-3 font-mono tabular-nums'
                    style={{ borderColor: 'var(--semi-color-border)' }}
                  >
                    {formatLatency(row.avg_ttft_ms)}
                  </td>
                  <td
                    className='border-b px-3 py-3 font-mono tabular-nums'
                    style={{
                      borderColor: 'var(--semi-color-border)',
                      color: 'var(--semi-color-text-2)',
                    }}
                  >
                    {formatLatency(row.avg_latency_ms)}
                  </td>
                  <td
                    className='border-b px-3 py-3'
                    style={{ borderColor: 'var(--semi-color-border)' }}
                  >
                    <SuccessRateSparkline
                      series={row.series}
                      overall={row.success_rate}
                      maxPoints={24}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section>
        <SectionHeader
          icon={<IconClock size='small' />}
          title={t('延迟趋势（最近 24 小时）')}
          description={t('平均首 Token 延迟')}
        />
        <LatencyTrendChart series={view.latencySeries} t={t} />
      </section>

      <section>
        <SectionHeader
          icon={<IconPulse size='small' />}
          title={t('可用率（最近 24 小时）')}
          description={t('过去 24 小时请求成功率')}
          accent={
            view.incidentCount > 0 ? (
              <span className='inline-flex items-center gap-1 text-xs font-medium text-semi-color-warning'>
                <IconAlertTriangle size='small' />
                {view.incidentCount}
              </span>
            ) : null
          }
        />
        <UptimeTrendChart series={view.uptimeSeries} t={t} />
      </section>
    </div>
  );
};

export default ModelPerformancePanel;
