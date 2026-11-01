# Run manually in your own terminal. Saves the initial version and pushes it.
param([string]$RemoteUrl = 'git@github.com:zhouyulong/toolbox.git')
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    if (-not (Test-Path -LiteralPath '.git')) {
        & git init -b main
        if ($LASTEXITCODE -ne 0) { throw 'Git 初始化失败。' }
    }
    $commitCount = & git rev-list --all --count
    if ($LASTEXITCODE -ne 0) { throw '读取本地历史失败。' }
    if ([int]$commitCount -eq 0) {
        & (Join-Path $PSScriptRoot 'initialize-version.ps1')
    }
    $branch = & git branch --show-current
    if ($LASTEXITCODE -ne 0 -or $branch -ne 'main') {
        throw '此脚本只发布 main 分支，请先确认当前分支。'
    }
    $pending = & git status --porcelain
    if ($LASTEXITCODE -ne 0) { throw '读取工作区状态失败。' }
    if ($pending) { throw '存在未提交文件，请先保存所需改动后再发布，脚本不会忽略这些改动继续推送。' }

    $version = (Get-Content -LiteralPath VERSION -Raw).Trim()
    if ($version -notmatch '^\d+\.\d+\.\d+$') { throw 'VERSION 格式无效。' }
    $tag = "v$version"
    $tagRef = "refs/tags/$tag"
    $headCommit = & git rev-parse HEAD
    if ($LASTEXITCODE -ne 0) { throw '读取当前提交失败。' }
    $localTag = & git tag --list $tag
    if ($LASTEXITCODE -ne 0) { throw '读取本地标签失败。' }
    if ($localTag) {
        $tagCommit = & git rev-parse "$tagRef^{commit}"
        if ($LASTEXITCODE -ne 0 -or $tagCommit -ne $headCommit) {
            throw "$tag 已指向其他提交。请检查版本，不要覆盖已有标签。"
        }
    } else {
        & git tag -a $tag -m "ToolboxSelf $tag"
        if ($LASTEXITCODE -ne 0) { throw '创建版本标签失败。' }
    }

    $remotes = @(& git remote)
    if ($LASTEXITCODE -ne 0) { throw '读取远程配置失败。' }
    if ($remotes -contains 'origin') {
        $existingUrl = & git remote get-url origin
        if ($LASTEXITCODE -ne 0 -or $existingUrl -ne $RemoteUrl) {
            throw "origin 与目标仓库不同，未修改已有配置。目标：$RemoteUrl"
        }
        $pushUrls = @(& git remote get-url --push --all origin)
        if ($LASTEXITCODE -ne 0 -or $pushUrls.Count -ne 1 -or $pushUrls[0] -ne $RemoteUrl) {
            throw 'origin 的推送地址与目标仓库不同或配置了多个地址，请先检查 remote.pushurl。'
        }
    } else {
        & git remote add origin $RemoteUrl
        if ($LASTEXITCODE -ne 0) { throw '配置 origin 失败。' }
    }

    $remoteRefs = @(& git ls-remote origin)
    if ($LASTEXITCODE -ne 0) { throw '读取远程失败。初版已保存在本地，请检查 SSH 认证和仓库权限后重试。' }
    $remoteHeads = @($remoteRefs | Where-Object { $_ -match '\srefs/heads/' })
    $hasMain = @($remoteHeads | Where-Object { $_ -match '\srefs/heads/main$' }).Count -gt 0
    if ($remoteHeads.Count -gt 0 -and -not $hasMain) {
        throw '远程已有其他分支，但没有 main。请先检查远程历史并确定目标分支，脚本已停止。'
    }
    if ($hasMain) {
        & git fetch --no-tags origin 'refs/heads/main:refs/remotes/origin/main'
        if ($LASTEXITCODE -ne 0) { throw '获取远程 main 失败。' }
        $previousPreference = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        & git merge-base --is-ancestor refs/remotes/origin/main HEAD
        $ancestorExit = $LASTEXITCODE
        $ErrorActionPreference = $previousPreference
        if ($ancestorExit -ne 0) { throw '远程 main 不是本地历史的祖先，请先审查并合并远程历史；未执行强制推送。' }
    }
    $remoteTagObject = ''
    $remoteTagCommit = ''
    foreach ($line in $remoteRefs) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -ne 2) { continue }
        if ($parts[1] -eq $tagRef) { $remoteTagObject = $parts[0] }
        if ($parts[1] -eq "$tagRef^{}") { $remoteTagCommit = $parts[0] }
    }
    if (-not $remoteTagCommit) { $remoteTagCommit = $remoteTagObject }
    if ($remoteTagCommit -and $remoteTagCommit -ne $headCommit) {
        throw "远程 $tag 已指向其他提交，推送已停止。"
    }
    # If an equivalent remote tag already exists, leave its tag object intact.
    $pushRefs = @('main')
    if (-not $remoteTagObject) { $pushRefs += $tagRef }
    & git push --atomic --set-upstream origin @pushRefs
    if ($LASTEXITCODE -ne 0) { throw '推送失败，本地提交和标签已保留；请检查错误后重试。' }
    Write-Output "Saved and pushed $tag to $RemoteUrl"
    & git log -1 --oneline
} finally {
    Pop-Location
}
