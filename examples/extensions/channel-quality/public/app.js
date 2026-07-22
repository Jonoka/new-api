(() => {
  'use strict'

  const API_ROOT = '/api/channel-analytics'
  const HOST_CONTEXT_API = '/api/extensions/host/me'
  const REQUIRED_HOST_VERSION = 'v1.0.0-rc.10.1.10.200'
  const REQUIRED_CAPABILITY_API = `${API_ROOT}/filters/models?model_dimension=requested&page=1&page_size=1`
  const MODULE_VERSION = '0.3.6'
  const UI_ASSET_VERSION = MODULE_VERSION
  const REQUIRED_UI_ELEMENT_IDS = [
    'dataOriginFilter',
    'groupFilter',
    'operationsView',
    'requestedModelSearch',
    'upstreamModelSearch',
    'stabilityDimension',
    'stabilityModelDimension',
    'stabilitySort',
    'stabilityHead',
    'stabilityRows',
    'stabilityPagination',
    'groupChannelsButton',
    'channelSort',
    'modelDimension',
  ]
  const DEFAULT_PAGE_SIZE = 30
  const MODEL_PAGE_SIZE = 30
  const STABILITY_CHILD_PAGE_SIZE = 30
  const STABILITY_WINDOWS = [900, 3600, 21600, 86400, 604800]
  const STABILITY_NODE_LABELS = Object.freeze({
    group: '分组',
    channel: '渠道',
    model: '模型',
  })
  const STABILITY_TREE_PLANS = Object.freeze({
    group_model: Object.freeze({ dimension: 'group_model', levels: Object.freeze(['group', 'model']) }),
    group_channel: Object.freeze({ dimension: 'group_channel', levels: Object.freeze(['group', 'channel']) }),
    channel_model: Object.freeze({ dimension: 'channel_model', levels: Object.freeze(['channel', 'model']) }),
    group_channel_model: Object.freeze({
      dimension: 'group_channel_model',
      levels: Object.freeze(['group', 'channel', 'model']),
    }),
  })

  const channelTypes = {
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
  }

  const outcomeLabels = {
    success: '成功',
    http_error: 'HTTP 错误',
    transport_error: '连接错误',
    protocol_error: '协议错误',
    stream_error: '流式错误',
    local_error: '本地错误',
    dispatch_error: '派发错误',
    client_cancelled: '客户端取消',
  }

  const stageLabels = {
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
  }

  const failureOwnerLabels = {
    channel: '渠道',
    client: '客户端',
    gateway: '网关',
    unknown: '未知',
  }

  const state = {
    view: 'overview',
    range: 'today',
    granularity: 'auto',
    customStart: 0,
    customEnd: 0,
    retentionDays: 7,
    statusScope: 'upstream',
    modelDimension: 'requested',
    stabilityPage: 1,
    stabilityTotal: 0,
    stabilityItems: [],
    stabilityWindows: [...STABILITY_WINDOWS],
    stabilityTreeMode: false,
    stabilityTreePlan: null,
    stabilityTreeGeneration: 0,
    stabilityTreeQueryParams: null,
    stabilityExpandedNodes: new Set(),
    stabilityTreeEntries: new Map(),
    stabilityTreeNodes: new Map(),
    channelPage: 1,
    channelTotal: 0,
    failurePage: 1,
    failureTotal: 0,
    loadingId: 0,
    controller: null,
    initialized: false,
    hostCompatible: false,
    hostVersion: '',
    filtersLoaded: false,
    channels: [],
    expandedChannels: new Set(),
    channelModels: new Map(),
    probeChannels: [],
    probeKeyword: '',
    probeModels: new Map(),
    probeResults: new Map(),
    testingChannels: new Set(),
    summary: null,
    trend: null,
    statuses: null,
    failures: null,
  }

  const $ = (id) => document.getElementById(id)
  const $$ = (selector, root = document) => Array.from(root.querySelectorAll(selector))

  function verifyUIAssetCompatibility() {
    const documentVersion = String(document.documentElement.dataset.channelQualityUiVersion || '').trim()
    const missingElements = REQUIRED_UI_ELEMENT_IDS.filter((id) => !$(id))
    if (documentVersion === UI_ASSET_VERSION && !missingElements.length) return true

    const message =
      `检测到渠道可观测性静态资源版本不一致（页面：${documentVersion || '旧版或未知'}，` +
      `脚本：${UI_ASSET_VERSION}）。请先强制刷新页面；若问题仍存在，请确认负载均衡后的所有节点` +
      '都安装了同一版本扩展。'
    const loadingState = $('loadingState')
    const emptyState = $('emptyState')
    const errorState = $('errorState')
    const errorMessage = $('errorMessage')

    loadingState?.classList.add('is-hidden')
    emptyState?.classList.add('is-hidden')
    $$('.view-panel').forEach((panel) => panel.classList.add('is-hidden'))

    if (errorState && errorMessage) {
      errorMessage.textContent = message
      errorState.classList.remove('is-hidden')
      const retryButton = $('retryButton')
      if (retryButton) {
        retryButton.textContent = '重新加载最新版本'
        retryButton.addEventListener('click', () => {
          const url = new URL(window.location.href)
          url.searchParams.set('_module_reload', String(Date.now()))
          window.location.replace(url.toString())
        }, { once: true })
      }
    } else {
      const fallback = document.createElement('section')
      fallback.setAttribute('role', 'alert')
      fallback.style.cssText =
        'margin:24px;padding:18px;border:1px solid #fca5a5;border-radius:12px;background:#fff7f7;color:#991b1b;'
      const title = document.createElement('strong')
      title.textContent = '扩展静态资源加载不完整'
      const detail = document.createElement('p')
      detail.style.margin = '8px 0 0'
      detail.textContent = message
      fallback.append(title, detail)
      document.body.prepend(fallback)
    }
    return false
  }

  function escapeHtml(value) {
    return String(value ?? '')
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll("'", '&#039;')
  }

  function firstDefined(object, paths, fallback) {
    for (const path of paths) {
      const parts = path.split('.')
      let cursor = object
      let found = true
      for (const part of parts) {
        if (cursor == null || !Object.prototype.hasOwnProperty.call(cursor, part)) {
          found = false
          break
        }
        cursor = cursor[part]
      }
      if (found && cursor !== undefined && cursor !== null) return cursor
    }
    return fallback
  }

  function numberValue(object, paths, fallback = 0) {
    const raw = firstDefined(object, paths, fallback)
    const value = Number(raw)
    return Number.isFinite(value) ? value : fallback
  }

  function arrayValue(object, paths) {
    const value = firstDefined(object, paths, [])
    return Array.isArray(value) ? value : []
  }

  function booleanValue(object, paths, fallback = false) {
    const value = firstDefined(object, paths, fallback)
    if (typeof value === 'string') return value === 'true' || value === '1'
    return Boolean(value)
  }

  function getUserId() {
    try {
      const uid = window.localStorage.getItem('uid')
      if (uid) return uid
      const user = JSON.parse(window.localStorage.getItem('user') || 'null')
      if (user?.id) return String(user.id)
    } catch (_error) {
      return ''
    }
    return ''
  }

  function commonHeaders(extra = {}) {
    const headers = { Accept: 'application/json', ...extra }
    const userId = getUserId()
    if (userId) headers['New-Api-User'] = userId
    return headers
  }

  async function requestJson(path, options = {}) {
    let response
    try {
      response = await fetch(path, {
        credentials: 'include',
        cache: 'no-store',
        ...options,
        headers: commonHeaders(options.headers),
      })
    } catch (error) {
      if (error?.name === 'AbortError') throw error
      throw new Error(`无法连接到宿主接口：${error?.message || '网络错误'}`)
    }

    const text = await response.text()
    let body = null
    if (text) {
      try {
        body = JSON.parse(text)
      } catch (_error) {
        if (!response.ok) {
          const error = new Error(`接口返回 HTTP ${response.status}`)
          error.status = response.status
          throw error
        }
        throw new Error('接口返回了无法解析的数据')
      }
    }

    if (!response.ok || body?.success === false) {
      const message = body?.message || body?.error || `接口返回 HTTP ${response.status}`
      const error = new Error(message)
      error.status = response.status
      throw error
    }

    if (body && Object.prototype.hasOwnProperty.call(body, 'data')) return body.data
    return body ?? {}
  }

  function hostVersionAtLeast(current, required) {
    // 宿主版本包含 rc 段，按全部数字片段逐段比较。
    const currentParts = String(current || '').match(/\d+/g)?.map(Number) || []
    const requiredParts = String(required || '').match(/\d+/g)?.map(Number) || []
    if (!currentParts.length || !requiredParts.length) return true
    const length = Math.max(currentParts.length, requiredParts.length)
    for (let index = 0; index < length; index += 1) {
      const currentPart = currentParts[index] || 0
      const requiredPart = requiredParts[index] || 0
      if (currentPart !== requiredPart) return currentPart > requiredPart
    }
    return true
  }

  function hostCompatibilityError(currentVersion) {
    const error = new Error(
      `当前宿主为 ${currentVersion || '未知版本'}，` +
      `渠道可观测性 ${MODULE_VERSION} 要求至少 ${REQUIRED_HOST_VERSION}，请先升级所有后端节点后再使用。`,
    )
    error.hostCompatibility = true
    return error
  }

  async function verifyHostCompatibility(signal) {
    if (state.hostCompatible) return

    // 旧版宿主只会读取 manifest.host，不会真正执行最低版本约束。
    // 因此必须在任何统计查询前探测当前模块依赖的新接口，避免把
    // 404 或旧版查询参数错误误报成统计数据故障。
    const [hostResult, capabilityResult] = await Promise.allSettled([
      requestJson(HOST_CONTEXT_API, { signal }),
      requestJson(REQUIRED_CAPABILITY_API, { signal }),
    ])

    if (hostResult.status === 'fulfilled') {
      state.hostVersion = String(firstDefined(hostResult.value, ['version'], '')).trim()
    }
    if (state.hostVersion && !hostVersionAtLeast(state.hostVersion, REQUIRED_HOST_VERSION)) {
      throw hostCompatibilityError(state.hostVersion)
    }
    if (capabilityResult.status === 'fulfilled') {
      state.hostCompatible = true
      return
    }

    const capabilityError = capabilityResult.reason
    if (capabilityError?.name === 'AbortError') throw capabilityError
    if (capabilityError?.status !== 404) throw capabilityError

    throw hostCompatibilityError(state.hostVersion)
  }

  function showToast(message) {
    const toast = $('toast')
    toast.textContent = String(message || '')
    toast.classList.remove('is-hidden')
    window.clearTimeout(showToast.timer)
    showToast.timer = window.setTimeout(() => toast.classList.add('is-hidden'), 3800)
  }

  function formatProbeError(message) {
    const normalized = String(message || '探测失败').replace(/\s+/g, ' ').trim()
    const limit = 280
    return normalized.length > limit ? `${normalized.slice(0, limit - 1)}…` : normalized
  }

  function formatInteger(value) {
    const number = Number(value)
    if (!Number.isFinite(number)) return '-'
    return Math.round(number).toLocaleString('zh-CN')
  }

  function formatCompact(value, digits = 1) {
    const number = Number(value)
    if (!Number.isFinite(number)) return '-'
    const absolute = Math.abs(number)
    const units = [
      [1e12, 'T'],
      [1e9, 'B'],
      [1e6, 'M'],
      [1e3, 'K'],
    ]
    for (const [threshold, suffix] of units) {
      if (absolute >= threshold) {
        return `${(number / threshold).toFixed(digits).replace(/\.0$/, '')}${suffix}`
      }
    }
    return absolute >= 100 ? Math.round(number).toLocaleString('zh-CN') : number.toLocaleString('zh-CN', {
      maximumFractionDigits: digits,
    })
  }

  function percentNumber(value) {
    if (value === null || value === undefined || value === '') return null
    const number = Number(value)
    if (!Number.isFinite(number)) return null
    return Math.abs(number) <= 1 ? number * 100 : number
  }

  function formatPercent(value, digits = 1) {
    const percent = percentNumber(value)
    if (percent === null) return '-'
    return `${percent.toFixed(digits)}%`
  }

  function formatDuration(value) {
    const milliseconds = Number(value)
    if (!Number.isFinite(milliseconds) || milliseconds < 0) return '-'
    if (milliseconds < 1000) return `${Math.round(milliseconds)}ms`
    if (milliseconds < 60000) return `${(milliseconds / 1000).toFixed(milliseconds < 10000 ? 2 : 1)}s`
    return `${(milliseconds / 60000).toFixed(1)}min`
  }

  function formatDateTime(value, includeDate = true) {
    const timestamp = normalizeTimestamp(value)
    if (!timestamp) return '-'
    return new Intl.DateTimeFormat('zh-CN', {
      month: includeDate ? '2-digit' : undefined,
      day: includeDate ? '2-digit' : undefined,
      hour: '2-digit',
      minute: '2-digit',
      second: includeDate ? '2-digit' : undefined,
      hour12: false,
    }).format(new Date(timestamp * 1000))
  }

  function formatRelativeTime(value) {
    const timestamp = normalizeTimestamp(value)
    if (!timestamp) return '-'
    const seconds = Math.max(0, Math.floor(Date.now() / 1000) - timestamp)
    if (seconds < 60) return '刚刚'
    if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`
    if (seconds < 86400 * 7) return `${Math.floor(seconds / 86400)} 天前`
    return formatDateTime(timestamp)
  }

  function normalizeTimestamp(value) {
    if (value instanceof Date) return Math.floor(value.getTime() / 1000)
    if (typeof value === 'string' && !/^\d+(\.\d+)?$/.test(value)) {
      const parsed = Date.parse(value)
      return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : 0
    }
    const number = Number(value)
    if (!Number.isFinite(number) || number <= 0) return 0
    return number > 1e12 ? Math.floor(number / 1000) : Math.floor(number)
  }

  function formatCurrencyFromSummary(summary) {
    const microUsd = numberValue(summary, ['charged_micro_usd', 'usage.charged_micro_usd'], NaN)
    if (Number.isFinite(microUsd)) {
      return {
        label: '预估费用',
        value: `$${(microUsd / 1e6).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`,
      }
    }

    const amount = numberValue(summary, ['charged_amount', 'cost', 'estimated_cost', 'usage.cost'], NaN)
    if (Number.isFinite(amount)) {
      return {
        label: '预估费用',
        value: `$${amount.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`,
      }
    }

    return {
      label: '计费额度',
      value: formatCompact(numberValue(summary, ['charged_quota', 'quota', 'usage.charged_quota'])),
    }
  }

  function localDateTimeInput(timestamp) {
    const date = new Date(timestamp * 1000)
    const offset = date.getTimezoneOffset() * 60000
    return new Date(date.getTime() - offset).toISOString().slice(0, 16)
  }

  function rangeTimestamps() {
    const now = new Date()
    const end = Math.floor(now.getTime() / 1000)
    const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    const todayTimestamp = Math.floor(todayStart.getTime() / 1000)

    switch (state.range) {
      case '1h':
        return [end - 3600, end]
      case 'yesterday':
        return [todayTimestamp - 86400, todayTimestamp]
      case '7d':
        return [end - 7 * 86400, end]
      case 'custom': {
        const customEnd = Math.min(state.customEnd || end, end)
        return [state.customStart || customEnd - 3600, customEnd]
      }
      case 'today':
      default:
        return [todayTimestamp, end]
    }
  }

  function readFilterState() {
    const requestedOption = $('requestedModelFilter').selectedOptions[0]
    const upstreamOption = $('upstreamModelFilter').selectedOptions[0]
    return {
      channelId: $('channelFilter').value,
      channelType: $('channelTypeFilter').value,
      group: $('groupFilter').value,
      requestedModel: requestedOption?.dataset.model || $('requestedModelFilter').value,
      requestedModelHash: requestedOption?.dataset.modelHash || '',
      upstreamModel: upstreamOption?.dataset.model || $('upstreamModelFilter').value,
      upstreamModelHash: upstreamOption?.dataset.modelHash || '',
      outcome: $('outcomeFilter').value,
      statusCode: $('statusCodeFilter').value.trim(),
      stream: $('streamFilter').value,
      trafficSource: $('trafficSourceFilter').value || 'relay',
      dataOrigin: $('dataOriginFilter').value || 'live,legacy',
    }
  }

  function buildQuery(extra = {}, options = {}) {
    const [start, end] = rangeTimestamps()
    const filters = readFilterState()
    const params = new URLSearchParams({
      start_timestamp: String(start),
      end_timestamp: String(end),
      granularity: state.granularity,
      traffic_source: filters.trafficSource,
      data_origin: filters.dataOrigin,
    })

    if (filters.channelId) params.set('channel_ids', filters.channelId)
    if (filters.channelType) params.set('channel_types', filters.channelType)
    if (filters.group) params.set('groups', filters.group)
    // 新接口用完整哈希筛选，避免模型展示快照被截断后误伤查询；旧宿主仍回退到文本参数。
    if (filters.requestedModelHash) params.set('requested_model_hashes', filters.requestedModelHash)
    else if (filters.requestedModel) params.set('requested_models', filters.requestedModel)
    if (filters.upstreamModelHash) params.set('upstream_model_hashes', filters.upstreamModelHash)
    else if (filters.upstreamModel) params.set('upstream_models', filters.upstreamModel)
    if (filters.outcome && options.includeOutcome !== false) params.set('outcome', filters.outcome)
    if (filters.stream && options.includeStream !== false) params.set('stream', filters.stream)
    if (filters.statusCode && options.includeStatus !== false) {
      const key = options.statusScope === 'client' ? 'client_status_codes' : 'upstream_status_codes'
      params.set(key, filters.statusCode)
    }

    Object.entries(extra).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== '') params.set(key, String(value))
    })
    return params
  }

  function effectiveQueryStart() {
    const [start, end] = rangeTimestamps()
    if (state.view !== 'operations' || !state.stabilityWindows.length) return start
    return end - Math.max(...state.stabilityWindows)
  }

  function extractMeta(...payloads) {
    for (const payload of payloads) {
      if (!payload || typeof payload !== 'object') continue
      const meta = firstDefined(payload, ['data_quality', 'meta', 'metadata', 'quality'], null)
      if (meta && typeof meta === 'object') return meta
    }
    return {}
  }

  function renderDataQuality(...payloads) {
    const meta = extractMeta(...payloads)
    const messages = []
    const partial = booleanValue(meta, ['partial', 'is_partial'])
    const invalidSamples = numberValue(meta, ['invalid_sample_count'])
    const overflow = numberValue(meta, ['dimension_overflow_count'])
    const hashCollisions = numberValue(meta, ['dimension_hash_collision_count'])
    const droppedMetrics = numberValue(meta, ['dropped_metric_event_count', 'dropped_metrics'])
    const droppedFailures = numberValue(meta, ['dropped_failure_event_count', 'dropped_failures'])
    const pendingBatches = numberValue(meta, ['runtime_pending_batch_count'])
    const flushFailures = numberValue(meta, ['runtime_flush_failure_count'])
    const lastFlushErrorAt = normalizeTimestamp(firstDefined(meta, ['runtime_last_flush_error_at'], 0))
    const lastFlushedAt = normalizeTimestamp(firstDefined(meta, ['last_flushed_at'], 0))
    const flushErrorOutstanding = lastFlushErrorAt > lastFlushedAt
    const uncovered = arrayValue(meta, ['uncovered_channel_types', 'uncovered_transports'])
    const reliableFrom = normalizeTimestamp(firstDefined(meta, ['reliable_from_ts', 'reliable_from'], 0))
    const detailAvailable = firstDefined(meta, ['detail_available'], true)
    const selectedOrigins = $('dataOriginFilter')?.value || 'live,legacy'
    const backfill = firstDefined(meta, ['backfill'], null)
    const backfillStatus = String(firstDefined(backfill, ['status'], ''))
    const backfillTotal = numberValue(backfill, ['total_rows'])
    const backfillScanned = numberValue(backfill, ['scanned_rows'])
    const backfillConverted = numberValue(backfill, ['converted_rows'])
    const backfillSkipped = numberValue(backfill, ['skipped_rows'])
    const backfillProgress = backfillTotal > 0 ? Math.min(100, backfillScanned / backfillTotal * 100) : 0

    if (partial) messages.push('当前区间数据不完整')
    if (invalidSamples > 0) messages.push(`${formatInteger(invalidSamples)} 条无效样本未计入统计`)
    if (overflow > 0) messages.push(`${formatInteger(overflow)} 个高基数维度已合并到“其他”`)
    if (hashCollisions > 0) messages.push(`${formatInteger(hashCollisions)} 次维度哈希冲突已隔离`)
    if (droppedMetrics > 0) messages.push(`${formatInteger(droppedMetrics)} 条指标因保护上限被丢弃`)
    if (droppedFailures > 0) messages.push(`${formatInteger(droppedFailures)} 条失败明细未保存`)
    if (pendingBatches > 0) messages.push(`${formatInteger(pendingBatches)} 个指标批次等待重新写入`)
    if (flushFailures > 0 && flushErrorOutstanding) {
      messages.push(`指标写入最近失败于 ${formatDateTime(lastFlushErrorAt)}`)
    }
    if (uncovered.length) {
      const names = uncovered.map((type) => channelTypes[type] || `类型 ${type}`)
      messages.push(`部分渠道类型尚未完整采集：${names.join('、')}`)
    }
    if (detailAvailable === false) messages.push('完整日志明细当前不可用')
    if (selectedOrigins.includes('legacy')) {
      messages.push('历史日志为推导口径，重试链、失败请求用量、TTFT 和上游原始状态码可能不完整')
      if (backfillStatus === 'running' || backfillStatus === 'pending') {
        messages.push(`历史日志回填中：${formatInteger(backfillScanned)} / ${formatInteger(backfillTotal)}（${backfillProgress.toFixed(1)}%）`)
      } else if (backfillStatus === 'failed') {
        const lastError = String(firstDefined(backfill, ['last_error'], '未知错误'))
        messages.push(`历史日志回填失败：${formatProbeError(lastError)}`)
      } else if (backfillStatus === 'completed') {
        messages.push(`历史日志已完成回填：转换 ${formatInteger(backfillConverted)} 条，跳过 ${formatInteger(backfillSkipped)} 条`)
      }
    }
    if (reliableFrom && effectiveQueryStart() < reliableFrom) {
      messages.push(selectedOrigins.includes('legacy')
        ? `实时精确采集始于 ${formatDateTime(reliableFrom)}，更早区间使用历史日志推导`
        : `实时精确统计始于 ${formatDateTime(reliableFrom)}`)
    }

    const notice = $('qualityNotice')
    if (!messages.length) {
      notice.classList.add('is-hidden')
      notice.textContent = ''
      return
    }
    notice.classList.toggle(
      'quality-notice--danger',
      backfillStatus === 'failed' || invalidSamples > 0 || hashCollisions > 0 || droppedMetrics > 0 || pendingBatches > 0 || flushErrorOutstanding
    )
    notice.innerHTML = `<span aria-hidden="true">⚠</span><div><strong>数据质量提示：</strong>${escapeHtml(messages.join('；'))}</div>`
    notice.classList.remove('is-hidden')
  }

  function updateFreshness(status, meta = {}) {
    const freshness = $('freshness')
    const timestamp = normalizeTimestamp(firstDefined(meta, ['last_flushed_at', 'generated_at', 'data_end_ts'], 0))
    const pendingBatches = numberValue(meta, ['runtime_pending_batch_count'])
    const lastFlushErrorAt = normalizeTimestamp(firstDefined(meta, ['runtime_last_flush_error_at'], 0))
    const lastFlushedAt = normalizeTimestamp(firstDefined(meta, ['last_flushed_at'], 0))
    const runtimeWarning = status === 'ok' && (pendingBatches > 0 || lastFlushErrorAt > lastFlushedAt)
    const backfill = firstDefined(meta, ['backfill'], null)
    const backfillStatus = String(firstDefined(backfill, ['status'], ''))
    const backfillTotal = numberValue(backfill, ['total_rows'])
    const backfillScanned = numberValue(backfill, ['scanned_rows'])
    const backfillProgress = backfillTotal > 0 ? Math.min(100, backfillScanned / backfillTotal * 100) : 0
    const selectedOrigins = $('dataOriginFilter')?.value || 'live,legacy'
    const backfillRunning = selectedOrigins.includes('legacy') &&
      (backfillStatus === 'running' || backfillStatus === 'pending')
    const label = status === 'loading'
      ? '正在刷新统计数据'
      : status === 'error'
        ? '最近刷新失败'
        : backfillRunning
          ? `历史日志回填中 ${backfillProgress.toFixed(1)}%`
        : status === 'empty'
          ? '已查询 · 暂无真实转发样本'
        : runtimeWarning
          ? '指标写入异常，当前统计可能滞后'
        : timestamp
          ? `数据更新于 ${formatRelativeTime(timestamp)}`
          : '已完成刷新'
    const dotClass = status === 'loading'
      ? 'status-dot--loading'
      : status === 'error'
        ? 'status-dot--warning'
        : backfillRunning
          ? 'status-dot--loading'
        : status === 'empty'
          ? 'status-dot--idle'
        : runtimeWarning
          ? 'status-dot--warning'
        : 'status-dot--ok'
    freshness.innerHTML = `<span class="status-dot ${dotClass}"></span><span>${escapeHtml(label)}</span>`
  }

  function setLoading(loading) {
    $('loadingState').classList.toggle('is-hidden', !loading)
    $('refreshButton').disabled = loading
    if (loading) {
      $('errorState').classList.add('is-hidden')
      $('emptyState').classList.add('is-hidden')
      $$('.view-panel').forEach((panel) => panel.classList.add('is-hidden'))
      updateFreshness('loading')
    }
  }

  function showError(error) {
    const message = error?.hostCompatibility
      ? error.message
      : error?.status === 404
      ? '宿主程序尚未提供渠道统计接口，请先升级后端并确认指标采集已启用。'
      : error?.message || '请稍后重试。'
    $('errorMessage').textContent = message
    $('errorState').classList.remove('is-hidden')
    $('emptyState').classList.add('is-hidden')
    $$('.view-panel').forEach((panel) => panel.classList.add('is-hidden'))
    updateFreshness('error')
  }

  function showEmpty(empty) {
    $('emptyState').classList.toggle('is-hidden', !empty)
    $$('.view-panel').forEach((panel) => {
      panel.classList.toggle('is-hidden', empty || panel.dataset.panel !== state.view)
    })
  }

  function updateFilterScopeHint() {
    const messages = {
      overview: '状态码筛选只在“状态码与失败”视图生效。',
      operations: '矩阵固定比较多个重叠时间窗；复合维度按层级逐级展开并懒加载子项。状态码筛选不参与矩阵。',
      channels: '状态码筛选不适用于渠道尝试聚合；展开模型时按所选模型口径统计。',
      failures: '响应方式只影响状态码分布，失败明细暂不支持该筛选；历史明细来自脱敏日志推导，重试链和原始上游状态码可能不完整。',
      probe: '主动探测只应用渠道、渠道类型和请求模型；时间、粒度及其他业务筛选不参与探测。',
    }
    $('filterScopeHint').textContent = messages[state.view] || ''
  }

  function updateStabilityDimensionControls() {
    const dimension = $('stabilityDimension').value
    const usesModel = dimension === 'model' || dimension.endsWith('_model')
    const modelDimension = $('stabilityModelDimension')
    modelDimension.disabled = !usesModel
    modelDimension.closest('.compact-field')?.classList.toggle('is-disabled', !usesModel)
    modelDimension.title = usesModel ? '' : '当前分析维度不包含模型'
  }

  function activateView(view, shouldLoad = true) {
    if (!['overview', 'operations', 'channels', 'failures', 'probe'].includes(view)) return
    if (state.view === 'operations' && view !== 'operations') resetStabilityTree()
    state.view = view
    $$('.view-tab').forEach((tab) => {
      const active = tab.dataset.view === view
      tab.classList.toggle('is-active', active)
      tab.setAttribute('aria-selected', String(active))
    })
    updateFilterScopeHint()
    if (shouldLoad) loadCurrentView()
  }

  function summaryMetrics(payload) {
    const source = firstDefined(payload, ['summary', 'metrics'], payload || {})
    const finalRequests = numberValue(source, [
      'final_request_count', 'client_request_count', 'request_count', 'requests', 'total_requests',
    ])
    const attempts = numberValue(source, [
      'channel_attempt_count', 'attempt_count', 'upstream_attempt_count', 'attempts',
    ])
    const upstreamCalls = numberValue(source, [
      'upstream_call_count', 'transport_call_count', 'upstream_calls',
    ])
    const failures = numberValue(source, [
      'failed_attempt_count', 'failure_count', 'failed_attempts', 'failures',
    ])
    const retries = numberValue(source, ['retry_count', 'retried_attempt_count', 'retries'])
    const input = numberValue(source, [
      'input_tokens_total', 'input_tokens', 'prompt_tokens', 'usage.input_tokens_total',
    ])
    const output = numberValue(source, ['output_tokens', 'completion_tokens', 'usage.output_tokens'])
    const cacheRead = numberValue(source, ['cache_read_tokens', 'cached_tokens', 'usage.cache_read_tokens'])
    const cacheWrite = numberValue(source, ['cache_write_tokens', 'usage.cache_write_tokens'])
    const uncachedInput = numberValue(source, [
      'uncached_input_tokens', 'usage.uncached_input_tokens',
    ], Math.max(0, input - cacheRead - cacheWrite))
    const totalTokens = numberValue(source, ['total_tokens', 'usage.total_tokens'], input + output)

    return {
      source,
      finalRequests,
      attempts,
      upstreamCalls,
      failures,
      retries,
      retryRate: firstDefined(source, ['retry_rate'], attempts ? retries / attempts : null),
      clientSuccessRate: firstDefined(source, [
        'client_success_rate', 'final_request_success_rate', 'success_rates.client',
      ], null),
      qualitySuccessRate: firstDefined(source, [
        'channel_quality_success_rate', 'quality_success_rate', 'success_rates.quality',
      ], null),
      attemptSuccessRate: firstDefined(source, [
        'attempt_success_rate', 'attempt_business_success_rate', 'success_rates.attempt',
      ], null),
      input,
      output,
      cacheRead,
      cacheWrite,
      uncachedInput,
      totalTokens,
      cacheRequestHitRate: firstDefined(source, [
        'cache_request_hit_rate', 'cache_hit_request_rate', 'cache.request_hit_rate',
      ], null),
      cacheTokenHitRate: firstDefined(source, [
        'cache_token_hit_rate', 'cache_hit_token_rate', 'cache.token_hit_rate',
      ], null),
      avgLatency: numberValue(source, ['avg_latency_ms', 'average_latency_ms', 'latency.avg_ms'], NaN),
      p95Latency: numberValue(source, ['p95_latency_ms', 'latency_p95_ms', 'latency.p95_ms'], NaN),
      avgTtft: numberValue(source, ['avg_ttft_ms', 'average_ttft_ms', 'ttft.avg_ms'], NaN),
      p95Ttft: numberValue(source, ['p95_ttft_ms', 'ttft_p95_ms', 'ttft.p95_ms'], NaN),
      cost: formatCurrencyFromSummary(source),
    }
  }

  function metricCard(label, value, hint, tone, icon, title) {
    return `
      <article class="metric-card ${tone ? `metric-card--${tone}` : ''}"${title ? ` title="${escapeHtml(title)}"` : ''}>
        <div class="metric-card__heading">
          <span>${escapeHtml(label)}</span>
          <span class="metric-icon" aria-hidden="true">${escapeHtml(icon)}</span>
        </div>
        <div class="metric-card__value">${escapeHtml(value)}</div>
        <div class="metric-card__foot"><span>${escapeHtml(hint)}</span></div>
      </article>`
  }

  function renderSummary(payload) {
    const metrics = summaryMetrics(payload)
    const cards = [
      metricCard('客户端请求', formatInteger(metrics.finalRequests), '每个逻辑请求只计一次', '', '↗'),
      metricCard('渠道尝试', formatInteger(metrics.attempts), `底层调用 ${formatInteger(metrics.upstreamCalls)}`, 'purple', '⇄'),
      metricCard('客户端成功率', formatPercent(metrics.clientSuccessRate), '最终返回给客户端的结果', 'green', '✓'),
      metricCard('渠道质量成功率', formatPercent(metrics.qualitySuccessRate), '只统计可归因的渠道样本', 'green', '◇'),
      metricCard('失败尝试', formatInteger(metrics.failures), `重试率 ${formatPercent(metrics.retryRate)}`, 'red', '×'),
      metricCard('已记录 Token', formatCompact(metrics.totalTokens), '来自具有可用量记录的渠道尝试', 'cyan', 'T'),
      metricCard('输入 Token', formatCompact(metrics.input), `缓存 Token 占比 ${formatPercent(metrics.cacheTokenHitRate)}`, '', '↓'),
      metricCard('输出 Token', formatCompact(metrics.output), '模型生成用量', 'purple', '↑'),
      metricCard('缓存读取', formatCompact(metrics.cacheRead), `缓存写入 ${formatCompact(metrics.cacheWrite)}`, 'cyan', 'C'),
      metricCard(metrics.cost.label, metrics.cost.value, `延迟 ${formatDuration(metrics.avgLatency)} / ${formatDuration(metrics.p95Latency)}；TTFT ${formatDuration(metrics.avgTtft)} / ${formatDuration(metrics.p95Ttft)}`, 'amber', '$'),
    ]
    $('summaryCards').innerHTML = cards.join('')
    renderTokenComposition(metrics)
    renderSuccessScopes(metrics)
  }

  function renderTokenComposition(metrics) {
    const items = [
      ['未缓存输入', metrics.uncachedInput, 'var(--primary)'],
      ['缓存读取', metrics.cacheRead, 'var(--amber)'],
      ['缓存写入', metrics.cacheWrite, 'var(--purple)'],
      ['输出', metrics.output, 'var(--cyan)'],
    ]
    const denominator = Math.max(1, metrics.totalTokens)
    $('tokenComposition').innerHTML = `
      <div class="composition-total"><span>总 Token</span><strong>${escapeHtml(formatCompact(metrics.totalTokens))}</strong></div>
      ${items.map(([label, value, color]) => {
        const percentage = Math.max(0, Math.min(100, value / denominator * 100))
        return `<div class="composition-item">
          <div class="composition-item__head"><span>${escapeHtml(label)}</span><span><strong>${escapeHtml(formatCompact(value))}</strong> · ${percentage.toFixed(1)}%</span></div>
          <div class="progress-track"><div class="progress-fill" style="--bar-color:${color};width:${percentage}%"></div></div>
        </div>`
      }).join('')}`
  }

  function renderSuccessScopes(metrics) {
    const items = [
      ['客户端成功率', metrics.clientSuccessRate, 'final_request：一次客户端逻辑请求只计一次'],
      ['渠道质量成功率', metrics.qualitySuccessRate, 'channel_attempt：排除客户端、本地和未知归因'],
      ['尝试业务成功率', metrics.attemptSuccessRate, 'channel_attempt：反映每次选渠后的业务结果'],
    ]
    $('successScopes').innerHTML = items.map(([label, rate, hint]) => `
      <div class="scope-item">
        <div class="scope-item__head"><span>${escapeHtml(label)}</span><strong>${escapeHtml(formatPercent(rate))}</strong></div>
        <p>${escapeHtml(hint)}</p>
      </div>`).join('')
  }

  function normalizeTrend(payload) {
    const rows = arrayValue(payload, ['points', 'items', 'buckets', 'trend', 'data'])
    return rows.map((row, index) => ({
      timestamp: normalizeTimestamp(firstDefined(row, ['bucket_ts', 'timestamp', 'ts', 'time'], index + 1)),
      requests: numberValue(row, ['final_request_count', 'client_request_count', 'request_count', 'requests']),
      failures: numberValue(row, ['failed_attempt_count', 'failure_count', 'failures']),
      tokens: numberValue(row, ['total_tokens', 'usage_tokens', 'tokens']),
    })).sort((a, b) => a.timestamp - b.timestamp)
  }

  function renderTrend(payload) {
    const points = normalizeTrend(payload)
    const chart = $('trendChart')
    const series = [
      { key: 'requests', label: '客户端请求', color: '#3987f6' },
      { key: 'failures', label: '失败尝试', color: '#e6525e' },
      { key: 'tokens', label: '已记录 Token', color: '#12b8a6' },
    ]
    $('trendLegend').innerHTML = series.map((item) => `
      <span class="legend-item"><span class="legend-dot" style="background:${item.color}"></span>${escapeHtml(item.label)}</span>`).join('')

    if (!points.length) {
      chart.innerHTML = '<div class="table-empty">当前范围暂无趋势数据</div>'
      return
    }

    const width = 860
    const height = 300
    const padding = { top: 18, right: 24, bottom: 35, left: 48 }
    const innerWidth = width - padding.left - padding.right
    const innerHeight = height - padding.top - padding.bottom
    const leftMax = Math.max(1, ...points.flatMap((point) => [point.requests, point.failures]))
    const tokenMax = Math.max(1, ...points.map((point) => point.tokens))
    const x = (index) => padding.left + (points.length === 1 ? innerWidth / 2 : index / (points.length - 1) * innerWidth)
    const y = (value, max) => padding.top + innerHeight - value / max * innerHeight
    const pathFor = (key, max) => points.map((point, index) => `${index ? 'L' : 'M'}${x(index).toFixed(2)},${y(point[key], max).toFixed(2)}`).join(' ')
    const requestArea = `${pathFor('requests', leftMax)} L${x(points.length - 1)},${padding.top + innerHeight} L${x(0)},${padding.top + innerHeight} Z`
    const labelIndexes = Array.from(new Set([0, Math.floor((points.length - 1) / 2), points.length - 1]))
    const grid = [0, 0.25, 0.5, 0.75, 1].map((ratio) => {
      const gridY = padding.top + innerHeight - ratio * innerHeight
      return `<line class="chart-grid-line" x1="${padding.left}" x2="${width - padding.right}" y1="${gridY}" y2="${gridY}" />
        <text class="chart-axis-label" x="${padding.left - 8}" y="${gridY + 3}" text-anchor="end">${escapeHtml(formatCompact(leftMax * ratio))}</text>`
    }).join('')
    const xLabels = labelIndexes.map((index) => `
      <text class="chart-axis-label" x="${x(index)}" y="${height - 9}" text-anchor="middle">${escapeHtml(formatDateTime(points[index].timestamp, state.range !== 'today' && state.range !== '1h'))}</text>`).join('')
    const dots = series.flatMap((item) => {
      const max = item.key === 'tokens' ? tokenMax : leftMax
      return points.map((point, index) => `<circle class="chart-dot" cx="${x(index)}" cy="${y(point[item.key], max)}" r="2.8" fill="${item.color}"><title>${escapeHtml(`${item.label}：${formatCompact(point[item.key])}`)}</title></circle>`)
    }).join('')

    chart.innerHTML = `<div class="chart-scroll"><svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" aria-hidden="true">
      ${grid}
      <path class="chart-area" d="${requestArea}" fill="#3987f6"></path>
      <path class="chart-line" d="${pathFor('requests', leftMax)}" stroke="#3987f6"></path>
      <path class="chart-line" d="${pathFor('failures', leftMax)}" stroke="#e6525e"></path>
      <path class="chart-line" d="${pathFor('tokens', tokenMax)}" stroke="#12b8a6"></path>
      ${dots}${xLabels}
    </svg></div>`
  }

  function normalizeStatusItems(payload) {
    const rows = arrayValue(payload, ['status_codes', 'items', 'codes', 'distribution'])
    return rows.map((row) => {
      const present = firstDefined(row, ['status_present', 'present'], true)
      const code = Number(firstDefined(row, ['status_code', 'code'], 0))
      let label = firstDefined(row, ['label', 'name'], '')
      if (!label) {
        if (present === false) label = '未知 / 不适用'
        else if (code === 0) label = '无 HTTP 响应'
        else label = String(code)
      }
      return {
        code,
        present: present !== false,
        label: String(label),
        count: numberValue(row, ['count', 'request_count', 'call_count', 'total']),
      }
    }).sort((a, b) => b.count - a.count)
  }

  function statusTone(code, present = true) {
    if (!present || code === 0) return { color: 'var(--muted)', className: '' }
    if (code >= 200 && code < 300) return { color: 'var(--green)', className: 'status-pill--success' }
    if (code >= 400 && code < 500) return { color: 'var(--amber)', className: 'status-pill--warning' }
    if (code >= 500) return { color: 'var(--red)', className: 'status-pill--danger' }
    return { color: 'var(--primary)', className: 'status-pill--info' }
  }

  function renderStatusBars(targetId, rows, limit = 8) {
    const target = $(targetId)
    const shown = rows.slice(0, limit)
    const max = Math.max(1, ...shown.map((row) => row.count))
    target.innerHTML = shown.length ? shown.map((row) => {
      const tone = statusTone(row.code, row.present)
      const width = Math.max(2, row.count / max * 100)
      return `<div class="status-bar">
        <div class="status-bar__head"><span>${escapeHtml(row.label)}</span><strong>${escapeHtml(formatInteger(row.count))}</strong></div>
        <div class="progress-track"><div class="progress-fill" style="--bar-color:${tone.color};width:${width}%"></div></div>
      </div>`
    }).join('') : '<div class="table-empty">暂无状态码数据</div>'
  }

  function renderOverviewStatuses(payload) {
    if (payload?.optional_error) {
      $('overviewStatuses').innerHTML = `<div class="table-empty">上游状态加载失败：${escapeHtml(payload.optional_error)}</div>`
      return
    }
    renderStatusBars('overviewStatuses', normalizeStatusItems(payload), 6)
  }

  async function loadOverview(signal) {
    const base = buildQuery({}, { includeStatus: false })
    const statusQuery = buildQuery({ metric_scope: 'upstream_call' }, {
      statusScope: 'upstream',
      includeStatus: false,
    })
    const [summary, trend, statusResult] = await Promise.all([
      requestJson(`${API_ROOT}/summary?${base}`, { signal }),
      requestJson(`${API_ROOT}/trend?${base}`, { signal }),
      requestJson(`${API_ROOT}/status-codes?${statusQuery}`, { signal }).catch((error) => {
        if (error?.name === 'AbortError') throw error
        return { items: [], optional_error: error?.message }
      }),
    ])
    state.summary = summary
    state.trend = trend
    state.statuses = statusResult
    renderSummary(summary)
    renderTrend(trend)
    renderOverviewStatuses(statusResult)
    renderDataQuality(summary, trend, statusResult)
    const metrics = summaryMetrics(summary)
    const empty = metrics.finalRequests === 0 && metrics.attempts === 0 && metrics.upstreamCalls === 0
    return { empty, meta: extractMeta(summary, trend, statusResult) }
  }

  function stabilityWindowLabel(seconds) {
    const labels = {
      900: '15 分钟',
      3600: '1 小时',
      21600: '6 小时',
      86400: '24 小时',
      604800: '7 天',
    }
    return labels[Number(seconds)] || `${formatCompact(seconds)} 秒`
  }

  function stabilityWindowMap(item) {
    const result = new Map()
    arrayValue(item, ['windows']).forEach((window) => {
      const seconds = numberValue(window, ['window_seconds'])
      if (seconds > 0) result.set(seconds, window)
    })
    return result
  }

  function stabilityRateTone(rate, sufficient) {
    if (!sufficient) return 'stability-rate--insufficient'
    const percent = percentNumber(rate)
    if (percent === null) return 'stability-rate--insufficient'
    if (percent < 95) return 'stability-rate--danger'
    if (percent < 99) return 'stability-rate--warning'
    return 'stability-rate--healthy'
  }

  function renderStabilityWindow(window, seconds) {
    if (!window) {
      return '<div class="stability-rate stability-rate--insufficient"><strong>-</strong><span>无样本</span></div>'
    }
    const sampleCount = numberValue(window, ['quality_eligible_count'])
    const calls = numberValue(window, ['channel_attempt_count'])
    const minimumSamples = numberValue(window, ['minimum_sample_count'], 10)
    const sufficient = booleanValue(window, ['sample_sufficient'], sampleCount >= minimumSamples)
    const rate = firstDefined(window, ['quality_success_rate', 'attempt_success_rate'], null)
    const tone = stabilityRateTone(rate, sufficient)
    const failures = numberValue(window, ['failure_count'])
    const hint = !sufficient && calls > 0
      ? `样本不足 ${formatInteger(sampleCount)}/${formatInteger(minimumSamples)} · 调用 ${formatInteger(calls)} · 失败 ${formatInteger(failures)}`
      : `样本 ${formatInteger(sampleCount)} · 调用 ${formatInteger(calls)} · 失败 ${formatInteger(failures)}`
    const title = `${stabilityWindowLabel(seconds)}：质量成功率 ${formatPercent(rate)}，可归因样本 ${formatInteger(sampleCount)}，渠道尝试 ${formatInteger(calls)}`
    return `<div class="stability-rate ${tone}" title="${escapeHtml(title)}"><strong>${escapeHtml(formatPercent(rate))}</strong><span>${escapeHtml(hint)}</span></div>`
  }

  function stabilityIdentity(item, { onlyTypes = null } = {}) {
    const includedTypes = onlyTypes ? new Set(onlyTypes) : null
    const includes = (type) => !includedTypes || includedTypes.has(type)
    const group = String(firstDefined(item, ['group'], ''))
    const groupName = String(firstDefined(item, ['group_name'], group))
    const channelId = numberValue(item, ['channel_id'])
    const channelName = String(firstDefined(item, ['channel_name'], channelId ? `渠道 #${channelId}` : ''))
    const model = String(firstDefined(item, ['requested_model', 'upstream_model'], ''))
    const parts = []
    if (group && includes('group')) parts.push(`<span class="identity-part"><small>分组</small><strong>${escapeHtml(groupName || group)}</strong>${groupName && groupName !== group ? `<em>${escapeHtml(group)}</em>` : ''}</span>`)
    if (channelId && includes('channel')) parts.push(`<span class="identity-part"><small>渠道</small><strong>${escapeHtml(channelName)}</strong><em>#${channelId}</em></span>`)
    if (model && includes('model')) parts.push(`<span class="identity-part"><small>模型</small><strong title="${escapeHtml(model)}">${escapeHtml(model)}</strong></span>`)
    const missingLabel = onlyTypes?.length === 1 ? STABILITY_NODE_LABELS[onlyTypes[0]] : '维度'
    return parts.length ? parts.join('') : `<span class="cell-sub">未标记${escapeHtml(missingLabel || '维度')}</span>`
  }

  function stabilityDetailWindow(windows) {
    const values = Array.from(windows.values())
    return windows.get(86400) || windows.get(604800) || values[values.length - 1] || null
  }

  function stabilityDetailWindowSeconds() {
    if (state.stabilityWindows.includes(86400)) return 86400
    if (state.stabilityWindows.includes(604800)) return 604800
    return state.stabilityWindows[state.stabilityWindows.length - 1] || 0
  }

  function resetStabilityTree() {
    state.stabilityTreeGeneration += 1
    state.stabilityTreeEntries.forEach((entry) => entry.controller?.abort())
    state.stabilityExpandedNodes.clear()
    state.stabilityTreeEntries.clear()
    state.stabilityTreeNodes.clear()
    state.stabilityTreeQueryParams = null
    state.stabilityTreePlan = null
    state.stabilityTreeMode = false
  }

  function renderStabilityMetricRow(item, identity, rowClass = '') {
    const windows = stabilityWindowMap(item)
    const detail = stabilityDetailWindow(windows)
    const failures = numberValue(detail, ['failure_count'])
    const retries = numberValue(detail, ['retry_count'])
    const retryRate = firstDefined(detail, ['retry_rate'], null)
    const p95 = numberValue(detail, ['p95_latency_ms'], NaN)
    const p95Ttft = numberValue(detail, ['p95_ttft_ms'], NaN)
    const upstream429 = numberValue(detail, ['upstream_429_count'])
    const upstream5xx = numberValue(detail, ['upstream_5xx_count'])
    const transportErrors = numberValue(detail, ['transport_error_count'])
    const streamErrors = numberValue(detail, ['stream_error_count'])
    const statusSamples = numberValue(detail, ['upstream_status_sample_count'])
    const statusCoverage = firstDefined(detail, ['upstream_status_coverage_rate'], null)
    const liveRate = firstDefined(detail, ['live_event_rate'], null)
    const legacyRate = firstDefined(detail, ['legacy_event_rate'], null)
    const lastFailure = normalizeTimestamp(firstDefined(detail, ['last_failure_bucket_ts'], 0))
    const totalTokens = numberValue(detail, ['total_tokens'])
    const cacheRead = numberValue(detail, ['cache_read_tokens'])
    const cacheRate = firstDefined(detail, ['cache_token_hit_rate'], null)
    const usageCoverage = firstDefined(detail, ['usage_success_coverage_rate'], null)
    return `<tr${rowClass ? ` class="${rowClass}"` : ''}>
        <td>${identity}</td>
        ${state.stabilityWindows.map((seconds) => `<td>${renderStabilityWindow(windows.get(seconds), seconds)}</td>`).join('')}
        <td><div class="stability-detail">
          <strong>失败 ${escapeHtml(formatInteger(failures))} · 重试 ${escapeHtml(formatInteger(retries))}</strong>
          <span>重试率 ${escapeHtml(formatPercent(retryRate))}</span>
          ${statusSamples > 0
            ? `<span>429 ${escapeHtml(formatInteger(upstream429))} · 5xx ${escapeHtml(formatInteger(upstream5xx))} · 状态覆盖 ${escapeHtml(formatPercent(statusCoverage))}</span>`
            : '<span>上游状态码暂无可用样本</span>'}
          <span>连接 ${escapeHtml(formatInteger(transportErrors))} · 流中断 ${escapeHtml(formatInteger(streamErrors))}</span>
          <span>P95 ${escapeHtml(formatDuration(p95))} · TTFT ${escapeHtml(formatDuration(p95Ttft))}</span>
          <span>${lastFailure ? `最近失败 ${escapeHtml(formatRelativeTime(lastFailure))}` : '暂无失败记录'}</span>
        </div></td>
        <td><div class="stability-detail">
          <strong>Token ${escapeHtml(formatCompact(totalTokens))}</strong>
          <span>缓存读取 ${escapeHtml(formatCompact(cacheRead))}</span>
          <span>Token 命中 ${escapeHtml(formatPercent(cacheRate))}</span>
          <span>用量覆盖 ${escapeHtml(formatPercent(usageCoverage))}</span>
          <span>实时 ${escapeHtml(formatPercent(liveRate))} · 历史 ${escapeHtml(formatPercent(legacyRate))}</span>
        </div></td>
      </tr>`
  }

  function stabilityGroupCode(item) {
    return String(firstDefined(item, ['group'], '')).trim()
  }

  function stabilityGroupDisplayName(item) {
    const group = stabilityGroupCode(item)
    return String(firstDefined(item, ['group_name'], group)).trim() || group
  }

  function stabilityChannelId(item) {
    const channelId = numberValue(item, ['channel_id'])
    return Number.isInteger(channelId) && channelId > 0 ? channelId : 0
  }

  function stabilityModelName(item) {
    return String(firstDefined(item, ['requested_model', 'upstream_model'], '')).trim()
  }

  function stabilityNodeValue(item, type) {
    if (type === 'group') return stabilityGroupCode(item)
    if (type === 'channel') return stabilityChannelId(item) ? String(stabilityChannelId(item)) : ''
    if (type === 'model') {
      return String(firstDefined(item, ['model_hash'], '')).trim() || stabilityModelName(item)
    }
    return ''
  }

  function stabilityNodeDisplayName(item, type) {
    if (type === 'group') return stabilityGroupDisplayName(item) || '未标记分组'
    if (type === 'channel') {
      const channelId = stabilityChannelId(item)
      return String(firstDefined(item, ['channel_name'], channelId ? `渠道 #${channelId}` : '未标记渠道'))
    }
    if (type === 'model') return stabilityModelName(item) || '未标记模型'
    return '未标记维度'
  }

  function stabilityNodeKey(item, levelIndex, plan = state.stabilityTreePlan) {
    if (!plan || levelIndex < 0 || levelIndex >= plan.levels.length) return ''
    const values = plan.levels.slice(0, levelIndex + 1).map((type) => stabilityNodeValue(item, type))
    return `${plan.dimension}:${levelIndex}:${JSON.stringify(values)}`
  }

  function stabilityNodeDescriptor(item, levelIndex, ancestorKeys = []) {
    const plan = state.stabilityTreePlan
    if (!plan || levelIndex < 0 || levelIndex >= plan.levels.length) return null
    const type = plan.levels[levelIndex]
    const key = stabilityNodeKey(item, levelIndex, plan)
    return {
      key,
      item,
      levelIndex,
      type,
      ancestorKeys,
      filterable: Boolean(stabilityNodeValue(item, type)),
      expandable: levelIndex < plan.levels.length - 1,
    }
  }

  function stabilityTreeEntry(descriptor) {
    let entry = state.stabilityTreeEntries.get(descriptor.key)
    if (!entry) {
      entry = {
        items: [],
        total: 0,
        page: 0,
        loaded: false,
        loading: false,
        error: '',
        controller: null,
        descriptor,
        ancestorKeys: [...descriptor.ancestorKeys],
      }
      state.stabilityTreeEntries.set(descriptor.key, entry)
    } else {
      entry.descriptor = descriptor
      entry.ancestorKeys = [...descriptor.ancestorKeys]
    }
    return entry
  }

  function renderStabilityTreeIdentity(descriptor) {
    const { item, key, levelIndex, type } = descriptor
    const canExpand = descriptor.expandable && descriptor.filterable
    const style = `--stability-tree-level:${levelIndex}`
    if (!canExpand) {
      return `<div class="stability-tree-leaf${levelIndex === 0 ? ' is-root' : ''}" style="${style}">
        ${levelIndex > 0 ? '<span class="stability-tree-branch" aria-hidden="true">└</span>' : ''}
        <div class="stability-tree-identity stability-identity">${stabilityIdentity(item, { onlyTypes: [type] })}</div>
      </div>`
    }

    const expanded = state.stabilityExpandedNodes.has(key)
    const entry = state.stabilityTreeEntries.get(key)
    const childType = state.stabilityTreePlan.levels[levelIndex + 1]
    const childLabel = STABILITY_NODE_LABELS[childType]
    let meta = `点击展开${childLabel}`
    if (entry?.loading) {
      meta = entry.items.length ? `正在加载更多${childLabel}…` : `正在加载${childLabel}…`
    } else if (entry?.error) {
      meta = entry.items.length
        ? `已加载 ${formatInteger(entry.items.length)} 个${childLabel}，继续加载失败`
        : `${childLabel}加载失败`
    } else if (entry?.loaded) {
      meta = `已加载 ${formatInteger(entry.items.length)} / ${formatInteger(entry.total)} 个${childLabel}`
    }
    const displayName = stabilityNodeDisplayName(item, type)
    return `<div class="stability-tree-parent" style="${style}">
      <button class="stability-tree-toggle" type="button"
        data-stability-node-toggle="${escapeHtml(key)}"
        aria-expanded="${expanded}"
        aria-busy="${Boolean(entry?.loading)}"
        aria-label="${expanded ? '收起' : '展开'}${STABILITY_NODE_LABELS[type]} ${escapeHtml(displayName)} 的${childLabel}">
        <span class="stability-tree-chevron" aria-hidden="true">›</span>
        <span class="stability-tree-identity stability-identity">${stabilityIdentity(item, { onlyTypes: [type] })}</span>
      </button>
      <span class="stability-tree-meta">${escapeHtml(meta)}</span>
    </div>`
  }

  function renderStabilityChildState(descriptor, content, { error = false } = {}) {
    const childLevel = descriptor.levelIndex + 1
    return `<tr class="stability-child-state stability-tree-row--level-${childLevel}${error ? ' stability-child-state--error' : ''}" data-stability-child-state="${escapeHtml(descriptor.key)}">
      <td colspan="${state.stabilityWindows.length + 3}">
        <div class="stability-child-actions" style="--stability-tree-level:${childLevel}" role="${error ? 'alert' : 'status'}" aria-live="polite">${content}</div>
      </td>
    </tr>`
  }

  function restoreStabilityNodeFocus(nodeKey) {
    if (!nodeKey) return
    const button = $$('[data-stability-node-toggle]').find((candidate) =>
      candidate.dataset.stabilityNodeToggle === nodeKey
    )
    if (!button) return
    try {
      button.focus({ preventScroll: true })
    } catch (_error) {
      button.focus()
    }
  }

  function renderStabilityNodeChildren(descriptor) {
    if (!descriptor?.expandable || !state.stabilityExpandedNodes.has(descriptor.key)) return ''
    const entry = state.stabilityTreeEntries.get(descriptor.key)
    const childType = state.stabilityTreePlan.levels[descriptor.levelIndex + 1]
    const childLabel = STABILITY_NODE_LABELS[childType]
    const parentLabel = STABILITY_NODE_LABELS[descriptor.type]
    if (!entry) {
      return renderStabilityChildState(descriptor, `<span>正在准备${childLabel}数据…</span>`)
    }

    const childAncestorKeys = [...descriptor.ancestorKeys, descriptor.key]
    const rows = entry.items.map((item) => renderStabilityTreeNode(
      item,
      descriptor.levelIndex + 1,
      childAncestorKeys,
    ))
    if (entry.loading) {
      rows.push(renderStabilityChildState(
        descriptor,
        `<span>${entry.items.length ? `正在加载更多${childLabel}…` : `正在加载该${parentLabel}的${childLabel}…`}</span>`,
      ))
    } else if (entry.error) {
      rows.push(renderStabilityChildState(
        descriptor,
        `<span>${escapeHtml(entry.error)}</span><button class="text-button" type="button" data-stability-node-retry="${escapeHtml(descriptor.key)}">重试</button>`,
        { error: true },
      ))
    } else if (entry.loaded && !entry.items.length) {
      rows.push(renderStabilityChildState(descriptor, `<span>该${parentLabel}在当前筛选条件下没有${childLabel}样本</span>`))
    } else if (entry.loaded && entry.items.length < entry.total) {
      rows.push(renderStabilityChildState(
        descriptor,
        `<span>已加载 ${escapeHtml(formatInteger(entry.items.length))} / ${escapeHtml(formatInteger(entry.total))} 个${childLabel}</span><button class="text-button" type="button" data-stability-node-more="${escapeHtml(descriptor.key)}">加载更多</button>`,
      ))
    }
    return rows.join('')
  }

  function renderStabilityTreeNode(item, levelIndex, ancestorKeys = []) {
    const descriptor = stabilityNodeDescriptor(item, levelIndex, ancestorKeys)
    if (!descriptor) return ''
    state.stabilityTreeNodes.set(descriptor.key, descriptor)
    const expanded = descriptor.expandable && state.stabilityExpandedNodes.has(descriptor.key)
    const rowClass = [
      'stability-tree-row',
      `stability-tree-row--level-${levelIndex}`,
      `stability-tree-row--${descriptor.type}`,
      expanded ? 'is-expanded' : '',
    ].filter(Boolean).join(' ')
    return renderStabilityMetricRow(item, renderStabilityTreeIdentity(descriptor), rowClass) +
      (expanded ? renderStabilityNodeChildren(descriptor) : '')
  }

  function renderStabilityTable() {
    const activeDataset = document.activeElement?.dataset
    const focusedNode = activeDataset?.stabilityNodeToggle ||
      activeDataset?.stabilityNodeRetry ||
      activeDataset?.stabilityNodeMore || ''
    state.stabilityTreeNodes.clear()
    $('stabilityHead').innerHTML = `<tr>
      <th scope="col">分析对象</th>
      ${state.stabilityWindows.map((seconds) => `<th scope="col">${escapeHtml(stabilityWindowLabel(seconds))}</th>`).join('')}
      <th scope="col">${escapeHtml(stabilityWindowLabel(stabilityDetailWindowSeconds()))}运维详情</th>
      <th scope="col">用量与缓存</th>
    </tr>`

    if (!state.stabilityItems.length) {
      $('stabilityRows').innerHTML = `<tr><td class="table-empty" colspan="${state.stabilityWindows.length + 3}">当前筛选没有可比较的运维样本</td></tr>`
      renderStabilityPagination()
      restoreStabilityNodeFocus(focusedNode)
      return
    }
    if (state.stabilityTreeMode) {
      $('stabilityRows').innerHTML = state.stabilityItems.map((item) => renderStabilityTreeNode(item, 0)).join('')
    } else {
      $('stabilityRows').innerHTML = state.stabilityItems.map((item) => renderStabilityMetricRow(
        item,
        `<div class="stability-identity">${stabilityIdentity(item)}</div>`,
      )).join('')
    }
    renderStabilityPagination()
    restoreStabilityNodeFocus(focusedNode)
  }

  function renderStabilityPagination() {
    const totalPages = Math.max(1, Math.ceil(state.stabilityTotal / DEFAULT_PAGE_SIZE))
    const rootType = state.stabilityTreePlan?.levels[0]
    const objectLabel = state.stabilityTreeMode && rootType ? `个${STABILITY_NODE_LABELS[rootType]}` : '个分析对象'
    $('stabilityPagination').innerHTML = `
      <span>共 ${escapeHtml(formatInteger(state.stabilityTotal))} ${objectLabel} · 第 ${state.stabilityPage} / ${totalPages} 页</span>
      <button type="button" data-stability-page="${state.stabilityPage - 1}" ${state.stabilityPage <= 1 ? 'disabled' : ''}>上一页</button>
      <button type="button" data-stability-page="${state.stabilityPage + 1}" ${state.stabilityPage >= totalPages ? 'disabled' : ''}>下一页</button>`
  }

  function applyStabilityNodeFilters(params, descriptor) {
    const plan = state.stabilityTreePlan
    if (!plan) return false
    for (let levelIndex = 0; levelIndex <= descriptor.levelIndex; levelIndex += 1) {
      const type = plan.levels[levelIndex]
      if (type === 'group') {
        const group = stabilityGroupCode(descriptor.item)
        if (!group) return false
        params.set('groups', group)
      } else if (type === 'channel') {
        const channelId = stabilityChannelId(descriptor.item)
        if (!channelId) return false
        params.set('channel_ids', String(channelId))
      }
    }
    return true
  }

  async function loadStabilityNodeChildren(nodeKey, { append = false } = {}) {
    const descriptor = state.stabilityTreeNodes.get(nodeKey) || state.stabilityTreeEntries.get(nodeKey)?.descriptor
    const plan = state.stabilityTreePlan
    if (
      !state.stabilityTreeMode ||
      !plan ||
      !descriptor?.expandable ||
      !state.stabilityTreeQueryParams ||
      !state.stabilityExpandedNodes.has(nodeKey)
    ) return
    const entry = stabilityTreeEntry(descriptor)
    if (entry.loading || (!append && entry.loaded)) return

    const generation = state.stabilityTreeGeneration
    const planDimension = plan.dimension
    const page = append ? entry.page + 1 : 1
    const controller = new AbortController()
    entry.controller?.abort()
    entry.controller = controller
    entry.loading = true
    entry.error = ''
    if (!append) {
      entry.items = []
      entry.total = 0
      entry.page = 0
      entry.loaded = false
    }
    renderStabilityTable()

    const requestIsCurrent = () =>
      generation === state.stabilityTreeGeneration &&
      state.stabilityTreePlan?.dimension === planDimension &&
      state.stabilityTreeEntries.get(nodeKey) === entry &&
      entry.controller === controller

    try {
      // 展开、重试和加载更多必须复用根节点请求的完整查询快照，
      // 避免停留跨桶后父子行使用不同的 end_timestamp。
      const params = new URLSearchParams(state.stabilityTreeQueryParams)
      params.set('dimension', plan.levels.slice(0, descriptor.levelIndex + 2).join('_'))
      if (!applyStabilityNodeFilters(params, descriptor)) {
        throw new Error('当前节点缺少可用于下钻的稳定标识')
      }
      params.set('page', String(page))
      params.set('page_size', String(STABILITY_CHILD_PAGE_SIZE))
      const payload = await requestJson(`${API_ROOT}/stability?${params}`, { signal: controller.signal })
      if (!requestIsCurrent()) return
      const items = arrayValue(payload, ['items', 'data'])
      if (append) {
        const childLevel = descriptor.levelIndex + 1
        const existing = new Set(entry.items.map((item) => stabilityNodeKey(item, childLevel, plan)))
        entry.items.push(...items.filter((item) => {
          const key = stabilityNodeKey(item, childLevel, plan)
          if (key && existing.has(key)) return false
          if (key) existing.add(key)
          return true
        }))
      } else {
        entry.items = items
      }
      entry.total = numberValue(payload, ['total', 'total_count'], entry.items.length)
      entry.page = page
      entry.loaded = true
    } catch (error) {
      if (error?.name === 'AbortError' || !requestIsCurrent()) return
      const childType = plan.levels[descriptor.levelIndex + 1]
      entry.error = error?.message || `${STABILITY_NODE_LABELS[childType]}数据加载失败`
    } finally {
      if (requestIsCurrent()) {
        entry.loading = false
        entry.controller = null
        renderStabilityTable()
      }
    }
  }

  function abortStabilityTreeEntry(entry) {
    if (!entry?.loading) return
    entry.controller?.abort()
    entry.controller = null
    entry.loading = false
  }

  function collapseStabilityNode(nodeKey) {
    state.stabilityExpandedNodes.delete(nodeKey)
    state.stabilityTreeEntries.forEach((entry, entryKey) => {
      if (entryKey !== nodeKey && !entry.ancestorKeys.includes(nodeKey)) return
      abortStabilityTreeEntry(entry)
      if (entryKey !== nodeKey) state.stabilityExpandedNodes.delete(entryKey)
    })
  }

  function toggleStabilityNode(nodeKey) {
    const descriptor = state.stabilityTreeNodes.get(nodeKey) || state.stabilityTreeEntries.get(nodeKey)?.descriptor
    if (!state.stabilityTreeMode || !descriptor?.expandable || !descriptor.filterable) return
    if (state.stabilityExpandedNodes.has(nodeKey)) {
      collapseStabilityNode(nodeKey)
      renderStabilityTable()
      return
    }

    state.stabilityExpandedNodes.add(nodeKey)
    const entry = state.stabilityTreeEntries.get(nodeKey)
    if (entry?.loaded || entry?.error) renderStabilityTable()
    else loadStabilityNodeChildren(nodeKey)
  }

  async function loadStability(signal) {
    const maxSeconds = Math.max(0, state.retentionDays * 86400)
    const windows = STABILITY_WINDOWS.filter((seconds) => seconds <= maxSeconds)
    state.stabilityWindows = windows.length ? windows : [Math.max(300, maxSeconds)]
    const sortBy = $('stabilitySort').value
    const selectedDimension = $('stabilityDimension').value
    updateStabilityDimensionControls()
    state.stabilityTreePlan = STABILITY_TREE_PLANS[selectedDimension] || null
    state.stabilityTreeMode = Boolean(state.stabilityTreePlan)
    const params = buildQuery({
      dimension: state.stabilityTreePlan?.levels[0] || selectedDimension,
      model_dimension: $('stabilityModelDimension').value,
      windows: state.stabilityWindows.join(','),
      page: state.stabilityPage,
      page_size: DEFAULT_PAGE_SIZE,
      sort_by: sortBy,
      sort_order: sortBy === 'quality_success_rate' ? 'asc' : 'desc',
    }, { includeStatus: false })
    state.stabilityTreeQueryParams = state.stabilityTreeMode ? new URLSearchParams(params) : null
    const payload = await requestJson(`${API_ROOT}/stability?${params}`, { signal })
    state.stabilityItems = arrayValue(payload, ['items', 'data'])
    state.stabilityTotal = numberValue(payload, ['total', 'total_count'], state.stabilityItems.length)
    renderStabilityTable()
    renderDataQuality(payload)
    return { empty: state.stabilityItems.length === 0, meta: extractMeta(payload) }
  }

  function normalizeChannel(row) {
    const id = Number(firstDefined(row, ['channel_id', 'id'], 0))
    const type = Number(firstDefined(row, ['channel_type', 'type'], 0))
    return {
      raw: row,
      id,
      name: String(firstDefined(row, ['channel_name', 'channel_name_snapshot', 'name'], `渠道 #${id}`)),
      type,
      typeName: String(firstDefined(row, ['channel_type_name', 'type_name'], channelTypes[type] || `类型 ${type}`)),
      group: String(firstDefined(row, ['group', 'channel_group'], '')),
      calls: numberValue(row, ['channel_attempt_count', 'attempt_count', 'request_count', 'calls']),
      retries: numberValue(row, ['retry_count', 'retries']),
      successRate: firstDefined(row, ['channel_quality_success_rate', 'quality_success_rate', 'success_rate'], null),
      input: numberValue(row, ['input_tokens_total', 'input_tokens']),
      output: numberValue(row, ['output_tokens']),
      cacheRead: numberValue(row, ['cache_read_tokens']),
      cacheWrite: numberValue(row, ['cache_write_tokens']),
      cacheHitRate: firstDefined(row, ['cache_request_hit_rate', 'cache_hit_rate'], null),
      avgLatency: numberValue(row, ['avg_latency_ms', 'average_latency_ms'], NaN),
      p95Latency: numberValue(row, ['p95_latency_ms'], NaN),
      avgTtft: numberValue(row, ['avg_ttft_ms', 'average_ttft_ms'], NaN),
      p95Ttft: numberValue(row, ['p95_ttft_ms'], NaN),
      chargedQuota: numberValue(row, ['charged_quota', 'quota']),
      chargedMicroUsd: numberValue(row, ['charged_micro_usd'], NaN),
      lastFailure: normalizeTimestamp(firstDefined(row, ['last_failure_at', 'last_failed_at'], 0)),
      statusItems: arrayValue(row, ['top_status_codes', 'status_codes']),
      failureCount: numberValue(row, ['failure_count', 'failed_attempt_count']),
    }
  }

  function rateClass(value) {
    const percent = percentNumber(value)
    if (percent === null) return ''
    if (percent < 90) return 'rate-value--danger'
    if (percent < 98) return 'rate-value--warning'
    return ''
  }

  function channelCost(channel) {
    if (Number.isFinite(channel.chargedMicroUsd)) return `$${(channel.chargedMicroUsd / 1e6).toFixed(4)}`
    return formatCompact(channel.chargedQuota)
  }

  function channelStatusBadges(channel) {
    const items = channel.statusItems.length
      ? channel.statusItems.slice(0, 3).map((item) => ({
          code: Number(firstDefined(item, ['status_code', 'code'], 0)),
          present: firstDefined(item, ['status_present', 'present'], true) !== false,
          count: numberValue(item, ['count', 'call_count']),
        }))
      : channel.failureCount
        ? [{ code: 0, present: true, count: channel.failureCount }]
        : []
    if (!items.length) return '<span class="cell-sub">暂无</span>'
    return items.map((item) => {
      const tone = statusTone(item.code, item.present)
      const label = !item.present ? '未知' : item.code === 0 ? '无响应' : item.code
      return `<span class="status-pill ${tone.className}" data-status-code="${escapeHtml(label)}">${escapeHtml(label)} · ${escapeHtml(formatCompact(item.count))}</span>`
    }).join(' ')
  }

  function renderChannelRows() {
    const target = $('channelRows')
    if (!state.channels.length) {
      target.innerHTML = '<tr><td class="table-empty" colspan="10">当前筛选条件下暂无渠道统计</td></tr>'
      renderChannelPagination()
      return
    }

    const rows = []
    for (const channel of state.channels) {
      const expanded = state.expandedChannels.has(channel.id)
      rows.push(`<tr class="channel-row" data-channel-id="${channel.id}">
        <td><div class="channel-cell">
          <button class="expand-button" type="button" data-expand-channel="${channel.id}" aria-expanded="${expanded}" aria-label="${expanded ? '收起' : '展开'} ${escapeHtml(channel.name)}">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m9 5 7 7-7 7" /></svg>
          </button>
          <div><div class="channel-name" title="${escapeHtml(channel.name)}">${escapeHtml(channel.name)}</div><div class="channel-sub">#${channel.id} · ${escapeHtml(channel.typeName)}${channel.group ? ` · 配置分组 ${escapeHtml(channel.group)}` : ''}</div></div>
        </div></td>
        <td><strong>${escapeHtml(formatInteger(channel.calls))}</strong><div class="cell-sub">重试 ${escapeHtml(formatInteger(channel.retries))}</div></td>
        <td><span class="rate-value ${rateClass(channel.successRate)}">${escapeHtml(formatPercent(channel.successRate))}</span><div class="cell-sub">可归因渠道样本</div></td>
        <td>${channelStatusBadges(channel)}</td>
        <td><strong>${escapeHtml(formatCompact(channel.input))}</strong> / ${escapeHtml(formatCompact(channel.output))}</td>
        <td>${escapeHtml(formatCompact(channel.cacheRead))} / ${escapeHtml(formatCompact(channel.cacheWrite))}</td>
        <td>${escapeHtml(formatPercent(channel.cacheHitRate))}</td>
        <td><strong>${escapeHtml(formatDuration(channel.avgLatency))} / ${escapeHtml(formatDuration(channel.p95Latency))}</strong><div class="cell-sub">TTFT ${escapeHtml(formatDuration(channel.avgTtft))} / ${escapeHtml(formatDuration(channel.p95Ttft))}</div></td>
        <td>${escapeHtml(channelCost(channel))}</td>
        <td>${channel.lastFailure ? `<button class="text-button" type="button" data-channel-failures="${channel.id}">${escapeHtml(formatRelativeTime(channel.lastFailure))}</button>` : '<span class="cell-sub">暂无</span>'}</td>
      </tr>`)

      if (expanded) rows.push(renderModelRows(channel))
    }
    target.innerHTML = rows.join('')
    renderChannelPagination()
  }

  function renderModelRows(channel) {
    const entry = state.channelModels.get(channel.id)
    if (!entry || entry.loading) {
      return `<tr class="model-row"><td class="detail-loading" colspan="10">正在加载 ${escapeHtml(channel.name)} 的模型统计…</td></tr>`
    }
    if (entry.error) {
      return `<tr class="model-row"><td class="detail-error" colspan="10">模型统计加载失败：${escapeHtml(entry.error)} <button type="button" class="text-button" data-retry-models="${channel.id}">重试</button></td></tr>`
    }
    if (!entry.items.length) {
      return '<tr class="model-row"><td class="detail-loading" colspan="10">当前范围暂无模型级统计</td></tr>'
    }

    const modelRows = entry.items.map((raw) => {
      const model = normalizeChannel(raw)
      const requested = String(firstDefined(raw, ['requested_model', 'model', 'model_name'], ''))
      const upstream = String(firstDefined(raw, ['upstream_model'], ''))
      const primaryModel = state.modelDimension === 'upstream'
        ? upstream || requested || '未知模型'
        : requested || upstream || '未知模型'
      const mappedModel = requested && upstream && requested !== upstream
        ? state.modelDimension === 'upstream'
          ? `请求：${requested}`
          : `上游：${upstream}`
        : ''
      return `<tr class="model-row">
        <td class="model-cell"><span class="model-indicator"></span><strong>${escapeHtml(primaryModel)}</strong>${mappedModel ? `<div class="cell-sub">${escapeHtml(mappedModel)}</div>` : ''}</td>
        <td><strong>${escapeHtml(formatInteger(model.calls))}</strong><div class="cell-sub">重试 ${escapeHtml(formatInteger(model.retries))}</div></td>
        <td><span class="rate-value ${rateClass(model.successRate)}">${escapeHtml(formatPercent(model.successRate))}</span></td>
        <td>${channelStatusBadges(model)}</td>
        <td>${escapeHtml(formatCompact(model.input))} / ${escapeHtml(formatCompact(model.output))}</td>
        <td>${escapeHtml(formatCompact(model.cacheRead))} / ${escapeHtml(formatCompact(model.cacheWrite))}</td>
        <td>${escapeHtml(formatPercent(model.cacheHitRate))}</td>
        <td><strong>${escapeHtml(formatDuration(model.avgLatency))} / ${escapeHtml(formatDuration(model.p95Latency))}</strong><div class="cell-sub">TTFT ${escapeHtml(formatDuration(model.avgTtft))} / ${escapeHtml(formatDuration(model.p95Ttft))}</div></td>
        <td>${escapeHtml(channelCost(model))}</td>
        <td>${model.lastFailure ? escapeHtml(formatRelativeTime(model.lastFailure)) : '<span class="cell-sub">暂无</span>'}</td>
      </tr>`
    }).join('')

    const page = Math.max(1, Number(entry.page) || 1)
    const total = Math.max(entry.items.length, Number(entry.total) || 0)
    const totalPages = Math.max(1, Math.ceil(total / MODEL_PAGE_SIZE))
    if (totalPages <= 1) return modelRows

    return `${modelRows}<tr class="model-row model-pagination-row"><td colspan="10">
      <div class="pagination model-pagination">
        <span>共 ${escapeHtml(formatInteger(total))} 个模型 · 第 ${page} / ${totalPages} 页</span>
        <button type="button" data-model-channel="${channel.id}" data-model-page="${page - 1}" ${page <= 1 ? 'disabled' : ''}>上一页</button>
        <button type="button" data-model-channel="${channel.id}" data-model-page="${page + 1}" ${page >= totalPages ? 'disabled' : ''}>下一页</button>
      </div>
    </td></tr>`
  }

  function renderChannelPagination() {
    const totalPages = Math.max(1, Math.ceil(state.channelTotal / DEFAULT_PAGE_SIZE))
    $('channelPagination').innerHTML = `
      <span>共 ${escapeHtml(formatInteger(state.channelTotal))} 个渠道 · 第 ${state.channelPage} / ${totalPages} 页</span>
      <button type="button" data-channel-page="${state.channelPage - 1}" ${state.channelPage <= 1 ? 'disabled' : ''}>上一页</button>
      <button type="button" data-channel-page="${state.channelPage + 1}" ${state.channelPage >= totalPages ? 'disabled' : ''}>下一页</button>`
  }

  async function loadChannels(signal) {
    const sortBy = $('channelSort').value
    const params = buildQuery({
      page: state.channelPage,
      page_size: DEFAULT_PAGE_SIZE,
      sort_by: sortBy,
      sort_order: sortBy === 'channel_name' ? 'asc' : 'desc',
    }, { includeStatus: false })
    const payload = await requestJson(`${API_ROOT}/channels?${params}`, { signal })
    const items = arrayValue(payload, ['items', 'channels', 'data'])
    // 查询范围变化后不能继续展示上一范围的模型子行。
    state.expandedChannels.clear()
    state.channelModels.clear()
    state.channels = items.map(normalizeChannel)
    state.channelTotal = numberValue(payload, ['total', 'total_count'], items.length)
    renderChannelRows()
    renderDataQuality(payload)
    return { empty: items.length === 0, meta: extractMeta(payload) }
  }

  async function toggleChannelModels(channelId, forceReload = false, requestedPage = 1) {
    if (state.expandedChannels.has(channelId) && !forceReload) {
      state.expandedChannels.delete(channelId)
      renderChannelRows()
      return
    }
    state.expandedChannels.add(channelId)
    const cached = state.channelModels.get(channelId)
    if (!forceReload && cached?.items) {
      renderChannelRows()
      return
    }

    const page = Math.max(1, Number(requestedPage) || 1)
    state.channelModels.set(channelId, {
      loading: true,
      items: [],
      error: '',
      page,
      total: Number(cached?.total) || 0,
    })
    renderChannelRows()
    try {
      const params = buildQuery({
        page,
        page_size: MODEL_PAGE_SIZE,
        sort_by: 'request_count',
        sort_order: 'desc',
        model_dimension: state.modelDimension,
      }, { includeStatus: false })
      const payload = await requestJson(`${API_ROOT}/channels/${encodeURIComponent(channelId)}/models?${params}`, {
        signal: state.controller?.signal,
      })
      const items = arrayValue(payload, ['items', 'models', 'data'])
      state.channelModels.set(channelId, {
        loading: false,
        items,
        error: '',
        page: numberValue(payload, ['page'], page),
        total: numberValue(payload, ['total', 'total_count'], items.length),
      })
    } catch (error) {
      if (error?.name === 'AbortError') return
      state.channelModels.set(channelId, {
        loading: false,
        items: [],
        error: error?.message || '未知错误',
        page,
        total: Number(cached?.total) || 0,
      })
    }
    renderChannelRows()
  }

  function normalizeStages(payload) {
    return arrayValue(payload, ['error_stages', 'stages', 'stage_distribution']).map((row) => {
      const key = String(firstDefined(row, ['error_stage', 'stage', 'name'], 'unknown'))
      return {
        code: -1,
        present: true,
        label: stageLabels[key] || key,
        count: numberValue(row, ['count', 'failure_count', 'total']),
      }
    }).sort((a, b) => b.count - a.count)
  }

  function renderFailureStatus(payload) {
    const items = normalizeStatusItems(payload)
    const groups = { success: 0, client: 0, server: 0, noResponse: 0, unknown: 0 }
    items.forEach((item) => {
      if (!item.present) groups.unknown += item.count
      else if (item.code === 0) groups.noResponse += item.count
      else if (item.code >= 200 && item.code < 300) groups.success += item.count
      else if (item.code >= 400 && item.code < 500) groups.client += item.count
      else if (item.code >= 500) groups.server += item.count
      else groups.unknown += item.count
    })
    const pills = [
      ['2xx', groups.success, 'success'],
      ['4xx', groups.client, 'warning'],
      ['5xx', groups.server, 'danger'],
      ['无响应', groups.noResponse, 'danger'],
      ['未知', groups.unknown, ''],
    ]
    $('statusCodeSummary').innerHTML = pills.map(([label, count, tone]) => `<span class="status-pill ${tone ? `status-pill--${tone}` : ''}">${escapeHtml(label)} · ${escapeHtml(formatInteger(count))}</span>`).join('')
    renderStatusBars('statusCodeBars', items, 12)
    renderStatusBars('errorStageBars', normalizeStages(payload), 10)
  }

  function normalizeFailure(row) {
    const statusValue = (prefix) => {
      if (!booleanValue(row, [`${prefix}_status_present`])) return null
      const value = Number(firstDefined(row, [`${prefix}_status_code`], NaN))
      return Number.isFinite(value) ? value : null
    }
    return {
      id: String(firstDefined(row, ['event_id', 'id'], '')),
      requestId: String(firstDefined(row, ['request_id'], '')),
      channelId: Number(firstDefined(row, ['channel_id'], 0)),
      channelName: String(firstDefined(row, ['channel_name', 'channel_name_snapshot'], '未知渠道')),
      requestedModel: String(firstDefined(row, ['requested_model'], '')),
      upstreamModel: String(firstDefined(row, ['upstream_model'], '')),
      attemptSeq: numberValue(row, ['attempt_seq'], 0),
      retryPlanned: booleanValue(row, ['retry_planned']),
      retryReason: String(firstDefined(row, ['retry_reason'], '')),
      outcome: String(firstDefined(row, ['outcome'], 'unknown')),
      dataOrigin: String(firstDefined(row, ['data_origin'], 'live')),
      failureOwner: String(firstDefined(row, ['failure_owner'], 'unknown')),
      partialResponse: booleanValue(row, ['partial_response']),
      errorStage: String(firstDefined(row, ['error_stage'], 'unknown')),
      upstreamStatus: statusValue('upstream'),
      normalizedStatus: statusValue('normalized'),
      clientStatus: statusValue('client'),
      latency: numberValue(row, ['latency_ms'], NaN),
      summary: String(firstDefined(row, ['error_summary', 'summary', 'message'], '未提供错误摘要')),
      timestamp: normalizeTimestamp(firstDefined(row, ['created_at', 'timestamp', 'ts'], 0)),
      logUrl: String(firstDefined(row, ['log_url', 'log_href'], '')),
    }
  }

  function safeRelativeUrl(value) {
    if (!value || typeof value !== 'string') return ''
    return value.startsWith('/') && !value.startsWith('//') ? value : ''
  }

  function renderFailures(payload) {
    const items = arrayValue(payload, ['items', 'failures', 'events', 'data']).map(normalizeFailure)
    state.failureTotal = numberValue(payload, ['total', 'total_count'], items.length)
    $('failureCount').textContent = `共 ${formatInteger(state.failureTotal)} 条近期失败`
    $('failureRows').innerHTML = items.length ? items.map((failure) => {
      const selectedCode = state.statusScope === 'client' ? failure.clientStatus : failure.upstreamStatus
      const statusLabel = selectedCode === null ? '状态未知' : selectedCode === 0 ? '无响应' : String(selectedCode)
      const status = statusTone(selectedCode ?? -1, selectedCode !== null)
      const logUrl = safeRelativeUrl(failure.logUrl)
      return `<article class="failure-item">
        <div class="failure-heading">
          <div class="failure-title">
            <span class="status-pill ${status.className}">${escapeHtml(statusLabel)}</span>
            <strong>${escapeHtml(failure.channelName)} · ${escapeHtml(failure.requestedModel || failure.upstreamModel || '未知模型')}</strong>
          </div>
          <span class="failure-time">${escapeHtml(formatDateTime(failure.timestamp))}</span>
        </div>
        <div class="failure-summary">${escapeHtml(failure.summary)}</div>
        <div class="failure-meta">
          ${failure.requestId ? `<span>请求 ID <code>${escapeHtml(failure.requestId)}</code></span>` : ''}
          <span>${escapeHtml(outcomeLabels[failure.outcome] || failure.outcome)}</span>
          <span>数据 ${failure.dataOrigin === 'legacy' ? '历史日志推导' : '实时精确采集'}</span>
          <span>${escapeHtml(stageLabels[failure.errorStage] || failure.errorStage)}</span>
          <span>尝试 #${failure.attemptSeq || '-'}</span>
          <span>${failure.retryPlanned ? '已计划重试' : '未继续重试'}</span>
          ${failure.retryReason ? `<span>重试原因 ${escapeHtml(failure.retryReason)}</span>` : ''}
          <span>归因 ${escapeHtml(failureOwnerLabels[failure.failureOwner] || failure.failureOwner || '未知')}</span>
          ${failure.upstreamStatus !== null ? `<span>上游状态 ${escapeHtml(failure.upstreamStatus === 0 ? '无响应' : failure.upstreamStatus)}</span>` : ''}
          ${failure.normalizedStatus !== null ? `<span>规范状态 ${escapeHtml(failure.normalizedStatus)}</span>` : ''}
          ${failure.clientStatus !== null ? `<span>客户端状态 ${escapeHtml(failure.clientStatus)}</span>` : ''}
          ${failure.partialResponse ? '<span>已返回部分响应</span>' : ''}
          <span>耗时 ${escapeHtml(formatDuration(failure.latency))}</span>
          ${failure.upstreamModel && failure.upstreamModel !== failure.requestedModel ? `<span>上游模型 ${escapeHtml(failure.upstreamModel)}</span>` : ''}
          ${logUrl ? `<a class="text-button" href="${escapeHtml(logUrl)}" target="_top">查看完整日志</a>` : ''}
        </div>
      </article>`
    }).join('') : '<div class="table-empty">当前范围没有近期失败明细</div>'
    renderFailurePagination()
  }

  function renderFailurePagination() {
    const totalPages = Math.max(1, Math.ceil(state.failureTotal / DEFAULT_PAGE_SIZE))
    $('failurePagination').innerHTML = `
      <span>第 ${state.failurePage} / ${totalPages} 页</span>
      <button type="button" data-failure-page="${state.failurePage - 1}" ${state.failurePage <= 1 ? 'disabled' : ''}>上一页</button>
      <button type="button" data-failure-page="${state.failurePage + 1}" ${state.failurePage >= totalPages ? 'disabled' : ''}>下一页</button>`
  }

  async function loadFailures(signal) {
    const metricScope = state.statusScope === 'client' ? 'final_request' : 'upstream_call'
    const statusParams = buildQuery({ metric_scope: metricScope }, {
      statusScope: state.statusScope,
    })
    const failureParams = buildQuery({ page: state.failurePage, page_size: DEFAULT_PAGE_SIZE }, {
      statusScope: state.statusScope,
      // 失败事件首期未保存 stream 维度；状态分布仍会应用该筛选。
      includeStream: false,
    })
    const [statuses, failures] = await Promise.all([
      requestJson(`${API_ROOT}/status-codes?${statusParams}`, { signal }),
      requestJson(`${API_ROOT}/failures?${failureParams}`, { signal }),
    ])
    state.statuses = statuses
    state.failures = failures
    $('statusScopeHint').textContent = state.statusScope === 'client'
      ? '最终客户端状态码，按逻辑请求统计'
      : '上游原始状态码，按底层调用统计'
    renderFailureStatus(statuses)
    renderFailures(failures)
    renderDataQuality(statuses, failures)
    const statusCount = normalizeStatusItems(statuses).reduce((sum, item) => sum + item.count, 0)
    return {
      empty: statusCount === 0 && state.failureTotal === 0,
      meta: extractMeta(statuses, failures),
    }
  }

  function normalizeProbeChannel(row) {
    const id = Number(firstDefined(row, ['id', 'channel_id'], 0))
    const type = Number(firstDefined(row, ['type', 'channel_type'], 0))
    const modelsRaw = firstDefined(row, ['models'], [])
    const models = Array.isArray(modelsRaw)
      ? modelsRaw.map(String).filter(Boolean)
      : String(modelsRaw || '').split(',').map((item) => item.trim()).filter(Boolean)
    return {
      id,
      name: String(firstDefined(row, ['name', 'channel_name'], `渠道 #${id}`)),
      type,
      typeName: channelTypes[type] || `类型 ${type}`,
      status: Number(firstDefined(row, ['status'], 0)),
      models,
      responseTime: numberValue(row, ['response_time'], NaN),
      testTime: normalizeTimestamp(firstDefined(row, ['test_time'], 0)),
    }
  }

  function probeStatus(channel) {
    if (channel.status === 1) return ['已启用', 'badge--success']
    if (channel.status === 2) return ['手动禁用', 'badge--warning']
    if (channel.status === 3) return ['自动禁用', 'badge--danger']
    return ['已禁用', 'badge--warning']
  }

  function filteredProbeChannels() {
    const filters = readFilterState()
    const keyword = state.probeKeyword.toLocaleLowerCase('zh-CN')
    return state.probeChannels.filter((channel) => {
      if (filters.channelId && String(channel.id) !== filters.channelId) return false
      if (filters.channelType && String(channel.type) !== filters.channelType) return false
      if (filters.requestedModel && !channel.models.includes(filters.requestedModel)) return false
      if (!keyword) return true
      return `${channel.name} ${channel.id} ${channel.typeName}`.toLocaleLowerCase('zh-CN').includes(keyword)
    })
  }

  function renderProbeRows() {
    const channels = filteredProbeChannels()
    const target = $('probeRows')
    if (!channels.length) {
      target.innerHTML = '<tr><td class="table-empty" colspan="7">没有匹配的渠道</td></tr>'
      return
    }
    target.innerHTML = channels.map((channel) => {
      const [statusLabel, statusClass] = probeStatus(channel)
      const selected = state.probeModels.get(channel.id) || channel.models[0] || ''
      const result = state.probeResults.get(channel.id)
      const testing = state.testingChannels.has(channel.id)
      const resultHtml = result
        ? `<span class="probe-result probe-result--${result.success ? 'success' : 'error'}">${result.success ? '✓' : '×'} ${escapeHtml(result.success ? formatDuration(result.duration) : formatProbeError(result.message))}</span>`
        : Number.isFinite(channel.responseTime)
          ? escapeHtml(formatDuration(channel.responseTime))
          : '<span class="cell-sub">未测试</span>'
      return `<tr>
        <td><div class="channel-name">${escapeHtml(channel.name)}</div><div class="channel-sub">#${channel.id}</div></td>
        <td><span class="badge ${statusClass}">${escapeHtml(statusLabel)}</span></td>
        <td>${escapeHtml(channel.typeName)}</td>
        <td><select data-probe-model="${channel.id}" aria-label="${escapeHtml(channel.name)} 的测试模型" ${channel.models.length ? '' : 'disabled'}>
          ${channel.models.length ? channel.models.map((model) => `<option value="${escapeHtml(model)}" ${model === selected ? 'selected' : ''}>${escapeHtml(model)}</option>`).join('') : '<option value="">无可用模型</option>'}
        </select></td>
        <td>${escapeHtml(formatRelativeTime(channel.testTime))}</td>
        <td>${resultHtml}</td>
        <td><div class="probe-actions"><button class="button button--secondary" type="button" data-test-channel="${channel.id}" ${testing || !selected ? 'disabled' : ''}>${testing ? '测试中…' : '运行测试'}</button></div></td>
      </tr>`
    }).join('')
  }

  async function loadProbeChannels(signal) {
    const all = []
    let page = 1
    let total = 0
    do {
      const params = new URLSearchParams({
        keyword: '',
        group: '',
        model: '',
        id_sort: 'true',
        tag_mode: 'false',
        p: String(page),
        page_size: '100',
      })
      const payload = await requestJson(`/api/channel/search?${params}`, { signal })
      const items = arrayValue(payload, ['items', 'channels', 'data'])
      total = numberValue(payload, ['total'], items.length)
      all.push(...items)
      page += 1
      if (!items.length) break
    } while (all.length < total && page <= 101)
    state.probeChannels = all.map(normalizeProbeChannel)
    state.probeChannels.forEach((channel) => {
      if (!state.probeModels.has(channel.id) && channel.models[0]) state.probeModels.set(channel.id, channel.models[0])
    })
    renderProbeRows()
    $('qualityNotice').classList.add('is-hidden')
    return { empty: false, meta: { generated_at: Math.floor(Date.now() / 1000) } }
  }

  async function testProbeChannel(channelId) {
    const model = state.probeModels.get(channelId) || ''
    if (!model || state.testingChannels.has(channelId)) return
    state.testingChannels.add(channelId)
    state.probeResults.delete(channelId)
    renderProbeRows()
    const started = performance.now()
    try {
      const params = new URLSearchParams({ model })
      const result = await requestJson(`/api/channel/test/${encodeURIComponent(channelId)}?${params}`)
      const duration = numberValue(result, ['time'], (performance.now() - started) / 1000) * 1000
      state.probeResults.set(channelId, { success: true, duration })
      showToast(`渠道 #${channelId} 探测成功，耗时 ${formatDuration(duration)}`)
    } catch (error) {
      state.probeResults.set(channelId, { success: false, message: error?.message || '探测失败' })
      showToast(`渠道 #${channelId} 探测失败：${formatProbeError(error?.message || '未知错误')}`)
    } finally {
      state.testingChannels.delete(channelId)
      renderProbeRows()
    }
  }

  async function loadCurrentView() {
    if (state.controller) state.controller.abort()
    if (state.view === 'operations') resetStabilityTree()
    state.controller = new AbortController()
    const signal = state.controller.signal
    const loadId = ++state.loadingId
    setLoading(true)

    try {
      await verifyHostCompatibility(signal)
      if (!state.filtersLoaded) await loadFilterOptions()
      let result
      if (state.view === 'overview') result = await loadOverview(signal)
      else if (state.view === 'operations') result = await loadStability(signal)
      else if (state.view === 'channels') result = await loadChannels(signal)
      else if (state.view === 'failures') result = await loadFailures(signal)
      else result = await loadProbeChannels(signal)
      if (loadId !== state.loadingId) return
      setLoading(false)
      $('errorState').classList.add('is-hidden')
      showEmpty(result.empty)
      updateFreshness(result.empty ? 'empty' : 'ok', result.meta)
    } catch (error) {
      if (error?.name === 'AbortError' || loadId !== state.loadingId) return
      setLoading(false)
      showError(error)
    }
  }

  function replaceSelectOptions(select, placeholder, items, valueKey = 'value', labelKey = 'label') {
    const selected = select.value
    const selectedOption = select.selectedOptions[0]
    const selectedSnapshot = selected
      ? {
          value: selected,
          label: selectedOption?.textContent || selected,
          model: selectedOption?.dataset.model || '',
          modelHash: selectedOption?.dataset.modelHash || '',
        }
      : null
    const fragment = document.createDocumentFragment()
    const empty = document.createElement('option')
    empty.value = ''
    empty.textContent = placeholder
    fragment.appendChild(empty)
    const seen = new Set()
    items.forEach((item) => {
      const value = typeof item === 'object' ? firstDefined(item, [valueKey, 'id', 'name'], '') : item
      const label = typeof item === 'object' ? firstDefined(item, [labelKey, 'name', 'title'], value) : item
      if (value === '' || value === null || value === undefined || seen.has(String(value))) return
      seen.add(String(value))
      const option = document.createElement('option')
      option.value = String(value)
      option.textContent = String(label)
      if (typeof item === 'object') {
        const model = firstDefined(item, ['model', 'label', 'name'], '')
        const modelHash = firstDefined(item, ['model_hash', 'hash'], '')
        if (model) option.dataset.model = String(model)
        if (modelHash) option.dataset.modelHash = String(modelHash)
      }
      fragment.appendChild(option)
    })
    // 远程搜索或刷新可能只返回部分选项，保留当前选中项，
    // 避免控件显示“全部”但页面仍是上一个筛选结果。
    if (selectedSnapshot && !seen.has(selectedSnapshot.value)) {
      const option = document.createElement('option')
      option.value = selectedSnapshot.value
      option.textContent = selectedSnapshot.label
      if (selectedSnapshot.model) option.dataset.model = selectedSnapshot.model
      if (selectedSnapshot.modelHash) option.dataset.modelHash = selectedSnapshot.modelHash
      fragment.appendChild(option)
      seen.add(selectedSnapshot.value)
    }
    select.replaceChildren(fragment)
    if (seen.has(selected)) select.value = selected
  }

  function channelTypeOptions(value) {
    if (Array.isArray(value)) {
      return value.map((item) => typeof item === 'object'
        ? { value: firstDefined(item, ['value', 'type', 'id'], ''), label: firstDefined(item, ['label', 'name'], '') }
        : { value: item, label: channelTypes[item] || `类型 ${item}` })
    }
    if (value && typeof value === 'object') {
      return Object.entries(value).map(([key, label]) => ({ value: key, label: String(label) }))
    }
    return Object.entries(channelTypes).map(([valueKey, label]) => ({ value: valueKey, label }))
  }

  async function loadFilterOptions() {
    replaceSelectOptions($('channelTypeFilter'), '全部类型', channelTypeOptions(null))
    try {
      const payload = await requestJson(`${API_ROOT}/filters`)
      const channels = arrayValue(payload, ['channels', 'channel_options'])
      const types = firstDefined(payload, ['channel_types', 'channel_type_options'], null)
      const groups = arrayValue(payload, ['groups', 'group_options'])
      const requestedModels = arrayValue(payload, ['requested_model_options', 'requested_models', 'models.requested'])
      const upstreamModels = arrayValue(payload, ['upstream_model_options', 'upstream_models', 'models.upstream'])
      const meta = extractMeta(payload)
      const retentionDays = numberValue(meta, ['retention_days'], state.retentionDays)
      if (retentionDays > 0) state.retentionDays = retentionDays
      replaceSelectOptions($('channelFilter'), '全部渠道', channels, 'channel_id', 'channel_name')
      replaceSelectOptions($('channelTypeFilter'), '全部类型', channelTypeOptions(types))
      replaceSelectOptions($('groupFilter'), '全部分组', groups, 'code', 'name')
      replaceSelectOptions($('requestedModelFilter'), '全部请求模型', requestedModels)
      replaceSelectOptions($('upstreamModelFilter'), '全部上游模型', upstreamModels)
      state.filtersLoaded = true
    } catch (error) {
      // 筛选项接口是增强能力，不阻断主视图，但必须显式提示局部失败。
      showToast(`部分筛选项加载失败：${error?.message || '未知错误'}`)
    }
  }

  async function searchModelFilterOptions(dimension, query) {
    const upstream = dimension === 'upstream'
    const select = upstream ? $('upstreamModelFilter') : $('requestedModelFilter')
    const placeholder = upstream ? '全部上游模型' : '全部请求模型'
    const key = upstream ? 'upstream' : 'requested'
    if (!searchModelFilterOptions.controllers) searchModelFilterOptions.controllers = new Map()
    searchModelFilterOptions.controllers.get(key)?.abort()
    const controller = new AbortController()
    searchModelFilterOptions.controllers.set(key, controller)
    try {
      const params = new URLSearchParams({
        model_dimension: key,
        q: query,
        page: '1',
        page_size: '100',
      })
      const payload = await requestJson(`${API_ROOT}/filters/models?${params}`, { signal: controller.signal })
      replaceSelectOptions(select, placeholder, arrayValue(payload, ['items', 'data']))
    } catch (error) {
      if (error?.name !== 'AbortError') showToast(`模型搜索失败：${error?.message || '未知错误'}`)
    }
  }

  function bindModelSearch(inputId, dimension) {
    $(inputId).addEventListener('input', (event) => {
      const query = event.target.value.trim()
      window.clearTimeout(bindModelSearch[dimension])
      bindModelSearch[dimension] = window.setTimeout(() => {
        if (query) searchModelFilterOptions(dimension, query)
        else {
          searchModelFilterOptions.controllers?.get(dimension)?.abort()
          loadFilterOptions()
        }
      }, 260)
    })
  }

  function resetFilters(reload = true) {
    state.range = 'today'
    state.granularity = 'auto'
    state.stabilityPage = 1
    state.channelPage = 1
    state.failurePage = 1
    $('granularity').value = 'auto'
    $('channelFilter').value = ''
    $('channelTypeFilter').value = ''
    $('groupFilter').value = ''
    $('requestedModelFilter').value = ''
    $('upstreamModelFilter').value = ''
    $('requestedModelSearch').value = ''
    $('upstreamModelSearch').value = ''
    $('outcomeFilter').value = ''
    $('statusCodeFilter').value = ''
    $('streamFilter').value = ''
    $('trafficSourceFilter').value = 'relay'
    $('dataOriginFilter').value = 'live,legacy'
    $('stabilityDimension').value = 'group_model'
    $('stabilityModelDimension').value = 'requested'
    $('stabilitySort').value = 'failure_count'
    $('channelSort').value = 'channel_name'
    $('modelDimension').value = 'requested'
    state.modelDimension = 'requested'
    updateStabilityDimensionControls()
    $('customRange').classList.add('is-hidden')
    $$('.range-button').forEach((button) => button.classList.toggle('is-active', button.dataset.range === 'today'))
    if (reload) loadCurrentView()
  }

  function selectRange(range) {
    if (!['1h', 'today', 'yesterday', '7d', 'custom'].includes(range)) return
    state.range = range
    $$('.range-button').forEach((button) => button.classList.toggle('is-active', button.dataset.range === range))
    const custom = range === 'custom'
    $('customRange').classList.toggle('is-hidden', !custom)
    if (custom) {
      const end = Math.floor(Date.now() / 1000)
      const start = end - 3600
      if (!state.customStart) state.customStart = start
      if (!state.customEnd) state.customEnd = end
      $('customStart').value = localDateTimeInput(state.customStart)
      $('customEnd').value = localDateTimeInput(state.customEnd)
      return
    }
    state.channelPage = 1
    state.failurePage = 1
    state.stabilityPage = 1
    loadCurrentView()
  }

  function applyCustomRange() {
    const start = Math.floor(new Date($('customStart').value).getTime() / 1000)
    const end = Math.floor(new Date($('customEnd').value).getTime() / 1000)
    const now = Math.floor(Date.now() / 1000)
    if (!Number.isFinite(start) || !Number.isFinite(end) || start <= 0 || end <= start) {
      showToast('请选择有效的开始和结束时间')
      return
    }
    if (end > now) {
      showToast('结束时间不能晚于当前时间')
      return
    }
    if (end - start > state.retentionDays * 86400) {
      showToast(`当前统计最多支持 ${state.retentionDays} 天范围`)
      return
    }
    state.customStart = start
    state.customEnd = end
    state.channelPage = 1
    state.failurePage = 1
    state.stabilityPage = 1
    loadCurrentView()
  }

  function bindEvents() {
    $$('.view-tab').forEach((button) => button.addEventListener('click', () => activateView(button.dataset.view)))
    $$('.range-button').forEach((button) => button.addEventListener('click', () => selectRange(button.dataset.range)))
    $('refreshButton').addEventListener('click', () => {
      state.filtersLoaded = false
      loadCurrentView()
    })
    $('retryButton').addEventListener('click', loadCurrentView)
    $('emptyResetButton').addEventListener('click', () => resetFilters(true))
    $('resetFilters').addEventListener('click', () => resetFilters(true))
    $('applyCustomRange').addEventListener('click', applyCustomRange)
    bindModelSearch('requestedModelSearch', 'requested')
    bindModelSearch('upstreamModelSearch', 'upstream')

    $('advancedToggle').addEventListener('click', () => {
      const button = $('advancedToggle')
      const expanded = button.getAttribute('aria-expanded') === 'true'
      button.setAttribute('aria-expanded', String(!expanded))
      $('advancedFilters').classList.toggle('is-hidden', expanded)
    })

    $('granularity').addEventListener('change', (event) => {
      state.granularity = event.target.value
      loadCurrentView()
    })

    ;[
      'channelFilter', 'channelTypeFilter', 'requestedModelFilter', 'upstreamModelFilter',
      'groupFilter', 'outcomeFilter', 'streamFilter', 'trafficSourceFilter', 'dataOriginFilter',
    ].forEach((id) => $(id).addEventListener('change', () => {
      state.stabilityPage = 1
      state.channelPage = 1
      state.failurePage = 1
      loadCurrentView()
    }))

    const applyStatusCodeFilter = () => {
      state.failurePage = 1
      loadCurrentView()
    }
    $('statusCodeFilter').addEventListener('change', applyStatusCodeFilter)
    $('statusCodeFilter').addEventListener('keydown', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault()
        applyStatusCodeFilter()
      }
    })
    $('channelSort').addEventListener('change', () => {
      state.channelPage = 1
      loadCurrentView()
    })

    $('groupChannelsButton').addEventListener('click', () => {
      const modelDimension = $('modelDimension').value === 'upstream' ? 'upstream' : 'requested'
      state.modelDimension = modelDimension
      $('stabilityDimension').value = 'group_channel_model'
      $('stabilityModelDimension').value = modelDimension
      updateStabilityDimensionControls()
      state.stabilityPage = 1
      activateView('operations')
    })

    ;['stabilityDimension', 'stabilityModelDimension', 'stabilitySort'].forEach((id) => {
      $(id).addEventListener('change', () => {
        if (id === 'stabilityDimension') updateStabilityDimensionControls()
        state.stabilityPage = 1
        loadCurrentView()
      })
    })

    $('modelDimension').addEventListener('change', (event) => {
      state.modelDimension = event.target.value === 'upstream' ? 'upstream' : 'requested'
      state.expandedChannels.clear()
      state.channelModels.clear()
      renderChannelRows()
    })

    document.addEventListener('click', (event) => {
      const goView = event.target.closest('[data-go-view]')
      if (goView) activateView(goView.dataset.goView)

      const expand = event.target.closest('[data-expand-channel]')
      if (expand) toggleChannelModels(Number(expand.dataset.expandChannel))

      const retryModels = event.target.closest('[data-retry-models]')
      if (retryModels) {
        const channelId = Number(retryModels.dataset.retryModels)
        toggleChannelModels(channelId, true, state.channelModels.get(channelId)?.page || 1)
      }

      const modelPage = event.target.closest('[data-model-page]')
      if (modelPage && !modelPage.disabled) {
        toggleChannelModels(
          Number(modelPage.dataset.modelChannel),
          true,
          Number(modelPage.dataset.modelPage)
        )
      }

      const channelFailure = event.target.closest('[data-channel-failures]')
      if (channelFailure) {
        $('channelFilter').value = channelFailure.dataset.channelFailures
        activateView('failures')
      }

      const channelPage = event.target.closest('[data-channel-page]')
      if (channelPage && !channelPage.disabled) {
        state.channelPage = Math.max(1, Number(channelPage.dataset.channelPage))
        loadCurrentView()
      }

      const stabilityPage = event.target.closest('[data-stability-page]')
      if (stabilityPage && !stabilityPage.disabled) {
        state.stabilityPage = Math.max(1, Number(stabilityPage.dataset.stabilityPage))
        loadCurrentView()
      }

      const stabilityNodeToggle = event.target.closest('[data-stability-node-toggle]')
      if (stabilityNodeToggle) {
        toggleStabilityNode(stabilityNodeToggle.dataset.stabilityNodeToggle || '')
      }

      const stabilityNodeRetry = event.target.closest('[data-stability-node-retry]')
      if (stabilityNodeRetry) {
        const nodeKey = stabilityNodeRetry.dataset.stabilityNodeRetry || ''
        const entry = state.stabilityTreeEntries.get(nodeKey)
        loadStabilityNodeChildren(nodeKey, { append: Boolean(entry?.items.length) })
      }

      const stabilityNodeMore = event.target.closest('[data-stability-node-more]')
      if (stabilityNodeMore) {
        loadStabilityNodeChildren(stabilityNodeMore.dataset.stabilityNodeMore || '', { append: true })
      }

      const failurePage = event.target.closest('[data-failure-page]')
      if (failurePage && !failurePage.disabled) {
        state.failurePage = Math.max(1, Number(failurePage.dataset.failurePage))
        loadCurrentView()
      }

      const statusScope = event.target.closest('[data-status-scope]')
      if (statusScope && !statusScope.classList.contains('is-active')) {
        state.statusScope = statusScope.dataset.statusScope
        if (state.statusScope === 'client' && $('statusCodeFilter').value.trim() === '0') {
          $('statusCodeFilter').value = ''
          showToast('客户端状态码不使用 0，已清除“无 HTTP 响应”筛选')
        }
        $$('[data-status-scope]').forEach((button) => {
          const active = button === statusScope
          button.classList.toggle('is-active', active)
          button.setAttribute('aria-pressed', String(active))
        })
        state.failurePage = 1
        loadCurrentView()
      }

      const probeButton = event.target.closest('[data-test-channel]')
      if (probeButton) testProbeChannel(Number(probeButton.dataset.testChannel))
    })

    $('probeRows').addEventListener('change', (event) => {
      const select = event.target.closest('[data-probe-model]')
      if (!select) return
      state.probeModels.set(Number(select.dataset.probeModel), select.value)
    })

    $('probeKeyword').addEventListener('input', (event) => {
      state.probeKeyword = event.target.value.trim()
      window.clearTimeout(bindEvents.probeTimer)
      bindEvents.probeTimer = window.setTimeout(renderProbeRows, 120)
    })
  }

  function init() {
    if (state.initialized) return
    state.initialized = true
    // 必须先验证 HTML 与脚本属于同一 UI 结构版本；否则旧页面会在
    // bindEvents/readFilterState 中以 null.value 等无意义错误中断。
    if (!verifyUIAssetCompatibility()) return
    bindEvents()
    updateFilterScopeHint()
    loadCurrentView()
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init)
  else init()
})()
