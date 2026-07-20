# 渠道可观测性中心设计

状态：已确认
日期：2026-07-21
范围：管理员侧渠道可观测性，不替代完整用户用量或财务分析中心

## 1. 背景

现有 `channel-quality` 是上传即用的静态扩展，主要读取渠道列表，并调用渠道测试接口。它展示的是渠道配置状态、余额和最近一次主动测试耗时，不是生产请求的真实统计。

这次重构把模块定位调整为“渠道可观测性中心”，用于回答以下问题：

- 当前真实业务请求是否正常；
- 哪个渠道、哪个模型正在失败或变慢；
- 每个渠道和模型分别承载了多少调用、Token、缓存和计费额度；
- 错误集中在哪些状态码、请求阶段和重试链路；
- 主动探测结果与真实业务流量是否一致。

设计参考 Sub2API 和 CPA Manager Plus 的信息架构，但不复制其旁路采集服务或 PostgreSQL 专属实现。new-api 已经拥有渠道实体、请求上下文、用量结算和日志系统，应直接在主程序内建立稳定的渠道指标事实源。

## 2. 设计目标

### 2.1 目标

- 提供“全局汇总 -> 渠道 -> 渠道内模型 -> 请求明细”的稳定下钻路径；
- 同时提供客户端请求结果和上游渠道尝试结果，且不混淆二者；
- 结构化记录状态码、耗时、Token、缓存和重试信息；
- 真实流量与主动探测分域展示；
- 聚合查询兼容 SQLite、MySQL 5.7.8+ 和 PostgreSQL 9.6+；
- 在多节点部署中获得最终一致、可解释的统计；
- 保持扩展为轻量 `static` 模块，不增加独立 Manager Server；
- 对历史数据缺失和日志明细不可用提供明确覆盖提示。

### 2.2 非目标

- 首期不替代现有用户用量日志、财务结算或账单中心；
- 首期不实现 API Key、用户、认证文件等完整排行体系；
- 首期不实现自动禁用、自动切流或告警策略；
- 首期不把异步任务最终完成状态混入同步请求成功率；
- 不承诺与上游供应商账单完全一致；
- 不引入独立采集服务、外部消息队列或外部分析数据库。

## 3. 核心统计口径

### 3.1 客户端请求与渠道尝试

一次客户端请求可能经历多个上游渠道尝试。例如渠道 A 失败后重试到渠道 B 并成功：

- 客户端请求数：1；
- 客户端成功数：1；
- 上游尝试数：2；
- 渠道 A 尝试失败数：1；
- 渠道 B 尝试成功数：1。

因此系统必须同时提供：

- 客户端成功率 = 最终成功请求数 / 排除客户端主动取消后的最终请求数；
- 尝试业务成功率 = 成功渠道尝试数 / 排除客户端主动取消后的渠道尝试数；
- 渠道质量成功率 = 渠道成功样本数 / `quality_eligible=true` 的渠道样本数；
- 重试率 = 非首次尝试数 / 渠道尝试数。

全局总览展示客户端成功率和渠道质量成功率，并允许查看尝试业务成功率。渠道和渠道内模型默认展示渠道质量成功率，避免客户端参数错误、网关本地错误或结算错误被误归因给上游渠道。

采集事件显式区分三个 scope：

- `final_request`：每个客户端逻辑请求恰好一条最终结果；
- `channel_attempt`：每次外层选定渠道恰好一条尝试结果；
- `upstream_call`：每次底层 HTTP transport 调用恰好一条结果。

三个 scope 可以进入同一套指标桶，但 API 必须显式选择 scope。所有实时样本都带 `data_origin=live`，历史回填样本带 `data_origin=legacy`，该字段必须参与指标桶维度和哈希。Token、缓存和额度只计入最终成功且已完成结算的 `channel_attempt`，避免跨 scope 重复求和。页面将其标注为“已成功结算用量”，不解释为供应商实际承载的全部 Token。渠道调用量默认使用 `channel_attempt`；客户端成功率使用 `final_request`；完整上游状态码分布使用 `upstream_call`。

### 3.2 业务结果与 HTTP 状态码

HTTP 2xx 不等于业务成功。流式请求可能已经向客户端返回 200，但随后发生上游中断。统计中分别记录：

- `outcome`：`success`、`http_error`、`transport_error`、`protocol_error`、`stream_error`、`local_error`、`dispatch_error`、`client_cancelled`；
- `partial_response`：失败前是否已经向客户端发送了部分响应，不再把“部分失败”作为与错误原因重叠的 outcome；
- `failure_owner`：`channel`、`client`、`gateway`、`unknown` 或空值；
- `quality_eligible`：该样本是否进入渠道质量成功率分母；
- `client_status_code`：最终返回给客户端的状态码；
- `upstream_status_code`：底层 transport 收到的原始上游状态码；
- `normalized_status_code`：解析上游嵌套错误后的规范状态码；
- `error_stage`：鉴权、选渠、连接、上游响应、流式传输、解析、结算等阶段；
- `*_status_present`：对应状态码字段是否适用于当前 scope；
- `upstream_status_present=true && upstream_status_code=0`：实时 transport 网络错误、超时或其他没有 HTTP 响应的情况。

