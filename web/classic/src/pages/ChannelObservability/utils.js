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

export const API_ROOT = '/api/channel-analytics';
export const PAGE_SIZE = 30;
export const STABILITY_WINDOWS = [900, 3600, 21600, 86400, 604800];

export const TREE_PLANS = {
  group_model: { levels: ['group', 'model'] },
  group_channel: { levels: ['group', 'channel'] },
  channel_model: { levels: ['channel', 'model'] },
  group_channel_model: { levels: ['group', 'channel', 'model'] },
};

export const CHANNEL_TYPES = {
  0: '未知',
  1: 'OpenAI',
  2: 'Midjourney',
  3: 'Azure',
  4: 'Ollama',
  5: 'MidjourneyPlus',
  6: 'OpenAIMax',
  7: 'OhMyGPT',
  8: 'Custom',
  9: 'AILS',
  10: 'AIProxy',
  11: 'PaLM',
  12: 'API2GPT',
  13: 'AIGC2D',
  14: 'Anthropic',
  15: '百度',
  16: '智谱',
  17: '阿里云',
  18: '讯飞',
  19: '360',
  20: 'OpenRouter',
  21: 'AIProxyLibrary',
  22: 'FastGPT',
  23: '腾讯',
  24: 'Gemini',
  25: 'Moonshot',
  26: '智谱 V4',
  27: 'Perplexity',
  31: '零一万物',
  33: 'AWS',
  34: 'Cohere',
  35: 'MiniMax',
  36: 'SunoAPI',
  37: 'Dify',
  38: 'Jina',
  39: 'Cloudflare',
  40: 'SiliconFlow',
  41: 'Vertex AI',
  42: 'Mistral',
  43: 'DeepSeek',
  44: 'MokaAI',
  45: '火山引擎',
  46: '百度 V2',
  47: 'Xinference',
  48: 'xAI',
  49: 'Coze',
  50: '可灵',
  51: '即梦',
  52: 'Vidu',
  53: '子模型',
  54: '豆包视频',
  55: 'Sora',
  56: 'Replicate',
  57: 'Codex',
};

export const OUTCOME_LABELS = {
  success: '成功',
  http_error: 'HTTP 错误',
  transport_error: '连接错误',
  protocol_error: '协议错误',
  stream_error: '流式错误',
  local_error: '本地错误',
  dispatch_error: '派发错误',
  client_cancelled: '客户端取消',
};

export const STAGE_LABELS = {
  auth: '鉴权',
  authentication: '鉴权',
  dispatch: '选渠与派发',
  channel_selection: '选渠',
  pre_upstream: '上游请求前',
  connect: '连接上游',
  upstream_response: '上游响应',
  stream: '流式传输',
  stream_transfer: '流式传输',
  parse: '协议解析',
  parsing: '协议解析',
  settlement: '结算',
  unfinalized_call: '调用未正常收尾',
  unknown: '未知阶段',
};

export const FAILURE_OWNER_LABELS = {
  channel: '渠道',
  client: '客户端',
  gateway: '网关',
  unknown: '未知',
};

export const getErrorMessage = (error, fallback = '加载失败') =>
  error?.response?.data?.message || error?.message || fallback;

export const normalizeTimestamp = (value) => {
  if (value instanceof Date) return Math.floor(value.getTime() / 1000);
  if (typeof value === 'string' && !/^\d+(\.\d+)?$/.test(value)) {
    const parsed = Date.parse(value);
    return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : 0;
  }
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) return 0;
  return number > 1e12 ? Math.floor(number / 1000) : Math.floor(number);
};

export const formatInteger = (value) => {
  const number = Number(value);
  return Number.isFinite(number)
    ? Math.round(number).toLocaleString('zh-CN')
    : '-';
};

export const formatCompact = (value, digits = 1) => {
  const number = Number(value);
  if (!Number.isFinite(number)) return '-';
  const absolute = Math.abs(number);
  const units = [
    [1e12, 'T'],
    [1e9, 'B'],
    [1e6, 'M'],
    [1e3, 'K'],
  ];
  for (const [threshold, suffix] of units) {
    if (absolute >= threshold) {
      return `${(number / threshold).toFixed(digits).replace(/\.0$/, '')}${suffix}`;
    }
  }
  return absolute >= 100
    ? Math.round(number).toLocaleString('zh-CN')
    : number.toLocaleString('zh-CN', { maximumFractionDigits: digits });
};

export const percentNumber = (value) => {
  if (value === null || value === undefined || value === '') return null;
  const number = Number(value);
  if (!Number.isFinite(number)) return null;
  return Math.abs(number) <= 1 ? number * 100 : number;
};

export const formatPercent = (value, digits = 1) => {
  const percent = percentNumber(value);
  return percent === null ? '-' : `${percent.toFixed(digits)}%`;
};

