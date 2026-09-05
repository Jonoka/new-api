# 日志隐私、SQL 脱敏与统计修复

## 目标与范围

本次修复收紧普通用户可读取的使用日志，避免以后新增诊断字段时自动暴露；同时确保
GORM 在主数据库和独立日志数据库中都不打印绑定参数，并修复 RPM/TPM 查询覆盖历史
额度合计的问题。修改不涉及数据库表结构、持久化日志格式或历史数据重写。

## 用户日志投影

`GET /api/log/self`、只读令牌日志接口及复用这些结果的前端导出只返回显式投影。主记录
保留时间、类型、内容、令牌名称、模型、额度、输入/输出 token、耗时、流式标记、分组、
用户请求 ID 和经过过滤的 `other`。数据库行 ID 改为原有分页显示序号。用户/令牌内部
ID、用户名、渠道 ID/名称、IP 和上游请求 ID 不进入普通用户投影。

`other` 采用默认拒绝的字段清单，并校验布尔、数字、字符串、字符串数组以及可为字符串
或数字的订阅 ID。保留当前两个前端消费的计费、token、缓存、音频、图片、搜索、异步
任务、订阅和请求格式字段。以下内容始终排除：

- `admin_info`、`root_info`、`audit_info` 及其所有嵌套内容；
- 渠道字段、`reject_reason`、`stream_status`、`timing_diagnostics` 和参数覆盖审计 `po`；
- 未登记字段，以及已登记字段中类型不符合契约的对象或混合数组。

字符串数组逐元素验证；含 `null` 的数组也视为类型错误，不进入用户投影。

无法解析、`null` 或形状错误的旧 `other` 返回空对象。过滤使用 `json.RawMessage`，避免
将大整数经 `float64` 往返后改变字面值。管理员日志查询继续返回原始主记录和原始
`other`，普通用户投影通过新对象构造，不会修改管理员读取的记录。

## GORM 日志

SQLite、MySQL 和 PostgreSQL 的所有 `gorm.Open` 路径共用同一配置。SQL 在正常、慢查询、
错误和 `DEBUG=true` 的 Info 日志中始终保留占位符，不输出绑定值。驱动错误仅保留数据库
类型与错误码；上下文取消和超时保留稳定错误身份；其他错误统一记录为
`database operation failed`，防止包装后的 GORM sentinel 或服务端错误正文夹带参数。

GORM 1.25.2 的 `Scan` 会临时改用不实现 `ParamsFilter` 的 recorder。数据库配置因此同时
注册 Row 回调，在 SQL 插值前给 recorder 补上参数过滤接口，并复制请求级配置以避免并发
请求之间修改共享 logger。只设置 `ParameterizedQueries=true` 不能覆盖这个路径；回归测试
必须使用完整数据库配置并实际执行 `Raw(...).Scan(...)`，不能只测试 logger 的配置值。

`SQL_SLOW_THRESHOLD_MS` 默认 `200`，`0` 关闭慢查询判定，合法范围为 0 到 3600000。
越界值记录配置错误并回退到默认值。该配置只改变慢查询日志阈值，不改变查询执行。

## 统计修复

历史额度合计继续使用完整筛选时间段。RPM/TPM 使用同一组用户名、令牌、模型、渠道和
分组筛选，再额外限制最近 60 秒。两个查询扫描到不同结构，最后只把 RPM/TPM 复制到
返回值，因此空的最近窗口不会再把非零历史额度覆盖为零。

## CI 验证计划

以下命令必须在 GitHub-hosted Actions 对候选 SHA 执行，本工作区不执行构建或测试：

```bash
go test ./model -run 'Test(FormatUserLogsUsesStrictProjectionWithoutMutatingSource|ProjectUserLogOtherRejectsMalformedAndWrongShapes|UserAndTokenReadsProjectWhileAdminReadRemainsComplete|SumUsedQuotaKeepsHistoricalQuotaAndRecentRatesWithFilters|SanitizeDBErrorNeverReturnsDriverOrWrappedValues|SanitizedGormWriterRemovesSyntheticSentinelValue|GormLoggerRedactsNormalSlowErrorAndDebugSQL)$' -count=1
go test -race ./model -run 'Test(UserAndTokenReadsProjectWhileAdminReadRemainsComplete|SumUsedQuotaKeepsHistoricalQuotaAndRecentRatesWithFilters|GormLoggerRedactsNormalSlowErrorAndDebugSQL)$' -count=1
go test ./...
```

重点断言包括合法计费字段与大整数保留、未知/嵌套/错误形状删除、用户与令牌读取一致、
管理员结果完整、四类 SQL 日志均无合成 secret，以及历史额度与最近 RPM/TPM 同时正确。
