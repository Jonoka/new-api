# 扩展模块开发

扩展模块适合放临时性、独立演进的功能，例如自动注册入库、批量同步、外部后台页面、一次性运维工具。模块可以调用主程序已有 API，也可以把自己的页面嵌入后台，不需要为了每个小功能重新发布主程序。

扩展系统支持两类运行时：

- `static`：主程序直接托管模块目录里的静态文件，适合纯页面、工具面板、调用主程序 API 的轻量模块。
- `http`：模块自己提供 HTTP 服务，适合后台任务、长连接、队列处理或需要独立运行时的模块。

扩展系统不使用 Go `plugin`：

- Windows 不支持 Go `plugin`。
- Go `plugin` 不能可靠热卸载。
- 外部进程崩溃不会拖垮主程序。
- HTTP 模块可以用 Go、Node.js、Python、PHP 或任何能提供 HTTP 服务的语言开发。

## 安装模块

默认模块目录是 `data/modules`。可以通过环境变量覆盖：

```bash
EXTENSIONS_ROOT=/path/to/modules
```

目录结构：

```text
data/modules/
├── state.json
└── auto-register/
    ├── manifest.json
    └── public/
        └── index.html
```

`state.json` 由主程序维护，用来记录启用状态。不要手工编辑它，除非你知道当前线上状态。

也可以用 root 账号进入 **扩展模块 -> 模块管理**，点击 **上传模块** 直接上传 zip 模块包。zip 支持两种结构：

```text
auto-register.zip
├── manifest.json
└── public/
    └── index.html
```

或：

```text
auto-register.zip
└── auto-register/
    ├── manifest.json
    └── public/
        └── index.html
```

上传后主程序会读取 `manifest.json`，并按 `manifest.id` 安装到 `data/modules/<module-id>`。

## 制作轻量模块

扩展模块默认优先按 `static` 轻量模块设计。主程序已经提供登录态、角色过滤、侧边栏入口、iframe 嵌入、代理转发和用户上下文接口，模块不需要重复实现这些能力。

推荐原则：

- 优先调用主程序已有 API，不在模块里复制用户、渠道、账单、权限等通用逻辑。
- UI 优先使用原生 HTML/CSS/JS 或很小的依赖，不默认引入 React/Vite/Next 等完整工程。
- 模块包只保留运行必需文件，不携带 `node_modules`、`dist`、`build`、日志、数据库和历史压缩包。
- 高权限操作使用单独服务账号 Access Token，密钥放环境变量或外部配置，不写入 `manifest.json`。

项目内提供了模块制作 skill：

```text
.agents/skills/newapi-extension-module-workflow/
```

创建新模块时可以参考其中的轻量模板：

```text
.agents/skills/newapi-extension-module-workflow/assets/http-module-template/
```

## 轻量化打包

在仓库根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "examples/extensions/echo"
```

默认输出：

```text
artifacts/extensions/<module-id>-<version>.zip
```

这个 zip 可以直接在 **扩展模块 -> 模块管理** 里上传。脚本会把 `manifest.json` 放在 zip 根目录，并默认排除重型目录和临时文件。

## 热加载流程

1. 上传 zip 模块包，或把模块目录放到 `data/modules/<module-id>`。
2. 如果是 `http` 模块，启动模块自己的 HTTP 服务；`static` 模块不需要单独启动。
3. 用 root 账号进入 **扩展模块 -> 模块管理**。
4. 如果是手动放目录，点击 **刷新**。
5. 开启模块开关。

刷新和启停都会立即生效，不需要重启主程序。

## 编写 manifest.json

每个模块必须提供 `manifest.json`：

```json
{
  "id": "auto-register",
  "name": "自动注册入库",
  "version": "0.1.0",
  "description": "自动注册并写入 new-api",
  "host": {
    "min": "v1.0.0-rc.10.1.10.144"
  },
  "runtime": {
    "type": "static",
    "static_dir": "public",
    "health_path": "/"
  },
  "ui": {
    "nav": [
      {
        "title": "自动注册",
        "page": "index",
        "icon": "Bot",
        "section": "admin",
        "order": 100
      }
    ],
    "pages": [
      {
        "key": "index",
        "title": "自动注册",
        "path": "/ui",
        "embed": true
      }
    ]
  },
  "permissions": {
    "roles": ["root"]
  }
}
```

字段说明：

- `id`：模块唯一 ID。建议只用字母、数字、短横线。
- `name`：后台显示名称。
- `version`：模块版本。
- `runtime.type`：支持 `static` 和 `http`。
- `runtime.static_dir`：`static` 模块的静态目录，默认 `public`。
- `runtime.base_url`：`http` 模块服务地址，只支持 `http` 和 `https`。
- `runtime.health_path`：健康检查路径，默认版前端会对 `http` 模块做可达性检查。
- `ui.nav`：写入主程序侧边栏的入口。
- `ui.pages`：模块页面定义。
- `permissions.roles`：允许访问的角色。可选值：`user`、`admin`、`root`。

`ui.nav.section` 支持：

- `chat`
- `console`：默认版前端会映射到 `General` 分组。
- `personal`
- `admin`

`ui.nav.icon` 使用默认版前端的自定义导航图标名称，例如 `Bot`、`Puzzle`、`Settings`、`Globe`、`Key`、`FileText`。

## 模块页面

如果页面配置为：

```json
{
  "key": "index",
  "path": "/ui",
  "embed": true
}
```

主程序会把它嵌入到：

```text
/extensions/<module-id>/index
```

`static` 模块实际请求会映射到：

```text
data/modules/<module-id>/public/index.html
```

`http` 模块实际请求会被代理到：

```text
<runtime.base_url>/ui
```

如果 `embed` 是 `false`，前端会提供一个外部打开按钮。

## 模块接收用户上下文

主程序代理 `http` 模块请求时会注入这些请求头：

```text
X-NewAPI-Module-ID: auto-register
X-NewAPI-User-ID: 1
X-NewAPI-Username: root
X-NewAPI-User-Role: 100
X-NewAPI-User-Group: default
X-NewAPI-Use-Access-Token: false
```

模块可以用这些头做审计、页面展示和权限判断。真正的高权限写操作建议仍然使用主程序 API，并使用单独创建的服务账号 Access Token。

## 模块调用主程序 API

模块可以调用标准 API。推荐做法：

1. 在主程序里创建一个专用服务账号。
2. 给这个账号最小必要权限。
3. 生成 Access Token。
4. 模块用这个 token 调用主程序 API。

请求需要同时带：

```text
Authorization: <access-token>
New-Api-User: <service-user-id>
```

模块也可以调用当前用户上下文接口：

```text
GET /api/extensions/host/me
```

这个接口会返回当前登录用户、角色、分组和主程序版本。

## 示例模块

`examples/extensions/host-context-probe` 提供了一个最小静态模块。打包后可以直接上传，不需要单独启动服务。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "examples/extensions/host-context-probe"
```

`examples/extensions/echo` 提供了一个最小模块：

```bash
cd examples/extensions/echo
node server.mjs
```

然后把示例目录复制到模块目录：

```text
data/modules/echo/
```

进入 **扩展模块 -> 模块管理**，点击 **刷新**，启用 `Echo Extension`。

## 安全边界

扩展模块是可信后台能力，不是给陌生人上传代码的平台。

- 只安装可信模块。
- 不要把 `runtime.base_url` 指向不可信公网服务。
- 模块的密钥放在模块自己的环境变量或配置里，不要写进 `manifest.json`。
- 高权限模块使用单独服务账号，不要复用 root 用户 token。
- 模块目录和主程序目录分开管理，便于回滚和删除。
