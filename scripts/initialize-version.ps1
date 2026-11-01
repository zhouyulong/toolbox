# Run in your own terminal to create the initial commit and tag.
param()
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    if (-not (Test-Path -LiteralPath '.git')) {
        & git init -b main
        if ($LASTEXITCODE -ne 0) { throw 'Git 初始化失败。' }
    }
    $existingCommitCount = & git rev-list --all --count
    if ($LASTEXITCODE -ne 0) { throw '读取 Git 提交记录失败。' }
    if ([int]$existingCommitCount -gt 0) {
        throw '仓库已有提交；此脚本仅用于空仓库的首次版本初始化。后续请按 docs/DEVELOPMENT.md 维护版本。'
    }
    $version = (Get-Content -LiteralPath VERSION -Raw).Trim()
    $trackedPaths = @(
        '.gitattributes', '.gitignore', '.github', 'LICENSE', 'README.md', 'CHANGELOG.md',
        'VERSION', 'app.go', 'main.go', 'go.mod', 'go.sum', 'wails.json',
        'frontend', 'internal', 'scripts', 'docs',
        'build/appicon.png', 'build/appicon.svg', 'build/windows'
    )
    & git add -- @trackedPaths
    if ($LASTEXITCODE -ne 0) { throw '暂存文件失败。请确认当前用户可写 .git 目录。' }
    & git commit -m 'feat: initial cross-platform personal LLM toolbox'
    if ($LASTEXITCODE -ne 0) { throw '创建初始提交失败。请检查 Git 的 user.name、user.email 与目录权限。' }
    & git tag -a "v$version" -m "ToolboxSelf v$version"
    if ($LASTEXITCODE -ne 0) { throw '提交已创建，但版本标签失败；请按开发文档手动添加标签。' }
    & git log -1 --oneline
    Write-Output "Created v$version. No remote push was performed."
} finally {
    Pop-Location
}
