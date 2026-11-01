param([string]$Platform = 'windows/amd64')
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
        throw '未找到 Wails CLI。请先运行 go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2，并将 Go bin 目录加入 PATH。'
    }
    & wails build -clean -platform $Platform
    if ($LASTEXITCODE -ne 0) { throw 'Wails 构建失败。' }
} finally {
    Pop-Location
}
