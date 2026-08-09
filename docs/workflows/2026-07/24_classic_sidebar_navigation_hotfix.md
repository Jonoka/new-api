# Classic 后台侧栏点击后页面不切换

## 背景

`v1.0.0-rc.10.1.10.204` 升级后，classic 后台出现侧栏点击后页面内容不切换，
需要手动刷新页面才进入目标页面的现象。

## 根因

classic 侧栏使用 Semi UI `Nav` 的 `renderWrapper` 将菜单项包装为 React Router
`Link`。该组合在复杂菜单和动态模块项下容易只更新菜单选中态，未稳定触发
React Router 导航，表现为后台内切页面无响应。

## 修改范围

- classic 侧栏改为在 `onSelect` 中显式调用 `useNavigate()`。
- 内部路由不再依赖 `Link` 包装菜单项。
- 外部自定义导航仍保留 `<a>`，并阻止冒泡避免被 `Nav` 当作内部菜单选择处理。

## 验证

```bash
npm run build
```

结果：classic 前端构建通过。

## 部署注意

该修复仅影响 classic 后台侧栏导航，不涉及数据库结构变更。
