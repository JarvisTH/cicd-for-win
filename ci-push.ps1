param([string]$ProjectName)

# 设置输出编码为 UTF-8，避免中文乱码
$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$configPath = Join-Path $PSScriptRoot "projects.json"
$config = Get-Content $configPath -Raw | ConvertFrom-Json
$proj = $config.projects | Where-Object { $_.name -eq $ProjectName -and $_.enabled } | Select-Object -First 1
if (-not $proj) { Write-Error "未找到项目: $ProjectName"; exit 1 }

Push-Location $proj.path

$allSuccess = $true
foreach ($remote in $proj.remotes) {
    if (-not $remote.enabled) { continue }
    $existing = & "git.exe" remote -v 2>&1 | Where-Object { $_ -match "^$($remote.name)\s" }
    if (-not $existing) {
        & "git.exe" remote add $remote.name $remote.url 2>&1
        Write-Output "  添加远程: $($remote.name) → $($remote.url)"
    }
    Write-Output "📤 推送到 $($remote.name)..."
    $result = & "git.exe" push $remote.name main 2>&1
    if ($LASTEXITCODE -eq 0) { Write-Output "  ✅ $($remote.name): 推送成功" }
    else { Write-Output "  ❌ $($remote.name): 推送失败"; $allSuccess = $false }
}

Pop-Location
if (-not $allSuccess) { exit 1 }
