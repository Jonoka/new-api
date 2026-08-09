# 扩展模块开发

这份文档说明模块如何复用主程序能力。通知事件接入请先阅读[通知中心与模块事件](notifications.md)，二开能力总览见[本项目二次开发能力](custom-development.md)。

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

扩展模块默认优先按 `static` 轻量模块设计。主程序已经提供登录态、角色过滤、侧边栏入口、iframe 嵌入、宿主原生渲染槽、代理转发和用户上下文接口，模块不需要重复实现这些能力。

渠道可观测性中心以 `channel-quality` 静态模块交付，通过 **扩展模块 -> 模块管理** 上传、启停和升级。正式页面使用 `native v1` 渲染槽：模块包提供 Default 和 Classic 入口，宿主分别用当前前端的真实组件渲染；模块仍保持独立 zip，主程序不注册渠道可观测性的固定业务路由。旧宿主通过包内兼容页提示升级。

推荐原则：

- 优先调用主程序已有 API，不在模块里复制用户、渠道、账单、权限等通用逻辑。
- 可信的一方页面、看板和后台工具默认使用 `native v1`，直接复用 Default/Classic 的真实宿主组件。
- iframe 只用于仍受信任但需要独立 DOM、样式或框架运行时的第三方/遗留页面，以及远程 `http` UI；主题变量桥接不等于原生 UI。
- iframe 不是安全沙箱；不可信模块禁止安装，不能通过 iframe 绕过可信模块边界。
- 原生入口只打包业务组件和状态逻辑，React、ReactDOM、Semi 和 Base UI 等依赖由宿主 SDK 提供。
- 模块包只保留运行必需文件，不携带 `node_modules`、`dist`、`build`、日志、数据库和历史压缩包。
- 高权限操作使用单独服务账号 Access Token，密钥放环境变量或外部配置，不写入 `manifest.json`。

项目内提供了模块制作 skill：

```text
.agents/skills/newapi-extension-module-workflow/
```

创建页面模块时参考已经落地的原生模块：

```text
examples/extensions/channel-quality/
```

只有需要独立 HTTP 服务时才参考：

```text
.agents/skills/newapi-extension-module-workflow/assets/http-module-template/
```

## 轻量化打包

