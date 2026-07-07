---
name: newapi-extension-module-workflow
description: 在 new-api 项目中流程化创建、改造、排错和轻量化打包扩展模块。用户提到“写模块”“扩展模块”“模块管理”“manifest.json”“上传模块”“轻量模块”“package-extension-lite.ps1”“自动注册入库模块”等场景时使用。
---

# NewAPI Extension Module Workflow

## 目标

- 默认产出可上传到 **扩展模块 -> 模块管理** 的轻量 `.zip` 模块包。
- 优先复用主程序现有 API、鉴权、用户上下文、后台页面嵌入和代理能力。
- 避免把完整前端工程、`node_modules`、测试产物和运行时依赖打进模块包。
- 默认产出 **上传即用的 static 模块**。除非用户明确要求后台进程、长连接、队列或独立服务，否则不要创建 `http` 模块。
- 模块页面必须有可见的加载态、空态和错误态；任何接口失败都要在页面内显示错误，不能让 iframe 白屏。

## 流程决策

1. 新增模块：执行“步骤 1 -> 步骤 2 -> 步骤 3 -> 步骤 4”。
2. 修改已有模块：执行“步骤 1 -> 步骤 2（只改动相关范围）-> 步骤 3 -> 步骤 4”。
3. 仅打包：执行“步骤 1（最小检查）-> 步骤 3 -> 步骤 4”。
4. 只要本次任务改动了模块目录内的 `manifest.json`、服务脚本、静态页面或配置，交付前默认执行一次轻量化打包。
5. 如果模块只是页面、看板、批量操作面板、调用主程序 API 的工具，必须做成 `static`；不得因为示例里有 `server.mjs` 就套用 `http`。

## 步骤 1：收集上下文

1. 优先读取：
   - `docs/developer/extensions.md`
   - `scripts/package-extension-lite.ps1`
   - `examples/extensions/echo/manifest.json`
2. 定位目标模块：
   - 使用 `rg --files examples data modules . | rg "manifest\.json$"`。
   - 模块根目录必须直接包含 `manifest.json`。
3. 对齐宿主边界：
   - 运行时支持 `static` 和 `http`。
   - `static` 模块由主程序托管模块目录里的静态文件，适合纯页面和调用主程序 API 的轻量工具。
   - `http` 模块由模块自己的 HTTP 服务承载，适合后台任务、队列、长连接或独立运行时逻辑。
   - 主程序负责：登录态、角色过滤、侧边栏入口、iframe 嵌入、代理转发和用户上下文接口。
   - 模块负责：少量业务逻辑和必要的 UI 页面。

## 步骤 2：实现模块

1. 新模块默认做 `static` 轻量模块：
   - `runtime.type` 必须为 `static`。
   - `runtime.static_dir` 默认 `public`。
   - 页面入口默认 `public/index.html`。
   - `ui.pages[].path` 推荐使用 `/`，避免静态模块路径和入口文件不一致。
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
4. 页面实现要求：
   - HTML body 必须默认可见，首屏不能依赖接口成功后才渲染。
   - 所有 `fetch` 必须 `try/catch`，失败时显示错误块和重试按钮。
   - 调主程序 API 使用绝对站内路径，例如 `/api/channel/search`，不要写死域名。
   - 调需要 `UserAuth` / `AdminAuth` / `RootAuth` 的主程序 API 时，必须带 `New-Api-User` 请求头；优先从 `localStorage.uid` 读取，缺失时从 `localStorage.user.id` 回退。
   - 需要当前用户信息时，静态模块调用 `GET /api/extensions/host/me`。
   - 页面中不得使用未打包的外部 CDN 资源，避免离线或网络失败导致白屏。
5. 默认做轻量模块：
   - 不引入 React/Vite/Next 等完整构建链，除非用户明确需要复杂前端。
   - UI 优先使用原生 HTML/CSS/JS，或由主程序页面/API 完成通用能力。
   - 调主程序接口时优先使用主程序已有 API，不复制用户管理、账单、渠道等逻辑。
   - 模块包不携带数据库、浏览器驱动、模型文件、`node_modules`、构建缓存。
6. 若模块需要当前登录用户信息：
   - 读取主程序代理注入的请求头：`X-NewAPI-User-ID`、`X-NewAPI-Username`、`X-NewAPI-User-Role`、`X-NewAPI-User-Group`。
   - 或在前端页面调用 `GET /api/extensions/host/me`。
7. 若模块需要高权限调用主程序：
   - 使用单独服务账号 Access Token。
   - 不复用 root token，不把密钥写进 `manifest.json`。

## 步骤 3：轻量化打包

在仓库根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "examples/extensions/echo"
```

产物默认输出：

```text
artifacts/extensions/<moduleId>-<version>.zip
```

默认打包策略：

- 包根目录直接放 `manifest.json`。
- 保留运行需要的文件。
- 排除 `node_modules/`、`dist/`、`build/`、`.git/`、缓存、日志、临时库、测试覆盖率和已有压缩包。
- 使用 `manifest.id` 与 `manifest.version` 命名产物。

## 步骤 4：校验产物

1. 确认 zip 存在：`artifacts/extensions/<moduleId>-<version>.zip`。
2. 解包结构必须满足：
   - 根目录有 `manifest.json`。
   - `static` 模块必须有 `public/index.html`。
   - `http` 模块才允许只提供 `server.mjs`。
   - 不包含 `node_modules/`、`dist/`、`.git/` 等重型目录。
3. 对 `manifest.json` 做最终检查：
   - 页面模块应是 `runtime.type=static`。
   - `ui.pages[].path` 和实际入口一致，默认 `/`。
   - `ui.nav[].page` 对应存在的 `ui.pages[].key`。
4. 静态页面必须打开源码检查首屏元素和错误态元素存在，例如标题、加载文案、错误容器和重试按钮。
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
