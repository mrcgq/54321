#!/bin/bash

set -e

echo "========================================"
echo "  Xlink Wails Client Build Script"
echo "========================================"
echo

# 检测操作系统
OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
    Linux*)     PLATFORM="linux";;
    Darwin*)    PLATFORM="darwin";;
    *)          echo "Unsupported OS: $OS"; exit 1;;
esac

case "$ARCH" in
    x86_64)     GOARCH="amd64";;
    arm64)      GOARCH="arm64";;
    aarch64)    GOARCH="arm64";;
    *)          echo "Unsupported architecture: $ARCH"; exit 1;;
esac

echo "[INFO] Building for $PLATFORM/$GOARCH"

# 检查依赖
command -v go >/dev/null 2>&1 || { echo "[ERROR] Go is not installed"; exit 1; }
command -v node >/dev/null 2>&1 || { echo "[ERROR] Node.js is not installed"; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "[ERROR] npm is not installed"; exit 1; }

# 检查/安装 Wails
if ! command -v wails >/dev/null 2>&1; then
    echo "[INFO] Installing Wails CLI..."
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
fi

# 安装前端依赖
echo "[INFO] Installing frontend dependencies..."
cd frontend
npm install
cd ..

# 构建
echo "[INFO] Building application..."
wails build -platform "${PLATFORM}/${GOARCH}"

# 复制资源文件
echo "[INFO] Copying resources..."
if [ -d "resources" ]; then
    cp -r resources/* build/bin/ 2>/dev/null || true
fi

echo
echo "========================================"
echo "  Build completed successfully!"
echo "  Output: build/bin/"
echo "========================================"
