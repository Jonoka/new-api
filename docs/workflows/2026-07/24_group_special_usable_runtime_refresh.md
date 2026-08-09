# 专属可用分组规则保存后不立即生效

## 背景

后台「分组与模型定价设置」里的专属可用分组规则保存后，数据库和
`OptionMap` 会更新，但运行时可用分组计算仍可能沿用旧值，表现为用户侧
分组列表、模型广场或后续请求没有立刻应用新规则。

## 根因

`group_ratio_setting.group_special_usable_group` 是分层配置字段，内部类型为
`types.RWMap[string, map[string]string]`。此前更新流程只走通用反射配置更新，
缺少和 `GroupRatio`、`GroupGroupRatio` 一样的显式校验与运行时加载路径。

通用更新还有一个问题：反序列化失败时不会把错误返回给接口层，可能让坏 JSON
显示保存成功但实际运行配置未改变。

## 修改范围

- 为专属可用分组规则增加显式 JSON 校验和加载函数。
- `updateOptionMap` 对 `group_ratio_setting.group_special_usable_group` 单独处理：
  保存后立即刷新运行时 `RWMap`。
- 专属可用分组规则、分组倍率、专属倍率更新后清理定价缓存，避免页面继续读旧结果。
- 增加回归测试：
  - 保存后运行时配置立即可见；
  - 非法 JSON 会返回错误，并保持原运行时配置不变。

## 验证

```bash
go test ./setting/ratio_setting ./model -run "TestUpdateGroupSpecialUsableGroup|TestValidateOptionValue|TestUpdateModelPrice|TestUpdateModelPriceUnit" -count=1 -timeout 60s
```

结果：通过。

## 部署注意

该修复为运行时配置刷新逻辑，不涉及数据库结构变更。生产发版后，管理员无需重启服务，
再次保存专属可用分组规则即可立即生效。