只有 `outcome=success` 计入业务成功数，状态码仅用于协议层分布和排障。由客户端主动断开造成的 `client_cancelled` 单独统计，默认不进入三种成功率分母；上游密钥失效、上游限流、上游超时、上游断流和上游协议错误归属 `failure_owner=channel` 且进入渠道质量分母；无效客户端参数归属 `client`，本地转换或结算失败归属 `gateway`，两者不进入渠道质量分母。无法可靠分类的错误归属 `unknown`，单独披露且默认不进入渠道质量分母，避免静默污染或美化渠道质量。

outcome 判定优先级固定为：客户端主动取消；流式传输错误；transport 错误；HTTP 错误；协议解析错误；网关本地或派发错误；成功。`partial_response` 只描述响应是否已经开始。`upstream_call` 与父级 `channel_attempt` 分别判定 outcome：例如响应头为 200、随后断流时，两者都记录 `stream_error`，同时保留原始上游码 200。

### 3.3 模型口径

每条样本同时保留：

- `requested_model`：客户端请求的原始模型；
- `upstream_model`：经过模型映射后实际请求上游的模型。

渠道主表默认按 `requested_model` 展示，并允许切换到上游模型口径。发生映射时，请求明细同时展示两者。

### 3.4 缓存口径

不同上游对输入 Token 是否包含缓存 Token 的定义不同。采集前统一归一化为：

- `input_tokens_total`：包含缓存部分的总输入；
- `uncached_input_tokens`：未命中的普通输入；
- `cache_read_tokens`：缓存读取；
- `cache_write_tokens`：缓存创建或写入；
- `output_tokens`：输出 Token。

页面分别展示：

- 缓存命中请求率 = 有缓存读取的成功请求数 / 有有效用量的成功请求数；
- 缓存 Token 命中率 = 缓存读取 Token / 归一化总输入 Token。

两种比率必须明确命名，不使用含义不清的单一“缓存命中率”。

### 3.5 流量来源

所有指标样本带 `traffic_source`：

- `relay`：真实同步转发流量；
- `probe`：手工或定时渠道探测；
- `task`：异步任务提交或完成事件；
- `playground`：后台调试请求。

所有业务统计默认只包含 `relay`。主动探测只进入“主动探测”视图，不能参与业务成功率、调用量和费用统计。

所有指标样本还带 `data_origin`：实时原生采集固定为 `live`，可选历史回填固定为 `legacy`。业务成功率、渠道质量和状态码默认只读取 `live`；调用量、Token 和额度趋势可以由管理员显式加入 `legacy`，并同时显示覆盖范围。

## 4. 总体架构

系统分为四条互相独立、可以关联的链路：

```text
真实渠道尝试
  -> 渠道指标采集器
  -> 内存热桶
  -> LOG_DB GORM 增量 Upsert
  -> 渠道指标聚合 API
  -> 渠道可观测性扩展

失败渠道尝试
  -> 最小化脱敏失败事件
  -> LOG_DB 短期保留
  -> 失败下钻 API
  -> 渠道可观测性扩展

现有消费/错误日志
  -> 日志深链与可选历史回填
  -> 渠道可观测性扩展

手工/定时渠道测试
  -> 现有渠道测试能力
  -> 独立的主动探测视图
```

聚合和近期失败明细的正确性不依赖 `LogConsumeEnabled` 或 `ErrorLogEnabled`。日志开关只影响现有用量日志深链和旧历史可恢复范围。

## 5. 渠道指标采集

### 5.1 插桩生命周期

每次选定渠道后开始一个新的尝试计时，不能继续使用整条客户端请求的总开始时间。主 Relay 在 `getChannel` 成功、请求 body 完成复位后创建尝试，在对应的 Claude、Gemini、WSS 或通用 relay helper 返回后立即完成该尝试。

尝试生命周期为：

1. 选定渠道并完成请求上下文设置；
2. 初始化请求内单调递增的尝试序号、渠道快照、开始时间和模型信息；
3. 每次进入底层 HTTP transport 时创建 `upstream_call`；`client.Do` 返回时只捕获映射前的原始上游状态码和响应头耗时，网络错误可立即以状态码 0 完成；
4. 请求上游并等待当前 helper 返回；
5. 非流式响应在正文读取、错误归一化和协议解析结束后由适配器显式完成 `upstream_call`；流式响应在扫描器完成 `StreamStatus` 与软错误判定后显式完成；受观测的 `ReadCloser` 只记录 EOF、读取错误和提前 Close 等正文证据，不能在 EOF 时提前判定成功；
6. 失败时先完成重试决策，再以 `retry_planned` 完成失败尝试；该字段表示已经作出的重试计划，不保证下一次选渠一定成功；
7. 成功时在用量和计费信息可用后完成成功尝试；
8. 流式请求还必须检查本次尝试独立的 `StreamStatus`，只有正常结束且没有软错误才算成功，不能只用 helper 是否返回 nil 判断；
9. `channel_attempt` 样本完成后保持不可变，不在其中写入事后才能确定的“最终尝试”标记；外层 finalizer 通过最大已启动 `attempt_seq` 生成 `last_started_attempt_seq`，并为请求内暂存的失败明细标记 `is_last_started_attempt`；
10. 客户端 `final_request` 结果由外层 finalizer 在响应和错误处理完成后恰好记录一次，不能直接使用某次尝试结束时的 `c.Writer.Status()`。

