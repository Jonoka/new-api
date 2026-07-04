# 宿主上下文探针

这是一个静态轻量扩展模块示例，用于验证 new-api 扩展模块系统的基础链路：

- 模块上传和安装
- 模块启用和侧边栏入口
- iframe 嵌入页面
- 主程序静态托管
- 当前登录用户上下文接口

## 使用方式

这个模块不需要单独启动 Node 或其他服务。上传 zip 并启用后，
主程序会直接托管 `public/index.html`。

## 打包

在仓库根目录执行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/package-extension-lite.ps1 -ModuleDir "examples/extensions/host-context-probe"
```

产物会生成到：

```text
artifacts/extensions/host-context-probe-0.1.0.zip
```
