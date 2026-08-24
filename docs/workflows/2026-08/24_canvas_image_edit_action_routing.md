# Canvas 图片编辑任务 action 路由修复

日期：2026-08-24

## 问题

无限画布向 `/canvas/v1/images/edits` 提交异步图片编辑请求且未附带
`action` query 时，任务被保存并重放为 `images/generations`。multipart 请求在
generation 分发路径无法取得原始模型，后续使用默认模型 `dall-e`，最终报告该模型
在当前分组无可用渠道。

## 根因

`CanvasImageTaskSubmit` 只根据 `action` query 计算任务动作；query 为空时默认使用
`images/generations`，没有复用已有的 `canvasImageTaskAction` 路径识别逻辑。

## 修复

提交入口改为调用 `canvasImageTaskAction`。该函数优先保留显式 `action` query；query
缺失时从 `/canvas/v1/images/edits` 或 `/canvas/v1/images/generations` 识别动作。任务表
中的 `action` 与后台重放路径使用同一结果。

## 兼容性

- 显式 `action=edits` 和 `action=generations` 的行为不变；
- `/canvas/v1/images/generations` 缺少 query 时仍使用 `images/generations`；
- 不修改请求体、模型映射、渠道配置、计费规则或数据库结构。

## 验证

已增加控制器回归测试，通过真实 `CanvasImageTaskSubmit` 入口分别提交 edit path、
generation path 和显式 action 请求，检查持久化任务与后台 relay request 的 action
一致。当前 Windows 与 WSL 环境均无 Go 工具链，因此本地未执行 Go 测试；发布前
必须由 GitHub Actions 完成聚焦测试及仓库要求的验证，生产只部署验证通过的不可变
GHCR 镜像。
