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
import { Layers3, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import {
  API_ROOT,
  PAGE_SIZE,
  formatCompact,
  formatCost,
  formatDuration,
  formatInteger,
  formatPercent,
  formatRelativeTime,
  getErrorMessage,
  normalizeChannel,
  statusTagColor,
} from './utils';

const { Text } = Typography;

const StatusTags = ({ item }) => {
  const statuses = item.statusItems.length
    ? item.statusItems.slice(0, 3)
    : item.failureCount
      ? [{ status_code: 0, status_present: true, count: item.failureCount }]
      : [];
  if (!statuses.length) return <Text type='tertiary'>-</Text>;
  return (
    <Space wrap spacing={4}>
      {statuses.map((status, index) => {
        const present = status.status_present !== false;
        const code = Number(status.status_code ?? 0);
        return (
          <Tag
            key={`${present}-${code}-${index}`}
            color={statusTagColor(code, present)}
          >
            {!present ? '未知' : code === 0 ? '无响应' : code} ·{' '}
            {formatCompact(status.count)}
          </Tag>
        );
      })}
    </Space>
  );
};

const MetricPair = ({ primary, secondary, secondaryLabel }) => (
  <div>
    <Text strong>{primary}</Text>
    <div>
      <Text type='tertiary' size='small'>
        {secondaryLabel ? `${secondaryLabel} ` : ''}
        {secondary}
      </Text>
    </div>
  </div>
);

const ChannelsView = ({
  makeParams,
  queryKey,
  refreshKey,
  onStatus,
  onGroupChannels,
  onShowFailures,
}) => {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [sortBy, setSortBy] = useState('channel_name');
  const [modelDimension, setModelDimension] = useState('requested');
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [expandedKeys, setExpandedKeys] = useState([]);
  const [modelsByChannel, setModelsByChannel] = useState(new Map());
  const modelControllers = useRef(new Map());
  const modelGeneration = useRef(0);

  useEffect(() => {
    setPage(1);
  }, [queryKey]);

  useEffect(() => {
    modelGeneration.current += 1;
    modelControllers.current.forEach((controller) => controller.abort());
    modelControllers.current.clear();
    setExpandedKeys([]);
    setModelsByChannel(new Map());
  }, [modelDimension, page, queryKey, refreshKey, sortBy]);

  useEffect(
    () => () => {
      modelGeneration.current += 1;
      modelControllers.current.forEach((controller) => controller.abort());
      modelControllers.current.clear();
    },
    [],
  );

  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      setLoading(true);
      setError('');
      onStatus?.({ loading: true, error: '', meta: null });
      try {
        const params = makeParams(
          {
            page,
            page_size: PAGE_SIZE,
            sort_by: sortBy,
            sort_order: sortBy === 'channel_name' ? 'asc' : 'desc',
          },
          { includeStatus: false },
        );
        const response = await API.get(`${API_ROOT}/channels`, {
          params,
          signal: controller.signal,
          skipErrorHandler: true,
        });
        const payload = response.data?.data || {};
        const rows = (payload.items || []).map(normalizeChannel);
        setItems(rows);
        setTotal(Number(payload.total || rows.length));
        setExpandedKeys([]);
        setModelsByChannel(new Map());
        onStatus?.({
          loading: false,
          error: '',
          meta: payload.meta,
          empty: rows.length === 0,
        });
      } catch (requestError) {
        if (requestError?.name === 'CanceledError' || controller.signal.aborted)
          return;
        const message = getErrorMessage(requestError, t('渠道统计加载失败'));
        setError(message);
        onStatus?.({ loading: false, error: message, meta: null });
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };
    load();
    return () => controller.abort();
  }, [makeParams, onStatus, page, queryKey, refreshKey, sortBy, t]);

  useEffect(() => {
    setExpandedKeys([]);
    setModelsByChannel(new Map());
  }, [modelDimension]);

  const loadModels = useCallback(
    async (channelId, modelPage = 1) => {
      modelControllers.current.get(channelId)?.abort();
      const controller = new AbortController();
      const generation = modelGeneration.current;
      modelControllers.current.set(channelId, controller);
      setModelsByChannel((current) => {
        const next = new Map(current);
        next.set(channelId, {
          ...(current.get(channelId) || {}),
          loading: true,
          error: '',
          page: modelPage,
        });
        return next;
      });
      try {
        const params = makeParams(
          {
            page: modelPage,
            page_size: PAGE_SIZE,
            sort_by: 'request_count',
            sort_order: 'desc',
            model_dimension: modelDimension,
          },
          { includeStatus: false },
        );
        const response = await API.get(
          `${API_ROOT}/channels/${encodeURIComponent(channelId)}/models`,
          { params, signal: controller.signal, skipErrorHandler: true },
        );
        if (controller.signal.aborted || generation !== modelGeneration.current)
          return;
        const payload = response.data?.data || {};
        const rows = (payload.items || []).map(normalizeChannel);
        setModelsByChannel((current) => {
          const next = new Map(current);
          next.set(channelId, {
            loading: false,
            error: '',
            items: rows,
            page: Number(payload.page || modelPage),
            total: Number(payload.total || rows.length),
          });
          return next;
        });
      } catch (requestError) {
        if (
          requestError?.name === 'CanceledError' ||
          controller.signal.aborted ||
          generation !== modelGeneration.current
        )
          return;
        setModelsByChannel((current) => {
          const next = new Map(current);
          next.set(channelId, {
            ...(current.get(channelId) || {}),
            loading: false,
            error: getErrorMessage(requestError, t('模型统计加载失败')),
          });
          return next;
        });
      } finally {
        if (modelControllers.current.get(channelId) === controller) {
          modelControllers.current.delete(channelId);
        }
      }
    },
    [makeParams, modelDimension, t],
  );

  const handleExpand = (expanded, record) => {
    setExpandedKeys((current) =>
      expanded
        ? [...new Set([...current, record.id])]
        : current.filter((key) => key !== record.id),
    );
    if (!expanded) {
      modelControllers.current.get(record.id)?.abort();
      modelControllers.current.delete(record.id);
      setModelsByChannel((current) => {
        const entry = current.get(record.id);
        if (!entry?.loading) return current;
        const next = new Map(current);
        next.set(record.id, { ...entry, loading: false });
        return next;
      });
    } else if (!modelsByChannel.get(record.id)?.items) {
      loadModels(record.id, 1);
    }
  };

  const columns = useMemo(
    () => [
      {
        title: t('渠道'),
        dataIndex: 'name',
        width: 230,
        fixed: 'left',
        render: (_, record) => (
          <div className='min-w-0'>
            <Text strong ellipsis={{ showTooltip: true }}>
              {record.name}
            </Text>
            <div>
              <Text type='tertiary' size='small'>
                #{record.id} · {record.typeName}
                {record.group ? ` · ${t('配置分组')} ${record.group}` : ''}
              </Text>
            </div>
          </div>
        ),
      },
      {
        title: t('调用 / 重试'),
        width: 130,
        render: (_, record) => (
          <MetricPair
            primary={formatInteger(record.calls)}
            secondary={formatInteger(record.retries)}
            secondaryLabel={t('重试')}
          />
        ),
      },
      {
        title: t('质量成功率'),
        width: 130,
        render: (_, record) => (
          <MetricPair
            primary={formatPercent(record.successRate)}
            secondary={t('可归因渠道样本')}
          />
        ),
      },
      {
        title: t('状态码'),
        width: 190,
        render: (_, record) => <StatusTags item={record} />,
      },
      {
        title: t('输入 / 输出'),
        width: 130,
        render: (_, record) =>
          `${formatCompact(record.input)} / ${formatCompact(record.output)}`,
      },
      {
        title: t('缓存读 / 写'),
        width: 130,
        render: (_, record) =>
          `${formatCompact(record.cacheRead)} / ${formatCompact(record.cacheWrite)}`,
      },
      {
        title: t('缓存命中'),
        width: 100,
        render: (_, record) => formatPercent(record.cacheHitRate),
      },
      {
        title: t('平均 / P95'),
        width: 180,
        render: (_, record) => (
          <MetricPair
            primary={`${formatDuration(record.avgLatency)} / ${formatDuration(record.p95Latency)}`}
            secondary={`${formatDuration(record.avgTtft)} / ${formatDuration(record.p95Ttft)}`}
            secondaryLabel='TTFT'
          />
        ),
      },
      {
        title: t('费用'),
        width: 100,
        render: (_, record) => formatCost(record),
      },
      {
        title: t('最近失败'),
        width: 120,
        render: (_, record) =>
          record.lastFailure ? (
            <Text
              link
              onClick={() => onShowFailures?.(record.id)}
              style={{ cursor: 'pointer' }}
            >
              {formatRelativeTime(record.lastFailure)}
            </Text>
          ) : (
            <Text type='tertiary'>{t('暂无')}</Text>
          ),
      },
    ],
    [onShowFailures, t],
  );

  const renderExpanded = (channel) => {
    const entry = modelsByChannel.get(channel.id);
    if (!entry || entry.loading) {
      return (
        <div className='flex min-h-24 items-center justify-center'>
          <Spin size='small' tip={t('正在加载模型统计')} />
        </div>
      );
    }
    if (entry.error) {
      return (
        <Banner
          type='danger'
          description={entry.error}
          closeIcon={null}
          action={
            <Button
              size='small'
              icon={<RefreshCw size={14} />}
              onClick={() => loadModels(channel.id, entry.page || 1)}
            >
              {t('重试')}
            </Button>
          }
        />
      );
    }
    if (!entry.items?.length) {
      return <Empty description={t('当前范围暂无模型级统计')} />;
    }

    const rows = entry.items.map((model) => {
      const requested = String(model.requested_model || '');
      const upstream = String(model.upstream_model || '');
      return {
        ...model,
        modelName:
          modelDimension === 'upstream'
            ? upstream || requested || t('未知模型')
            : requested || upstream || t('未知模型'),
        mapping:
          requested && upstream && requested !== upstream
            ? modelDimension === 'upstream'
              ? `${t('请求')}：${requested}`
              : `${t('上游')}：${upstream}`
            : '',
      };
    });
    const modelColumns = [
      {
        title: modelDimension === 'upstream' ? t('上游模型') : t('请求模型'),
        width: 230,
        render: (_, record) => (
          <div>
            <Text strong ellipsis={{ showTooltip: true }}>
              {record.modelName}
            </Text>
            {record.mapping && (
              <div>
                <Text type='tertiary' size='small'>
                  {record.mapping}
                </Text>
              </div>
            )}
          </div>
        ),
      },
      ...columns.slice(1),
    ];
    return (
      <div className='space-y-3 py-1'>
        <Table
          size='small'
          columns={modelColumns}
          dataSource={rows}
          rowKey={(record) => record.model_hash || record.modelName}
          pagination={false}
          scroll={{ x: 1400 }}
        />
        {entry.total > PAGE_SIZE && (
          <div className='flex justify-end'>
            <Pagination
              size='small'
              currentPage={entry.page}
              pageSize={PAGE_SIZE}
              total={entry.total}
              onPageChange={(nextPage) => loadModels(channel.id, nextPage)}
            />
          </div>
        )}
      </div>
    );
  };

  return (
    <Card title={t('渠道与模型')}>
      {error ? (
        <Banner type='danger' description={error} />
      ) : (
        <>
          <div className='mb-4 flex flex-wrap items-center gap-2 border-b pb-3'>
            <Select
              value={modelDimension}
              onChange={setModelDimension}
              style={{ width: 'min(130px, 100%)' }}
              optionList={[
                { value: 'requested', label: t('请求模型') },
                { value: 'upstream', label: t('上游模型') },
              ]}
            />
            <Select
              value={sortBy}
              onChange={(value) => {
                setSortBy(value);
                setPage(1);
              }}
              style={{ width: 'min(170px, 100%)' }}
              optionList={[
                { value: 'channel_name', label: t('渠道名称') },
                { value: 'request_count', label: t('调用量') },
                { value: 'quality_success_rate', label: t('渠道质量成功率') },
                { value: 'p95_latency_ms', label: t('P95 延迟') },
                { value: 'charged_quota', label: t('计费额度') },
                { value: 'failure_count', label: t('失败数') },
              ]}
            />
            <Button
              icon={<Layers3 size={16} />}
              onClick={() => onGroupChannels?.(modelDimension)}
            >
              {t('按请求分组')}
            </Button>
          </div>
          <Table
            size='small'
            columns={columns}
            dataSource={items}
            rowKey='id'
            loading={loading}
            pagination={false}
            expandedRowKeys={expandedKeys}
            onExpand={handleExpand}
            expandedRowRender={renderExpanded}
            scroll={{ x: 1510 }}
            empty={<Empty description={t('当前筛选条件下暂无渠道统计')} />}
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
  );
};

export default ChannelsView;