同一渠道尝试内部可能发生多个底层上游调用，例如 Responses continuation fallback 的二次请求。它们共享父级 `channel_attempt`，但各自记录 `upstream_call`，避免重复增加渠道尝试数，同时保留完整状态码和 transport 调用量。`upstream_call` 的 `success` 代表该次 transport、正文消费和协议处理正常完成，不代表整个客户端请求最终成功。helper 边界的 defer 必须关闭所有仍未完成的 call：已有读取错误时按对应错误完成；只有 EOF 或 Close、但适配器没有提交解析结果时按 `protocol_error` 和 `error_stage=unfinalized_call` 完成，绝不能兜底记为成功。

尝试序号不能直接使用现有 RetryIndex：跨组选择可能重置 retry 值，也可能重新使用已经尝试过的渠道。采集器使用独立的 `request_id + attempt_seq` 标识父级尝试，并使用 `call_index` 标识其内部上游调用。

每次 `channel_attempt` 必须创建独立流状态。整条 `RelayInfo` 上累计的响应数量不能直接作为单次尝试结果，否则重试后的流状态会互相污染。

现有 `perf_metrics` 保留当前“模型广场最终请求性能”语义，新渠道采集器不修改其表结构或 API。

### 5.2 样本内容

单次 `channel_attempt` 样本至少包含：

```text
metric_scope=channel_attempt / request_id / attempt_seq / retry_planned
channel_present / channel_id / channel_name_snapshot / channel_type
requested_model_present / requested_model
upstream_model_present / upstream_model / group
traffic_source / data_origin=live / stream / outcome
failure_owner / quality_eligible / partial_response
error_stage / upstream_started / stream_end_reason
latency_ms / ttft_ms
input_tokens_total / uncached_input_tokens / output_tokens
cache_read_tokens / cache_write_tokens
charged_quota / charged_micro_usd
```

单次 `upstream_call` 样本至少包含：

```text
metric_scope=upstream_call / request_id / attempt_seq / call_index
channel_present / channel_id / channel_name_snapshot / channel_type
requested_model_present / requested_model
upstream_model_present / upstream_model / group
traffic_source / data_origin=live / stream / outcome / partial_response
upstream_status_present / upstream_status_code
normalized_status_present / normalized_status_code
error_stage / response_header_ms / latency_ms
```

单次 `final_request` 样本至少包含：

```text
metric_scope=final_request / request_id / retry_count
requested_model_present / requested_model
final_upstream_model_present / final_upstream_model / group
traffic_source / data_origin=live / stream / outcome / partial_response
client_status_present / client_status_code
final_channel_present / final_channel_id / final_channel_type
last_started_attempt_present / last_started_attempt_seq
total_latency_ms
```

不保存请求正文、响应正文、API Key 或完整错误 body。错误详情继续由现有脱敏日志系统负责。

### 5.3 成功样本与结算解耦

Token、缓存和费用在结算阶段才能完整获得，而尝试结果由转发控制器决定。实现时采用“附加用量、统一完成”的方式：

- 结算服务只把归一化用量和额度附加到当前尝试上下文；
- 转发控制器在 helper 返回并完成重试判断后，负责恰好完成一次成功或失败的 `channel_attempt` 样本；
- transport 层负责创建每个 `upstream_call`、捕获原始状态码并记录正文读取证据；适配器或流扫描器在解析和流状态判定后显式完成样本，helper 边界负责将遗漏的未完成调用以非成功结果兜底关闭；
- 外层 finalizer 负责恰好完成一次 `final_request` 样本；
- 没有用量的成功响应仍记录请求和延迟，Token 字段为 0；
- 流式中断按失败记录，即使客户端状态码已经是 200。

该约束避免在多个结算函数中重复计数。

实现前必须确认请求上下文能稳定取得当前 `RelayInfo`。如果重试判断仍通过未设置的字符串键读取 `relay_info`，应先统一为正式上下文常量并在生成 RelayInfo 后写入，否则流式响应开始后的错误可能被错误重试，也会破坏指标归因。

选渠成功但尚未发起 HTTP 请求时发生的转换或参数错误，记录为 `outcome=local_error`、`error_stage=pre_upstream` 并设置 `upstream_started=false`。它可以进入尝试业务失败分析，但 `quality_eligible=false`，也不进入上游 HTTP 状态码分母。选渠前发生的本地错误不创建 `channel_attempt`。

通用 HTTP transport 可以覆盖大部分 OpenAI、Claude、Gemini 兼容渠道，但 WebSocket 握手、AWS SDK、讯飞和部分旧任务渠道可能绕过该边界。实现时维护按渠道类型和 transport 的覆盖矩阵：WebSocket 成功握手记录 101；绕过通用 transport 的适配器必须增加专用 `upstream_call` 插桩，尚未覆盖的渠道在 API 中标记为 partial，不能以零调用或 100% 成功伪装。

## 6. 聚合数据模型

渠道指标桶和最小化失败事件都存放在 `LOG_DB`。当 `LOG_SQL_DSN` 独立配置时，迁移、方言判断、事务和查询必须使用日志库连接本身，不能误用主库的全局 PostgreSQL/MySQL 标记，也不能跨 `DB` 与 `LOG_DB` 做 JOIN。需要当前渠道元数据时，先从主库批量读取，再在服务层内存合并；历史名称以指标快照为准。

新模型使用接收 `*gorm.DB` 参数的独立日志事实表迁移函数，并在 `InitLogDB` 已经确定最终 `LOG_DB` 句柄后由主节点执行。`LOG_SQL_DSN` 为空时只对 `LOG_DB=DB` 执行一次；配置独立日志库时只对独立 `LOG_DB` 执行，不把这些表无条件加入更早执行的主库普通迁移或当前没有调用点的快速迁移列表。所有方言分支以实际 `LOG_DB.Dialector.Name()` 为准。

