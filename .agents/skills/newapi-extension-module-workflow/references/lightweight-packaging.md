# NewAPI 扩展模块轻量化打包

## 目标

- 输出可在 **扩展模块 -> 模块管理** 上传的 zip 包。
- 默认包体保持在几十 KB 到几百 KB。
- 包内只保留运行时必须文件。
- 页面、看板、工具类模块默认必须是 `static`，上传后无需启动额外服务。
- 可信的一方页面默认使用 `native v1`，但仍以轻量 zip 交付；React 和宿主组件库不进入模块包。

## 推荐命令

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "<module-dir>"
```

指定输出目录：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "modules/auto-register" -OutDir "artifacts/extensions"
```

## 默认排除

- 依赖与构建产物：`node_modules/`、`dist/`、`build/`、`.next/`、`.vite/`
- 版本与编辑器目录：`.git/`、`.idea/`、`.vscode/`
- 缓存与日志：`.cache/`、`.turbo/`、`logs/`、`*.log`
- 本地数据：`*.db`、`*.db-shm`、`*.db-wal`
- 凭据文件：`.env`、`.env.*`、`*.pem`、`*.key`、`*.pfx`、`secrets.json`
- 历史包：`*.zip`、`*.tpm`
- 测试产物：`coverage/`、`.nyc_output/`

## 产物结构

轻量静态包根目录应类似：

```text
host-context-probe-0.1.0.zip
├── manifest.json
└── public/
    └── index.html
```

可信原生模块应类似：

```text
operations-panel-0.1.0.zip
├── manifest.json
└── public/
    ├── compat.html
    └── native/
        ├── default.mjs
        ├── default.css
        ├── classic.mjs
        └── classic.css
```

`public/index.html` 不是 native 契约必需文件；如保留，建议显示与 `compat.html` 相同的升级提示，不能继续承载旧业务 UI。

`native-src/`、`node_modules/`、React、Semi UI 和 Base UI 只用于构建或由宿主提供，不得进入 zip。打包脚本会校验 manifest 声明的双前端入口与样式是否真实存在。

HTTP 模块可以包含服务入口：

```text
auto-register-0.1.0.zip
├── manifest.json
├── server.mjs
└── public/
    └── index.html
```

## 排错

- 上传失败并提示找不到 `manifest.json`：检查 zip 根目录是否直接包含 `manifest.json`，或是否只有一个顶层目录包含它。
- 包体过大：先用解压工具查看是否误带 `node_modules`、截图、数据库、历史包。
- 静态页面打不开：检查 `runtime.static_dir` 是否存在，入口文件是否在该目录内。
- 原生页面打不开：检查宿主版本、`ui.native`、`sdk=v1`、Default/Classic 目标、兼容页和全部原生资源；不要退回 iframe 掩盖缺失入口。
- 页面空白：优先检查模块是否误做成 `http` 但没有启动服务；页面工具应改为 `static`。
- HTTP 页面打不开：检查模块服务是否已启动，`runtime.base_url` 是否正确，`ui.pages[].path` 是否以 `/` 开头。
