---
name: newapi-extension-module-workflow
description: 在 new-api 项目中流程化创建、改造、排错和轻量化打包扩展模块。用户提到“写模块”“扩展模块”“模块管理”“manifest.json”“上传模块”“轻量模块”“package-extension-lite.ps1”“自动注册入库模块”等场景时使用。
---

# NewAPI Extension Module Workflow

## 目标

- 默认产出可上传到 **扩展模块 -> 模块管理** 的轻量 `.zip` 模块包。
- 优先复用主程序现有 API、鉴权、用户上下文、宿主原生 UI 和代理能力。
- 避免把完整前端工程、`node_modules`、测试产物和运行时依赖打进模块包。
- 默认产出 **上传即用的 static 模块**。除非用户明确要求后台进程、长连接、队列或独立服务，否则不要创建 `http` 模块。
- 模块页面必须有可见的加载态、空态和错误态；任何接口失败都要在页面内显示错误，不能让原生页面或 iframe 白屏。

## 流程决策

1. 新增模块：执行“步骤 1 -> 步骤 2 -> 步骤 3 -> 步骤 4”。
2. 修改已有模块：执行“步骤 1 -> 步骤 2（只改动相关范围）-> 步骤 3 -> 步骤 4”。
3. 仅打包：执行“步骤 1（最小检查）-> 步骤 3 -> 步骤 4”。
4. 只要本次任务改动了模块目录内的 `manifest.json`、服务脚本、静态页面或配置，交付前默认执行一次轻量化打包。
5. 如果模块只是页面、看板、批量操作面板、调用主程序 API 的工具，必须做成 `static`；不得因为示例里有 `server.mjs` 就套用 `http`。
6. 可信的一方页面、看板和后台工具默认使用 `native v1`，让业务入口在宿主 DOM 中复用 Default/Classic 的真实组件。只有仍受信任但需要独立 DOM、样式或框架运行时的第三方/遗留页面，以及远程 `http` UI 才使用 iframe；iframe 不是安全沙箱，不可信模块禁止安装。不得把 iframe 主题变量桥接描述成原生 UI。

## 步骤 1：收集上下文

1. 优先读取：
   - `docs/developer/extensions.md`
   - `scripts/package-extension-lite.ps1`
   - 页面模块读取 `examples/extensions/channel-quality/manifest.json` 和 `scripts/build-channel-quality-native.mjs`。
   - native 页面同时读取 `web/default/src/features/extensions/native-sdk.ts` 和 `web/classic/src/pages/Extensions/native-sdk.js`，确认所需模块已在两套 SDK 白名单中。
   - 只有需要独立服务时才读取 `examples/extensions/echo/manifest.json`。
2. 定位目标模块：
   - 使用 `rg --files examples data modules . | rg "manifest\.json$"`。
   - 模块根目录必须直接包含 `manifest.json`。
3. 对齐宿主边界：
   - 运行时支持 `static` 和 `http`。
   - `static` 模块由主程序托管模块目录里的静态文件，适合纯页面和调用主程序 API 的轻量工具。
   - `http` 模块由模块自己的 HTTP 服务承载，适合后台任务、队列、长连接或独立运行时逻辑。
   - 主程序负责：登录态、角色过滤、侧边栏入口、`native v1` 原生渲染槽、iframe 隔离、代理转发和用户上下文接口。
   - 模块负责：少量业务逻辑和必要的 UI 页面。

## 步骤 2：实现模块

1. 新模块默认做 `static` 轻量模块：
   - `runtime.type` 必须为 `static`。
   - `runtime.static_dir` 默认 `public`。
   - 可信的一方页面模块默认声明 `render.type=native`、`render.sdk=v1` 和 `ui.native`，同时提供 Default 与 Classic 入口。
   - 原生页面的 `ui.pages[].path` 使用 `/compat.html`，对应文件为 `public/compat.html`；只有 iframe 静态页面才默认使用 `public/index.html` 和 `/`。
2. 只有满足以下任一条件时，才允许做 `http` 模块：
   - 模块必须常驻后台任务、队列、定时任务或长连接。
   - 模块必须保存自己的服务端状态，且无法复用主程序 API。
   - 用户明确要求独立运行时。
   选择 `http` 时，交付说明必须写清楚启动命令、端口、健康检查地址，并实际验证 `runtime.health_path` 可访问。
3. `manifest.json` 必须和模块服务一致：
   - `id` 稳定唯一，只用字母、数字、短横线、下划线。
   - `runtime.type=static` 时设置 `runtime.static_dir`，默认 `public`。
   - `runtime.type=http` 时 `runtime.base_url` 指向模块 HTTP 服务。
   - `ui.nav[].page` 必须能在 `ui.pages[].key` 找到。
   - `ui.pages[].path` 必须以 `/` 开头。
   - `permissions.roles` 明确最小角色范围。
