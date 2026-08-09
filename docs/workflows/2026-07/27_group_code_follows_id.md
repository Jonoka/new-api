# 新建分组内部标识跟随 ID

## 问题

分组的稳定身份已经是数据库 ID，但管理页面新建分组时仍会生成 `group_2`、`group_3`
一类兼容 code。虽然页面主要展示名称和 ID，这些 code 仍可能出现在接口、复制结果或旧配置
中，看起来像另一个需要理解和维护的分组名称。

## 目标与范围

- 通过结构化分组接口新建的分组，持久化 `code` 必须等于真实分组 ID 的十进制文本。
- 前端生成的 `group_N` 只作为一次保存请求内关联高级配置的临时引用，不得持久化。
- 同一请求中的分组特殊倍率、充值倍率和特殊可用分组规则必须原子改写为最终 ID code。
- `default` 继续保留固定 code；虚拟 `auto` 仍不是实体分组。
- 已存在分组的 code 不自动修改，避免破坏能力表字符串主键、历史令牌、渠道和旧版本配置。

## 数据契约

`PUT /api/group/details` 对 `id <= 0` 的分组把请求 `code` 视为临时引用。后端创建实体取得
ID 后，以 `strconv.Itoa(id)` 作为最终 `code`，并在响应 `data` 中返回最终值。调用方后续
必须使用响应中的 ID 和 code，不得假设请求临时引用会被保留。

对 `id > 0` 的既有分组，code 仍然只读且不可修改。这样滚动升级期间旧实例和现有业务
数据仍可继续按原 code 运行。

## 原子性与安全边界

- 分组创建、最终 code 更新、选项引用改写和投影更新位于同一数据库事务。
- 候选数字 code 若已被另一旧分组或历史别名占用，事务会保留当前占位行并继续申请下一个
  数据库 ID；最终仍严格满足 `code == ID`，不会生成另一种隐藏别名。
- 请求重试按唯一显示名称识别已经创建成功的同名分组，避免网络重试产生重复实体。
- 运行时权限、倍率和渠道选择仍以现有 ID 绑定及最终 code 投影为准。

## 兼容性

实现只使用 GORM 创建、查询和更新，需同时兼容 SQLite、MySQL 和 PostgreSQL。旧分组的
`group_2` 等 code 不在新建流程中自动迁移；管理员可使用
`POST /api/group/code-migration/preview` 预检，并在全部实例升级一致后通过显式迁移接口同步
重建能力表字符串主键和所有当前业务引用。完整边界见
[旧分组标识显式迁移为稳定 ID](2026-07-27_group_code_explicit_migration.md)。

## 实现

- 后端在事务中先使用不可见占位名称和占位 code 创建新分组，取得 ID 后立即把 code
  更新为该 ID 的十进制文本；最终显示名称和其他属性仍在同一事务中写入。
- 保存请求维护“临时 code → 最终数字 code”映射，并改写 `GroupGroupRatio`、
  `group_ratio_setting.group_group_ratio`、`TopupGroupRatio` 和
  `group_ratio_setting.group_special_usable_group` 中的所有者及目标引用。
- 候选数字 code 与现有 code 或历史别名冲突时，会在同一事务中跳过该 ID 并继续分配；
  最终 code 仍等于实际 ID，成功后会清理用于跳号的占位行。
- Default 和 Classic 仍可在未保存行中使用 `group_N`，相关帮助函数已明确命名为临时引用
  生成器；保存成功后以接口响应返回的 ID 和 code 替换本地数据。

## 测试计划

- 新建分组的最终 code 等于其 ID。
- 同一请求中新分组的高级配置引用被改写为最终 code。
- 网络重试不会重复创建同名分组。
- 候选数字 code 冲突时跳过占用 ID，且不残留事务占位行。
- 既有分组 code 仍不可修改。
- Default、Classic 新建与保存测试、类型检查和生产构建通过。

## 验证结果

- `go test ./model -count=1 -timeout 60s` 通过。
- `go test ./controller ./middleware ./service -count=1 -timeout 60s` 通过。
- `go vet ./model ./controller ./middleware ./service` 通过。
- Default 临时引用测试 5 项、TypeScript 类型检查和生产构建通过。
- Classic 分组与显示名称测试 25 项、相关 ESLint、Prettier 和生产构建通过。
- Default ESLint 因当前依赖树中的 `brace-expansion` 导出不兼容而无法启动；本次改动已由
  TypeScript 类型检查、Prettier 和生产构建覆盖，未调整依赖锁文件。
- `git diff --check` 通过。