### 6.1 渠道指标桶

新增独立聚合表，建议模型名为 `ChannelMetricBucket`，表名为 `channel_metric_buckets`。

维度包括：

```text
bucket_level / bucket_ts / dimension_hash / metric_scope
channel_present / channel_id / channel_name_snapshot / channel_type
requested_model_present / requested_model / requested_model_hash
upstream_model_present / upstream_model / upstream_model_hash / group
traffic_source / data_origin / stream / outcome / error_stage
failure_owner / quality_eligible / partial_response
client_status_present / client_status_code
upstream_status_present / upstream_status_code
normalized_status_present / normalized_status_code
```

`dimension_hash` 由除时间桶外的完整维度计算。哈希输入包含版本前缀、字段适用性标记和固定字段顺序，使用固定宽整数与带长度前缀的字符串编码，禁止直接 `strings.Join` 或无类型 map JSON，避免“不适用”、空值、状态码 0 和分隔符产生歧义。使用完整 SHA-256，并以 `varchar(64)` 小写十六进制字符串保存；同时保存 `dimension_version=1`，便于三种数据库兼容、演进和人工排查。

模型名是客户端可控高基数字段。采集器对完整原始模型名计算 `*_model_hash`，展示快照按 UTF-8 安全边界限制长度并附加短哈希后缀；精确过滤同时比较模型哈希和展示快照。指标身份使用模型哈希，不使用截断后的展示值。每个节点和时间桶设置最大活跃维度数，超过上限的新维度合并到明确的 `__other__` 溢出桶，并增加 `dimension_overflow_count`；API 必须将这种情况标记为 `partial`。

唯一索引为：

```text
(bucket_level, bucket_ts, dimension_hash)
```

这样可以避免 MySQL 5.7 在多个 UTF-8 字符串复合索引上的长度风险。维度原值仍保存在普通列中，用于过滤和分组。

Flush 对尚未验证的 `dimension_hash` 分批读取已有明文维度并比较，进程内缓存已验证映射；若发现同一哈希对应不同维度，则隔离该批次、记录数据质量错误并停止该哈希的累加，禁止碰撞时静默合并。测试和诊断工具也必须覆盖该保护。

建议仅增加以下组合索引，避免为每个筛选字段盲目建索引：

```text
(bucket_level, metric_scope, traffic_source, data_origin, bucket_ts)
(bucket_level, metric_scope, traffic_source, data_origin, channel_id, bucket_ts)
(bucket_level, metric_scope, traffic_source, data_origin, requested_model_hash, bucket_ts)
(bucket_level, metric_scope, traffic_source, data_origin, upstream_status_present, upstream_status_code, bucket_ts)
```

`bucket_level`、`metric_scope`、`traffic_source`、`data_origin`、`outcome` 和 `failure_owner` 使用有明确上限的短字符串列；模型筛选索引只使用固定 64 字符哈希，不直接索引原始模型名，从而避开 MySQL 5.7 的 UTF-8 复合索引长度限制。`group` 查询使用日志库对应的保留字引用方式。

计数和求和字段包括：

```text
event_count / success_count / non_first_attempt_count / retry_planned_count
quality_eligible_count / quality_success_count / partial_response_count
usage_sample_count / cache_hit_request_count
input_tokens_total / uncached_input_tokens / output_tokens
cache_read_tokens / cache_write_tokens
charged_quota / charged_micro_usd
latency_sum_ms / latency_count / ttft_sum_ms / ttft_count
latency_histogram_buckets... / dimension_overflow_count
dropped_metric_event_count / dropped_failure_event_count
```

`charged_quota` 使用 int64 保存请求结算时的原始额度；`charged_micro_usd` 使用请求当时换算得到的整数 micro-USD 快照，禁止累计 float。页面明确标注这是用户结算金额，不是上游供应商实际成本。

### 6.2 最小化失败事件

新增 `ChannelFailureEvent`，表名建议为 `channel_failure_events`。它只在 `channel_attempt` 失败或客户端取消时写入，用于独立于现有日志开关的近期失败下钻，不复制完整消费日志。

字段至少包括：

```text
event_id / created_at / request_id / attempt_seq
retry_planned / is_last_started_attempt / causal_call_index
channel_id / channel_name_snapshot / channel_type
requested_model / upstream_model / group / traffic_source
outcome / failure_owner / quality_eligible / partial_response
error_stage / stream_end_reason
upstream_status_present / upstream_status_code
normalized_status_present / normalized_status_code
client_status_present / client_status_code
latency_ms / ttft_ms / retry_reason
masked_error_summary
```

一个尝试包含多个 `upstream_call` 时，失败事件只保存明确标记为根因的 `causal_call_index` 及其状态码；无法确定根因时状态码保持不适用，不能随意取最后一次调用。失败尝试先作为不可变草稿保存在请求内有界列表中，外层 finalizer 再标记最大已启动序号对应的 `is_last_started_attempt`。只有该失败确实是最后已启动尝试时才附加最终客户端状态码；非最终失败的客户端码保持不适用，避免把重试后的 200 错记到前一个失败渠道。

