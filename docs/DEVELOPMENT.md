# 开发、构建与版本管理

## 1. 仓库结构

```text
main.go                    # 窗口、嵌入资源、版本、生命周期
app.go                     # Wails 绑定、系统对话框、流式事件
internal/toolbox/
  types.go                 # 可持久化数据与前后端 DTO
  store.go                 # JSON 文件、路径、原子替换
  service.go               # 工作区、供应商、会话与并发管理
  providers.go             # 三类请求协议和 SSE 响应适配
frontend/
  index.html
  style.css
  app.js                   # 原生 DOM 界面与安全 Markdown 子集
build/                     # 图标与平台打包资源，bin/ 被忽略
docs/                      # 用户、开发、架构文档
scripts/                   # 构建及版本号维护脚本
.github/workflows/         # 各平台构建与草稿发布
VERSION                    # 程序内嵌版本号
wails.json                 # Wails 与平台元数据
go.mod / go.sum            # 锁定 Go 模块版本及校验值
CHANGELOG.md
```

## 2. 构建依赖

推荐 Go 1.23 或更新兼容版本。源码最低声明 Go 1.22；Wails 固定为 **v2.10.2**。请保持 CLI 版本与 `go.mod` 一致。Node 不参与应用构建，仅可用于版本维护脚本。

### Windows 10/11

安装 Go、Git、Microsoft WebView2 Runtime。标准 Windows amd64 构建通常不需要自行配置 C 编译器；其他目标架构、WebView 特殊配置以 `wails doctor` 输出为准。

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
# 将 (go env GOPATH)\bin 加入 PATH，或者直接使用该目录下的 wails.exe。
wails doctor
.\scripts\build.ps1
```

产物：`build/bin/ToolboxSelf.exe`。如只需快速生成无平台资源打包的本机 exe，也可使用：

```powershell
go build -trimpath -tags desktop,production -ldflags '-s -w -H windowsgui' -o build/bin/ToolboxSelf.exe .
```

直接 `go build` 的结果可执行，但不会自动生成 Wails 的 Windows 图标、版本资源和清单；正式发布使用 Wails CLI。

### Ubuntu 24.04 / WebKitGTK 4.1

```sh
sudo apt-get update
sudo apt-get install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
export PATH="$(go env GOPATH)/bin:$PATH"
wails doctor
sh scripts/build.sh
```

脚本默认 Linux 使用 `webkit2_41` 标签。若目标发行版采用 WebKitGTK 4.0，请先安装 `libwebkit2gtk-4.0-dev`，并直接执行 `wails build -clean`。不要在使用 4.1 运行库的环境分发按 4.0 链接的产物。

产物：`build/bin/ToolboxSelf`。Linux 应基于目标发行版构建；较新 glibc 编译出的文件不一定能在旧系统运行。

### macOS

```sh
xcode-select --install
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
export PATH="$(go env GOPATH)/bin:$PATH"
wails doctor
sh scripts/build.sh
# 需要 Intel + Apple Silicon 通用应用时：
wails build -clean -platform darwin/universal
```

产物：`build/bin/ToolboxSelf.app`。macOS 构建需要 Apple SDK；无法从普通 Windows 或 Linux 的 `GOOS=darwin` 构建出可分发应用。通用包会增加体积，也可分发各架构独立包。

本仓库 CI 未配置 Apple 证书、签名、notarization 或 Windows 签名。公开分发前可按实际发布需求添加这些步骤；不把私钥放入仓库。

## 3. 日常开发

```sh
go mod download
wails dev
```

前端直接从 `frontend/` 提供资源，无 npm 构建过程。修改后按开发模式提示刷新，或重启 `wails dev`。不要直接双击 `index.html`，浏览器中没有 Wails 桥接。

整理 Go 格式：

```sh
gofmt -w main.go app.go internal/toolbox
```

仓库首版未添加自动化测试。需要新增验证时，建议覆盖历史截取、各协议 SSE、取消、配置损坏、JSON 写入失败与工作区迁移，并先使用本地模拟供应商，避免消耗真实模型费用。不要把构建成功等同于实际界面或真实供应商已验证。

## 4. Git 管理

仓库采用 Git，构建产物、缓存、密钥和运行数据不入库。默认分支为 `main`，`origin` 指向 `git@github.com:zhouyulong/toolbox.git`。首版保存了完整源码与文档，并合并保留远程已有的 MIT LICENSE 初始提交。后续按正常流程维护：

```sh
git status
git switch -c feat/your-change
# 完成修改后检查差异
git diff
git add <明确要提交的文件>
git commit -m "feat: describe the change"
```

推荐提交前缀：`feat:`、`fix:`、`docs:`、`refactor:`、`build:`、`chore:`。避免把工作区数据放在源码目录；若必须这样做，应先给其具体路径加 `.gitignore`。

查看远程与推送普通提交：

```sh
git remote -v
git push -u origin main
```

`initialize-version.ps1` 仅用于没有提交的全新仓库；已有仓库不再运行初始版本脚本。`publish-first-version.ps1` 是首次发布辅助脚本：

```powershell
.\scripts\publish-first-version.ps1
```

脚本先保存本地初始提交和版本标签，再配置 `origin`。它会检查已有远程地址、分支历史及同名标签，以普通原子推送发布 `main` 和新标签；已有远程历史不兼容时停止，不自动强制推送或改写历史。需要本机已配置可访问该仓库的 SSH 凭据。脚本执行失败时保留已创建的本地提交，以便处理问题后重试。

## 5. 版本约定

使用语义化版本：

- PATCH：兼容的数据处理修复、界面修复、文档修正。
- MINOR：向后兼容的新能力。
- MAJOR：不兼容的数据格式或行为变化；0.x 阶段仍需在 Changelog 明确说明破坏性改动。

应用版本由 `VERSION` 通过 Go embed 内嵌；Windows/macOS 平台资源版本由 `wails.json` 的 `info.productVersion` 提供，必须同步。维护脚本会同时更新这两个位置：

```sh
node scripts/set-version.mjs 0.1.1
```

`schemaVersion` 是文件格式版本，**不要**跟随每次应用更新增加。只有需要显式数据迁移时才变更。

## 6. 发布流程

1. 更新代码、文档及 `CHANGELOG.md`，记录用户能感知的变化。
2. 更新 `VERSION` 与 `wails.json`。
3. 在对应系统完成所需构建与人工使用确认；当前代码没有自动化测试任务。
4. 提交并创建标签，例如：

```sh
git add VERSION wails.json CHANGELOG.md
git commit -m "chore: release v0.1.1"
git tag -a v0.1.1 -m "ToolboxSelf v0.1.1"
git push origin main
git push origin v0.1.1
```

推送代码或创建 PR 会构建 Windows x64、Linux x64 和 macOS universal，并保存构建产物。推送 `v*` 标签时，独立发布任务在所有构建成功后创建 **草稿** GitHub Release，附带三个平台的压缩包及 SHA-256 文件。人工检查后再发布。

Actions 不能在没有 GitHub 远程仓库时自动执行。标签名必须与 `VERSION` 对应，否则工作流会退出。工作流不自动修改源码或创建版本号。

版本标签之后应保持不可变；修复发布问题时创建新版本，不覆盖已公开的二进制。

## 7. 维护边界

首版刻意保持单进程、本地 JSON、纯文本问答。增加供应商时只扩展协议适配；避免把工具用途硬编码到后端。增加数据迁移时遵循先备份、后转换、成功才登记新版本的规则，详见架构文档。
