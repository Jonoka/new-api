# classic 后台原生导航兜底

## 背景

生产环境出现 classic 后台地址栏已经变化，但页面内容仍停留在旧页面的问题。例如地址栏进入 `/console` 后，页面主体仍显示通知中心。该现象说明浏览器地址已经变化，但 React Router 的页面内容没有完成对应重渲染。

## 处理策略

为优先恢复生产后台可用性，classic 顶栏与侧栏的内部导航改为浏览器原生链接跳转。这样点击菜单后会自动加载目标路径，效果等价于用户手动刷新目标页面，但不需要用户再人工点击浏览器刷新按钮。

## 修改范围

- `Navigation.jsx`
  - 顶栏内部链接由 React Router `Link` 改为原生 `<a href>`。
  - 外部链接仍保持原有新标签页行为。

- `HeaderLogo.jsx`
  - 顶栏 Logo 回首页改为原生 `<a href>`。

- `UserArea.jsx`
  - 顶栏右侧登录、注册入口改为原生 `<a href>`。
  - 已登录头像下拉中的个人设置、令牌管理、钱包管理改为原生页面跳转。

- `useHeaderBar.js`
  - 退出登录后的跳转改为原生进入 `/login`。

- `SiderBar.jsx`
  - 侧栏内部链接由 React Router `Link` 改为原生 `<a href>`。
  - 保留菜单选中态、外部链接、新标签页与移动端抽屉关闭逻辑。

## 影响

- 后台导航稳定性优先。
- 点击顶栏/侧栏会触发页面加载，单页应用局部切换速度会略慢。
- 不改变接口、权限、数据库与业务数据。

## 验证

- classic 前端生产构建。
- 重点验证 `/notification-center`、`/console`、`/console/log`、`/console/topup` 之间点击导航后页面内容与地址一致。

## 回滚

如后续定位并修复 React Router 不重渲染的根因，可将顶栏和侧栏内部导航恢复为 React Router `Link`。
