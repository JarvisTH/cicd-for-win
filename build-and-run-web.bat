@echo off
chcp 65001 >nul 2>&1
title CI/CD Console
setlocal

cd /d "%~dp0"

set "GOROOT=D:\software\go"
set "PATH=%GOROOT%\bin;%PATH%"

taskkill /f /im ci.exe >nul 2>&1

echo ============================================
echo  [1/2] Building CI/CD tool...
echo ============================================
go build -o "%~dp0ci.exe" ".\cmd\ci"
if errorlevel 1 (
    echo.
    echo [!!] Build FAILED. Check code or Go environment.
    echo     Make sure Go is installed at: %GOROOT%
    echo.
    pause
    exit /b 1
)
echo [OK] Build success - ci.exe generated
echo.

if not exist "%~dp0ci.exe" (
    echo [!!] ci.exe not found after build
    pause
    exit /b 1
)

echo ============================================
echo  [2/2] Starting Web UI...
echo ============================================
echo URL:  http://localhost:8080
echo User: admin / 123456
echo.
echo  Starting in background (no console window)...
echo  To stop: right-click tray icon - exit
echo.

REM Use VBS to launch ci.exe silently (no cmd window)
start "" /b wscript //nologo "%~dp0start-web.vbs"
exit /b 0
