# Classic 通知任务 Chat ID 未确认输入修复记录

日期：2026-07-24

## 问题

Classic 后台通知中心创建通知任务时，用户在“接收 Chat ID”输入框里输入了
Telegram Chat ID，但如果没有按回车把输入内容转成标签，直接点击“确定”，
前端保存校验仍会提示“请至少添加一个接收 Chat ID”。

## 根因

Classic 使用 Semi `TagInput`。该组件默认只在按回车时把临时输入值写入
`value` 列表，普通输入态的内容只是组件内部 `inputValue`。

原实现只读取 `taskForm.targets`，没有读取 `TagInput` 的临时输入值。
因此界面上能看到 Chat ID，但提交逻辑看到的接收人列表仍为空。

## 修复方案

- 为通知任务表单新增 `taskChatIdInput` 状态，接管 `TagInput.inputValue`；
- 打开 `TagInput.addOnBlur`，失焦时自动把当前输入变成标签；
- 保存任务时把尚未按回车的临时 Chat ID 合并到 `targets`；
- 合并时去重，并保留已有目标的提及用户、提及名称和启用状态。

## 兼容性

- 不改变后端接口和数据库结构；
- 已按回车添加标签的旧流程不受影响；
- 编辑已有任务时会清空临时输入态，不影响已有目标；
- Default 前端使用自研 `TagInput`，已有失焦自动加入逻辑，本次只修复 Classic。

## 验证计划

- 执行 Classic 前端构建；
- 检查代码差异只包含通知中心表单和本修复记录；
- 执行 `git diff --check`。

## 验证结果

- `npm run build`（`web/classic`）：通过；
- `git diff --check`：通过。
