# NewAPI 扩展模块开发检查清单

## 模块结构

- 模块根目录直接包含 `manifest.json`。
- 轻量 HTTP 模块优先包含 `server.mjs` 或等价入口文件。
- 按需包含 `public/`、`views/`、`config.example.json`。
- 不把 `node_modules/`、`dist/`、`build/`、`.git/`、日志、数据库、`.env`、私钥和历史 zip 包放进模块包。

## manifest 必填项

- `id`：稳定唯一标识，发布后不要随意变更。
- `name`：后台显示名称。
- `version`：语义化版本。
- `runtime.type`：当前只支持 `http`。
- `runtime.base_url`：模块服务地址，只支持 `http` 或 `https`。
- `ui.pages[].key`：页面唯一键。
- `ui.pages[].path`：代理路径，必须以 `/` 开头。

## UI 与导航

- `ui.nav[].page` 必须对应 `ui.pages[].key`。
- `ui.nav[].title` 是侧边栏显示文字。
- `ui.nav[].icon` 使用默认版前端支持的图标名，例如 `Puzzle`、`Bot`、`Settings`、`Key`。
- `permissions.roles` 按最小权限填写：`user`、`admin`、`root`。

## 轻量化原则

- 优先调用主程序 API，不在模块里复制用户、渠道、账单、权限等通用逻辑。
- UI 优先原生 HTML/CSS/JS，复杂交互再考虑引入小型依赖。
- 后台任务优先做成模块自己的 HTTP 接口，由主程序页面触发。
- 密钥放环境变量或外部配置，不写入 `manifest.json`。

## 打包前自查

- `manifest.version` 已按本次改动递增。
- `runtime.base_url` 与实际启动端口一致。
- 模块服务能响应 `runtime.health_path`。
- zip 根目录能直接看到 `manifest.json`。
- 包体明显偏大时先解包排查重型目录。
