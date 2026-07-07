# NewAPI 扩展模块开发检查清单

## 模块结构

- 模块根目录直接包含 `manifest.json`。
- 默认必须是轻量静态模块，并包含 `public/index.html`。
- HTTP 模块只用于后台任务、长连接、队列或用户明确要求独立服务的场景。
- HTTP 模块包含 `server.mjs` 或等价入口文件，并必须提供启动命令与健康检查。
- 按需包含 `public/`、`views/`、`config.example.json`。
- 不把 `node_modules/`、`dist/`、`build/`、`.git/`、日志、数据库、`.env`、私钥和历史 zip 包放进模块包。

## manifest 必填项

- `id`：稳定唯一标识，发布后不要随意变更。
- `name`：后台显示名称。
- `version`：语义化版本。
- `runtime.type`：页面/看板/工具模块必须为 `static`。
- `runtime.static_dir`：静态资源目录，默认 `public`。
- `runtime.base_url`：仅 HTTP 模块填写，服务地址只支持 `http` 或 `https`。
- `ui.pages[].key`：页面唯一键。
- `ui.pages[].path`：代理路径，必须以 `/` 开头；静态入口推荐 `/`。

## UI 与导航

- `ui.nav[].page` 必须对应 `ui.pages[].key`。
- `ui.nav[].title` 是侧边栏显示文字。
- `ui.nav[].icon` 使用默认版前端支持的图标名，例如 `Puzzle`、`Bot`、`Settings`、`Key`。
- `permissions.roles` 按最小权限填写：`user`、`admin`、`root`。

## 轻量化原则

- 优先调用主程序 API，不在模块里复制用户、渠道、账单、权限等通用逻辑。
- UI 优先原生 HTML/CSS/JS，复杂交互再考虑引入小型依赖。
- 纯页面工具优先做成静态模块，后台任务再做成模块自己的 HTTP 接口。
- 密钥放环境变量或外部配置，不写入 `manifest.json`。
- 不写死主程序域名；调用接口使用 `/api/...` 站内绝对路径。
- 不依赖外部 CDN，否则网络失败会导致 iframe 白屏。

## 页面可用性要求

- HTML body 必须默认渲染标题、工具栏、加载态或占位内容。
- 所有异步请求必须 `try/catch`，失败时显示错误块和重试按钮。
- 空数据必须显示空态，不能只留下空表格或空白区域。
- 静态模块需要登录用户时调用 `GET /api/extensions/host/me`。
- 调用 `/api/channel`、`/api/option` 等鉴权 API 时必须带 `New-Api-User` 请求头；从 `localStorage.uid` 或 `localStorage.user.id` 读取。
- 页面脚本出错也应尽量保留首屏 HTML，避免完全白屏。

## 打包前自查

- `manifest.version` 已按本次改动递增。
- `static` 模块的 `runtime.static_dir` 存在，且包含入口页面。
- `static` 模块的 `public/index.html` 能看到标题、加载态、错误态和重试按钮。
- `http` 模块的 `runtime.base_url` 与实际启动端口一致。
- `http` 模块服务能响应 `runtime.health_path`。
- zip 根目录能直接看到 `manifest.json`。
- 包体明显偏大时先解包排查重型目录。
