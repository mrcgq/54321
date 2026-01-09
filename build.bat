@echo off
setlocal enabledelayedexpansion

echo ========================================
echo   Xlink Wails Client Build Script
echo ========================================
echo.

:: 检查 Go
where go >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] Go is not installed or not in PATH
    exit /b 1
)

:: 检查 Node.js
where node >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] Node.js is not installed or not in PATH
    exit /b 1
)

:: 检查 Wails
where wails >nul 2>nul
if %errorlevel% neq 0 (
    echo [INFO] Installing Wails CLI...
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
)

:: 安装前端依赖
echo [INFO] Installing frontend dependencies...
cd frontend
call npm install
if %errorlevel% neq 0 (
    echo [ERROR] Failed to install frontend dependencies
    exit /b 1
)
cd ..

:: 构建
echo [INFO] Building application...
wails build -platform windows/amd64 -ldflags "-H windowsgui -s -w"
if %errorlevel% neq 0 (
    echo [ERROR] Build failed
    exit /b 1
)

:: 复制资源文件
echo [INFO] Copying resources...
if not exist "build\bin\resources" mkdir "build\bin\resources"
if exist "resources\*" copy "resources\*" "build\bin\" >nul 2>nul

echo.
echo ========================================
echo   Build completed successfully!
echo   Output: build\bin\xlink-client.exe
echo ========================================
echo.

pause
