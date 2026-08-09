import http from 'node:http'

const port = Number(process.env.PORT || 39001)

function sendJson(res, value) {
  res.writeHead(200, {
    'Content-Type': 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
  })
  res.end(JSON.stringify(value, null, 2))
}

function sendHtml(res, req) {
  const headers = {
    module: req.headers['x-newapi-module-id'] || '',
    userId: req.headers['x-newapi-user-id'] || '',
    username: req.headers['x-newapi-username'] || '',
    role: req.headers['x-newapi-user-role'] || '',
    group: req.headers['x-newapi-user-group'] || '',
  }

  res.writeHead(200, {
    'Content-Type': 'text/html; charset=utf-8',
    'Cache-Control': 'no-store',
  })
  res.end(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Echo Extension</title>
    <style>
      body {
        margin: 0;
        font-family: Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        background: #f8fafc;
        color: #0f172a;
      }
      main {
        padding: 24px;
      }
      section {
        max-width: 720px;
        border: 1px solid #e2e8f0;
        border-radius: 8px;
        background: #fff;
        padding: 20px;
      }
      h1 {
        margin: 0 0 12px;
        font-size: 20px;
      }
      pre {
        overflow: auto;
        border-radius: 8px;
        background: #0f172a;
        color: #e2e8f0;
        padding: 16px;
      }
    </style>
  </head>
  <body>
    <main>
      <section>
        <h1>Echo Extension</h1>
        <p>这些字段来自主程序代理注入的用户上下文请求头。</p>
        <pre>${escapeHtml(JSON.stringify(headers, null, 2))}</pre>
      </section>
    </main>
  </body>
</html>`)
}

function escapeHtml(value) {
  return value
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
      sendHtml(res, req)
      return
    }
    sendJson(res, {
      path: req.url,
      headers: req.headers,
    })
  })
  .listen(port, '127.0.0.1', () => {
    console.log(`Echo extension listening on http://127.0.0.1:${port}`)
  })