4. 页面渲染方式选择：
   - 默认：可信的一方 `static` 页面使用 `native v1`。
   - iframe 仅用于仍受信任但需要独立 DOM、样式或框架运行时的第三方/遗留页面、远程 `http` UI 或外部系统兼容场景。
   - iframe 只提供页面结构和运行时隔离，不是安全沙箱；不可信代码不得通过 static、http 或 iframe 模块安装。
   - `embed: false` 仅用于确实需要新标签页完成的外部工作流，不作为规避原生适配或承载不可信代码的手段。
5. 通用页面要求：
   - 所有异步请求必须捕获错误，失败时显示错误块和重试按钮。
   - 必须提供加载态、空态和错误态，动态内容不得导致页面布局跳动或溢出。
   - 调主程序 API 使用绝对站内路径，例如 `/api/channel/search`，不要写死域名。
   - 调需要 `UserAuth` / `AdminAuth` / `RootAuth` 的主程序 API 时，必须带 `New-Api-User` 请求头；优先从 `localStorage.uid` 读取，缺失时从 `localStorage.user.id` 回退。
   - 需要当前用户信息时，静态模块调用 `GET /api/extensions/host/me`。
   - 可见文案必须接入宿主国际化；数字、日期和相对时间跟随当前语言。
6. 原生页面要求：
   - 使用宿主 SDK 暴露的 React、请求、国际化、图表和 UI 组件，不复制宿主依赖，也不自行实现一套相似控件。
   - 遵循宿主页面密度、间距、卡片、表格、筛选器和响应式模式；不重复绘制宿主页头，不在卡片里嵌套装饰性卡片。
   - Default 和 Classic 必须功能等价，并分别验证亮色、暗色、桌面和移动端。
7. iframe 页面要求：
   - HTML body 必须默认可见，首屏不能依赖接口成功后才渲染。
   - `embed: true` 页面优先使用宿主主题桥接的 `--page-bg`、`--surface`、`--surface-muted`、`--border`、`--text`、`--muted`、`--primary`、`--green`、`--amber`、`--red`、`--radius` 和 `--host-font-family` 等语义变量，并为独立打开提供回退值。
   - 监听 `new-api-host-theme` 消息时必须校验 `event.origin === window.location.origin`；`embedded: true` 可用于隐藏宿主已经显示的重复页头。
   - 页面中不得使用未打包的外部 CDN 资源，避免离线或网络失败导致白屏。
8. 默认做轻量模块：
   - 原生入口只打包业务组件和状态逻辑，React、ReactDOM、Semi、Base UI 等宿主依赖必须 externalize 到 `native v1` SDK。
   - iframe 页面不引入 React/Vite/Next 等完整构建链，除非兼容场景确实需要独立前端运行时。
   - 调主程序接口时优先使用主程序已有 API，不复制用户管理、账单、渠道等逻辑。
   - 模块包不携带数据库、浏览器驱动、模型文件、`node_modules`、构建缓存。
   - 仅构建期需要的文件放在 `native-src/`，或用模块根目录 `.extensionignore` 按精确相对路径排除；忽略规则不得使用绝对路径、`..` 或通配符。
9. 若模块需要当前登录用户信息：
   - 读取主程序代理注入的请求头：`X-NewAPI-User-ID`、`X-NewAPI-Username`、`X-NewAPI-User-Role`、`X-NewAPI-User-Group`。
   - 或在前端页面调用 `GET /api/extensions/host/me`。
10. 若模块需要高权限调用主程序：
   - 使用单独服务账号 Access Token。
   - 不复用 root token，不把密钥写进 `manifest.json`。

### Native v1 原生页面

可信的一方页面模块默认采用本模式。原生页面仍然是可上传、可启停、可卸载的 `static` 模块，但需要额外声明：

```json
{
  "ui": {
    "pages": [
      {
        "key": "index",
        "path": "/compat.html",
        "embed": true,
        "render": {
          "type": "native",
          "sdk": "v1",
          "targets": {
            "default": {
              "entry": "/native/default.mjs",
              "styles": ["/native/default.css"]
            },
            "classic": {
              "entry": "/native/classic.mjs",
              "styles": ["/native/classic.css"]
            }
          }
        }
      }
    ]
  },
  "permissions": {
    "capabilities": ["ui.native"]
  }
}
```

