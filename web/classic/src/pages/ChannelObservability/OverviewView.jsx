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
import {
  Banner,
  Card,
  Empty,
  Progress,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import {
  Activity,
  BadgeDollarSign,
  CheckCircle2,
  Clock3,
  Database,
  RefreshCw,
  RotateCcw,
  Server,
  Sigma,
  TriangleAlert,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import { CHART_CONFIG } from '../../constants/dashboard.constants';
import {
  API_ROOT,
  formatCompact,
  formatDateTime,
  formatDuration,
  formatInteger,
  formatPercent,
  getErrorMessage,
  normalizeStatusItems,
  percentNumber,
  statusTagColor,
} from './utils';

const { Text, Title } = Typography;

let chartThemeInitialized = false;

const MetricItem = ({ icon, label, value, hint }) => (
  <div
    className='min-w-0 border-b p-3 last:border-b-0 sm:p-4'
    style={{ borderColor: 'var(--semi-color-border)' }}
  >
    <div className='flex items-start justify-between gap-3'>
      <div className='min-w-0'>
        <Text type='tertiary' size='small'>
          {label}
        </Text>
        <Title heading={5} className='mt-1'>
          {value}
        </Title>
        <Text type='secondary' size='small' ellipsis={{ showTooltip: true }}>
          {hint}
        </Text>
      </div>
      <div
        className='flex h-8 w-8 shrink-0 items-center justify-center rounded-md'
        style={{
          color: 'var(--semi-color-primary)',
          backgroundColor: 'var(--semi-color-primary-light-default)',
        }}
      >
        {icon}
      </div>
    </div>
  </div>
);

const SummaryProgress = ({ label, value, hint }) => {
  const percent = percentNumber(value);
  return (
    <div>
      <div className='mb-1 flex items-center justify-between gap-3'>
        <Text strong>{label}</Text>
        <Text>{formatPercent(value)}</Text>
      </div>
      <Progress
        percent={Math.max(0, Math.min(100, percent || 0))}
        showInfo={false}
      />
      <Text type='tertiary' size='small'>
        {hint}
      </Text>
    </div>
  );
};

const OverviewView = ({
  makeParams,
  queryKey,
  refreshKey,
  onStatus,
  onViewChange,
}) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [summaryPayload, setSummaryPayload] = useState(null);
  const [trendPayload, setTrendPayload] = useState(null);
  const [statusPayload, setStatusPayload] = useState(null);

  useEffect(() => {
    if (!chartThemeInitialized) {
      initVChartSemiTheme({ isWatchingThemeSwitch: true });
      chartThemeInitialized = true;
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      setLoading(true);
      setError('');
      onStatus?.({ loading: true, error: '', meta: null });
      try {
        const base = makeParams({}, { includeStatus: false });
        const statusParams = makeParams(
          { metric_scope: 'upstream_call' },
          { includeStatus: false },
        );
        const [summaryRes, trendRes, statusRes] = await Promise.all([
          API.get(`${API_ROOT}/summary`, {
            params: base,
            signal: controller.signal,
            skipErrorHandler: true,
          }),
          API.get(`${API_ROOT}/trend`, {
            params: base,
            signal: controller.signal,
            skipErrorHandler: true,
          }),
          API.get(`${API_ROOT}/status-codes`, {
            params: statusParams,
            signal: controller.signal,
            skipErrorHandler: true,
          }),
        ]);
        const summary = summaryRes.data?.data || {};
        const trend = trendRes.data?.data || {};
        const statuses = statusRes.data?.data || {};
        setSummaryPayload(summary);
        setTrendPayload(trend);
        setStatusPayload(statuses);
        const metrics = summary.summary || summary;
        onStatus?.({
          loading: false,
          error: '',
          meta: summary.meta || trend.meta || statuses.meta,
          empty:
            !metrics.final_request_count &&
            !metrics.channel_attempt_count &&
            !metrics.upstream_call_count,
        });
      } catch (requestError) {
        if (requestError?.name === 'CanceledError' || controller.signal.aborted)
          return;
        const message = getErrorMessage(requestError, t('统计数据加载失败'));
        setError(message);
        onStatus?.({ loading: false, error: message, meta: null });
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };
    load();
    return () => controller.abort();
  }, [makeParams, onStatus, queryKey, refreshKey, t]);

  const metrics = summaryPayload?.summary || {};
  const points = trendPayload?.points || [];
  const statuses = normalizeStatusItems(statusPayload || {});
  const totalTokens = Number(metrics.total_tokens || 0);
  const tokenParts = [
    [t('未缓存输入'), Number(metrics.uncached_input_tokens || 0)],
    [t('缓存读取'), Number(metrics.cache_read_tokens || 0)],
    [t('缓存写入'), Number(metrics.cache_write_tokens || 0)],
    [t('输出 Token'), Number(metrics.output_tokens || 0)],
  ];

  const trendSpec = useMemo(() => {
    const values = points.flatMap((point) => [
      {
        time: formatDateTime(point.bucket_ts, false),
        metric: t('客户端请求'),
        value: Number(point.final_request_count || 0),
      },
      {
        time: formatDateTime(point.bucket_ts, false),
        metric: t('失败尝试'),
        value: Number(point.failed_attempt_count || 0),
      },
    ]);
    if (!values.length) return null;
    return {
      type: 'line',
      data: [{ id: 'channel-quality-trend', values }],
      xField: 'time',
      yField: 'value',
      seriesField: 'metric',
      point: { visible: true },
      line: { style: { lineWidth: 2, curveType: 'monotone' } },
      legends: { visible: true, orient: 'top' },
      background: 'transparent',
    };
  }, [points, t]);

  if (error) {
    return <Banner type='danger' description={error} />;
  }

  return (
    <Spin spinning={loading}>
      {!loading && !summaryPayload ? (
        <Empty description={t('当前筛选暂无调用数据')} />
      ) : (
        <div className='space-y-4'>
          <Card title={t('关键指标')} bodyStyle={{ padding: 0 }}>
            <div className='grid grid-cols-1 gap-x-4 sm:grid-cols-2 xl:grid-cols-5'>
              <MetricItem
                icon={<Activity size={18} />}
                label={t('客户端请求')}
                value={formatInteger(metrics.final_request_count)}
                hint={t('每个逻辑请求只计一次')}
              />
              <MetricItem
                icon={<Server size={18} />}
                label={t('渠道尝试')}
                value={formatInteger(metrics.channel_attempt_count)}
                hint={t('底层调用 {{count}}', {
                  count: formatInteger(metrics.upstream_call_count),
                })}
              />
              <MetricItem
                icon={<CheckCircle2 size={18} />}
                label={t('渠道质量成功率')}
                value={formatPercent(metrics.channel_quality_success_rate)}
                hint={t('只统计可归因的渠道样本')}
              />
              <MetricItem
                icon={<TriangleAlert size={18} />}
                label={t('失败尝试')}
                value={formatInteger(metrics.failed_attempt_count)}
                hint={t('重试率 {{rate}}', {
                  rate: formatPercent(metrics.retry_rate),
                })}
              />
              <MetricItem
                icon={<Sigma size={18} />}
                label={t('已记录 Token')}
                value={formatCompact(metrics.total_tokens)}
                hint={t('具有可用量记录的渠道尝试')}
              />
              <MetricItem
                icon={<Database size={18} />}
                label={t('缓存读取')}
                value={formatCompact(metrics.cache_read_tokens)}
                hint={t('写入 {{count}} · 命中 {{rate}}', {
                  count: formatCompact(metrics.cache_write_tokens),
                  rate: formatPercent(metrics.cache_token_hit_rate),
                })}
              />
              <MetricItem
                icon={<RotateCcw size={18} />}
                label={t('重试次数')}
                value={formatInteger(metrics.retry_count)}
                hint={t('渠道切换或同渠道重试')}
              />
              <MetricItem
                icon={<Clock3 size={18} />}
                label={t('平均 / P95 延迟')}
                value={`${formatDuration(metrics.avg_latency_ms)} / ${formatDuration(metrics.p95_latency_ms)}`}
                hint={t('TTFT {{avg}} / {{p95}}', {
                  avg: formatDuration(metrics.avg_ttft_ms),
                  p95: formatDuration(metrics.p95_ttft_ms),
                })}
              />
              <MetricItem
                icon={<BadgeDollarSign size={18} />}
                label={t('预估费用')}
                value={`$${(Number(metrics.charged_micro_usd || 0) / 1e6).toFixed(4)}`}
                hint={t('计费额度 {{quota}}', {
                  quota: formatCompact(metrics.charged_quota),
                })}
              />
            </div>
          </Card>

          <div className='grid grid-cols-1 gap-4 xl:grid-cols-3'>
            <Card
              className='xl:col-span-2'
              title={t('流量与质量趋势')}
              headerExtraContent={
                <Text type='tertiary' size='small'>
                  {t('客户端请求与失败尝试')}
                </Text>
              }
            >
              {trendSpec ? (
                <div className='h-72'>
                  <VChart spec={trendSpec} option={CHART_CONFIG} />
                </div>
              ) : (
                <Empty description={t('当前范围暂无趋势数据')} />
              )}
            </Card>

            <Card title={t('Token 构成')}>
              <div className='mb-4 flex items-center justify-between'>
                <Text type='tertiary'>{t('总 Token')}</Text>
                <Title heading={5}>{formatCompact(totalTokens)}</Title>
              </div>
              <div className='space-y-4'>
                {tokenParts.map(([label, value]) => {
                  const percent =
                    totalTokens > 0 ? (value / totalTokens) * 100 : 0;
                  return (
                    <div key={label}>
                      <div className='mb-1 flex justify-between gap-3'>
                        <Text size='small'>{label}</Text>
                        <Text size='small'>
                          {formatCompact(value)} · {percent.toFixed(1)}%
                        </Text>
                      </div>
                      <Progress
                        percent={Math.min(100, percent)}
                        showInfo={false}
                      />
                    </div>
                  );
                })}
              </div>
            </Card>
          </div>

          <div className='grid grid-cols-1 gap-4 xl:grid-cols-2'>
            <Card title={t('成功率口径')}>
              <div className='space-y-5'>
                <SummaryProgress
                  label={t('客户端成功率')}
                  value={metrics.client_success_rate}
                  hint={t('最终返回给客户端的逻辑请求')}
                />
                <SummaryProgress
                  label={t('渠道质量成功率')}
                  value={metrics.channel_quality_success_rate}
                  hint={t('排除客户端、本地和未知归因')}
                />
                <SummaryProgress
                  label={t('尝试业务成功率')}
                  value={metrics.attempt_success_rate}
                  hint={t('反映每次选渠后的业务结果')}
                />
              </div>
            </Card>

            <Card
              title={t('上游状态概览')}
              headerExtraContent={
                <Text
                  link
                  onClick={() => onViewChange?.('failures')}
                  style={{ cursor: 'pointer' }}
                >
                  {t('查看全部')}
                </Text>
              }
            >
              {statuses.length ? (
                <div className='space-y-3'>
                  {statuses.slice(0, 8).map((item) => (
                    <div key={`${item.present}-${item.code}`}>
                      <div className='mb-1 flex items-center justify-between gap-3'>
                        <Tag color={statusTagColor(item.code, item.present)}>
                          {item.label}
                        </Tag>
                        <Text strong>{formatInteger(item.count)}</Text>
                      </div>
                      <Progress
                        percent={
                          (item.count / Math.max(1, statuses[0].count)) * 100
                        }
                        showInfo={false}
                      />
                    </div>
                  ))}
                </div>
              ) : (
                <Empty description={t('暂无状态码数据')} />
              )}
            </Card>
          </div>
        </div>
      )}
    </Spin>
  );
};

export default OverviewView;
