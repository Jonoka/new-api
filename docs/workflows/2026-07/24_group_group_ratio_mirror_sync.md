# 分组特殊倍率新旧配置镜像同步

## 背景

生产环境中，管理员在后台「分组特殊倍率」只保留 `vip -> Codex-Plus: 0.08`
后，日志中仍偶发出现 `vip -> Codex-Pro` 的专属倍率。表现为部分请求的
`other.user_group_ratio` 为历史值，部分请求又显示为 `-1`。

## 根因

分组特殊倍率存在两个配置键：

- `GroupGroupRatio`：旧版设置页使用的兼容键。
- `group_ratio_setting.group_group_ratio`：分层配置系统导出的新键。

此前保存旧版设置页时只更新 `GroupGroupRatio`，而运行时加载分层配置时仍可能从
`group_ratio_setting.group_group_ratio` 读到历史残留。两个配置键内容不一致时，
启动加载、定时同步或不同保存路径会造成运行时配置被不同来源覆盖，导致专属倍率看起来
“有时生效、有时不生效”。

## 修改范围

- 将 `GroupGroupRatio` 与 `group_ratio_setting.group_group_ratio` 视为同一配置的
  新旧镜像。
- `UpdateOption` / `UpdateOptionsBulk` 保存任意一个键时，自动补齐另一个键，并在同一
  事务中落库。
- `updateOptionMap` 对两个键都走同一套运行时刷新逻辑，刷新后同步 `OptionMap` 两个键并
  清理定价缓存。
- `loadOptionsFromDatabase` 启动或定时加载时发现两个键不一致，优先使用旧兼容键
  `GroupGroupRatio`，并把分层键修正为同值，避免历史残留继续影响计费。
- 分组配置页的高级选项白名单、引用校验、删除分组清理逻辑同步纳入分层键。

## 兼容性

- 不涉及数据库结构变更，仅补齐同一配置的镜像行。
- 旧后台、默认后台和直接调用配置接口都可继续提交原有键。
- 如同一次请求同时提交两个键且内容不一致，将返回明确错误，避免保存出新的不一致状态。

## 验证

```bash
go test ./model ./setting/ratio_setting -run "Test.*GroupGroupRatio|TestSaveGroupConfigWithOptions|TestUpdateOptionsBulk" -count=1 -timeout 60s
```

同时检查：

- 仅保存 `GroupGroupRatio` 时，两条 options 均写入同值。
- 仅保存 `group_ratio_setting.group_group_ratio` 时，两条 options 均写入同值。
- 历史状态 `GroupGroupRatio={}`、`group_ratio_setting.group_group_ratio` 残留旧倍率时，
  启动加载后运行时和数据库镜像都以 `{}` 为准。

## 部署注意

发布后无需手工迁移数据库。服务启动或下一次配置同步会自动修正历史镜像不一致；管理员
再次保存分组特殊倍率也会立即同步两个配置键。