export const formatDuration = (value) => {
  const milliseconds = Number(value);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return '-';
  if (milliseconds < 1000) return `${Math.round(milliseconds)}ms`;
  if (milliseconds < 60000) {
    return `${(milliseconds / 1000).toFixed(milliseconds < 10000 ? 2 : 1)}s`;
  }
  return `${(milliseconds / 60000).toFixed(1)}min`;
};

export const formatDateTime = (value, includeDate = true) => {
  const timestamp = normalizeTimestamp(value);
  if (!timestamp) return '-';
  return new Intl.DateTimeFormat('zh-CN', {
    month: includeDate ? '2-digit' : undefined,
    day: includeDate ? '2-digit' : undefined,
    hour: '2-digit',
    minute: '2-digit',
    second: includeDate ? '2-digit' : undefined,
    hour12: false,
  }).format(new Date(timestamp * 1000));
};

export const formatRelativeTime = (value) => {
  const timestamp = normalizeTimestamp(value);
  if (!timestamp) return '-';
  const seconds = Math.max(0, Math.floor(Date.now() / 1000) - timestamp);
  if (seconds < 60) return '刚刚';
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`;
  if (seconds < 86400 * 7) return `${Math.floor(seconds / 86400)} 天前`;
  return formatDateTime(timestamp);
};

export const resolveRange = (range, customRange) => {
  const now = new Date();
  const end = Math.floor(now.getTime() / 1000);
  const today = Math.floor(
    new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime() / 1000,
  );
  if (range === '1h') return [end - 3600, end];
  if (range === 'yesterday') return [today - 86400, today];
  if (range === '7d') return [end - 7 * 86400, end];
  if (range === 'custom' && customRange?.length === 2) {
    return [
      normalizeTimestamp(customRange[0]),
      normalizeTimestamp(customRange[1]),
    ];
  }
  return [today, end];
};

export const normalizeStatusItems = (payload) =>
  (payload?.items || payload?.status_codes || [])
    .map((row) => {
      const present = row.status_present !== false;
      const code = Number(row.status_code ?? row.code ?? 0);
      const label =
        row.label ||
        (!present
          ? '未知 / 不适用'
          : code === 0
            ? '无 HTTP 响应'
            : String(code));
      return { code, present, label, count: Number(row.count || 0) };
    })
    .sort((left, right) => right.count - left.count);

export const statusTagColor = (code, present = true) => {
  if (!present || code === 0) return 'grey';
  if (code >= 200 && code < 300) return 'green';
  if (code >= 400 && code < 500) return 'amber';
  if (code >= 500) return 'red';
  return 'blue';
};

export const normalizeChannel = (row) => ({
  ...row,
  id: Number(row.channel_id ?? row.id ?? 0),
  name: String(
    row.channel_name || row.channel_name_snapshot || row.name || '未知渠道',
  ),
  type: Number(row.channel_type ?? row.type ?? 0),
  typeName: String(
    row.channel_type_name ||
      row.type_name ||
      CHANNEL_TYPES[row.channel_type ?? row.type] ||
      `类型 ${row.channel_type ?? row.type ?? 0}`,
  ),
  calls: Number(
    row.channel_attempt_count ?? row.attempt_count ?? row.request_count ?? 0,
  ),
  retries: Number(row.retry_count ?? 0),
  successRate:
    row.channel_quality_success_rate ?? row.quality_success_rate ?? null,
  input: Number(row.input_tokens_total ?? row.input_tokens ?? 0),
  output: Number(row.output_tokens ?? 0),
  cacheRead: Number(row.cache_read_tokens ?? 0),
  cacheWrite: Number(row.cache_write_tokens ?? 0),
  cacheHitRate: row.cache_request_hit_rate ?? row.cache_hit_rate ?? null,
  avgLatency: Number(row.avg_latency_ms ?? Number.NaN),
  p95Latency: Number(row.p95_latency_ms ?? Number.NaN),
  avgTtft: Number(row.avg_ttft_ms ?? Number.NaN),
  p95Ttft: Number(row.p95_ttft_ms ?? Number.NaN),
  chargedQuota: Number(row.charged_quota ?? 0),
  chargedMicroUsd: Number(row.charged_micro_usd ?? Number.NaN),
  lastFailure: normalizeTimestamp(row.last_failure_at),
  statusItems: Array.isArray(row.top_status_codes) ? row.top_status_codes : [],
  failureCount: Number(row.failure_count ?? row.failed_attempt_count ?? 0),
});

export const formatCost = (item) =>
  Number.isFinite(item.chargedMicroUsd)
    ? `$${(item.chargedMicroUsd / 1e6).toFixed(4)}`
    : formatCompact(item.chargedQuota);

export const modelOption = (item) => ({
  value: String(item?.value || item?.model_hash || item || ''),
  label: String(item?.label || item?.model || item || ''),
  model: String(item?.model || item?.label || item || ''),
  hash: String(item?.model_hash || ''),
});

export const appendCommonParams = (
  params,
  { filters, range, customRange, granularity },
  options = {},
) => {
  const [start, end] = resolveRange(range, customRange);
  params.set('start_timestamp', String(start));
  params.set('end_timestamp', String(end));
  params.set('granularity', granularity || 'auto');
  params.set('traffic_source', filters.trafficSource || 'relay');
  params.set('data_origin', filters.dataOrigin || 'live,legacy');
  if (filters.channelId) params.set('channel_ids', filters.channelId);
  if (filters.channelType) params.set('channel_types', filters.channelType);
  if (filters.group) params.set('groups', filters.group);
  if (filters.requestedModelHash) {
    params.set('requested_model_hashes', filters.requestedModelHash);
  } else if (filters.requestedModel) {
    params.set('requested_models', filters.requestedModel);
  }
  if (filters.upstreamModelHash) {
    params.set('upstream_model_hashes', filters.upstreamModelHash);
  } else if (filters.upstreamModel) {
    params.set('upstream_models', filters.upstreamModel);
  }
  if (options.includeOutcome !== false && filters.outcome) {
    params.set('outcome', filters.outcome);
  }
  if (options.includeStream !== false && filters.stream !== '') {
    params.set('stream', filters.stream);
  }
  if (options.includeStatus !== false && filters.statusCode) {
    params.set(
      options.statusScope === 'client'
        ? 'client_status_codes'
        : 'upstream_status_codes',
      filters.statusCode,
    );
  }
  return params;
};

export const stabilityWindowLabel = (seconds) =>
  ({
    900: '15 分钟',
    3600: '1 小时',
    21600: '6 小时',
    86400: '24 小时',
    604800: '7 天',
  })[Number(seconds)] || `${formatCompact(seconds)} 秒`;

export const buildQualityMessages = (meta, queryStart, dataOrigin) => {
  if (!meta || typeof meta !== 'object') return [];
  const messages = [];
  if (meta.partial) messages.push('当前区间数据不完整');
  if (meta.invalid_sample_count > 0) {
    messages.push(
      `${formatInteger(meta.invalid_sample_count)} 条无效样本未计入统计`,
    );
  }
  if (meta.dimension_overflow_count > 0) {
    messages.push(
      `${formatInteger(meta.dimension_overflow_count)} 个高基数维度已合并到“其他”`,
    );
  }
  if (meta.dimension_hash_collision_count > 0) {
    messages.push(
      `${formatInteger(meta.dimension_hash_collision_count)} 次维度哈希冲突已隔离`,
    );
  }
  if (meta.dropped_metric_event_count > 0) {
    messages.push(
      `${formatInteger(meta.dropped_metric_event_count)} 条指标因保护上限被丢弃`,
    );
  }
  if (meta.dropped_failure_event_count > 0) {
    messages.push(
      `${formatInteger(meta.dropped_failure_event_count)} 条失败明细未保存`,
    );
  }
  if (meta.runtime_pending_batch_count > 0) {
    messages.push(
      `${formatInteger(meta.runtime_pending_batch_count)} 个指标批次等待重新写入`,
    );
  }
  if (
    meta.runtime_last_flush_error_at > 0 &&
    meta.runtime_last_flush_error_at > meta.last_flushed_at
  ) {
    messages.push(
      `指标写入最近失败于 ${formatDateTime(meta.runtime_last_flush_error_at)}`,
    );
  }
  if (
    Array.isArray(meta.uncovered_channel_types) &&
    meta.uncovered_channel_types.length
  ) {
    messages.push(
      `部分渠道类型尚未完整采集：${meta.uncovered_channel_types
        .map((type) => CHANNEL_TYPES[type] || `类型 ${type}`)
        .join('、')}`,
    );
  }
  if (meta.detail_available === false) messages.push('完整日志明细当前不可用');
  if (String(dataOrigin).includes('legacy')) {
    messages.push(
      '历史日志为推导口径，重试链、失败请求用量、TTFT 和上游原始状态码可能不完整',
    );
    const backfill = meta.backfill;
    if (backfill?.status === 'running' || backfill?.status === 'pending') {
      const progress = backfill.total_rows
        ? Math.min(100, (backfill.scanned_rows / backfill.total_rows) * 100)
        : 0;
      messages.push(`历史日志回填中：${progress.toFixed(1)}%`);
    } else if (backfill?.status === 'failed') {
      messages.push(`历史日志回填失败：${backfill.last_error || '未知错误'}`);
    }
  }
  if (meta.reliable_from_ts && queryStart < meta.reliable_from_ts) {
    messages.push(`实时精确采集始于 ${formatDateTime(meta.reliable_from_ts)}`);
  }
  return messages;
};
