import http from 'node:http'

const port = Number(process.env.PORT || 39002)

function sendJson(res, value) {
  res.writeHead(200, {
    'Content-Type': 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
  })
  res.end(JSON.stringify(value, null, 2))
}

function readHostContext(req) {
  return {
    moduleId: req.headers['x-newapi-module-id'] || '',
    userId: req.headers['x-newapi-user-id'] || '',
    username: req.headers['x-newapi-username'] || '',
    role: req.headers['x-newapi-user-role'] || '',
    group: req.headers['x-newapi-user-group'] || '',
  }
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;')
}

function sendUi(res, req) {
  const context = readHostContext(req)
  res.writeHead(200, {
    'Content-Type': 'text/html; charset=utf-8',
    'Cache-Control': 'no-store',
  })
  res.end(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>渠道质量看板</title>
    <style>
      :root {
        color-scheme: light;
        --bg: #f7f8fb;
        --panel: #ffffff;
        --border: #dfe3ea;
        --text: #1f2937;
        --muted: #6b7280;
        --soft: #f1f5f9;
        --primary: #2563eb;
        --success: #15803d;
        --warning: #b45309;
        --danger: #b91c1c;
      }
      * {
        box-sizing: border-box;
      }
      body {
        margin: 0;
        background: var(--bg);
        color: var(--text);
        font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }
      main {
        min-height: 100vh;
        padding: 20px;
      }
      .toolbar {
        display: flex;
        justify-content: space-between;
        gap: 12px;
        align-items: flex-start;
        flex-wrap: wrap;
        margin-bottom: 16px;
      }
      h1 {
        margin: 0 0 6px;
        font-size: 22px;
        line-height: 1.25;
      }
      .muted {
        color: var(--muted);
        font-size: 13px;
      }
      .controls {
        display: flex;
        gap: 8px;
        align-items: center;
        flex-wrap: wrap;
      }
      input,
      select,
      button {
        height: 34px;
        border: 1px solid var(--border);
        border-radius: 6px;
        background: #fff;
        color: var(--text);
        font: inherit;
        font-size: 13px;
      }
      input {
        width: 220px;
        padding: 0 10px;
      }
      select {
        padding: 0 8px;
      }
      button {
        padding: 0 12px;
        cursor: pointer;
      }
      button.primary {
        border-color: var(--primary);
        background: var(--primary);
        color: #fff;
      }
      button:disabled {
        cursor: not-allowed;
        opacity: 0.6;
      }
      .grid {
        display: grid;
        grid-template-columns: repeat(4, minmax(0, 1fr));
        gap: 12px;
        margin-bottom: 12px;
      }
      .card {
        border: 1px solid var(--border);
        border-radius: 8px;
        background: var(--panel);
      }
      .stat {
        padding: 14px;
      }
      .stat-label {
        color: var(--muted);
        font-size: 12px;
      }
      .stat-value {
        margin-top: 8px;
        font-size: 24px;
        font-weight: 700;
      }
      .panel {
        padding: 14px;
      }
      .panel-title {
        display: flex;
        justify-content: space-between;
        gap: 10px;
        align-items: center;
        margin-bottom: 10px;
        font-weight: 650;
      }
      .split {
        display: grid;
        grid-template-columns: 1.1fr 0.9fr;
        gap: 12px;
        margin-bottom: 12px;
      }
      .bars {
        display: grid;
        gap: 8px;
      }
      .bar-row {
        display: grid;
        grid-template-columns: 96px 1fr 44px;
        gap: 10px;
        align-items: center;
        font-size: 13px;
      }
      .bar-track {
        height: 8px;
        overflow: hidden;
        border-radius: 999px;
        background: var(--soft);
      }
      .bar-fill {
        height: 100%;
        border-radius: inherit;
        background: var(--primary);
      }
      .table-wrap {
        overflow: auto;
      }
      table {
        width: 100%;
        border-collapse: collapse;
        font-size: 13px;
      }
      th,
      td {
        padding: 10px 12px;
        border-top: 1px solid var(--border);
        text-align: left;
        white-space: nowrap;
      }
      th {
        color: var(--muted);
        font-weight: 600;
        background: #fafafa;
      }
      .name-cell {
        min-width: 220px;
        white-space: normal;
      }
      .name-main {
        font-weight: 650;
      }
      .name-sub {
        margin-top: 3px;
        color: var(--muted);
        font-size: 12px;
      }
      .badge {
        display: inline-flex;
        align-items: center;
        min-height: 22px;
        padding: 2px 8px;
        border-radius: 999px;
        background: var(--soft);
        color: var(--muted);
        font-size: 12px;
        font-weight: 600;
      }
      .badge.success {
        background: #dcfce7;
        color: var(--success);
      }
      .badge.warning {
        background: #fef3c7;
        color: var(--warning);
      }
      .badge.danger {
        background: #fee2e2;
        color: var(--danger);
      }
      .badge.primary {
        background: #dbeafe;
        color: var(--primary);
      }
      .notice {
        border: 1px solid var(--border);
        border-radius: 8px;
        background: #fff;
        padding: 14px;
        color: var(--muted);
      }
      .toast {
        position: fixed;
        right: 18px;
        bottom: 18px;
        z-index: 10;
        max-width: 420px;
        border: 1px solid var(--border);
        border-radius: 8px;
        background: #111827;
        color: #fff;
        padding: 10px 12px;
        font-size: 13px;
        box-shadow: 0 10px 30px rgb(15 23 42 / 22%);
      }
      .hidden {
        display: none;
      }
      @media (max-width: 1000px) {
        .grid,
        .split {
          grid-template-columns: 1fr 1fr;
        }
      }
      @media (max-width: 700px) {
        main {
          padding: 14px;
        }
        .grid,
        .split {
          grid-template-columns: 1fr;
        }
        input {
          width: 100%;
        }
        .controls {
          width: 100%;
        }
        .controls > * {
          flex: 1;
        }
      }
    </style>
  </head>
  <body>
    <main>
      <div class="toolbar">
        <div>
          <h1>渠道质量看板</h1>
          <div class="muted">当前用户：${escapeHtml(context.username || '-')} · 模块：${escapeHtml(context.moduleId || '-')}</div>
        </div>
        <div class="controls">
          <input id="keyword" placeholder="搜索渠道名称、ID、Base URL" />
          <input id="model" placeholder="按模型过滤" />
          <select id="status">
            <option value="all">全部状态</option>
            <option value="enabled">已启用</option>
            <option value="disabled">已禁用</option>
          </select>
          <button id="refresh" class="primary">刷新</button>
        </div>
      </div>

      <section class="grid" id="stats"></section>

      <section class="split">
        <div class="card panel">
          <div class="panel-title">
            <span>质量分布</span>
            <span class="muted" id="summaryText">-</span>
          </div>
          <div class="bars" id="qualityBars"></div>
        </div>
        <div class="card panel">
          <div class="panel-title">
            <span>类型分布</span>
            <span class="muted">按渠道数量</span>
          </div>
          <div class="bars" id="typeBars"></div>
        </div>
      </section>

      <section class="card">
        <div class="panel-title panel">
          <span>需要关注的渠道</span>
          <span class="muted" id="tableHint">-</span>
        </div>
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>渠道</th>
                <th>状态</th>
                <th>类型</th>
                <th>响应</th>
                <th>余额</th>
                <th>已用额度</th>
                <th>上次测试</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody id="channelRows"></tbody>
          </table>
        </div>
      </section>

      <div id="empty" class="notice hidden">没有匹配的渠道。</div>
    </main>
    <div id="toast" class="toast hidden"></div>

    <script>
      const channelTypes = {
        0: 'Unknown',
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
        15: 'Baidu',
        16: 'Zhipu',
        17: 'Ali',
        18: 'Xunfei',
        19: '360',
        20: 'OpenRouter',
        21: 'AIProxyLibrary',
        22: 'FastGPT',
        23: 'Tencent',
        24: 'Gemini',
        25: 'Moonshot',
        26: 'ZhipuV4',
        27: 'Perplexity',
        31: 'LingYiWanWu',
        33: 'AWS',
        34: 'Cohere',
        35: 'MiniMax',
        36: 'SunoAPI',
        37: 'Dify',
        38: 'Jina',
        39: 'Cloudflare',
        40: 'SiliconFlow',
        41: 'VertexAI',
        42: 'Mistral',
        43: 'DeepSeek',
        44: 'MokaAI',
        45: 'VolcEngine',
        46: 'BaiduV2',
        47: 'Xinference',
        48: 'xAI',
        49: 'Coze',
        50: 'Kling',
        51: 'Jimeng',
        52: 'Vidu',
        53: 'Submodel',
        54: 'DoubaoVideo',
        55: 'Sora',
        56: 'Replicate',
        57: 'Codex',
      }
      const state = {
        channels: [],
        loading: false,
        testing: new Set(),
      }

      const $ = (id) => document.getElementById(id)
      const escapeText = (value) =>
        String(value ?? '')
          .replaceAll('&', '&amp;')
          .replaceAll('<', '&lt;')
          .replaceAll('>', '&gt;')
          .replaceAll('"', '&quot;')
          .replaceAll("'", '&#039;')

      function showToast(message) {
        const el = $('toast')
        el.textContent = message
        el.classList.remove('hidden')
        clearTimeout(showToast.timer)
        showToast.timer = setTimeout(() => el.classList.add('hidden'), 3600)
      }

      async function apiGet(path) {
        const res = await fetch(path, {
          credentials: 'include',
          headers: { Accept: 'application/json' },
        })
        if (!res.ok) throw new Error('HTTP ' + res.status)
        const body = await res.json()
        if (!body.success) throw new Error(body.message || '请求失败')
        return body.data
      }

      function readFilters() {
        return {
          keyword: $('keyword').value.trim(),
          model: $('model').value.trim(),
          status: $('status').value,
        }
      }

      async function loadChannels() {
        const button = $('refresh')
        const filters = readFilters()
        const params = new URLSearchParams({
          keyword: filters.keyword,
          group: '',
          model: filters.model,
          id_sort: 'true',
          tag_mode: 'false',
          p: '1',
          page_size: '100',
          sort_by: 'response_time',
          sort_order: 'desc',
        })
        if (filters.status !== 'all') params.set('status', filters.status)

        state.loading = true
        button.disabled = true
        button.textContent = '加载中'
        try {
          let page = 1
          let total = 0
          const items = []
          do {
            params.set('p', String(page))
            const data = await apiGet('/api/channel/search?' + params.toString())
            const pageItems = Array.isArray(data.items) ? data.items : []
            total = Number(data.total || pageItems.length)
            items.push(...pageItems)
            if (pageItems.length === 0) break
            page += 1
          } while (items.length < total && page <= 200)

          state.channels = items
          render()
        } catch (error) {
          showToast(error.message || '渠道加载失败')
        } finally {
          state.loading = false
          button.disabled = false
          button.textContent = '刷新'
        }
      }

      function getStatusInfo(status) {
        if (status === 1) return { text: '启用', className: 'success' }
        if (status === 2) return { text: '手动禁用', className: 'warning' }
        if (status === 3) return { text: '自动禁用', className: 'danger' }
        return { text: '禁用', className: 'warning' }
      }

      function getQuality(channel) {
        if (channel.status !== 1) {
          return { key: 'disabled', text: '禁用', className: 'warning', score: 4 }
        }
        if (channel.balance > 0 && channel.balance < 1) {
          return { key: 'lowBalance', text: '低余额', className: 'danger', score: 3 }
        }
        if (!channel.response_time || channel.response_time <= 0) {
          return { key: 'untested', text: '未测试', className: '', score: 2 }
        }
        if (channel.response_time > 8000) {
          return { key: 'slow', text: '慢响应', className: 'warning', score: 1 }
        }
        return { key: 'healthy', text: '健康', className: 'success', score: 0 }
      }

      function formatResponseTime(ms) {
        const value = Number(ms || 0)
        if (!value) return '未测试'
        if (value < 1000) return value + 'ms'
        return (value / 1000).toFixed(2) + 's'
      }

      function formatBalance(value) {
        const n = Number(value || 0)
        if (!Number.isFinite(n)) return '-'
        return '$' + n.toLocaleString(undefined, {
          minimumFractionDigits: n < 10 && n > 0 ? 4 : 2,
          maximumFractionDigits: n < 10 && n > 0 ? 4 : 2,
        })
      }

      function formatQuota(value) {
        const n = Number(value || 0)
        if (!Number.isFinite(n) || n <= 0) return '0'
        if (n >= 1000000000) return (n / 1000000000).toFixed(2) + 'B'
        if (n >= 1000000) return (n / 1000000).toFixed(2) + 'M'
        if (n >= 1000) return (n / 1000).toFixed(2) + 'K'
        return String(n)
      }

      function formatTime(ts) {
        const value = Number(ts || 0)
        if (!value) return '从未'
        const diff = Math.max(0, Date.now() - value * 1000)
        const minutes = Math.floor(diff / 60000)
        if (minutes < 1) return '刚刚'
        if (minutes < 60) return minutes + '分钟前'
        const hours = Math.floor(minutes / 60)
        if (hours < 24) return hours + '小时前'
        const days = Math.floor(hours / 24)
        return days + '天前'
      }

      function buildStats(channels) {
        const stats = {
          total: channels.length,
          enabled: 0,
          disabled: 0,
          lowBalance: 0,
          untested: 0,
          slow: 0,
          avgMs: 0,
          usedQuota: 0,
          balance: 0,
        }
        let timedCount = 0
        for (const channel of channels) {
          if (channel.status === 1) stats.enabled += 1
          else stats.disabled += 1
          if (channel.balance > 0 && channel.balance < 1) stats.lowBalance += 1
          if (!channel.response_time) stats.untested += 1
          if (channel.response_time > 8000) stats.slow += 1
          if (channel.response_time > 0) {
            stats.avgMs += channel.response_time
            timedCount += 1
          }
          stats.usedQuota += Number(channel.used_quota || 0)
          stats.balance += Number(channel.balance || 0)
        }
        stats.avgMs = timedCount ? Math.round(stats.avgMs / timedCount) : 0
        return stats
      }

      function renderStats(stats) {
        const items = [
          ['渠道总数', stats.total, '当前筛选结果'],
          ['启用渠道', stats.enabled, '可被路由使用'],
          ['平均响应', formatResponseTime(stats.avgMs), '最近测试结果'],
          ['低余额/异常', stats.lowBalance + stats.disabled + stats.slow, '建议优先处理'],
        ]
        $('stats').innerHTML = items
          .map(
            ([label, value, hint]) => '<div class="card stat"><div class="stat-label">' +
              escapeText(label) +
              '</div><div class="stat-value">' +
              escapeText(value) +
              '</div><div class="muted">' +
              escapeText(hint) +
              '</div></div>'
          )
          .join('')
      }

      function renderBars(id, entries, total) {
        const max = Math.max(1, total)
        $(id).innerHTML = entries
          .filter((entry) => entry.count > 0)
          .slice(0, 8)
          .map((entry) => {
            const width = Math.max(4, Math.round((entry.count / max) * 100))
            return '<div class="bar-row"><span>' +
              escapeText(entry.label) +
              '</span><div class="bar-track"><div class="bar-fill" style="width:' +
              width +
              '%"></div></div><span class="muted">' +
              entry.count +
              '</span></div>'
          })
          .join('') || '<div class="muted">暂无数据</div>'
      }

      function renderQuality(channels) {
        const qualityMap = new Map([
          ['healthy', { label: '健康', count: 0 }],
          ['slow', { label: '慢响应', count: 0 }],
          ['untested', { label: '未测试', count: 0 }],
          ['lowBalance', { label: '低余额', count: 0 }],
          ['disabled', { label: '禁用', count: 0 }],
        ])
        const typeMap = new Map()
        for (const channel of channels) {
          const quality = getQuality(channel)
          qualityMap.get(quality.key).count += 1
          const typeName = channelTypes[channel.type] || 'Unknown'
          typeMap.set(typeName, (typeMap.get(typeName) || 0) + 1)
        }
        renderBars(
          'qualityBars',
          Array.from(qualityMap.values()),
          channels.length
        )
        renderBars(
          'typeBars',
          Array.from(typeMap.entries())
            .map(([label, count]) => ({ label, count }))
            .sort((a, b) => b.count - a.count),
          channels.length
        )
        $('summaryText').textContent = channels.length ? '覆盖 ' + channels.length + ' 个渠道' : '-'
      }

      function pickAttentionRows(channels) {
        return [...channels]
          .sort((a, b) => {
            const qa = getQuality(a)
            const qb = getQuality(b)
            if (qa.score !== qb.score) return qb.score - qa.score
            return Number(b.response_time || 0) - Number(a.response_time || 0)
          })
          .filter((channel) => getQuality(channel).score > 0)
          .slice(0, 30)
      }

      async function testChannel(id, model) {
        state.testing.add(id)
        render()
        const params = new URLSearchParams()
        if (model) params.set('model', model)
        try {
          const data = await fetch('/api/channel/test/' + encodeURIComponent(id) + '?' + params.toString(), {
            credentials: 'include',
            headers: { Accept: 'application/json' },
          }).then((res) => res.json())
          if (!data.success) throw new Error(data.message || '测试失败')
          showToast('渠道 #' + id + ' 测试成功，耗时 ' + Number(data.time || 0).toFixed(2) + ' 秒')
          await loadChannels()
        } catch (error) {
          showToast('渠道 #' + id + ' ' + (error.message || '测试失败'))
        } finally {
          state.testing.delete(id)
          render()
        }
      }

      function firstModel(channel) {
        return String(channel.models || '')
          .split(',')
          .map((item) => item.trim())
          .filter(Boolean)[0] || ''
      }

      function renderRows(channels) {
        const rows = pickAttentionRows(channels)
        $('tableHint').textContent = rows.length ? '显示前 ' + rows.length + ' 个' : '暂无需关注项'
        $('empty').classList.toggle('hidden', channels.length > 0)
        $('channelRows').innerHTML = rows
          .map((channel) => {
            const status = getStatusInfo(channel.status)
            const quality = getQuality(channel)
            const testing = state.testing.has(channel.id)
            const model = firstModel(channel)
            return '<tr><td class="name-cell"><div class="name-main">' +
              escapeText(channel.name || '-') +
              '</div><div class="name-sub">#' +
              escapeText(channel.id) +
              ' · ' +
              escapeText(channel.group || '-') +
              (channel.tag ? ' · ' + escapeText(channel.tag) : '') +
              '</div></td><td><span class="badge ' +
              status.className +
              '">' +
              status.text +
              '</span></td><td>' +
              escapeText(channelTypes[channel.type] || 'Unknown') +
              '</td><td><span class="badge ' +
              quality.className +
              '">' +
              formatResponseTime(channel.response_time) +
              '</span></td><td>' +
              formatBalance(channel.balance) +
              '</td><td>' +
              formatQuota(channel.used_quota) +
              '</td><td>' +
              formatTime(channel.test_time) +
              '</td><td><button data-test-id="' +
              escapeText(channel.id) +
              '" data-model="' +
              escapeText(model) +
              '" ' +
              (testing ? 'disabled' : '') +
              '>' +
              (testing ? '测试中' : '测试') +
              '</button></td></tr>'
          })
          .join('') || '<tr><td colspan="8" class="muted">暂无需关注的渠道。</td></tr>'
      }

      function render() {
        const channels = state.channels
        const stats = buildStats(channels)
        renderStats(stats)
        renderQuality(channels)
        renderRows(channels)
      }

      $('refresh').addEventListener('click', loadChannels)
      $('status').addEventListener('change', loadChannels)
      $('keyword').addEventListener('keydown', (event) => {
        if (event.key === 'Enter') loadChannels()
      })
      $('model').addEventListener('keydown', (event) => {
        if (event.key === 'Enter') loadChannels()
      })
      $('channelRows').addEventListener('click', (event) => {
        const button = event.target.closest('button[data-test-id]')
        if (!button) return
        testChannel(button.dataset.testId, button.dataset.model || '')
      })

      loadChannels()
    </script>
  </body>
</html>`)
}

http
  .createServer((req, res) => {
    if (req.url === '/health') {
      sendJson(res, { ok: true })
      return
    }
    if (req.url === '/ui') {
      sendUi(res, req)
      return
    }
    sendJson(res, { path: req.url, context: readHostContext(req) })
  })
  .listen(port, '127.0.0.1', () => {
    console.log(`Channel quality extension listening on http://127.0.0.1:${port}`)
  })
