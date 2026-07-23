# NewAPI 扩展模块开发检查清单

## 模块结构

- 模块根目录直接包含 `manifest.json`。
- 默认必须是轻量静态模块。可信的一方页面默认包含 `public/compat.html` 和 `public/native/` 双前端入口；iframe 页面才以 `public/index.html` 为正式入口。
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
- `ui.pages[].path`：代理路径，必须以 `/` 开头；native 页面默认 `/compat.html`，iframe 静态页面默认 `/`。
- 可信的一方页面默认声明 `ui.pages[].render.type=native`、`sdk=v1`、Default/Classic 目标和 `permissions.capabilities=["ui.native"]`。

## UI 与导航

- `ui.nav[].page` 必须对应 `ui.pages[].key`。
- `ui.nav[].title` 是侧边栏显示文字。
- `ui.nav[].icon` 使用默认版前端支持的图标名，例如 `Puzzle`、`Bot`、`Settings`、`Key`。
- `permissions.roles` 按最小权限填写：`user`、`admin`、`root`。
- native 页面直接复用宿主组件；不得用 iframe 主题桥接冒充原生 UI。
- native 源码的每个外部导入都能在 Default/Classic SDK 白名单中找到；缺失能力已同步扩展双端契约，没有 deep import 或打包宿主依赖副本。
- iframe 仅用于仍受信任但需要独立 DOM、样式或框架运行时的第三方/遗留页面，以及远程 HTTP UI。
- iframe 不是安全沙箱；不可信模块禁止安装，不能通过 iframe 绕过可信模块边界。

## 轻量化原则

- 优先调用主程序 API，不在模块里复制用户、渠道、账单、权限等通用逻辑。
- 可信的一方后台 UI 优先 `native v1`；业务入口只打包业务代码，宿主 React 和组件库必须 externalize。
- 纯页面工具优先做成静态模块，后台任务再做成模块自己的 HTTP 接口。
- 密钥放环境变量或外部配置，不写入 `manifest.json`。
- 不写死主程序域名；调用接口使用 `/api/...` 站内绝对路径。
- 不依赖外部 CDN，否则网络失败会导致模块白屏。

## 页面可用性要求

- native 页面必须同时提供 Default 与 Classic 等价实现，并复用各自宿主组件。
- iframe 页面的 HTML body 必须默认渲染标题、工具栏、加载态或占位内容。
- 所有异步请求必须 `try/catch`，失败时显示错误块和重试按钮。
- 空数据必须显示空态，不能只留下空表格或空白区域。
- 静态模块需要登录用户时调用 `GET /api/extensions/host/me`。
- 调用 `/api/channel`、`/api/option` 等鉴权 API 时必须带 `New-Api-User` 请求头；从 `localStorage.uid` 或 `localStorage.user.id` 读取。
- 页面脚本出错也应尽量保留首屏 HTML，避免完全白屏。
- 可见文案接入宿主国际化，数字、日期和相对时间跟随当前语言。
- 浏览器验证覆盖 Default/Classic、亮色/暗色、桌面/移动端；native 页面断言 iframe 为 0、原生宿主容器为 1、无页面级水平溢出。

## 打包前自查

- `manifest.version` 已按本次改动递增。
- `static` 模块的 `runtime.static_dir` 存在，且包含 manifest 声明的全部运行资源。
- native 模块包含兼容页、Default/Classic 的 entry 与 styles，且构建源码和宿主依赖未进入 zip。
- iframe 模块的 `public/index.html` 能看到标题、加载态、错误态和重试按钮。
- `http` 模块的 `runtime.base_url` 与实际启动端口一致。
- `http` 模块服务能响应 `runtime.health_path`。
- zip 根目录能直接看到 `manifest.json`。
- 包体明显偏大时先解包排查重型目录。
