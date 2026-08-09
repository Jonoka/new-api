# 自引用渠道防护修复记录

日期：2026-07-24

## 问题

生产现场发现部分渠道的上游地址被误配置为当前站点自己的公开 API 域名，
例如面板反代域名或控制台公布的 API 域名。

这类渠道被调度命中后，请求会从网关重新回到网关自身，形成递归链路：

1. 客户端请求当前站点；
2. 调度命中误配置渠道；
3. 渠道请求当前站点公开 API 域名；
4. 当前站点再次鉴权、调度、限流；
5. 请求和 429 日志被放大。

## 根因

渠道保存和调度阶段都没有识别“上游地址指向当前站点”的配置。

当外部程序误添加这类渠道时，系统会把它当成普通上游参与调度。
如果该渠道继续请求本站 API 入口，就会制造自调用和重复限流。

## 修复方案

- 新增自引用域名识别：
  - 当前请求 `Host`；
  - 反代头 `X-Forwarded-Host`；
  - 反代头 `X-Original-Host`；
  - 系统设置 `ServerAddress`；
  - 控制台 API 信息中的公开 URL。
- 渠道保存层新增校验：
  - 新增或修改渠道时，如果上游地址指向当前站点，直接返回中文错误。
- 渠道调度层新增保护：
  - 如果缓存或数据库里已经存在自引用渠道，本次请求会跳过该渠道；
  - 被跳过的渠道会加入 `ExcludedChannelIDs`，避免同一次请求继续命中；
  - 如果当前优先级全是自引用渠道，会继续尝试下一优先级；
  - 日志记录中文说明，便于管理员定位误配置渠道。

## 兼容性

- 不新增数据库字段；
- 不改变渠道模型、分组、权重和优先级配置；
- 不影响真实外部上游域名；
- 兼容 SQLite、MySQL 和 PostgreSQL；
- 如果外部程序绕过后台直接写数据库，保存层无法阻止，但调度层仍会跳过。

## 验证计划

- 单元测试覆盖控制台 API 信息中的自引用域名；
- 单元测试覆盖系统地址中的自引用域名；
- 单元测试覆盖反代 Host 中的自引用域名；
- 单元测试覆盖外部上游不被误判；
- 单元测试覆盖自引用渠道会加入本次请求排除列表；
- 执行 `go test ./service -run "Test(ChannelBaseURLMatchesLocalEndpoint|ValidateChannelBaseURLNotSelf|ExcludeSelfReferentialChannel)" -count=1 -timeout 60s`；
- 执行 `go test ./controller -run TestExcludeChannelFromRetryPreservesControlledReuse -count=1 -timeout 60s`；
- 执行 `git diff --check`。

## 验证结果

- `go test ./service -run "Test(ChannelBaseURLMatchesLocalEndpoint|ValidateChannelBaseURLNotSelf|ExcludeSelfReferentialChannel)" -count=1 -timeout 60s`：通过；
- `go test ./controller -run TestExcludeChannelFromRetryPreservesControlledReuse -count=1 -timeout 60s`：通过；
- `go test ./service -count=1 -timeout 60s`：通过；
- `go test ./controller -count=1 -timeout 60s`：通过；
- `git diff --check`：通过。

## 回滚注意

如需回滚，只回滚本次代码即可，不涉及数据迁移。

回滚后应继续人工禁用或删除误配置渠道，否则仍可能再次出现递归请求和 429 放大。
