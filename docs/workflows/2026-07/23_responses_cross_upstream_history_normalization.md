# Responses 跨上游历史归一化

日期：2026-07-23

修订：2026-07-24

## 问题

Responses 客户端会把上一轮 `response.output` 作为下一轮 `input` 历史继续提交。
不同上游为输出项生成的 `id` 前缀和附加字段并不一致。例如一个上游返回的消息
`id` 为 `item_*`，切换到另一个要求 `msg_*` 的上游后，会返回
`invalid_id_prefix`；部分上游还会拒绝历史项中的 `status` 或 `namespace`。

这些字段多数描述原上游生成输出时的内部状态，不是无状态对话的语义关联。Relay
原样转发它们，会使正常的负载均衡和失败切换带上前一个上游的私有约束。消息角色和
内容类型则属于对话语义：助手消息必须继续使用 `output_text` 或 `refusal`，不能改成
只适用于输入角色的 `input_text`。

`.204` 曾错误地把助手消息的 `output_text` 和 `refusal` 转成 `input_text`。生产环境在
2026-07-24 08:05 至 08:21（Asia/Shanghai）运行该版本时，上游集中返回
`Invalid value: 'input_text'. Supported values are: 'output_text' and 'refusal'`，随后触发
跨渠道重试，并放大为 502、409、空 EOF 和零 token 错误日志。回滚 `.199` 后，同类
错误立即消失。

同一窗口还确认，客户端显式携带的 `previous_response_id` 在原渠道状态失效或切换
上游后，渠道会返回 409 `compact continuation is unknown or expired`。旧逻辑只允许
网关自动附加的 ID 降级，显式 ID 会被原样带到后续渠道，导致多个渠道连续返回 409。

## 目标与范围

- 非请求体透传的 `/v1/responses` 与 `/v1/responses/compact` 请求，在每次渠道尝试
  前统一归一化历史；
- 消息历史移除顶层 `id`、`status`、`namespace`；
- 助手消息保留 `output_text` 和 `refusal` 类型及文本，移除内容块中的输出私有元数据，
  保留消息 `phase`；
- 函数和自定义工具调用历史移除同类输出元数据，但保留用于调用配对的 `call_id`；
- 不修改工具名称、参数、输出、加密推理内容或 `previous_response_id`；
- 字符串输入、非对象数组元素和未知类型保持原样；
- 显式开启请求体透传时保持字节级透传语义，不做归一化。
- `client_metadata` 仅属于 Codex 后端扩展；Codex 渠道继续保留，普通 OpenAI 兼容
  渠道在转换阶段移除，避免上游返回 `Unknown parameter: client_metadata`。
- 普通 Responses 和 compact 对话的续传 ID 未知或过期，且 `input` 明确包含此前
  助手输出形成的完整历史时，在同一渠道去掉 `previous_response_id` 重放一次；只带
  本轮增量输入、工具输出或 `item_reference` 的依赖型续链不降级，避免静默丢失上下文。

## 方案

1. 在服务层解析 `OpenAIResponsesRequest.Input`。只处理数组中的已知可回放历史项：
   `type=message`、未声明 `type` 但带 `role` 的简易消息、`function_call`、
   `function_call_output`、`custom_tool_call` 和 `custom_tool_call_output`。
2. 删除这些项顶层的 `id`、`status`、`namespace`。`call_id` 是工具调用与结果之间的
   协议关联键，必须保留；不使用字符串前缀替换伪造另一个上游的 ID。
3. 对消息内容中的 `output_text` 和 `refusal` 重建为只含原始 `type` 与对应文本字段的
   内容。输出专属的注解、概率和状态不带入另一个上游，但助手角色所需的内容类型和
   消息 `phase` 原样保留。
4. 在 Relay 深拷贝请求、完成模型映射后，且在渠道适配器转换前调用归一化。这样同一
   请求的渠道重试和后续请求的渠道切换使用同一规则，各适配器也能收到可移植输入。
5. 归一化只修改每次尝试的请求副本，不回写客户端原始请求，也不影响下一次渠道选择。
6. 对 400/404 的 `previous_response_id` 失效和 409 的 compact continuation 失效，仅当
   `input` 是数组、明确包含此前助手输出，且不含 `*_call_output` 或
   `item_reference` 时，先删除 ID 在当前渠道重试一次；字符串输入、仅本轮用户输入和
   依赖上一响应对象的工具续链保持原错误，避免静默丢失上下文。

## 安全与兼容性

- 不记录或输出请求正文，只在调试日志记录归一化项数量；
- 不涉及数据库和配置迁移；
- `item_reference`、推理项和未知供应商类型不在本次自动改写范围内，避免删除可能具有
  引用语义的 `id`；
- 显式透传渠道仍由管理员承担上游协议一致性责任；
- 对已经没有私有元数据的请求保持幂等，不产生额外序列化。

## 测试计划

- 覆盖 `item_*` 消息 ID、`status`、`namespace` 被移除；
- 覆盖 `output_text` 和 `refusal` 类型不变，并保留文本与消息 `phase`；
- 覆盖无显式 `type` 但带 `role` 的消息历史；
- 覆盖函数及自定义工具调用保留 `call_id`、名称、参数和输出；
- 覆盖 `reasoning`、`item_reference`、未知类型和字符串输入保持原样；
- 覆盖第二次归一化不再修改请求；
- 覆盖 Responses 与 compact 在完整历史下显式续传 ID 的 409 降级重放，以及字符串、
  仅本轮输入和工具输出续链不降级；
- 端到端覆盖 Responses、compact、渠道请求体透传和全局请求体透传；
- 运行 `go test ./service ./relay -count=1 -timeout 60s`、相关静态检查、构建和
  `git diff --check`。

## 验证结果

- 定向测试通过：`go test` 覆盖 `./service`、`./relay`、`./relay/channel/openai`、
  `./relay/channel/codex`，参数为 `-count=1 -timeout 60s`；
- `go vet ./service ./relay ./relay/channel/openai ./relay/channel/codex`：通过；
- `go build ./service ./relay ./relay/channel/openai ./relay/channel/codex`：通过；
- `git diff --check`：通过；
- `go build ./...` 无法在当前干净发布工作树执行：基线未包含嵌入主程序所需的
  `web/default/dist` 和 `web/classic/dist`，失败发生在 Go 编译前；
- 竞态测试不可用：当前 Go 运行环境为 `windows/386` 且 `CGO_ENABLED=0`，不支持
  `-race`。

## 已知限制

只包含供应商侧引用、没有完整内容的 `item_reference` 无法跨上游还原。本修复会保留
这类引用，不会把不存在的内容或 ID 伪造成另一个上游的对象。