完成后的失败事件进入有界异步批次并以稳定 `event_id` 做唯一去重；队列满或重试上限耗尽时允许丢弃明细，但不能丢失对应聚合计数，并必须累计 `dropped_failure_event_count` 供 API 披露。该表不保存请求正文、响应正文、用户 Prompt、API Key 或完整错误 body。错误摘要复用现有敏感信息遮罩并限制长度。默认保留 14 天，可通过设置调整；清理采用按时间索引的小批量删除，不能阻塞主请求。

### 6.3 Flush 幂等记录

新增轻量 `ChannelMetricFlush`，表名建议为 `channel_metric_flushes`，至少保存唯一 `flush_id`、节点启动实例 ID、批次创建时间和提交时间。每次 drain 生成稳定的随机 `flush_id`，在同一个 `LOG_DB` 事务中先插入去重记录，再对指标桶执行增量 Upsert 并插入失败事件。

如果提交已经成功但节点只收到超时，使用同一 `flush_id` 重试时会命中去重记录并跳过整批增量，避免重复累计。事务回滚时去重记录与指标增量一同回滚。Flush 记录按大于最大重试窗口的保留期小批量清理。

### 6.4 延迟百分位

为兼容三种数据库，不使用 `percentile_cont` 等数据库专属函数。每个指标桶保存固定延迟直方图计数，建议边界为：

```text
100ms / 250ms / 500ms / 1s / 2s / 4s / 8s
15s / 30s / 60s / 120s / 300s / +Inf
```

延迟和 TTFT 各保存一组非累计分箱列，每个样本只增加一个分箱。API 合并直方图后计算近似 P50、P95 和 P99，并返回对应桶上界。页面和接口文档说明百分位为固定桶近似值；没有样本时返回 null，不能以 0ms 冒充有效延迟。

### 6.5 写入策略

- 请求线程只更新分片内存计数，不同步写数据库；
- 内存热桶使用完整、可比较的维度 struct 作为 key，持久化时才计算 `dimension_hash`；
- 进入异步刷新或队列的 Sample 必须是不可变值，不能持有会在重试过程中继续修改的 `RelayInfo` 指针；
- 桶时间使用 Unix UTC 秒按桶宽向下取整，查询统一采用 `[start_timestamp, end_timestamp)` 半开区间；
- 后台刷新任务每 15 至 30 秒 `drain` 热桶，包括尚未闭合的当前桶，并为不可变批次生成稳定 `flush_id`；
- 在一个 `LOG_DB` 事务中使用 GORM `clause.OnConflict` 完成 flush 去重、增量 Upsert 和失败事件去重，所有计数和求和列使用 `table.column + ?`；
- 每个节点独立刷新自己的增量，数据库唯一键负责合并；
- API 以数据库为主数据源，允许最多一个刷新周期的延迟；
- 刷新返回失败时保留原不可变批次并以同一 `flush_id` 重试；只有确认事务未提交且决定放弃批次时，才把计数与并发产生的新计数相加恢复到热桶；
- 未提交批次、热桶数量和内存字节数都有硬上限；持续数据库故障超过上限时优先合并同维度计数，再降级到 `__other__`，最终仍无法容纳时才丢弃最旧指标并累计 `dropped_metric_event_count`，API 必须把对应区间标记为不完整；
- 正常关闭时执行一次有超时上限的最终 drain；进程异常退出最多损失一个刷新周期的数据，接口通过数据质量字段披露最后刷新时间、维度溢出、指标丢弃和失败明细丢弃数。

首期不把 Redis 作为计数事实源，避免本地内存、Redis 与数据库三路相加造成重复计数。Redis 仅可用于 5 至 15 秒的查询结果缓存，并通过版本号或标签失效。后续如需秒级实时性，再增加带所有权协议的 Redis 热桶。

实时热桶刷新使用“带 flush 去重的增量 Upsert”；降采样和指定时间窗重算使用“覆盖 Upsert”。两种写入方法必须分开实现，禁止降采样复用增量写入导致重复累计。

首期优先使用与现有 `PerfMetric` 相同的单行增量 Upsert，降低三种方言差异。后续如增加批量累加，PostgreSQL/SQLite 使用 `excluded.column`，MySQL 5.7 使用 `VALUES(column)`，并按日志库方言显式分支；不能假设 GORM 会自动改写嵌套算术表达式。

### 6.6 保留与降采样

建议默认保留策略：

- 5 分钟桶：7 天；
- 1 小时桶：180 天；
- 1 天桶：730 天。

自动粒度建议为：48 小时以内使用 5 分钟桶，90 天以内使用小时桶，其余使用天桶。查询必须只选择一个 `bucket_level`，禁止叠加不同粒度造成重复计数；响应通过 `data_end_ts` 明确该粒度已经覆盖到的最后一个闭合源桶。

降采样任务使用 GORM 读取闭合的源桶，在 Go 中合并，再覆盖 Upsert 到更粗粒度，避免数据库专属聚合语法。当前小时或天可以由已经闭合的源桶形成明确标记的部分目标桶，并随新源桶重复覆盖；只有完整目标桶才能作为细粒度清理依据。任务保留至少两个源桶宽的延迟窗口，清理只删除已经完成降采样、完成校验且超过保留期的细粒度桶。

首个后端里程碑可以只启用 5 分钟桶和可配置保留期，用于验证采集正确性；面向管理员发布带“最近 30 天”预设的新模块前，必须完成小时桶降采样。天级降采样可以后续交付，未实现的粒度和超出可靠覆盖的时间预设不得在页面中伪装为可用。

