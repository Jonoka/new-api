# NewAPI 扩展模块轻量化打包

## 目标

- 输出可在 **扩展模块 -> 模块管理** 上传的 zip 包。
- 默认包体保持在几十 KB 到几百 KB。
- 包内只保留运行时必须文件。
- 页面、看板、工具类模块默认必须是 `static`，上传后无需启动额外服务。

## 推荐命令

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "examples/extensions/echo"
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
- 页面空白：优先检查模块是否误做成 `http` 但没有启动服务；页面工具应改为 `static`。
- HTTP 页面打不开：检查模块服务是否已启动，`runtime.base_url` 是否正确，`ui.pages[].path` 是否以 `/` 开头。
