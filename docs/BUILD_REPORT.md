# 首版交付记录

日期：2026-11-01；应用版本：0.1.0；数据格式：schemaVersion 1。

## 已完成

- 在 Windows amd64 主机使用 Go 1.22.5、Wails CLI v2.10.2 完成 production 构建。
- 产物为 `build/bin/ToolboxSelf.exe`，含图标、Windows 版本资源和 DPI 清单，约 9 MiB。
- 本地分发包为 `release/ToolboxSelf-v0.1.0-windows-amd64.zip`，包含 exe、LICENSE、README、Changelog 和详细文档；同目录提供 SHA-256 校验文件。
- 已保存首版 Git 源码，默认分支为 `main`，首版标签为 `v0.1.0`；远程地址为 `git@github.com:zhouyulong/toolbox.git`。保留了远程已有的 MIT LICENSE 及初始提交历史。远程发布状态以仓库分支、标签和 Actions 结果为准。
- 已提供 Windows、Ubuntu 24.04 x64、macOS universal 的 GitHub Actions 构建及草稿发布配置。

## 尚未执行的验证

- 未运行自动化测试或桌面界面交互测试。
- 未配置真实 API Key，未向真实供应商发送请求。
- 当前主机未执行 Linux/macOS 构建与运行；CI 配置在推送到相应远程仓库后才能运行。

编译成功说明本机 Go/Wails 代码和资源打包完成，不代表以上未执行的验证已通过。第一次使用请先在「模型供应商」填写自己的配置，再创建一个工具。