## 7. 请求明细与历史数据

### 7.1 实时失败下钻

近期失败列表以 `channel_failure_events` 为主数据源，因此不受错误日志开关影响。现有消费和错误日志继续提供更完整的管理员深链；对应日志不存在时只隐藏深链，不降低聚合完整性。若失败事件有队列丢弃，接口必须显示对应计数和受影响时间范围，不能把近期明细宣称为完整。

成功请求明细仍由现有消费日志页面承担，本模块首期不复制完整成功请求事件。

### 7.2 历史回填

历史回填是可选、可暂停、可恢复的管理员操作，不在启动迁移中自动扫描大日志表。

回填规则：

- 消费日志可回填成功请求、Token、缓存、额度和耗时；
- 错误日志只回填能够从现有字段可靠识别的最终失败样本和状态码，不能假装恢复旧请求的完整重试链或每次 transport 调用；
- `other` 统一在 Go 中通过 `common.Unmarshal` 解析，不在 SQL 中使用数据库专属 JSON 函数；
- 主动模型测试和 Playground 请求必须排除或单独标记；
- 历史成功日志没有真实状态码时设置 `status_present=false`，记录为“成功但状态码未知”，不能伪造为 200；
- 未开启错误日志的历史区间不能恢复真实失败数；
- 回填数据统一标记 `data_origin=legacy`，实时采集标记 `data_origin=live`；
- 固定 `live_cutover_ts` 和回填开始时的 `max_log_id`，按日志 ID 游标分批处理，避免与实时数据重叠；
- 回填任务使用独立 `channel_metric_backfill_jobs` 状态表，至少保存任务 ID、状态、`live_cutover_ts`、`max_log_id`、当前游标、最近错误和更新时间；checkpoint 与对应批次聚合写入同一个 `LOG_DB` 事务，保证断点续跑和幂等；
- 状态码未知统一使用 `status_present=false`；只有实时 `upstream_call` 的 `status_present=true && status_code=0` 表示没有 HTTP 响应；
- 成功率、状态码和渠道质量默认只读取 `live`，历史回填主要用于调用量、Token、缓存和额度趋势；
- 回填结果携带来源和覆盖范围，不能与原生实时采集伪装成同等精度。

## 8. API 设计

新增管理员接口组 `/api/channel-analytics`，统一使用 `AdminAuth`。

建议接口：

```text
GET /api/channel-analytics/summary
GET /api/channel-analytics/trend
GET /api/channel-analytics/channels
GET /api/channel-analytics/channels/:id/models
GET /api/channel-analytics/status-codes
GET /api/channel-analytics/failures
GET /api/channel-analytics/filters
```

公共过滤参数：

```text
start_timestamp / end_timestamp / granularity
channel_ids / channel_types / groups
requested_models / upstream_models
outcome / client_status_codes / upstream_status_codes
stream / traffic_source / data_origin
page / page_size / sort_by / sort_order
```

接口响应统一包含数据质量元数据：

```text
generated_at / reliable_from_ts / data_start_ts / data_end_ts
last_flushed_at / bucket_level / bucket_seconds
partial / detail_available / uncovered_channel_types
dimension_overflow_count / dropped_metric_event_count
dropped_failure_event_count
```

设计约束：

- 汇总、趋势、渠道表和状态码使用同一过滤器解析器，但由接口显式选择 scope 并生成 scope 专属查询，不能把不适用的过滤条件静默忽略；
- `/trend` 和 `/status-codes` 可以接受受限的 `metric_scope` 选择器；汇总、渠道和模型接口由服务端固定组合所需 scope，传入不支持的 scope 或过滤器时返回 400；
- `final_request` 支持请求模型、分组、来源、流式、outcome 和客户端状态码；渠道过滤表示“最终使用的渠道”，上游模型表示最终渠道实际使用的模型；没有成功选渠时这些字段为不适用；
- `channel_attempt` 支持渠道、渠道类型、请求模型、上游模型、分组、来源、流式、outcome、错误归属和错误阶段，不提供客户端或完整上游状态码分布；
- `upstream_call` 支持渠道、渠道类型、请求模型、上游模型、分组、来源、流式、outcome、错误阶段和上游状态码，不提供客户端状态码；
- 总览接口内部执行多个固定 scope 的聚合并在服务层组合，任何只适用于单一 scope 的过滤器都必须按上述语义应用或返回 400；
- 渠道列表先返回渠道汇总，模型子行按展开操作延迟加载；
- 状态码支持 `2xx`、`4xx`、`5xx` 分类及具体状态码；
- `status_present=false` 显示为“未知/不适用”；上游 `status_present=true && status_code=0` 显示为“无响应”，不并入 5xx；
- 失败明细只返回脱敏摘要，不返回完整上游响应体；
- `channel_ids` 等列表参数限制最多 100 项，分页大小限制最多 100；
- 模型过滤默认精确匹配，需要模糊搜索时使用独立搜索参数；
- 排序字段使用服务端白名单，禁止把前端字段直接拼入 SQL；
- 聚合分页总数使用跨库兼容的 GROUP 子查询，只依赖 SUM、COUNT、COALESCE、GROUP BY 和 HAVING；
- 分页、排序和时间范围设置上限，避免后台页面触发无界查询；
- 所有查询只读取一个 `bucket_level`，响应明确返回实际采用的粒度。

## 9. 扩展页面设计