在仓库根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "<module-dir>"
```

默认输出：

```text
artifacts/extensions/<module-id>-<version>.zip
```

这个 zip 可以直接在 **扩展模块 -> 模块管理** 里上传。脚本会把 `manifest.json` 放在 zip 根目录，并默认排除重型目录和临时文件。对于 `native` 页面，脚本还会在压缩前逐项确认 Default、Classic 的入口和样式文件确实存在于打包结果中，缺少任一运行资源时直接失败。

模块根目录可提供 `.extensionignore`，每行填写一个相对于模块根目录的精确文件路径。打包脚本会排除这些文件和 `native-src/` 构建源码；不支持绝对路径、目录穿越或通配符，避免忽略规则意外带走运行资源。

## 热加载流程

1. 上传 zip 模块包，或把模块目录放到 `data/modules/<module-id>`。
2. 如果是 `http` 模块，启动模块自己的 HTTP 服务；`static` 模块不需要单独启动。
3. 用 root 账号进入 **扩展模块 -> 模块管理**。
4. 如果是手动放目录，点击 **刷新**。
5. 开启模块开关。

刷新和启停都会立即生效，不需要重启主程序。

## 编写 manifest.json

每个模块必须提供 `manifest.json`。下面是可信一方页面模块的默认写法：

```json
{
  "id": "operations-panel",
  "name": "运维面板",
  "version": "0.1.0",
  "description": "复用宿主组件的运维工具面板",
  "host": {
    "min": "v1.0.0-rc.10.1.10.204"
  },
  "runtime": {
    "type": "static",
    "static_dir": "public",
    "health_path": "/compat.html"
  },
  "ui": {
    "nav": [
      {
        "title": "运维面板",
        "page": "index",
        "icon": "Activity",
        "section": "admin",
        "order": 100
      }
    ],
    "pages": [
      {
        "key": "index",
        "title": "运维面板",
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
    "roles": ["root"],
    "capabilities": ["ui.native"]
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
- `ui.pages`：模块页面定义；native 页面默认把 `path` 设为 `/compat.html`，iframe 静态页面默认使用 `/`。`path` 必须是以 `/` 开头的规范模块路径；除根路径 `/` 外，不能包含 `.`/`..` 点段、重复或尾随斜杠、反斜杠、百分号编码、查询参数、片段或控制字符。
- `permissions.roles`：允许访问的角色。可选值：`user`、`admin`、`root`。
- `permissions.capabilities`：模块使用的高权限宿主能力。原生页面必须声明 `ui.native`。

`ui.nav.section` 支持：

- `chat`
- `console`：默认版前端会映射到 `General` 分组。
- `personal`
- `admin`

`ui.nav.icon` 使用默认版前端的自定义导航图标名称，例如 `Activity`、`Bot`、`Puzzle`、`Settings`、`Globe`、`Key`、`FileText`。

## 模块页面

### 渲染方式选择

| 场景 | 默认方式 | 原因 |
| --- | --- | --- |
| 可信的一方页面、看板、后台工具 | `native v1` | 直接复用面板组件、主题、国际化和响应式布局 |
| 仍受信任的第三方/遗留页面，需要独立 DOM、样式或框架运行时 | iframe | 保持页面结构和依赖边界独立，不代表安全沙箱 |
| 远程 `http` UI | iframe | 保持进程、依赖和发布边界独立 |
| 外部新标签流程 | `embed: false` | 按交互边界打开，不用于承载不可信代码 |

原生页面仍然是独立 zip 模块，可上传、启停、升级和卸载；“优先原生”不等于把业务页面注册进主程序固定路由。

所有模块仍必须可信。当前 iframe 使用同源模块代理且不提供安全沙箱，不能把陌生或未经审查的代码装进 iframe 后视为隔离完成。

### iframe 页面（独立 DOM/运行时模式）

如果页面配置为：

```json
{
  "key": "index",
  "path": "/ui",
  "embed": true
}
```

页面会通过宿主代理打开。Default 前端通常使用 /extensions/<module-id>/<page-key>，Classic 前端使用 /console/extensions/<module-id>/<page-key>。模块应依赖稳定的代理资源入口：

```text
/api/extensions/<module-id>/proxy<ui.pages[].path>
```

static 模块最终读取 data/modules/<module-id>/<runtime.static_dir> 下的文件，http 模块最终转发到 runtime.base_url 加上页面 path。宿主不会先让浏览器规范化危险路径再转发，因此 `ui.pages[].path` 不能利用 `..`、反斜杠、重复斜杠、查询参数或片段跳出模块代理前缀。embed 为 false 时仍然在新标签页打开宿主代理 URL，不会绕过会话、角色和启用状态检查。

static 代理只提供静态目录内的普通文件，拒绝点文件、符号链接、非普通文件和解析后越出静态目录的路径。请求普通 SPA 路由且目标不存在时才回退根目录 `index.html`；非法路径和不安全资源直接拒绝，不参与首页回退。

#### 嵌入主题桥接

`embed: true` 的同源模块加载完成后，Default 和 Classic 会把宿主当前主题映射到 iframe 根元素。通用模块可以直接使用以下语义变量，并为独立打开场景提供回退值：

```css
:root {
  background: var(--page-bg, #f5f6f8);
  color: var(--text, #20242a);
  font-family: var(--host-font-family, system-ui, sans-serif);
}
```

当前桥接变量包括 `--page-bg`、`--surface`、`--surface-muted`、`--surface-soft`、`--border`、`--border-strong`、`--text`、`--text-soft`、`--muted`、`--primary`、`--primary-strong`、`--primary-soft`、`--green`、`--green-soft`、`--amber`、`--amber-soft`、`--red`、`--red-soft`、`--radius` 和 `--host-font-family`。

宿主同时设置 `document.documentElement.dataset.hostTheme`，值为 `light` 或 `dark`，并发送同源消息：

```js
{
  type: 'new-api-host-theme',
  themeMode: 'light' | 'dark',
  embedded: true
}
```

模块不得信任跨域消息；监听时必须校验 `event.origin === window.location.origin`。主题桥接只负责视觉变量，不授予权限，也不会把 Cookie、令牌或用户信息注入页面脚本。宿主已有标题栏时，模块可依据 `embedded: true` 隐藏重复页头。

### 宿主原生页面（可信模块默认）

可信的一方页面默认声明 `native v1` 渲染器，直接复用 Default 的 Base UI/Tailwind 组件或 Classic 的 Semi UI 组件。`native v1` 当前只允许本地 `static` 模块使用；`http` 模块仍通过 iframe 代理远程页面，不能把远程脚本作为宿主原生入口执行。

```json
{
  "runtime": {
    "type": "static",
    "static_dir": "public"
  },
  "ui": {
    "pages": [
      {
        "key": "index",
        "title": "运维看板",
        "path": "/compat.html",
        "embed": true,
        "render": {
          "type": "native",
          "sdk": "v1",
          "targets": {
            "default": {
              "entry": "/native/default.mjs",
              "styles": []
            },
            "classic": {
              "entry": "/native/classic.mjs",
              "styles": []
            }
          }
        }
      }
    ]
  },
  "permissions": {
    "roles": ["admin"],
    "capabilities": ["ui.native"]
  }
}
```

约束如下：

- 原生页面只允许来自本地 `static` 模块；`http` 模块和远程 URL 不能提供原生入口。
- `sdk` 当前固定为 `v1`，必须同时提供 `default` 和 `classic` 目标。
- `entry` 只能是模块静态目录内的 `.js` 或 `.mjs` 文件，`styles` 只能引用模块静态目录内的 `.css` 文件。
- 模块入口默认导出 React 页面组件，不得携带自己的 React、ReactDOM、Semi 或 Base UI 副本；它通过版本化宿主 SDK 使用当前前端的组件实例。
- 可导入的宿主模块和导出名以 `web/default/src/features/extensions/native-sdk.ts` 和 `web/classic/src/pages/Extensions/native-sdk.js` 的白名单为准。构建脚本会拒绝白名单外的裸模块、命名空间导入、SDK 未提供的默认或具名导出、绝对文件路径，以及越出对应 `native-src/<target>` 的相对导入；不得 deep import 宿主内部文件，也不得把依赖副本打进模块。缺少通用组件时必须先同步扩展两套 SDK 契约并验证双端构建。
- 模块样式必须包含源码实际使用的工具类或静态规则；可以复用宿主语义变量和 Tailwind `@theme inline` 映射，但不得注入 Preflight、`body/html` 重置或覆盖宿主主题值。
- 原生入口在主页面 JavaScript 上下文执行，能以当前登录用户身份调用接口。它只适用于可信模块，安装和启停仍只允许 root。
- `path` 保留为旧宿主兼容入口。`native v1` 从 `v1.0.0-rc.10.1.10.204` 起可用，native 模块应把 `host.min` 至少设为该版本；`.202` 和 `.203` 会忽略 `render` 并打开兼容页，不能把原生脚本当作 HTML 页面。
- 页面结构、信息密度、间距和交互遵循对应宿主现有模式，不重复绘制宿主页头，也不自行仿制一套“类似面板”的控件。
- 所有可见文案接入宿主国际化，数字、日期和相对时间跟随当前语言；Default 与 Classic 必须功能等价。
- 验收覆盖 Default/Classic、亮色/暗色、桌面/移动端，并断言正式页面没有 iframe、原生宿主容器存在、没有页面级水平溢出和控制台错误。

原生资源只通过精确路由读取，不使用会在文件不存在时回退首页的普通代理：

```text
GET /api/extensions/<module-id>/native/<page-key>/<target>/entry
GET /api/extensions/<module-id>/native/<page-key>/<target>/style-<index>
```

这些路由继续校验模块启用状态、角色和 manifest 声明，并设置 `nosniff`、同源和禁止缓存响应头。扫描与安装会读取所有声明的原生资源，校验其真实路径仍位于模块目录内，并计算确定性的 `asset_revision` 内容摘要；模块列表把该摘要返回给两套宿主，宿主将它加入入口和样式 URL，因此同 ID、同版本覆盖上传也不会复用浏览器中旧的 ESM 模块。

## 模块接收用户上下文

主程序代理 http 模块请求时会注入：

```text
X-NewAPI-Module-ID: auto-register
X-NewAPI-User-ID: 1
X-NewAPI-Username: root
X-NewAPI-User-Role: 100
X-NewAPI-User-Group: default
X-NewAPI-Use-Access-Token: false
```

HTTP 模块后端可以用这些头做审计和页面展示。static 模块的浏览器脚本读不到请求头，应调用 /api/extensions/host/me 获取当前用户、角色、分组和版本。代理会剥离浏览器 Cookie、Authorization 和 Proxy-Authorization。

## 模块调用主程序 API

static 模块使用同源 Session Cookie，并在调用 UserAuth、AdminAuth 或 RootAuth API 时携带 New-Api-User。先调用 /api/extensions/host/me 获取当前用户 ID：

```js
const me = await fetch("/api/extensions/host/me", {
  credentials: "same-origin",
}).then((response) => response.json());

const response = await fetch("/api/channel/search?p=1&page_size=20", {
  credentials: "same-origin",
  headers: { "New-Api-User": String(me.data.user_id) },
});
```

HTTP 模块后台使用独立服务账号 Access Token，并同时携带：

```http
Authorization: Bearer <service-account-access-token>
New-Api-User: <service-account-user-id>
```

Bearer 前缀必须使用大写 B。这里的 Access Token 是主程序用户的系统管理令牌，不是 OpenAI sk- API Token。现有 /api/extensions/host/me 和模块代理路由仍走 UserSessionAuth，只接受已登录浏览器会话；后台服务需要直接调用主程序 API。

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
- iframe 仅隔离页面 DOM、样式和框架运行时，不构成不可信代码的安全边界。
- 不要把 `runtime.base_url` 指向不可信公网服务。
- 模块的密钥放在模块自己的环境变量或配置里，不要写进 `manifest.json`。
- 高权限模块使用单独服务账号，不要复用 root 用户 token。
- 模块目录和主程序目录分开管理，便于回滚和删除。

## 当前实现补充

以下约定以当前源码为准，适用于已经安装的 new-api 扩展模块。

### 模块页面和资源 URL

Default 前端页面通常显示为 `/extensions/<module-id>/<page-key>`，Classic 前端显示为 `/console/extensions/<module-id>/<page-key>`。模块不要依赖这两个外层页面路径；稳定的资源入口是：

```text
/api/extensions/<module-id>/proxy<ui.pages[].path>
```

`embed: false` 仍然通过主程序代理 URL 在新标签页打开，不会绕过会话、角色和模块启用检查。

### 运行时和鉴权边界

| 场景                                 | 当前实现                                                                         |
| ------------------------------------ | -------------------------------------------------------------------------------- |
| static 模块页面                      | 主程序托管静态文件，浏览器使用同源 Session Cookie                                |
| http 模块页面                        | 主程序通过 `/api/extensions/:id/proxy/*path` 代理到 `runtime.base_url`           |
| native static 页面                   | 主程序精确读取模块入口，在宿主 DOM 中使用版本化 SDK 和真实宿主组件渲染            |
| 受保护的主程序 API                   | 需要 `New-Api-User`；HTTP 后台可用 `Authorization: Bearer <access-token>`        |
| `/api/extensions/host/me` 和模块代理 | 目前走 `UserSessionAuth`，只接受已登录浏览器会话                                 |
| 通知事件发布                         | `/api/extensions/:id/notification-events` 走 `RootAuth`，仅接受 Root 服务账号 Access Token |

受保护 API 会校验 `New-Api-User` 与当前账号 ID 一致。static 模块应先调用 `/api/extensions/host/me` 获取用户 ID，再发起普通受保护请求；通知事件发布是例外，当前只允许受信任 HTTP 模块使用独立 Root 服务账号。不要把令牌写入 manifest 或静态文件，Access Token 的 Bearer 前缀固定使用大写 B。

HTTP 模块只应依赖宿主注入的 `X-NewAPI-Module-ID`、`X-NewAPI-User-ID`、`X-NewAPI-Username`、`X-NewAPI-User-Role`、`X-NewAPI-User-Group` 和 `X-NewAPI-Use-Access-Token`。代理不会把浏览器的 `Cookie`、`Authorization` 或 `Proxy-Authorization` 转发给模块。

### Manifest 的通知能力

模块需要共享通知中心时，在 manifest 中同时声明：

```json
{
  "permissions": {
    "roles": ["root"],
    "capabilities": ["notification.events.publish"]
  },
  "notifications": {
    "events": [
      {
        "id": "created",
        "label": "新订单",
        "default_template": "{{mention}} 来了新订单：{{order_id}}",
        "variables": [
          {
            "name": "order_id",
            "type": "string",
            "required": true
          }
        ]
      }
    ]
  }
}
```

宿主把事件规范化为 `extension.<module-id>.<event-id>`，并在通知中心的事件类型列表中展示。模块只提交 `event_type`、稳定的 `event_key` 和声明过的 payload；Bot、Chat ID、提及和模板由通知中心管理。完整请求限制、幂等和响应状态见[通知中心与模块事件](notifications.md)。

### 安装和兼容性限制

- `EXTENSIONS_ROOT` 是首选模块目录环境变量，`MODULES_ROOT` 仅作兼容别名。
- zip 上传上限为 100 MiB，解压内容上限为 200 MiB。
- 上传相同模块 ID 会替换模块目录，但保留 `state.json` 中的启用状态。
- `runtime.type` 应显式填写；未填写时实现默认按 `http` 处理并要求 `base_url`。
- `runtime.health_path` 目前主要由 Default 前端探测 HTTP 模块，Classic 前端和后端不会统一做健康检查。
- `host.min` 和 `host.max` 当前只是 manifest 元数据，不会自动阻止不兼容版本；模块可以通过 `/api/extensions/host/me` 返回的 version 自行判断。
- `permissions.roles` 为空表示所有已登录用户，不等同于匿名访问。

### 事件和后台任务

通知事件发布入口是：

```text
POST /api/extensions/<module-id>/notification-events
```

HTTP 模块后台应使用 Root 服务账号 Access Token 和 `New-Api-User` 调用；static/native 模块不得把服务账号令牌放进前端，也不应依赖浏览器中的 Root 会话发布权威通知事件。当前 `RootAuth` 会接受已登录的 Root 会话，因此这是一条受信任模块政策，不是浏览器级技术隔离；权威业务事件必须由受信任的服务端模块发布。事件首次接收时进入统一 outbox，重复事件只返回 duplicate，不会绕过任务开关、目标配置、重试和最近五条历史限制。

### 安全清单

- 只安装可信模块；模块是受信任后台代码。
- 模块密钥放环境变量或外部配置，不要写入 manifest、静态资源或日志。
- HTTP 模块仍然拥有独立进程的后台权限，必须限制 `runtime.base_url` 指向可信地址。
- 用独立服务账号调用高权限 API，不要复用个人 root token。
- 当前不能向不完全可信的第三方开放模块。未来如需支持，必须先增加独立 origin、iframe sandbox、绑定模块的发布令牌和 scope，再开放上传权限。

### 相关文档

- [通知中心与模块事件](notifications.md)
- [本项目二次开发能力](custom-development.md)
- [开发文档索引](README.md)
