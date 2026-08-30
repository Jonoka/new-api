# Gemini Responses 流提前完成修复记录

日期：2026-08-31

## 问题

Google Gemini 协议渠道通过 `/v1/responses` 流式请求时，部分上游会在每个
Gemini SSE chunk 中携带 `finishReason: "STOP"`，但随后仍继续发送文本。转换后的
Responses 流提前发出 `response.output_text.done`，Hermes 等客户端按协议忽略其后
的文本增量，导致结果截断。

## 根因

`GeminiResponsesStreamHandler` 把每个 Gemini `STOP` 立即转换为 Chat Completions
结束 chunk。共享的 Chat-to-Responses 转换器收到该结束原因后发出 output done
事件，而 Gemini 上游流此时尚未结束。

## 修改范围

- 仅在 Gemini `/v1/responses` 流转换中忽略中间 chunk 的 `STOP` 结束信号，等待
  上游流正常结束后使用现有 finalizer 统一完成 Responses 生命周期；
- 不修改 `/v1/chat/completions`、原生 Gemini、Claude 或其他渠道的结束逻辑；
- 为共享 Chat-to-Responses 转换器生成的 `response.output_text.done` 补齐累计
  `text` 字段，不改变其他适配器的事件顺序或终止条件。

## 兼容性

非 `STOP` 结束原因仍由现有转换逻辑处理，`MAX_TOKENS` 和内容过滤等 incomplete
语义保持不变。用量、工具调用、空流及错误返回路径不变。新增的 done `text` 字段
属于 Responses 事件的兼容性补充。

## 验证

- 回归 fixture 连续发送两个都带 `STOP` 的非空文本 chunk；
- 断言全部 delta 按顺序输出，done 事件只出现一次且位于最后一个 delta 之后；
- 断言 output-text done、output-item done、response completed 的终止顺序和唯一性；
- 断言 done `text` 等于所有文本 delta 的拼接结果，并保留 usage；
- `git diff --check` 已通过；Go 格式化和测试由 GitHub Actions 执行，生产 VPS
  不运行源码构建或测试。

## 回滚

若 CI 或生产 smoke check 失败，恢复上一份已验证的 GHCR 镜像。代码回滚仅涉及
Gemini Responses 适配器、共享 Responses done 字段及对应测试，不需要数据库迁移
或渠道配置回滚。
