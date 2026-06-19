@echo off
chcp 65001 >nul
title CI/CD MCP Server
:: 通过 stdio 实现 MCP 协议（JSON-RPC 2.0）
:: 被 Host（AtomCode/Claude Desktop）自动调用

set "CI_DIR=%~dp0"
set "CI_EXE=%CI_DIR%ci.exe"

if not exist "%CI_EXE%" (
    if exist "%CI_DIR%ci.ps1" set "CI_EXE=powershell.exe -ExecutionPolicy Bypass -File %CI_DIR%ci.ps1"
)

:loop
set "LINE="
set /p LINE=
if not defined LINE goto :loop

:: 解析 JSON-RPC 请求（简化版：直接用 PowerShell 处理）
echo %LINE% | powershell.exe -ExecutionPolicy Bypass -Command -
goto :loop
