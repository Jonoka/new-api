# classic 升级后旧前端缓存兜底

## 背景

zzapi 生产环境采用容器内替换 `/new-api` 二进制的方式升级。该方式不会重建容器或清理浏览器侧缓存。升级后如果用户后台标签页仍运行旧 JS，或浏览器持有旧 HTML 并继续请求旧 hash 的入口资源，可能出现后台点击无反应、页面像静态壳、必须手动刷新才能进入目标页面的现象。

## 问题表现

- 后端 `/api/status` 已显示新版本。
- HTML 已指向新入口资源。
- 日志中仍可能看到旧入口资源请求，例如 `/assets/index-旧hash.js`。
- 用户侧表现为侧栏或顶栏点击后页面不切换，刷新目标路径后可以进入。

## 修改范围

- `web-router.go`
  - 在静态资源未命中时，对旧 hash 的 `/assets/index-*.js` 与 `/assets/index-*.css` 增加兜底。
  - 如果请求的是过期入口资源，返回当前主题对应的最新入口 JS/CSS。
  - 仅处理入口资源，不对普通 `/assets/*` 做宽泛重写，避免掩盖真实资源缺失。

- `PageLayout.jsx`
  - classic 前端加载 `/api/status` 后记录服务端版本。
  - 检测到服务端版本变化时，当前标签页自动刷新一次，避免继续运行旧前端逻辑。
  - 移动端侧栏点击后关闭抽屉延后一帧，避免与 Link 路由跳转抢同一个点击周期。

## 兼容性

- 不改变 API 接口和数据库。
- 不改变登录态存储。
- 不清理 localStorage、sessionStorage 或 Cookie。
- 旧入口资源兜底只对当前主题的入口 JS/CSS 生效；旧 chunk、LICENSE、map 等资源仍按真实存在情况返回。

## 验证计划

1. classic 前端执行生产构建。
2. Go 后端执行相关测试，至少覆盖 web router。
3. 线上升级后验证：
   - `/api/status` 返回新版本。
   - `/console/*` HTML 返回 200。
   - 新入口 JS 返回 200。
   - 请求旧 `/assets/index-*.js` 不再直接 404，而是返回当前入口 JS。

## 回滚

如需回滚，恢复 `web-router.go` 的 NoRoute 逻辑和 `PageLayout.jsx` 的版本检测逻辑即可。回滚不影响数据库。
