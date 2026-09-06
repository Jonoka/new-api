# 本项目二次开发能力

这份清单记录当前二开分支已落地的业务能力和继续开发的边界。

## 已落地能力

- 双前端与主题切换：Default、Classic 共用后端 API，发票、订阅、返利、通知中心和扩展都有入口。代码见 web/default/src/routes/\_authenticated、web/classic/src/App.jsx。
- 多支付网关充值：EPay、Stripe、Creem、Waffo、Waffo Pancake、BEPUSDT、OKPay，含回调、重试和订单快照。代码见 controller/topup\_\*.go、model/topup.go。
- 订阅计费和套餐：套餐 CRUD、启停、购买、周期配额重置、限制、退款和多渠道付款。路由为 /api/subscription/_、/api/subscription/admin/_。
- 发票中心：支付时申请、历史订单合并申请、服务费支付、个人/企业和普票/专票状态流转；后台支持保留审计数据的单条和批量软删除。订单首次进入待开票状态产生 invoice_pending。代码见 model/invoice.go、model/invoice_order.go、model/invoice_payment.go。
- 返利和提现：邀请关系、分级返佣、成熟期结算、提现账户、审批、风控冻结和追回。路由为 /api/affiliate/_、/api/affiliate/admin/_。
- 通知中心：Telegram Bot、多个任务和目标、提及、自定义模板、事务 outbox、429 重试、死信和每任务最新五条历史。管理路由为 /api/notification/\*，仅 root 可管理。
- 自定义 OAuth/OIDC：数据库动态注册、Discovery、字段映射、绑定和解绑。路由为 /api/custom-oauth-provider/\*、/api/oauth/:provider。
- 异步图片任务：POST/GET /v1/images/tasks，复用渠道、限流和计费链路并按 TTL 清理。代码见 controller/canvas_image_task.go。
- 动态计费表达式：覆盖预消费、结算、日志和版本快照；新增变量或函数前必须阅读 pkg/billingexpr/expr.md。
- 渠道运维：趋势、稳定性、状态码、失败链路、并发限制、亲和缓存和上游模型变更检测。单渠道并发上限调低时从当前在途并发开始，在一分钟内线性收敛；调高或改为不限制仍立即生效。多分组令牌按配置顺序跨组重试，每个分组最多发起一次上游请求，失败后立即进入下一组；每一次选渠道只使用当前重试分组的渠道候选集。分组可标记为独立，独立分组只能单独绑定令牌，历史冲突绑定在请求时返回 503；旧客户端缺失独立字段时保留原值，请求热路径使用可失效快照而不每次查库。结构化接口新建分组时，后端取得真实 ID 后以 ID 的十进制文本作为最终兼容 code；旧分组通过管理员显式预检和事务迁移切换为数字 code，并同步渠道、令牌、用户、能力、订阅、选项和缓存，`default` 保持固定标识。令牌分组迁移遇到已存在的目标绑定时只去重该令牌内部的分组，不删除独立令牌记录。
- 游戏钱包和预测玩法：主链路存在，但 JudgeProvider 尚未实现，自动判题会回落人工，标记为实验性。
- 站点与导航定制：Logo、页脚、公告、FAQ、自定义链接、分区、图标和排序。

## 扩展宿主

完整说明见[扩展模块开发](extensions.md)。当前提供 static、http 两种运行时、动态导航、兼容 iframe、`native v1` 宿主原生渲染槽、宿主上下文、模块代理、上传/启停/卸载，以及按 manifest 能力发布通知事件。原生模块仍由 zip 交付，业务页面不注册进主程序固定路由：

- /api/extensions/host/me
- /api/extensions/:id/proxy/\*path
- /api/extensions/:id/native/:pageKey/:target/:asset
- /api/extensions/:id/notification-events
- /api/extension-admin/\*

## 继续开发的接口

Batch D extends [wallet reservation concurrency](wallet-reservations.md) across
ordinary sessions and remaining legacy relay callers, retaining B/C transaction
ownership and the configured trust, subscription and post-use policies.

Batch C adds synchronous [Alpha Search and independent request replay](alpha-search.md)
on its candidate branch. Alpha uses existing native Codex/type1 gateway transports
and search-only accounting; real-gateway production acceptance remains pending.

Batch B accounting is under validation on its isolated candidate branch. Selected-group pricing resizes the live reservation before the next upstream attempt. Async tasks transfer reservation ownership to durable task accounting; terminal funding, token, used counters and status commit together, and independent log storage uses idempotent event delivery. See [final-group billing](../workflows/2026-09/05_final_group_billing.md) and [async accounting](../workflows/2026-09/05_async_task_accounting.md). Legacy terminal rows are not automatically repaired. An older image must not process B-owned in-flight tasks or pending accounting events; drain and reconcile before rollback.

- 新 AI 渠道：relay/channel/\* 适配器和渠道类型注册；支持 StreamOptions 时加入 streamSupportedChannels。
- 新异步任务：复用 service.TaskPollingAdaptor 和主节点调度器，明确租约、超时、清理、多节点行为。
- 新 OAuth：实现 oauth.Provider 并接入注册表。
- 新通知：外部模块声明 notifications.events；内置 Go 业务在事务中调用 model.EnqueueNotificationEventTx。
- 新计费：先阅读 pkg/billingexpr/expr.md。
- 模型端点覆盖：使用 ModelMeta.Endpoints。

## 必须遵守的约束

- JSON 使用 common.Marshal、common.Unmarshal、common.DecodeJson 等封装。
- 数据库同时兼容 SQLite、MySQL、PostgreSQL，优先 GORM。
- 通知事件和订单状态变更使用同一事务。
- 密钥不能写入 manifest、静态前端或日志。
- 新页面同时考虑 Default 和 Classic；模块页面必须有加载、空、错误状态。
- 可信的一方页面、看板和后台工具默认使用 `native v1` 并同时提供 Default、Classic 入口；仍受信任但需要独立 DOM、样式或框架运行时的第三方/遗留页面，以及远程 HTTP UI 才使用 iframe。iframe 不是安全沙箱，不可信模块禁止安装，也不得用主题变量桥接冒充原生 UI。
- 默认使用 static 轻量模块，需要常驻进程、队列或长连接才使用 http。
- 不得修改受保护的项目标识、署名和归属信息。

## 本地验证

固定使用后端 3000、Classic 3001 和 tmp-local-v10101.db：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action status
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action start
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action verify
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/local-test.ps1 -Action stop
```
