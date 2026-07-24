# 上游流式断开错误中文说明

## 背景

用户看到类似错误时不容易理解：

```text
status_code=500, upstream stream disconnected: connection reset by peer
```

该错误来自流式响应阶段，上游或中间网络链路已经开始返回数据后中途断开。原始英文错误对
排障有用，但普通用户容易误解为自己的请求参数、Key 或余额问题。

## 根因

`connection reset by peer` 是底层网络错误，表示连接被对端重置；与
`upstream stream disconnected` 组合出现时，含义是上游流式响应没有正常完整结束。
此前对外展示只保留技术原文，缺少中文解释。

## 修改范围

- 在统一错误展示层增加中文说明，不改变原始 `Err`、错误码、状态码和重试判断。
- 覆盖：
  - `ErrorWithStatusCode`
  - `MaskSensitiveError`
  - OpenAI 格式错误响应
  - Claude 格式错误响应
- 防止重复追加中文说明。

## 展示效果

```text
status_code=500, upstream stream disconnected: connection reset by peer
（中文说明：上游流式响应中途断开，连接被对端重置，通常是上游服务、代理或网络链路异常；可稍后重试或切换渠道。）
```

## 验证

```bash
go test ./types -count=1 -timeout 60s
```

## 部署注意

该变更仅调整错误展示文案，不涉及数据库结构、计费、渠道调度或重试策略。
