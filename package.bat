@echo off
setlocal enabledelayedexpansion
title CI/CD Package Tool

set "SOURCE=%~dp0"
set "OUTPUT=%SOURCE%ci-cd-dist.zip"
set "TEMP_DIR=%TEMP%\cicd-pack"

echo ============================================
echo   CI/CD Distribution Packager
echo ============================================
echo.

:: Check required file
if not exist "%SOURCE%ci-runner.ps1" (
    echo ERROR: Cannot find ci-runner.ps1
    pause
    exit /b 1
)

:: Build if needed
if not exist "%SOURCE%ci.exe" (
    echo Building ci.exe...
    "D:\software\go\bin\go.exe" build -ldflags="-s -w" -o "%SOURCE%ci.exe" "%SOURCE%cmd\ci"
    if errorlevel 1 (
        echo ERROR: Build failed
        pause
        exit /b 1
    )
)

:: Prepare temp
if exist "%TEMP_DIR%" rmdir /s /q "%TEMP_DIR%"
mkdir "%TEMP_DIR%"
mkdir "%TEMP_DIR%\rules"
mkdir "%TEMP_DIR%\hooks"

:: Copy files
copy "%SOURCE%ci.exe" "%TEMP_DIR%\" >nul
copy "%SOURCE%ci-runner.ps1" "%TEMP_DIR%\" >nul
copy "%SOURCE%cd-deploy.ps1" "%TEMP_DIR%\" >nul
copy "%SOURCE%ci-push.ps1" "%TEMP_DIR%\" >nul
copy "%SOURCE%install-hooks.bat" "%TEMP_DIR%\" >nul
copy "%SOURCE%README.txt" "%TEMP_DIR%\" >nul
copy "%SOURCE%rules\eslint-vue.mjs" "%TEMP_DIR%\rules\" >nul
copy "%SOURCE%rules\checkstyle.xml" "%TEMP_DIR%\rules\" >nul
copy "%SOURCE%hooks\pre-commit" "%TEMP_DIR%\hooks\" >nul
copy "%SOURCE%hooks\pre-push" "%TEMP_DIR%\hooks\" >nul

:: Zip
echo Creating zip...
powershell -Command "Compress-Archive -Path \"$env:TEMP\cicd-pack\*\" -DestinationPath \"%OUTPUT:\=\%\" -Force"
if errorlevel 1 (
    echo ERROR: Zip failed
    pause
    exit /b 1
)

:: Clean
rmdir /s /q "%TEMP_DIR%"

echo Done: %OUTPUT%
echo.
echo Extract and run: ci.exe serve
echo.
pause
