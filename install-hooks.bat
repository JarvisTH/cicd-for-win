@echo off
chcp 65001 >nul
title 安装 Git Hooks

set "CI_CD_DIR=%~dp0"
set "HOOKS_SRC=%CI_CD_DIR%hooks"
set "CONFIG_FILE=%CI_CD_DIR%projects.json"

echo ============================================
echo   CI/CD Git Hooks 安装工具
echo ============================================
echo.

if not exist "%HOOKS_SRC%\pre-commit" (
    echo 错误: 找不到 hooks 模板目录
    exit /b 1
)
if not exist "%CONFIG_FILE%" (
    echo 错误: 找不到 projects.json
    echo 请先添加项目后再安装 hooks
    exit /b 1
)

powershell.exe -ExecutionPolicy Bypass -Command ^
    $config = Get-Content '%CONFIG_FILE%' -Raw | ConvertFrom-Json; ^
    $hooksSrc = '%HOOKS_SRC%'; ^
    $installed = 0; ^
    $skipped = 0; ^
    foreach ($proj in $config.projects) { ^
        if (-not $proj.enabled) { $skipped++; continue }; ^
        $gitDir = Join-Path $proj.path '.git'; ^
        $hooksDir = Join-Path $gitDir 'hooks'; ^
        if (-not (Test-Path $gitDir)) { ^
            Write-Host "  ⚠ $($proj.name): 不是 Git 仓库，跳过" -ForegroundColor Yellow; ^
            $skipped++; ^
            continue; ^
        }; ^
        if (-not (Test-Path $hooksDir)) { New-Item -ItemType Directory -Path $hooksDir -Force | Out-Null }; ^
        Copy-Item (Join-Path $hooksSrc 'pre-commit') (Join-Path $hooksDir 'pre-commit') -Force; ^
        Copy-Item (Join-Path $hooksSrc 'pre-push') (Join-Path $hooksDir 'pre-push') -Force; ^
        Write-Host "  ✅ $($proj.name): hooks 已安装" -ForegroundColor Green; ^
        $installed++; ^
    }; ^
    Write-Host ''; ^
    Write-Host "安装完成: $installed 个项目已安装, $skipped 个项目已跳过" -ForegroundColor Cyan

echo.
echo ============================================
pause
