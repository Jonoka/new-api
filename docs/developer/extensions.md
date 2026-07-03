# 扩展模块开发

扩展模块适合放临时性、独立演进的功能，例如自动注册入库、批量同步、外部后台页面、一次性运维工具。模块可以调用主程序已有 API，也可以把自己的页面嵌入后台，不需要为了每个小功能重新发布主程序。

第一版扩展系统采用外部 HTTP 模块，而不是 Go `plugin`：

- Windows 不支持 Go `plugin`。
- Go `plugin` 不能可靠热卸载。
- 外部进程崩溃不会拖垮主程序。
- 模块可以用 Go、Node.js、Python、PHP 或任何能提供 HTTP 服务的语言开发。

## 放置模块

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
    ├── server.mjs
    └── data/
```

`state.json` 由主程序维护，用来记录启用状态。不要手工编辑它，除非你知道当前线上状态。

## 热加载流程

1. 把模块目录放到 `data/modules/<module-id>`。
2. 启动模块自己的 HTTP 服务。
3. 用 root 账号进入 **Extensions**。
4. 点击 **Refresh**。
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
    "type": "http",
    "base_url": "http://127.0.0.1:39001",
    "health_path": "/health"
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
- `runtime.type`：当前只支持 `http`。
- `runtime.base_url`：模块服务地址。只支持 `http` 和 `https`。
- `runtime.health_path`：健康检查路径，当前用于展示和约定，后续可接入自动检查。
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

实际请求会被代理到：

```text
<runtime.base_url>/ui
```

如果 `embed` 是 `false`，前端会提供一个外部打开按钮。

## 模块接收用户上下文

主程序代理请求时会注入这些请求头：

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

## 最小 Node.js 示例

`examples/extensions/echo` 提供了一个最小模块：

```bash
cd examples/extensions/echo
node server.mjs
```

然后把示例目录复制到模块目录：

```text
data/modules/echo/
```

进入 **Extensions**，点击 **Refresh**，启用 `Echo Extension`。

## 安全边界

扩展模块是可信后台能力，不是给陌生人上传代码的平台。

- 只安装可信模块。
- 不要把 `runtime.base_url` 指向不可信公网服务。
- 模块的密钥放在模块自己的环境变量或配置里，不要写进 `manifest.json`。
- 高权限模块使用单独服务账号，不要复用 root 用户 token。
- 模块目录和主程序目录分开管理，便于回滚和删除。
