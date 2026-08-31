# Gemini 上游缓存 usage 诊断记录

## 目标

排查渠道 100 的 Gemini 上游面板显示缓存 token、而 New API 请求日志记录
`cache_tokens=0` 的差异，确认缓存 usage 是否在上游响应中返回，以及是否在
解析边界丢失。

## 范围与安全边界

- 仅观察 Gemini 共用流解析器中、已成功解析的 SSE chunk。
- 通过 `GEMINI_USAGE_TRACE_CHANNEL_ID=100` 显式开启，其他渠道和默认运行不输出诊断行。
- 每行只包含请求 ID、渠道 ID、chunk 序号、`promptTokenCount`、
  `cachedContentTokenCount`、`candidatesTokenCount`、`totalTokenCount`，以及缓存字段是否存在。
- 不记录 prompt、请求头、API key、完整请求或响应正文。
- 诊断开关只用于短时间采样，采样完成后立即移除。

## 实现与验证

诊断位于 `relay/channel/gemini/relay-gemini.go` 的 `geminiStreamHandler`。
缓存字段的存在性从原始 JSON 的 `usageMetadata` 对象单独判断，因此可以区分：

1. 字段存在且值非零；
2. 字段存在且值为零；
3. 字段被省略。

回归测试覆盖三种字段状态、渠道门控和日志不包含原始 JSON。生产采样、镜像
摘要、实际请求 trace 和回收结果在部署后补录；在此之前不把上游缓存值当作已确认事实。
