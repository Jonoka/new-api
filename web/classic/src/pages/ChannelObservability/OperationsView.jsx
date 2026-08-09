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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Banner,
  Button,
  Card,
  Empty,
  Pagination,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { ChevronDown, ChevronRight, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import {
  API_ROOT,
  PAGE_SIZE,
  STABILITY_WINDOWS,
  TREE_PLANS,
  formatCompact,
  formatDuration,
  formatInteger,
  formatPercent,
  formatRelativeTime,
  getErrorMessage,
  percentNumber,
  stabilityWindowLabel,
} from './utils';

const { Text } = Typography;
const NODE_LABELS = { group: '分组', channel: '渠道', model: '模型' };

const nodeValue = (item, type) => {
  if (type === 'group') return String(item.group || '').trim();
  if (type === 'channel')
    return Number(item.channel_id) > 0 ? String(item.channel_id) : '';
  if (type === 'model') {
    return String(
      item.model_hash || item.requested_model || item.upstream_model || '',
    ).trim();
  }
  return '';
};

const nodeDisplay = (item, type, t) => {
  if (type === 'group') return item.group_name || item.group || t('未标记分组');
  if (type === 'channel') {
    return (
      item.channel_name ||
      (item.channel_id ? `${t('渠道')} #${item.channel_id}` : t('未标记渠道'))
    );
  }
  if (type === 'model') {
    return item.requested_model || item.upstream_model || t('未标记模型');
  }
  return t('未标记维度');
};

const makeNodeKey = (item, level, dimension) => {
  const plan = TREE_PLANS[dimension];
  if (!plan) return `${dimension}:${item.key || level}`;
  const values = plan.levels
    .slice(0, level + 1)
    .map((type) => nodeValue(item, type));
  return `${dimension}:${level}:${JSON.stringify(values)}`;
};

const windowMap = (item) =>
  new Map(
    (item.windows || []).map((window) => [
      Number(window.window_seconds),
      window,
    ]),
  );

const rateColor = (window) => {
  if (!window || !window.sample_sufficient) return 'grey';
  const percent = percentNumber(window.quality_success_rate);
  if (percent === null) return 'grey';
  if (percent < 95) return 'red';
  if (percent < 99) return 'amber';
  return 'green';
};

const WindowCell = ({ window, t }) => {
  if (!window) return <Text type='tertiary'>{t('无样本')}</Text>;
  return (
    <div>
      <Tag color={rateColor(window)}>
        {formatPercent(window.quality_success_rate)}
      </Tag>
      <div className='mt-1'>
        <Text type='tertiary' size='small'>
          {t('样本 {{sample}} · 调用 {{calls}}', {
            sample: formatInteger(window.quality_eligible_count),
            calls: formatInteger(window.channel_attempt_count),
          })}
        </Text>
      </div>
      {!window.sample_sufficient &&
        Number(window.channel_attempt_count || 0) > 0 && (
          <Text type='warning' size='small'>
            {t('样本不足 {{current}}/{{minimum}}', {
              current: formatInteger(window.quality_eligible_count),
              minimum: formatInteger(window.minimum_sample_count),
            })}
          </Text>
        )}
    </div>
  );
};

const OperationsView = ({
  makeParams,
  queryKey,
  refreshKey,
  onStatus,
  retentionDays = 7,
  preset,
}) => {
  const { t } = useTranslation();
  const [dimension, setDimension] = useState('group_channel_model');
  const [modelDimension, setModelDimension] = useState('requested');
  const [sortBy, setSortBy] = useState('failure_count');
  const [page, setPage] = useState(1);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [expanded, setExpanded] = useState(new Set());
  const [entries, setEntries] = useState(new Map());
  const generationRef = useRef(0);
  const querySnapshotRef = useRef(null);
  const childControllersRef = useRef(new Map());

  const windows = useMemo(() => {
    const maximum = Math.max(0, Number(retentionDays) * 86400);
    const available = STABILITY_WINDOWS.filter((seconds) => seconds <= maximum);
    return available.length ? available : [Math.max(300, maximum)];
  }, [retentionDays]);

  const plan = TREE_PLANS[dimension] || null;
  const usesModel = dimension === 'model' || dimension.endsWith('_model');

  const resetTree = useCallback(() => {
    generationRef.current += 1;
    childControllersRef.current.forEach((controller) => controller.abort());
    childControllersRef.current.clear();
    setExpanded(new Set());
    setEntries(new Map());
    querySnapshotRef.current = null;
  }, []);

  useEffect(() => () => resetTree(), [resetTree]);

  useEffect(() => {
    if (!preset?.version) return;
    setDimension(preset.dimension || 'group_channel_model');
    setModelDimension(preset.modelDimension || 'requested');
    setPage(1);
  }, [preset]);

  useEffect(() => {
    setPage(1);
    resetTree();
  }, [dimension, modelDimension, queryKey, resetTree, sortBy]);

  useEffect(() => {
    resetTree();
  }, [page, refreshKey, resetTree]);

  useEffect(() => {
    const controller = new AbortController();
    const generation = generationRef.current;
    const load = async () => {
      setLoading(true);
      setError('');
      onStatus?.({ loading: true, error: '', meta: null });
      try {
        const currentPlan = TREE_PLANS[dimension];
        const params = makeParams(
          {
            dimension: currentPlan?.levels[0] || dimension,
            model_dimension: modelDimension,
            windows: windows.join(','),
            page,
            page_size: PAGE_SIZE,
            sort_by: sortBy,
            sort_order: sortBy === 'quality_success_rate' ? 'asc' : 'desc',
          },
          { includeStatus: false },
        );
        querySnapshotRef.current = new URLSearchParams(params);
        const response = await API.get(`${API_ROOT}/stability`, {
          params,
          signal: controller.signal,
          skipErrorHandler: true,
        });
        if (generation !== generationRef.current) return;
        const payload = response.data?.data || {};
        const rows = payload.items || [];
        setItems(rows);
        setTotal(Number(payload.total || rows.length));
        onStatus?.({
          loading: false,
          error: '',
          meta: payload.meta,
          empty: rows.length === 0,
        });
      } catch (requestError) {
        if (requestError?.name === 'CanceledError' || controller.signal.aborted)
          return;
        const message = getErrorMessage(requestError, t('运维矩阵加载失败'));
        setError(message);
        onStatus?.({ loading: false, error: message, meta: null });
      } finally {
        if (
          !controller.signal.aborted &&
          generation === generationRef.current
        ) {
          setLoading(false);
        }
      }
    };
    load();
    return () => controller.abort();
  }, [
    dimension,
    makeParams,
    modelDimension,
    onStatus,
    page,
    queryKey,
    refreshKey,
    sortBy,
    t,
    windows,
  ]);

  const applyAncestorFilters = (params, descriptor) => {
    if (!plan) return false;
    for (let index = 0; index <= descriptor.level; index += 1) {
      const type = plan.levels[index];
      if (type === 'group') {
        const group = String(descriptor.item.group || '').trim();
        if (!group) return false;
        params.set('groups', group);
      }
      if (type === 'channel') {
        const channelId = Number(descriptor.item.channel_id);
        if (!channelId) return false;
        params.set('channel_ids', String(channelId));
      }
    }
    return true;
  };

  const loadChildren = useCallback(
    async (descriptor, append = false) => {
      const currentPlan = TREE_PLANS[dimension];
      if (!currentPlan || !descriptor.expandable || !querySnapshotRef.current)
        return;
      const previous = entries.get(descriptor.key);
      if (previous?.loading || (!append && previous?.loaded)) return;
      const childPage = append ? Number(previous?.page || 0) + 1 : 1;
      const controller = new AbortController();
      childControllersRef.current.get(descriptor.key)?.abort();
      childControllersRef.current.set(descriptor.key, controller);
      const generation = generationRef.current;

      setEntries((current) => {
        const next = new Map(current);
        next.set(descriptor.key, {
          ...(current.get(descriptor.key) || {}),
          descriptor,
          loading: true,
          error: '',
          items: append ? current.get(descriptor.key)?.items || [] : [],
          page: append ? current.get(descriptor.key)?.page || 0 : 0,
          total: append ? current.get(descriptor.key)?.total || 0 : 0,
          loaded: append ? current.get(descriptor.key)?.loaded || false : false,
        });
        return next;
      });

      try {
        const params = new URLSearchParams(querySnapshotRef.current);
        params.set(
          'dimension',
          currentPlan.levels.slice(0, descriptor.level + 2).join('_'),
        );
        if (!applyAncestorFilters(params, descriptor)) {
          throw new Error(t('当前节点缺少可用于下钻的稳定标识'));
        }
        params.set('page', String(childPage));
        params.set('page_size', String(PAGE_SIZE));
        const response = await API.get(`${API_ROOT}/stability`, {
          params,
          signal: controller.signal,
          skipErrorHandler: true,
        });
        if (generation !== generationRef.current) return;
        const payload = response.data?.data || {};
        const rows = payload.items || [];
        setEntries((current) => {
          const active = current.get(descriptor.key);
          if (!active) return current;
          const next = new Map(current);
          let merged = rows;
          if (append) {
            const childLevel = descriptor.level + 1;
            const seen = new Set(
              (active.items || []).map((item) =>
                makeNodeKey(item, childLevel, dimension),
              ),
            );
            merged = [
              ...(active.items || []),
              ...rows.filter((item) => {
                const key = makeNodeKey(item, childLevel, dimension);
                if (seen.has(key)) return false;
                seen.add(key);
                return true;
              }),
            ];
          }
          next.set(descriptor.key, {
            ...active,
            descriptor,
            items: merged,
            total: Number(payload.total || merged.length),
            page: childPage,
            loaded: true,
            loading: false,
            error: '',
          });
          return next;
        });
      } catch (requestError) {
        if (requestError?.name === 'CanceledError' || controller.signal.aborted)
          return;
        setEntries((current) => {
          const next = new Map(current);
          const active = current.get(descriptor.key) || {
            descriptor,
            items: [],
          };
          next.set(descriptor.key, {
            ...active,
            loading: false,
            error: getErrorMessage(requestError, t('子级数据加载失败')),
          });
          return next;
        });
      } finally {
        if (childControllersRef.current.get(descriptor.key) === controller) {
          childControllersRef.current.delete(descriptor.key);
        }
      }
    },
    [dimension, entries, plan, t],
  );

  const toggleNode = (descriptor) => {
    if (!descriptor.expandable || !descriptor.filterable) return;
    if (expanded.has(descriptor.key)) {
      setExpanded((current) => {
        const next = new Set(current);
        next.delete(descriptor.key);
        entries.forEach((entry, key) => {
          if (entry.descriptor?.ancestors?.includes(descriptor.key))
            next.delete(key);
        });
        return next;
      });
      entries.forEach((entry, key) => {
        if (
          key === descriptor.key ||
          entry.descriptor?.ancestors?.includes(descriptor.key)
        ) {
          childControllersRef.current.get(key)?.abort();
          childControllersRef.current.delete(key);
        }
      });
      return;
    }
    setExpanded((current) => new Set(current).add(descriptor.key));
    const entry = entries.get(descriptor.key);
    if (!entry?.loaded && !entry?.loading) loadChildren(descriptor, false);
  };

  const flattenedRows = useMemo(() => {
    const rows = [];
    const walk = (item, level, ancestors = []) => {
      const type = plan?.levels[level] || dimension;
      const key = makeNodeKey(item, level, dimension);
      const descriptor = {
        key,
        item,
        level,
        type,
        ancestors,
        filterable: Boolean(nodeValue(item, type)),
        expandable: Boolean(plan && level < plan.levels.length - 1),
      };
      rows.push({ kind: 'data', key, item, descriptor });
      if (!descriptor.expandable || !expanded.has(key)) return;
      const entry = entries.get(key);
      (entry?.items || []).forEach((child) =>
        walk(child, level + 1, [...ancestors, key]),
      );
      if (entry?.loading) {
        rows.push({
          kind: 'state',
          key: `${key}:loading`,
          descriptor,
          state: 'loading',
        });
      } else if (entry?.error) {
        rows.push({
          kind: 'state',
          key: `${key}:error`,
          descriptor,
          state: 'error',
          message: entry.error,
        });
      } else if (entry?.loaded && !entry.items?.length) {
        rows.push({
          kind: 'state',
          key: `${key}:empty`,
          descriptor,
          state: 'empty',
        });
      } else if (entry?.loaded && entry.items.length < entry.total) {
        rows.push({
          kind: 'state',
          key: `${key}:more`,
          descriptor,
          state: 'more',
          loaded: entry.items.length,
          total: entry.total,
        });
      }
    };
    items.forEach((item) => walk(item, 0));
    return rows;
  }, [dimension, entries, expanded, items, plan]);

  const detailSeconds = windows.includes(86400)
    ? 86400
    : windows[windows.length - 1];

  const renderIdentity = (record) => {
    if (record.kind === 'state') {
      const childType = plan?.levels[record.descriptor.level + 1];
      const childLabel = t(NODE_LABELS[childType] || '子级');
      const padding = (record.descriptor.level + 1) * 22;
      return (
        <div
          className='flex items-center gap-2 py-1'
          style={{ paddingLeft: padding }}
        >
          {record.state === 'loading' && <Spin size='small' />}
          <Text type={record.state === 'error' ? 'danger' : 'tertiary'}>
            {record.state === 'loading' &&
              t('正在加载{{label}}数据', { label: childLabel })}
            {record.state === 'error' && record.message}
            {record.state === 'empty' &&
              t('当前筛选下没有{{label}}样本', { label: childLabel })}
            {record.state === 'more' &&
              t('已加载 {{loaded}} / {{total}} 个{{label}}', {
                loaded: formatInteger(record.loaded),
                total: formatInteger(record.total),
                label: childLabel,
              })}
          </Text>
          {record.state === 'error' && (
            <Button
              size='small'
              theme='borderless'
              icon={<RefreshCw size={14} />}
              onClick={() =>
                loadChildren(
                  record.descriptor,
                  Boolean(entries.get(record.descriptor.key)?.items?.length),
                )
              }
            >
              {t('重试')}
            </Button>
          )}
          {record.state === 'more' && (
            <Button
              size='small'
              theme='borderless'
              onClick={() => loadChildren(record.descriptor, true)}
            >
              {t('加载更多')}
            </Button>
          )}
        </div>
      );
    }

    const descriptor = record.descriptor;
    const isExpanded = expanded.has(descriptor.key);
    const display = nodeDisplay(record.item, descriptor.type, t);
    const meta =
      descriptor.type === 'group'
        ? record.item.group_name && record.item.group_name !== record.item.group
          ? record.item.group
          : ''
        : descriptor.type === 'channel'
          ? `#${record.item.channel_id}${record.item.channel_type_name ? ` · ${record.item.channel_type_name}` : ''}`
          : modelDimension === 'upstream' && record.item.requested_model
            ? `${t('请求')}：${record.item.requested_model}`
            : modelDimension === 'requested' && record.item.upstream_model
              ? `${t('上游')}：${record.item.upstream_model}`
              : '';
    return (
      <div
        className='flex min-w-0 items-center'
        style={{ paddingLeft: descriptor.level * 22 }}
      >
        {descriptor.expandable ? (
          <Button
            theme='borderless'
            type='tertiary'
            icon={
              isExpanded ? (
                <ChevronDown size={16} />
              ) : (
                <ChevronRight size={16} />
              )
            }
            disabled={!descriptor.filterable}
            aria-label={isExpanded ? t('收起') : t('展开')}
            onClick={() => toggleNode(descriptor)}
          />
        ) : (
          <span className='inline-block w-8' />
        )}
        <div className='min-w-0'>
          <Text strong ellipsis={{ showTooltip: true }}>
            {display}
          </Text>
          <div>
            <Text type='tertiary' size='small'>
              {t(NODE_LABELS[descriptor.type] || descriptor.type)}
              {meta ? ` · ${meta}` : ''}
            </Text>
          </div>
        </div>
      </div>
    );
  };

  const columns = useMemo(() => {
    const windowColumns = windows.map((seconds) => ({
      title: t(stabilityWindowLabel(seconds)),
      width: 150,
      render: (_, record) =>
        record.kind === 'data' ? (
          <WindowCell window={windowMap(record.item).get(seconds)} t={t} />
        ) : null,
    }));
    return [
      {
        title: t('分析对象'),
        width: 270,
        fixed: 'left',
        render: (_, record) => renderIdentity(record),
      },
      ...windowColumns,
      {
        title: t('{{window}}运维详情', {
          window: t(stabilityWindowLabel(detailSeconds)),
        }),
        width: 270,
        render: (_, record) => {
          if (record.kind !== 'data') return null;
          const detail = windowMap(record.item).get(detailSeconds) || {};
          return (
            <div>
              <Text strong>
                {t('失败 {{failures}} · 重试 {{retries}}', {
                  failures: formatInteger(detail.failure_count),
                  retries: formatInteger(detail.retry_count),
                })}
              </Text>
              <div>
                <Text type='tertiary' size='small'>
                  {t('重试率 {{rate}}', {
                    rate: formatPercent(detail.retry_rate),
                  })}
                </Text>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  429 {formatInteger(detail.upstream_429_count)} · 5xx{' '}
                  {formatInteger(detail.upstream_5xx_count)}
                </Text>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  P95 {formatDuration(detail.p95_latency_ms)} · TTFT{' '}
                  {formatDuration(detail.p95_ttft_ms)}
                </Text>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  {detail.last_failure_bucket_ts
                    ? t('最近失败 {{time}}', {
                        time: formatRelativeTime(detail.last_failure_bucket_ts),
                      })
                    : t('暂无失败记录')}
                </Text>
              </div>
            </div>
          );
        },
      },
      {
        title: t('用量与缓存'),
        width: 220,
        render: (_, record) => {
          if (record.kind !== 'data') return null;
          const detail = windowMap(record.item).get(detailSeconds) || {};
          return (
            <div>
              <Text strong>
                {t('Token {{count}}', {
                  count: formatCompact(detail.total_tokens),
                })}
              </Text>
              <div>
                <Text type='tertiary' size='small'>
                  {t('缓存读取 {{count}}', {
                    count: formatCompact(detail.cache_read_tokens),
                  })}
                </Text>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  {t('Token 命中 {{rate}}', {
                    rate: formatPercent(detail.cache_token_hit_rate),
                  })}
                </Text>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  {t('用量覆盖 {{rate}}', {
                    rate: formatPercent(detail.usage_success_coverage_rate),
                  })}
                </Text>
              </div>
              <div>
                <Text type='tertiary' size='small'>
                  {t('实时 {{live}} · 历史 {{legacy}}', {
                    live: formatPercent(detail.live_event_rate),
                    legacy: formatPercent(detail.legacy_event_rate),
                  })}
                </Text>
              </div>
            </div>
          );
        },
      },
    ];
  }, [
    detailSeconds,
    entries,
    expanded,
    loadChildren,
    modelDimension,
    plan,
    t,
    windows,
  ]);

  return (
    <div className='space-y-4'>
      <Banner
        type='info'
        description={t(
          '同一行比较多个重叠时间窗；复合维度按层级逐级展开，并按需加载子项。',
        )}
      />
      <Card title={t('分组、模型与渠道稳定性')}>
        {error ? (
          <Banner type='danger' description={error} />
        ) : (
          <>
            <div className='mb-4 flex flex-wrap items-center gap-2 border-b pb-3'>
              <Select
                value={dimension}
                onChange={setDimension}
                style={{ width: 'min(190px, 100%)' }}
                optionList={[
                  {
                    value: 'group_channel_model',
                    label: t('分组 → 渠道 → 模型'),
                  },
                  { value: 'group_model', label: t('分组 → 模型') },
                  { value: 'channel_model', label: t('渠道 → 模型') },
                  { value: 'group_channel', label: t('分组 → 渠道') },
                  { value: 'model', label: t('仅模型') },
                  { value: 'channel', label: t('仅渠道') },
                  { value: 'group', label: t('仅分组') },
                ]}
              />
              <Select
                value={modelDimension}
                onChange={setModelDimension}
                disabled={!usesModel}
                style={{ width: 'min(130px, 100%)' }}
                optionList={[
                  { value: 'requested', label: t('请求模型') },
                  { value: 'upstream', label: t('上游模型') },
                ]}
              />
              <Select
                value={sortBy}
                onChange={setSortBy}
                style={{ width: 'min(180px, 100%)' }}
                optionList={[
                  { value: 'failure_count', label: t('15 分钟失败数') },
                  { value: 'quality_success_rate', label: t('15 分钟质量率') },
                  { value: 'request_count', label: t('15 分钟调用量') },
                  { value: 'p95_latency_ms', label: t('15 分钟 P95') },
                  { value: 'retry_rate', label: t('15 分钟重试率') },
                  { value: 'total_tokens', label: t('15 分钟 Token') },
                ]}
              />
            </div>
            <Table
              size='small'
              columns={columns}
              dataSource={flattenedRows}
              rowKey='key'
              loading={loading}
              pagination={false}
              scroll={{ x: 270 + windows.length * 150 + 490 }}
              empty={<Empty description={t('当前筛选没有可比较的运维样本')} />}
            />
            {total > PAGE_SIZE && (
              <div className='mt-4 flex justify-end'>
                <Pagination
                  currentPage={page}
                  pageSize={PAGE_SIZE}
                  total={total}
                  onPageChange={setPage}
                />
              </div>
            )}
          </>
        )}
      </Card>
    </div>
  );
};

export default OperationsView;