- 只能用于可信的本地 `static` 模块，不能从 `http` 模块加载远程原生代码。
- 必须同时提供 Default 与 Classic 入口；入口默认导出 React 页面组件。
- 入口只能依赖 `native v1` 宿主 SDK，不得打包 React、ReactDOM、Semi、Base UI 或宿主令牌。
- 原生源码只能导入两套 SDK 白名单已经暴露的模块路径。缺少组件时，先把通用能力同步加入 Default/Classic SDK 并验证双端构建；不得 deep import 宿主内部文件，也不得把依赖副本打进模块。
- `path` 指向可见的旧宿主兼容页；旧版本忽略 `render` 后必须显示升级提示，不能白屏。
- 构建源码可放在 `native-src/`；轻量打包脚本会排除该目录，只保留 `public/native/` 运行产物。
- Default 模块样式可提取宿主 `@theme inline` 语义映射生成所需工具类，但不得打包 Tailwind Preflight、全局元素重置或宿主主题值；不能依赖宿主碰巧生成同名工具类。
- 宿主会基于所有声明的原生资源计算 `asset_revision` 并用于入口和样式缓存失效；不得自行拼接固定版本 URL 绕过宿主加载器。
- 修改 native 源码后必须先重新构建入口，再执行轻量打包，并在 Default/Classic、亮暗模式和移动端分别验证。
- 浏览器验收必须确认正式页面不存在 iframe、`data-extension-native-host` 已出现、入口和样式请求携带 `asset_revision`，并检查页面级水平溢出与控制台错误。

## 步骤 3：轻量化打包

在仓库根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "<module-dir>"
```

产物默认输出：

```text
artifacts/extensions/<moduleId>-<version>.zip
```

默认打包策略：

- 包根目录直接放 `manifest.json`。
- 保留运行需要的文件。
- 排除 `node_modules/`、`dist/`、`build/`、`.git/`、缓存、日志、临时库、测试覆盖率和已有压缩包。
- native 模块打包前必须验证 Default/Classic 的 `entry` 与 `styles` 均已进入临时打包目录，缺少运行资源时不得生成 zip。
- 使用 `manifest.id` 与 `manifest.version` 命名产物。

## 步骤 4：校验产物

1. 确认 zip 存在：`artifacts/extensions/<moduleId>-<version>.zip`。
2. 解包结构必须满足：
   - 根目录有 `manifest.json`。
   - native static 模块必须有兼容页和 `public/native/` 中声明的全部资源，建议让 `public/index.html` 同样显示兼容提示。
   - iframe static 模块必须有 `public/index.html` 或 manifest 指向的其他正式页面。
   - `http` 模块才允许只提供 `server.mjs`。
   - 不包含 `node_modules/`、`dist/`、`.git/` 等重型目录。
3. 对 `manifest.json` 做最终检查：
   - 页面模块应是 `runtime.type=static`。
   - `ui.pages[].path` 和实际入口一致；native 默认 `/compat.html`，iframe static 默认 `/`。
   - `ui.nav[].page` 对应存在的 `ui.pages[].key`。
   - native 页面声明 `ui.native`，`sdk=v1`，且 Default/Classic 的入口与样式文件都在包内。
4. 原生页面检查兼容页、双前端入口、样式、加载态、空态、错误态和重试；iframe 页面检查首屏 HTML 与主题桥接。
5. 若包体超过 1 MiB，先检查是否误打进依赖、构建产物、截图、日志或数据库。

## 步骤 5：交付说明

1. 列出模块目录、产物路径、版本号。
2. 列出核心行为变化。
3. 列出执行过的验证命令。
4. 如果用户明确只要求“打包/编译模块”，最终回复必须包含产物绝对路径。

## 参考文件

- 检查清单：`references/module-development-checklist.md`
- 轻量化打包：`references/lightweight-packaging.md`
- HTTP 模块模板：`assets/http-module-template/`

## 共享通知中心

需要让模块向通知中心发布业务事件时：

1. 在 manifest 中声明 permissions.capabilities 为 notification.events.publish。
2. 在 notifications.events 中声明事件 ID、显示名称、默认模板和变量类型。
3. 使用 /api/extensions/<module-id>/notification-events 提交 event_type、稳定 event_key 和白名单 payload。
4. 让通知中心统一持有 Bot Token、Chat ID、提及、模板、重试和历史；模块不得直接调用 Telegram。
5. HTTP 模块后台使用独立服务账号 Access Token 加 New-Api-User。static 模块不得把服务账号令牌打进前端，只能使用当前会话。
6. 事件类型由宿主规范化为 extension.<module-id>.<event-id>；重复 event_key 不会再次通知。

模块代理目前只接受已登录浏览器会话；/api/extensions/host/me 用于 static 页面获取用户上下文。HTTP 代理会注入 X-NewAPI-* 头，并剥离 Cookie、Authorization 和 Proxy-Authorization。

完整契约见 docs/developer/notifications.md，二开能力索引见 docs/developer/custom-development.md。
