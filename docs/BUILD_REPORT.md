# 首版交付记录

日期：2026-11-01；应用版本：0.1.0；数据格式：schemaVersion 1。

## 已完成

- 在 Windows amd64 主机使用 Go 1.22.5、Wails CLI v2.10.2 完成 production 构建。
- 产物为 `build/bin/ToolboxSelf.exe`，含图标、Windows 版本资源和 DPI 清单，约 9 MiB。
- 本地分发包为 `release/ToolboxSelf-v0.1.0-windows-amd64.zip`，包含 exe、README、Changelog 和详细文档；同目录提供 SHA-256 校验文件。
- 已初始化 Git 仓库，默认分支名为 `main`，但初始提交与 `v0.1.0` 标签尚未创建：当前执行环境对 `.git` 只读，提权自动审批又因 `unknown provider for model codex-auto-review` 无法完成。用户已指定远程仓库 `git@github.com:zhouyulong/toolbox.git`，但尚未完成配置或推送。可在本机终端运行 `./scripts/publish-first-version.ps1` 完成本地保存和推送；只保存本地时使用 `./scripts/initialize-version.ps1`。
- 已提供 Windows、Ubuntu 24.04 x64、macOS universal 的 GitHub Actions 构建及草稿发布配置。

## 尚未执行的验证

- 未运行自动化测试或桌面界面交互测试。
- 未配置真实 API Key，未向真实供应商发送请求。
- 当前主机未执行 Linux/macOS 构建与运行；CI 配置在推送到相应远程仓库后才能运行。

编译成功说明本机 Go/Wails 代码和资源打包完成，不代表以上未执行的验证已通过。第一次使用请先在「模型供应商」填写自己的配置，再创建一个工具。
