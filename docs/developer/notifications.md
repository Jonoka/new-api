# 通知中心与模块事件

通知中心只负责发送。业务模块产生事件，通知中心根据任务、Bot、接收目标和模板完成投递。模块不能读取 Bot Token，也不能覆盖接收人和模板；一个 Bot 可以被多个任务复用。

## 内置 Go 业务接入

业务状态和通知必须使用同一个 GORM 事务：

```go
return model.EnqueueNotificationEventTx(
    tx,
    "subscription_expiring",
    fmt.Sprintf("subscription:%d", subscription.ID),
    map[string]any{"subscription_id": subscription.ID},
)
```

新增内置事件时同步提供事件值、显示名称、默认模板、变量白名单和两套前端展示。当前内置事件包括 invoice_pending，它在订单首次进入待开票状态时产生。

## 模块声明事件

manifest.json 示例：

```json
{
  "id": "orders",
  "name": "订单模块",
  "version": "1.0.0",
  "runtime": {
    "type": "http",
    "base_url": "http://127.0.0.1:39001",
    "health_path": "/health"
  },
  "permissions": {
    "roles": ["root"],
    "capabilities": ["notification.events.publish"]
  },
  "notifications": {
    "events": [
      {
        "id": "created",
        "label": "新订单",
        "description": "创建订单后触发",
        "default_template": "{{mention}} 来了新订单：{{order_id}}",
        "variables": [
          {
            "name": "order_id",
            "label": "订单 ID",
            "type": "string",
            "required": true
          }
        ]
      }
    ]
  }
}
```

宿主生成规范事件名 extension.<module-id>.<event-id>，上例为 extension.orders.created。事件 ID 使用小写字母、数字、短横线和下划线，完整名称最多 64 个字符。变量类型支持 string、number、boolean；mention、module_id、event_type、event_key 是宿主保留变量。

## 模块发布事件

入口：

```text
POST /api/extensions/<module-id>/notification-events
```

当前入口临时只允许 Root 调用。HTTP 模块后台必须使用 Root 服务账号 Access Token：

```bash
curl -X POST "https://your-host.example/api/extensions/orders/notification-events" \
  -H "Authorization: Bearer <service-account-access-token>" \
  -H "New-Api-User: <service-account-user-id>" \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "created",
    "event_key": "order:123456",
    "payload": {"order_id": "123456"}
  }'
```

Bearer 必须使用大写 B。该令牌是主程序用户的系统管理令牌，不是 OpenAI sk- API Token。服务账号必须是 Root，且仍需满足 permissions.roles；模块必须已安装、已启用并声明 notification.events.publish。

静态模块不能把 Root 服务令牌打进 JavaScript，也不能使用普通用户会话发布通知事件。当前 `RootAuth` 同样接受浏览器中的 Root 登录会话，因此 static/native 模块不依赖该会话发布权威事件是一条受信任模块政策，而非浏览器级技术隔离；权威业务事件应由受信任的服务端模块调用该入口。

响应沿用 success、data、message。首次创建投递时 status 为 queued；没有启用订阅任务时为 accepted_without_subscriber；相同事件类型和事件键再次提交时为 duplicate。delivery_count 只表示创建的投递数量。

## 请求限制、幂等和历史

- 请求体最大 16 KiB，payload 最多 32 个字段。
- event_key 必填，最多 128 个字符，必须是稳定业务键，不能使用随机时间戳。
- 只能提交 manifest 声明的字段；字符串最多 1024 个字符，类型必须匹配。
- 必填变量必须出现，可选变量缺失时使用空字符串、0 或 false。
- Bot Token、Access Token、密码等敏感数据禁止放入 payload。
- 同一规范事件类型和 event_key 在 90 天内按不可逆哈希去重；同一键后续提交的新 payload 会被视为重复而忽略。过期收据在事件入队事务中按固定批次清理，超过窗口后同一键可再次受理。
- 每个任务只保留最新五个终态事件的详细记录；无订阅事件只保留有 90 天有效期的去重收据。
- 任务新建或重新启用后不回放旧事件；停用模块拒绝新事件，但不取消已入队投递。
- 任务切换事件类型时会重建事件基线，并取消旧类型尚未开始发送的投递，避免旧事件套用新模板或新机器人。
- 任务激活、目标变更和事件入队共用数据库序列锁。SQLite 通过写锁串行化，MySQL 和 PostgreSQL 使用锁行，避免配置提交窗口丢失事件。

## 投递和安全

通知中心统一处理 Telegram 调用、429 重试、失败状态和历史清理。投递先进入 claimed，只有实际发起 Telegram 请求前才进入 sending；过期 claimed 可以安全重新领取，过期 sending 因结果不确定不会自动重发。HTTP 模块是受信任后端，宿主会注入 X-NewAPI-\* 上下文并剥离 Cookie、Authorization、Proxy-Authorization。事件发布入口当前额外要求 Root；未来若开放第三方模块，应改用绑定模块且可撤销的专用发布凭据，而不是放宽为普通用户令牌。