### 9.1 模块边界

保留稳定模块 ID `channel-quality`，运行时继续使用：

```text
runtime.type = static
runtime.static_dir = public
ui.pages[].path = /
permissions.roles = [admin]
```

模块只负责展示和调用宿主 API，不保存服务端状态，不增加 `server.mjs` 运行要求。实现时增加宿主最低版本约束，并将模块版本提升到新的次版本。

静态文件建议拆分为：

```text
public/index.html
public/app.css
public/app.js
```

不加载外部 CDN，不携带 `node_modules`。如需要图表能力，优先采用少量本地代码或体积可控的本地静态依赖。

### 9.2 全局筛选

页面顶部固定提供：

- 最近 1 小时、今天、昨天、最近 7 天、最近 30 天、自定义；
- 自动、5 分钟、小时、天粒度；
- 渠道、渠道类型、请求模型、上游模型；
- 成功/失败、状态码、流式、流量来源；
- 搜索和刷新。

所有视图共享同一筛选状态。筛选变化只重新请求当前视图需要的数据。

### 9.3 四个视图

#### 总览

展示：

- 客户端请求数、上游尝试数；
- 底层上游调用数；
- 客户端成功率、渠道质量成功率，并可查看尝试业务成功率；
- 失败尝试、重试率；
- 输入、输出、缓存读、缓存写 Token；
- 缓存请求命中率、缓存 Token 命中率；
- 计费额度或换算金额；
- 平均延迟、近似 P95；
- 请求、失败、Token、缓存和状态码趋势。

#### 渠道与模型

主表为“渠道汇总行 + 可展开模型子行”，建议列为：

```text
渠道 / 调用与重试 / 成功率 / 状态码
输入输出 / 缓存读写 / 缓存命中 / 延迟 / 费用 / 最近失败
```

点击渠道、模型或状态码会同步更新趋势和失败明细过滤器。

#### 状态码与失败

展示：

- 客户端状态码与上游状态码切换；
- 状态码类别和具体码分布；
- 错误阶段分布；
- 最近失败请求的请求 ID、渠道、请求模型、上游模型、尝试序号、重试链、状态码、耗时和脱敏摘要；
- 跳转到现有用量日志的深链入口。

#### 主动探测

保留现有手工渠道测试能力，并新增明确说明：

- 探测结果只回答“当前能否连通”；
- 探测成功率不等于真实业务成功率；
- 探测 Token 和费用不进入真实业务统计；
- 每个渠道允许选择模型运行测试，而不是固定使用模型列表第一项。

### 9.4 页面状态

静态页面必须具备：

- 默认可见的骨架或加载状态；
- 无数据空态；
- 接口失败错误块和重试按钮；
- 聚合未启用、历史覆盖不足、日志明细关闭等数据质量提示；
- 桌面和移动布局；
- 防止动态文本、图例和筛选器互相遮挡的稳定尺寸约束。

## 10. 安全与权限

- 所有聚合与失败明细接口使用 `AdminAuth`；
- 静态扩展请求沿用当前登录态和 `New-Api-User` 请求头；
- 不在模块中保存 root token、渠道密钥或服务账号密钥；
- 渠道名称属于管理员可见信息，请求正文和完整错误 body 不进入指标表；
- 失败摘要继续使用现有敏感信息遮罩；
- 历史回填、重算和清理等写操作如果实现，仅允许 root，并需要显式确认和审计。

## 11. 异常与边界处理

- 渠道在统计期内被改名或删除时，使用渠道快照保留历史可读性；
- 模型映射变化时，请求模型和上游模型双维度都可追溯；
- 没有 Token 的成功请求仍计入请求和成功率，不计入缓存分母；
- 没有延迟样本时平均值和百分位返回 null，不返回 0ms；
- 流式 200 后中断按失败结果记录，同时保留 200 状态码；
- 客户端主动断开单独记录，默认不进入客户端、尝试业务或渠道质量成功率分母；
- 选渠前发生的本地错误不归因到具体渠道；
- 无可用渠道时只记录 `final_request` 的 `dispatch_error`，不伪造 channel_id=0 的渠道尝试；
- 每次失败重试分别归因，最终成功不抹掉之前渠道的失败；
- 异步任务首期仅统计提交接口，不将提交成功解释为任务最终成功；
- 数据库刷新失败不影响主请求响应。

## 12. 测试策略

### 12.1 后端单元测试

- 单次成功、最终失败、失败后重试成功；
- 单次渠道尝试包含多个底层上游调用；
- 响应头状态捕获与正文、解析、流结束后的 `upstream_call` 恰好一次完成；
- EOF 只记录正文证据，解析未提交的调用由 helper 兜底为非成功；
- 流式 200 后中断；
- 客户端主动断开与上游断流分类；
- 客户端、渠道、网关和未知错误归属及 `quality_eligible` 口径；
- 不适用状态、未知状态与无 HTTP 响应状态码 0；
- 请求模型与上游模型映射；
- 不同 Provider 的缓存归一化；
- 主动测试和 Playground 默认排除；
- 热桶 drain、失败恢复和重复完成防护；
- 提交成功但调用方超时后使用同一 `flush_id` 重试不会重复累计；
- 延迟直方图与 P95 计算；
- 哈希版本、空值、Unicode、超长模型名和分隔符歧义；
- 活跃维度达到上限后进入 `__other__` 且数据质量标记为 partial；
- 持续 LOG_DB 故障触发内存硬上限后只按已披露策略降级，并准确累计指标丢弃数；
- 不可变 Sample，避免异步任务读取被重试修改的上下文；
- 渠道删除后的快照展示。

