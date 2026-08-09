# 自动分组顺序唯一索引迁移幂等性修复

日期：2026-08-09

## 问题

PostgreSQL 数据库首次启动后已存在 `auto_group_members` 的唯一索引
`idx_auto_group_position`，第二次执行 GORM `AutoMigrate` 时又生成了等价的
`idx_auto_group_members_position`，导致相邻两次启动的公开索引与约束目录不一致。

## 根因

`AutoGroupMember.Position` 通过模型字段的 `uniqueIndex` tag 交给 `AutoMigrate`
管理。GORM 对已有 PostgreSQL 唯一键的重复识别不稳定，会按默认命名补建第二个
唯一约束或索引。

## 修改范围

- 从业务模型字段移除唯一索引 tag，避免 `AutoMigrate` 再生成默认名称；
- 在分组关系表建立后显式执行幂等迁移：PostgreSQL 先删除遗留同名唯一约束，
  再删除仍存在的同名索引；
- 所有数据库都显式确保唯一索引 `idx_auto_group_position` 存在；
- 普通迁移和快速迁移都在分组回填前执行同一迁移，不修改分组成员数据。

## 兼容性

迁移继续支持 SQLite、MySQL 5.7.8+ 和 PostgreSQL 9.6+。PostgreSQL 已进入失败
第二次启动状态的数据库会清理重复键，仅保留 `idx_auto_group_position`；现有
`group_id`、`position` 与分组成员顺序不变。

## 测试

- SQLite 连续执行两次显式迁移，验证目标索引存在且 `position` 仍保持唯一；
- PostgreSQL 15 从无顺序索引的新表连续执行两次启动迁移，验证第一次创建
  目标唯一索引且第二次目录指纹不变；
- 再注入目标索引和遗留唯一约束并存的失败状态，连续执行两次启动迁移，
  验证清理后目录指纹保持一致；
- PostgreSQL 最终只允许 `idx_auto_group_position` 对 `position` 提供唯一性，
  不保留 `idx_auto_group_members_position` 约束或索引；
- GitHub Actions 使用数据库名以 `newapi_test_` 开头的临时 PostgreSQL 服务运行
  全量 Go 测试。

## 回滚

代码回滚不会自动恢复已删除的重复唯一键；该键与保留索引语义等价，无需恢复。
若候选版本启动或目录验证失败，停止发布并恢复升级前数据库备份与旧镜像，不允许
仅回滚镜像后继续使用已迁移数据库。
