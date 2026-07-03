---
name: newapi-extension-module-workflow
description: 在 new-api 项目中流程化创建、改造、排错和轻量化打包扩展模块。用户提到“写模块”“扩展模块”“模块管理”“manifest.json”“上传模块”“轻量模块”“package-extension-lite.ps1”“自动注册入库模块”等场景时使用。
---

# NewAPI Extension Module Workflow

## 目标

- 默认产出可上传到 **扩展模块 -> 模块管理** 的轻量 `.zip` 模块包。
- 优先复用主程序现有 API、鉴权、用户上下文、后台页面嵌入和代理能力。
- 避免把完整前端工程、`node_modules`、测试产物和运行时依赖打进模块包。

## 流程决策

1. 新增模块：执行“步骤 1 -> 步骤 2 -> 步骤 3 -> 步骤 4”。
2. 修改已有模块：执行“步骤 1 -> 步骤 2（只改动相关范围）-> 步骤 3 -> 步骤 4”。
3. 仅打包：执行“步骤 1（最小检查）-> 步骤 3 -> 步骤 4”。
4. 只要本次任务改动了模块目录内的 `manifest.json`、服务脚本、静态页面或配置，交付前默认执行一次轻量化打包。

## 步骤 1：收集上下文

1. 优先读取：
   - `docs/developer/extensions.md`
   - `scripts/package-extension-lite.ps1`
   - `examples/extensions/echo/manifest.json`
2. 定位目标模块：
   - 使用 `rg --files examples data modules . | rg "manifest\.json$"`。
   - 模块根目录必须直接包含 `manifest.json`。
3. 对齐宿主边界：
   - 运行时当前只支持 HTTP 模块。
   - 主程序负责：登录态、角色过滤、侧边栏入口、iframe 嵌入、代理转发、用户上下文请求头。
   - 模块负责：自己的 HTTP 服务、少量业务逻辑、必要的 UI 页面。

## 步骤 2：实现模块

1. 新模块优先从 `assets/http-module-template/` 创建骨架。
2. `manifest.json` 必须和模块服务一致：
   - `id` 稳定唯一，只用字母、数字、短横线、下划线。
   - `runtime.base_url` 指向模块 HTTP 服务。
   - `ui.nav[].page` 必须能在 `ui.pages[].key` 找到。
   - `ui.pages[].path` 必须以 `/` 开头。
   - `permissions.roles` 明确最小角色范围。
3. 默认做轻量模块：
   - 不引入 React/Vite/Next 等完整构建链，除非用户明确需要复杂前端。
   - UI 优先使用原生 HTML/CSS/JS，或由主程序页面/API 完成通用能力。
   - 调主程序接口时优先使用主程序已有 API，不复制用户管理、账单、渠道等逻辑。
   - 模块包不携带数据库、浏览器驱动、模型文件、`node_modules`、构建缓存。
4. 若模块需要当前登录用户信息：
   - 读取主程序代理注入的请求头：`X-NewAPI-User-ID`、`X-NewAPI-Username`、`X-NewAPI-User-Role`、`X-NewAPI-User-Group`。
   - 或在前端页面调用 `GET /api/extensions/host/me`。
5. 若模块需要高权限调用主程序：
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
   - 有模块运行所需入口文件，例如 `server.mjs`。
   - 不包含 `node_modules/`、`dist/`、`.git/` 等重型目录。
3. 若包体超过 1 MiB，先检查是否误打进依赖、构建产物、截图、日志或数据库。

## 步骤 5：交付说明

1. 列出模块目录、产物路径、版本号。
2. 列出核心行为变化。
3. 列出执行过的验证命令。
4. 如果用户明确只要求“打包/编译模块”，最终回复必须包含产物绝对路径。

## 参考文件

- 检查清单：`references/module-development-checklist.md`
- 轻量化打包：`references/lightweight-packaging.md`
- HTTP 模块模板：`assets/http-module-template/`