### 12.2 数据库与 API 测试

- GORM AutoMigrate 和 Upsert 兼容 SQLite、MySQL 5.7.8/8、PostgreSQL 9.6/当前版；
- `LOG_DB=DB` 与独立 `LOG_DB` 都只在最终日志库创建并迁移渠道指标事实表；
- 主库 SQLite、独立日志库 MySQL/PostgreSQL 时不会误用主库方言；
- 模型哈希索引不会因长模型名超过 MySQL 索引限制；
- 增量 Upsert 与覆盖 Upsert 相互隔离，并发累加总数守恒；
- 所有 API 共享过滤口径；
- 各 scope 不支持的过滤器返回 400，支持的渠道过滤遵循最终渠道或尝试渠道语义；
- 渠道汇总等于模型子项合计；
- 上游状态码分布等于 `upstream_call` 数，渠道调用量等于 `channel_attempt` 数；
- 客户端状态码分布等于 `final_request` 数；
- 分页、排序、空区间和时间边界；
- 日志关闭时聚合和最小化失败明细仍正确，完整日志深链明确不可用；
- 失败事件队列满时聚合不丢失，API 准确披露明细丢弃计数；
- 任意指标丢弃、维度溢出或 transport 未覆盖都会使对应响应 `partial=true`；
- 历史回填幂等、可恢复且不会重复计数；
- 5 分钟到小时、小时到天的覆盖重算可重复执行且不会混合桶层级；
- 10 万指标桶下关键查询命中预期索引，flush 不阻塞 relay。

### 12.3 扩展与端到端测试

- 静态模块首屏、加载态、空态、错误态和重试；
- 筛选器、Tab 懒加载、渠道模型展开和深链；
- 桌面与移动视口截图；
- 模块包根目录、manifest 和静态资源完整性；
- 本地测试统一使用 `scripts/local-test.ps1`，固定后端 3000、classic 前端 3001 和 `tmp-local-v10101.db`；
- 启动后验证 `/api/status`、classic 页面、默认账号登录、演示数据接口和渠道统计接口。

## 13. 发布与回滚

发布顺序：

1. 先发布后端采集器、数据表和只读 API；
2. 观察刷新耗时、表增长和接口延迟；
3. 启用保留策略、小时桶降采样和清理，并验证 30 天查询覆盖；
4. 再发布 `channel-quality` 新版本；
5. 历史回填由管理员按需单独启动。

回滚策略：

- 采集器支持配置关闭；
- 新表为附加数据，不影响现有请求、日志和计费；
- 旧版静态模块仍可继续使用渠道列表和测试接口；
- API 不可用时，新模块显示错误态，不影响主站其他功能；
- 回滚程序版本时不立即删除新表，避免不可逆数据损失。

## 14. 验收标准

- 能按时间范围查看整体请求、尝试、成功率、Token、缓存、状态码和延迟；
- 能展开任一渠道查看其每个模型的同口径统计；
- 一次失败后重试成功能够正确显示两个渠道尝试和一个最终成功请求；
- 同一渠道尝试内的二次 HTTP 请求不会重复增加渠道调用量，但会保留两个上游调用结果；
- HTTP 200 流式中断不会被统计成成功；
- 主动测试不污染真实业务调用量、费用和成功率；
- SQLite、MySQL、PostgreSQL 均能迁移、写入和查询；
- 日志关闭时聚合和近期失败明细仍可用，并明确说明完整日志深链不可用；
- 模块保持 `static`、上传即用、无外部 CDN 和独立运行时依赖；
- 固定本地测试流程与扩展打包校验全部通过。

## 15. 调研依据

本设计基于以下一手资料核对，而不是仅根据截图推断：

- Sub2API 渠道主动监控、探测历史和日聚合：
  `https://github.com/Wei-Shaw/sub2api/tree/d4b9797ff72024960a035cf22fdd8f213e149169/backend/ent/schema`；
- Sub2API 用量日志与 Ops 指标：
  `https://github.com/Wei-Shaw/sub2api/blob/d4b9797ff72024960a035cf22fdd8f213e149169/backend/ent/schema/usage_log.go`、
  `https://github.com/Wei-Shaw/sub2api/blob/d4b9797ff72024960a035cf22fdd8f213e149169/backend/internal/service/ops_dashboard_models.go`；
- CPA Manager Plus 用量分析、请求监控和事件归一化：
  `https://github.com/seakee/CPA-Manager-Plus/blob/main/apps/docs/manual/usage-analytics.md`、
  `https://github.com/seakee/CPA-Manager-Plus/blob/main/apps/docs/manual/monitoring.md`、
  `https://github.com/seakee/CPA-Manager-Plus/blob/main/apps/manager-server/internal/usage/event.go`；
- new-api 当前扩展、日志和性能指标实现：
  `examples/extensions/channel-quality/`、`model/log.go`、
  `model/perf_metric.go`、`pkg/perf_metrics/`、`controller/relay.go`。

调研结论是：主动探测、生产用量和运维错误必须分域；同时，现有参考项目都没有直接提供完整的“渠道 × 模型 × 客户端状态码 × 原始上游状态码”契约，因此本设计采用三级 scope 和独立结构化指标桶补齐该缺口。
