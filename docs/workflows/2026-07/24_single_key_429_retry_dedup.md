# 单 Key 渠道 429 重试去重

日期：2026-07-24

## 问题

生产现场发现渠道 `384` 在短时间内出现大量
`status_code=429, 您已达到请求数限制：1分钟内最多请求500次`。
日志里的 `admin_info.use_channel` 可出现 `384 -> 384 -> 384 -> 384`，
说明同一个请求在 429 后仍反复命中同一个渠道。

该 429 来自渠道上游 `https://api.maolaoapi.cc`，不是本机入口的 500。
当前自动重试状态码包含 429，符合“上游限流时切换候选渠道”的预期；
异常点在于单 Key 渠道也被当成可控复用对象，导致没有换到其它渠道。

## 根因

`excludeChannelFromRetry` 把所有 429 都视为“受控复用”，在单分组场景下不会把
当前渠道加入 `ExcludedChannelIDs`。这个设计原本为了保留 429 退避，但对单 Key
渠道没有换 Key 空间，只会在重试预算内重复撞同一个限流渠道并放大错误日志。

## 修复方案

- 429 仍然遵循管理员配置的自动重试状态码；
- 单 Key 渠道返回 429 后加入本次请求排除列表，下一次重试优先切换其它候选；
- 多 Key 渠道继续允许同渠道复用，用于同渠道内换 Key 或根据 `Retry-After` 退避；
- 跨组或显式多分组重试继续优先切换未尝试渠道。

## 兼容性

- 不改变 `RetryTimes`、渠道优先级、权重和分组顺序；
- 不改变指定渠道请求的“不重试”行为；
- 不涉及数据库结构或数据库专属 SQL，兼容 SQLite、MySQL 和 PostgreSQL；
- 如果单分组下只有一个单 Key 渠道，429 后会更快返回最终错误，不再重复刷同一渠道。

## 验证计划

- 单元测试覆盖单 Key 429 会排除当前渠道；
- 单元测试覆盖多 Key 429 仍允许同渠道复用；
- 单元测试覆盖跨组 429 仍排除当前渠道；
- 执行 `go test ./controller -run TestExcludeChannelFromRetryPreservesControlledReuse -count=1 -timeout 60s`；
- 执行 `git diff --check`。

## 验证结果

- `go test ./controller -run TestExcludeChannelFromRetryPreservesControlledReuse -count=1 -timeout 60s`：通过；
- `go test ./controller -run "Test(WriteRelayErrorResponseSkipsCommittedStreamAndCancellation|ShouldRetryWithReason|RemainingRelayRetriesUsesGlobalAttemptIndex|ExcludeChannelFromRetryPreservesControlledReuse|RepeatedChannelRetryDelay|ChannelRetryState)" -count=1 -timeout 60s`：通过；
- `go test ./controller -count=1 -timeout 60s`：通过；
- `go test ./types -count=1 -timeout 60s`：通过；
- `go test ./middleware -run TestBuildModelRequestRateLimitRuleUsesMostSpecificOverride -count=1 -timeout 60s`：通过；
- `git diff --check`：通过。
