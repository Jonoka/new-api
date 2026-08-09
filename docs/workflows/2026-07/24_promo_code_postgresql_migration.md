# PostgreSQL 优惠码唯一键迁移修复

日期：2026-07-24

## 问题

`v1.0.0-rc.10.1.10.204` 在已有 PostgreSQL 数据库上启动时，优惠码删除标识迁移会
持续失败并使应用容器进入重启循环，对外表现为 Nginx 502：

```text
ERROR: cannot drop index idx_promo_codes_code because constraint
idx_promo_codes_code on table promo_codes requires it (SQLSTATE 2BP01)
```

生产应急处理中已将应用二进制回滚至 `.199`，PostgreSQL 与 Redis 容器及业务数据
均未改动。

## 根因

旧版 PostgreSQL 将 `promo_codes.code` 的唯一性保存为名为
`idx_promo_codes_code` 的 `UNIQUE CONSTRAINT`。PostgreSQL 不允许直接删除约束的
支撑索引；原迁移仅调用 GORM `DropIndex`，因此每次启动都会在同一位置失败。

SQLite 和 MySQL 中该旧唯一键仍可能表现为普通唯一索引，不能统一改成删除约束。

## 修改范围

- PostgreSQL 先检测并使用 GORM `DropConstraint` 删除同名旧唯一约束；
- 约束删除后再次检测同名索引，存在时再通过 `DropIndex` 清理；
- SQLite 和 MySQL 保持原来的索引删除路径；
- 迁移保持幂等，组合唯一索引 `idx_promo_codes_code_deleted_id` 的创建顺序不变；
- 不修改优惠码、订单或使用记录的数据内容。

## 测试计划

- 继续执行现有 SQLite 迁移与优惠码复用测试；
- 在临时 PostgreSQL 数据库中创建同名 `UNIQUE CONSTRAINT`，复现生产结构；
- 连续执行两次迁移，验证旧约束和索引消失、组合唯一索引存在；
- 验证历史软删除记录完成 `deleted_id` 回填，且相同优惠码可重新创建；
- PostgreSQL 测试仅接受数据库名以 `newapi_test_` 开头的显式 DSN。

## 部署注意事项

- 修复版发布前必须在临时 PostgreSQL 实例完成真实迁移测试；
- 生产升级前保持 `zzapi` 运行 `.199`，不得再次安装 `.204`；
- 升级后先观察应用启动和迁移日志，再验证 `/api/status` 与后台页面，禁止通过手工
  删除生产约束绕过代码缺陷。

## 验证结果

- SQLite 优惠码迁移与生命周期定向测试：通过；
- PostgreSQL 15 临时实例真实约束迁移测试：通过；
- PostgreSQL 迁移连续执行两次：通过；
- `go test ./model -count=1`：通过；
- `go test ./middleware -count=1`：通过；
- `go vet ./model ./middleware`：通过；
- 临时测试容器已停止并自动删除，未使用生产数据库。
