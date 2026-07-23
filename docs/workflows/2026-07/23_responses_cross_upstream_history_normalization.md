# Responses 跨上游历史归一化

日期：2026-07-23

## 问题

Responses 客户端会把上一轮 `response.output` 作为下一轮 `input` 历史继续提交。
不同上游为输出项生成的 `id` 前缀和附加字段并不一致。例如一个上游返回的消息
`id` 为 `item_*`，切换到另一个要求 `msg_*` 的上游后，会返回
`invalid_id_prefix`；部分上游还会拒绝历史项中的 `status` 或 `namespace`。

这些字段多数描述原上游生成输出时的内部状态，不是无状态对话的语义关联。Relay
原样转发它们，会使正常的负载均衡和失败切换带上前一个上游的私有约束。官方类型
还区分带必填输出 ID 的输出消息和不带 ID 的简易输入消息，因此不能只删除 ID：助手
输出内容也必须由 `output_text` 或 `refusal` 转成可重放的 `input_text`。

## 目标与范围

- 非请求体透传的 `/v1/responses` 与 `/v1/responses/compact` 请求，在每次渠道尝试
  前统一归一化历史；
- 消息历史移除顶层 `id`、`status`、`namespace`；
- 助手消息的 `output_text` 和 `refusal` 内容转换为 `input_text`，保留文本与消息
  `phase`；
- 函数和自定义工具调用历史移除同类输出元数据，但保留用于调用配对的 `call_id`；
- 不修改工具名称、参数、输出、加密推理内容或 `previous_response_id`；
- 字符串输入、非对象数组元素和未知类型保持原样；
- 显式开启请求体透传时保持字节级透传语义，不做归一化。

## 方案

1. 在服务层解析 `OpenAIResponsesRequest.Input`。只处理数组中的已知可回放历史项：
   `type=message`、未声明 `type` 但带 `role` 的简易消息、`function_call`、
   `function_call_output`、`custom_tool_call` 和 `custom_tool_call_output`。
2. 删除这些项顶层的 `id`、`status`、`namespace`。`call_id` 是工具调用与结果之间的
   协议关联键，必须保留；不使用字符串前缀替换伪造另一个上游的 ID。
3. 对消息内容中的 `output_text` 和 `refusal` 重建为只含 `type: input_text` 与文本的
   输入内容。输出专属的注解、概率和状态不带入另一个上游，消息 `phase` 原样保留。
4. 在 Relay 深拷贝请求、完成模型映射后，且在渠道适配器转换前调用归一化。这样同一
   请求的渠道重试和后续请求的渠道切换使用同一规则，各适配器也能收到可移植输入。
5. 归一化只修改每次尝试的请求副本，不回写客户端原始请求，也不影响下一次渠道选择。

## 安全与兼容性

- 不记录或输出请求正文，只在调试日志记录归一化项数量；
- 不涉及数据库和配置迁移；
- `item_reference`、推理项和未知供应商类型不在本次自动改写范围内，避免删除可能具有
  引用语义的 `id`；
- 显式透传渠道仍由管理员承担上游协议一致性责任；
- 对已经没有私有元数据的请求保持幂等，不产生额外序列化。

## 测试计划

- 覆盖 `item_*` 消息 ID、`status`、`namespace` 被移除；
- 覆盖 `output_text` 和 `refusal` 转成 `input_text`，并保留文本与消息 `phase`；
- 覆盖无显式 `type` 但带 `role` 的消息历史；
- 覆盖函数及自定义工具调用保留 `call_id`、名称、参数和输出；
- 覆盖 `reasoning`、`item_reference`、未知类型和字符串输入保持原样；
- 覆盖第二次归一化不再修改请求；
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
