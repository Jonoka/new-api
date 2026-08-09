import http from 'node:http'

const port = Number(process.env.PORT || {{port}})

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
    <title>{{module_name}}</title>
    <style>
      body { margin: 0; font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #f8fafc; color: #0f172a; }
      main { padding: 24px; }
      section { max-width: 760px; border: 1px solid #e2e8f0; border-radius: 8px; background: #fff; padding: 20px; }
      h1 { margin: 0 0 12px; font-size: 20px; }
      pre { overflow: auto; border-radius: 8px; background: #0f172a; color: #e2e8f0; padding: 16px; }
      button { height: 32px; border: 1px solid #cbd5e1; border-radius: 6px; background: #fff; cursor: pointer; }
    </style>
  </head>
  <body>
    <main>
      <section>
        <h1>{{module_name}}</h1>
        <p>这里放模块的轻量页面。优先调用主程序已有 API，不重复实现通用能力。</p>
        <pre>${escapeHtml(JSON.stringify(context, null, 2))}</pre>
      </section>
    </main>
  </body>
</html>`)
}

function escapeHtml(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#039;')
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
    console.log(`{{module_name}} listening on http://127.0.0.1:${port}`)
  })
