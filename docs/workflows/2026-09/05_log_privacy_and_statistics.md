# 日志隐私、SQL 脱敏与统计修复

## 目标与范围

本次修复收紧普通用户可读取的使用日志，避免以后新增诊断字段时自动暴露；同时确保
GORM 在主数据库和独立日志数据库中都不打印绑定参数，并修复 RPM/TPM 查询覆盖历史
额度合计的问题。修改不涉及数据库表结构、持久化日志格式或历史数据重写。

## 用户日志投影

`GET /api/log/self`、只读令牌日志接口及复用这些结果的前端导出只返回显式投影。主记录
保留时间、类型、经过类型约束的内容、令牌名称、模型、额度、输入/输出 token、耗时、
流式标记、分组、用户请求 ID 和经过过滤的 `other`。数据库行 ID 改为原有分页显示序号。
用户/令牌内部 ID、用户名、渠道 ID/名称、IP 和上游请求 ID 不进入普通用户投影。

### 主记录 `content` 契约

`content` 是否公开由日志类型决定，不能根据字符串看起来是否安全来判断。当前写入链路的
决定如下：

| 日志类型 | 实际写入来源 | 普通用户读取 | 原因 |
| --- | --- | --- | --- |
| `topup` (1) | 充值订单、兑换码等受控模板 | 保留原文 | 是用户账务事实；支付 IP、交易号等仍只在 `admin_info` |
| `consume` (2) | 文本和任务计费摘要、差额补扣 | 保留原文 | 是用户使用和计费说明 |
| `manage` (3) | 管理额度、绑定、游戏记录等应用内受控事件 | 保留原文 | 是面向该用户的管理事件；结构化管理员详情不公开 |
| `system` (4) | 注册奖励、签到、安全设置、旧任务退款等受控模板 | 保留原文 | 是面向该用户的系统事件和账务说明 |
| `error` (5) | relay 错误正文，可能包含上游拒绝、渠道或凭据 | 固定为 `Request failed.` | 原始诊断只供管理员读取 |
| `refund` (6) | 失败退款或实际额度低于预扣额度的差额退款 | 固定为 `Quota refunded.` | 不公开任意失败或重算原因，但保留退款额度事实 |
| 未知或未来类型 | 无已审计的稳定写入契约 | 空字符串 | 新日志类型必须先经过隐私审计才能公开正文 |

错误和退款的公开 `other.reason` 也不复制存储值，而是分别固定为 `request_failed` 和
`quota_refunded`。这两个稳定类别供 API/导出消费者判断事件；管理员接口仍返回原始
`content` 和原始 `other.reason`。投影创建新 `Log`，不修改数据库记录或管理员对象。

`other` 采用默认拒绝的字段清单，并校验布尔、数字、字符串、枚举字符串、字符串数组以及
可为字符串或数字的订阅 ID。保留当前写入链路产生的计费、token、缓存、音频、图片、
搜索、异步任务、订阅和请求格式字段。以下内容始终排除：

- `admin_info`、`root_info`、`audit_info` 及其所有嵌套内容；
- 渠道字段、原始 `reason`、`reject_reason`、`error`/`message`/`fail_reason` 等诊断别名、
  `stream_status`、`timing_diagnostics` 和参数覆盖审计 `po`；
- 未登记字段，以及已登记字段中类型不符合契约的对象或混合数组。

字符串数组逐元素验证；含 `null` 的数组也视为类型错误，不进入用户投影。

无法解析、`null` 或形状错误的旧 `other` 返回空对象。过滤使用 `json.RawMessage`，避免
将大整数经 `float64` 往返后改变字面值。管理员日志查询继续返回原始主记录和原始
`other`，普通用户投影通过新对象构造，不会修改管理员读取的记录。

### 写入方到公开计费字段

本轮补齐的是已有写入方已经持久化、但旧投影遗漏的标量事实；没有开放任意 map、object
或动态倍率透传。

| 写入方 | 字段 | 公开校验与语义 |
| --- | --- | --- |
| `service/text_quota.go` | `usage_semantic` | 只接受当前实际写入值 `anthropic`；缺失表示非 Anthropic 路径，其他字符串拒绝 |
| `service/text_quota.go` | `cache_write_tokens` | 有限 JSON 数字；归一化缓存写入 token 总数 |
| `service/text_quota.go` | `input_tokens_total` | 有限 JSON 数字；仅在非 Claude 且上游/转换已提供可靠总输入和 usage source 时写入 |
| `service/text_quota.go` | `cache_write_tokens_source` | 只接受 `upstream`、`explicit_zero`、`inferred_missing_field`、`inferred_untrusted_explicit_zero` |
| `service/text_quota.go` | `inferred_cache_write_tokens` | 有限 JSON 数字；缺字段策略推断的缓存写入量 |
| `service/text_quota.go` | `inferred_cache_write_billable` | JSON 布尔值；上述推断量是否参与计费 |
| 图片/任务计费写入方 | `n`、`size`、`resolution`、`seconds` | 有限 JSON 数字；实际数量、尺寸/分辨率倍率和时长 |

缓存推断三项属于用户计费可解释性，而不是渠道诊断，因此公开；它们只公开固定枚举、数字
和布尔值，不公开启用该策略的渠道 ID、渠道名称或策略对象。`OtherRatios`、`BillingMeta`
虽然由 map 生成，投影仍只接受逐项登记的键；例如任意 `dynamic_ratio` 或嵌套对象不会进入
普通用户结果。

### 兼容性与回滚

本修复不改变表结构、写入格式或历史记录。普通用户 API 中依赖错误原文、退款原始
`reason` 或未知类型 `content` 的客户端会改为收到上述稳定类别/通用说明，这是有意的隐私
边界变更；管理员 API 不变。代码回滚不需要数据迁移，但会重新暴露仍保存在历史行中的原始
诊断内容，因此不能把回滚视为隐私等价操作。

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
go test ./model -run 'Test(FormatUserLogsUsesStrictProjectionWithoutMutatingSource|ProjectUserLogOtherRejectsMalformedAndWrongShapes|ProjectUserLogOtherKeepsWriterDefinedBillingFacts|ProjectUserLogOtherAcceptsOnlyWriterDefinedEnumValues|ProjectUserLogContentUsesExplicitPerTypeContract|SensitiveErrorAndRefundDiagnosticsRemainAdminOnly|UserAndTokenReadsProjectWhileAdminReadRemainsComplete|SumUsedQuotaKeepsHistoricalQuotaAndRecentRatesWithFilters|SanitizeDBErrorNeverReturnsDriverOrWrappedValues|SanitizedGormWriterRemovesSyntheticSentinelValue|GormLoggerRedactsNormalSlowErrorAndDebugSQL)$' -count=1
go test -race ./model -run 'Test(SensitiveErrorAndRefundDiagnosticsRemainAdminOnly|UserAndTokenReadsProjectWhileAdminReadRemainsComplete|SumUsedQuotaKeepsHistoricalQuotaAndRecentRatesWithFilters|GormLoggerRedactsNormalSlowErrorAndDebugSQL)$' -count=1
go test ./...
```

重点断言包括真实写入格式的计费字段与大整数保留、未知/嵌套/错误形状删除、错误/退款
原始正文和所有诊断别名不进入用户、令牌或其导出 JSON、管理员结果和源对象完整、四类
SQL 日志均无合成 secret，以及历史额度与最近 RPM/TPM 同时正确。
