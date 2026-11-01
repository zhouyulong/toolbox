# ToolboxSelf

轻量、无需登录的个人大模型桌面工具箱。用 **system 提示词**定义工具，用**独立工作区**管理数据。双击打开，配置自己的 API 或本地模型后即可问答。

技术栈：Go + Wails v2 + 原生 HTML/CSS/JavaScript。使用操作系统的 WebView，不随应用打包 Chromium；前端无 npm 依赖、CDN 或框架。

## 功能

- 无账户、无认证服务器，无遥测、无自动更新。
- 多供应商、多模型：OpenAI Chat Completions 兼容、Anthropic Messages、Gemini 原生接口。
- 支持 Ollama、LM Studio 等提供 OpenAI 兼容接口的本地服务。
- 每个工具独立保存名称、system、模型参数与会话；可自行选择不同磁盘上的数据目录。
- 文本流式问答、停止生成、多轮上下文、会话重命名和删除、复制、Markdown 导出。
- 工作区导入与移除登记；移除工具不会删除目录。
- 跟随系统明暗主题；本地 JSON 数据；原子替换写入；单实例保护。

只做原生文本 LLM 问答，不含 agent、函数调用、检索、附件或第三方插件执行。API Key 存于本机配置文件中，当前未加密。

## 快速使用

1. 运行对应系统的应用。Windows 为 `ToolboxSelf.exe`；macOS 为 `ToolboxSelf.app`；Linux 为 `ToolboxSelf`。
2. 左下角打开「模型供应商」，填写协议、Base URL、API Key 和实际可用的模型 ID。
3. 创建工具，填写 system 提示词、选择模型，指定一个空目录作为工作区。
4. 输入问题，Enter 发送；Shift + Enter 换行。

本机直接构建的 Windows 文件位于 `build/bin/ToolboxSelf.exe`（构建产物不提交 Git）。开箱使用的是本机客户端；云端模型仍需自己的供应商密钥，本地模型服务需要自行启动。

## 平台与运行环境

| 平台 | 运行依赖 | 发布形式 |
| --- | --- | --- |
| Windows 10/11 x64 | Microsoft Edge WebView2 Runtime（多数系统已有） | `.exe` |
| Linux x64（以 Ubuntu 24.04 为构建基线） | GTK 3、WebKitGTK 4.1 | ELF 可执行文件 |
| macOS | 系统 WKWebView；具体最低版本受构建 SDK/部署目标约束 | 通用 `.app` 包（Intel + Apple Silicon） |

轻量指复用系统 WebView；不承诺固定内存或包体大小。Linux 可执行文件依赖系统共享库，不是完全静态二进制。不同系统应在对应系统上构建，不能仅设置 `GOOS` 就生成全部可用桌面产物。

## 从源码构建

推荐 Go 1.23 或更新兼容版本（项目声明最低 Go 1.22），Git，Wails CLI **v2.10.2**。

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
wails doctor
wails build -clean
```

Windows 需要将 Go 的 bin 目录加入 PATH。Linux 需先安装开发库，并用 `wails build -clean -tags webkit2_41`。macOS 需安装 Xcode Command Line Tools。

本项目不需要 `npm install`，也不需要启动前端开发服务器。完整平台安装步骤见[开发与发布文档](docs/DEVELOPMENT.md)。GitHub Actions 已配置三个系统的构建、压缩与 SHA-256 校验文件；推送 `v*` 标签后创建草稿 Release。

源码仓库：[zhouyulong/toolbox](https://github.com/zhouyulong/toolbox)，默认分支为 `main`，首版为 `v0.1.0`。日常提交、版本标签和发布流程见开发文档。

## 数据存放

全局配置：Windows `%APPDATA%\ToolboxSelf\config.json`；macOS `~/Library/Application Support/ToolboxSelf/config.json`；Linux `$XDG_CONFIG_HOME/ToolboxSelf/config.json`（通常为 `~/.config/ToolboxSelf/config.json`）。

每个工具选择自己的目录：

```text
你选择的工作区/
├── toolbox.workspace.json   # 工具定义、system、模型与参数
└── chats/
    ├── <session-id>.json    # 一个会话一个文件
    └── ...
```

留空时默认为 `~/ToolboxSelf/<tool-id>/`，也可使用完全不同的位置。程序拒绝两个工具的目录相同或互相嵌套。全局配置仅存供应商、密钥和工作区登记，不集中保存会话。

## 文档

- [详细使用指南与排错](docs/USER_GUIDE.md)
- [开发环境、构建、Git 与版本发布](docs/DEVELOPMENT.md)
- [架构、数据格式与维护约定](docs/ARCHITECTURE.md)
- [版本记录](CHANGELOG.md)
- [首版构建与交付范围](docs/BUILD_REPORT.md)

## 当前交付范围

代码提供跨平台构建流程；Windows 本机构建结果见交付说明。Linux、macOS 的实际运行与真实供应商连通性需在相应环境中确认。仓库不包含密钥、真实会话或自动化测试；不宣称未经执行的测试已通过。

## 许可证

采用 [MIT License](LICENSE)，保留远程仓库已有的许可证及版权声明。
