# Xlink Wails Client Makefile

.PHONY: all dev build build-windows build-darwin build-linux clean install-deps

# 默认目标
all: build

# 安装依赖
install-deps:
	@echo "Installing Go dependencies..."
	go mod download
	@echo "Installing frontend dependencies..."
	cd frontend && npm install
	@echo "Installing Wails CLI..."
	go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 开发模式
dev:
	wails dev

# 构建所有平台
build: build-windows

# Windows 构建
build-windows:
	@echo "Building for Windows..."
	wails build -platform windows/amd64 -ldflags "-H windowsgui -s -w"
	@echo "Build complete: build/bin/xlink-client.exe"

# macOS 构建
build-darwin:
	@echo "Building for macOS (Intel)..."
	wails build -platform darwin/amd64
	@echo "Building for macOS (Apple Silicon)..."
	wails build -platform darwin/arm64

# Linux 构建
build-linux:
	@echo "Building for Linux..."
	wails build -platform linux/amd64

# 清理构建产物
clean:
	@echo "Cleaning..."
	rm -rf build/bin
	rm -rf frontend/dist
	rm -rf frontend/node_modules
	rm -f config_*.json

# 生成图标
generate-icon:
	@echo "Generating icons..."
	wails generate icons build/appicon.png

# 运行测试
test:
	go test -v ./...

# 代码检查
lint:
	go vet ./...
	cd frontend && npm run type-check

# 打包发布
package: build-windows
	@echo "Packaging..."
	mkdir -p dist
	cp build/bin/xlink-client.exe dist/
	cp resources/* dist/ 2>/dev/null || true
	@echo "Package complete: dist/"

# 帮助
help:
	@echo "Xlink Wails Client Build System"
	@echo ""
	@echo "Usage:"
	@echo "  make install-deps  - Install all dependencies"
	@echo "  make dev           - Start development server"
	@echo "  make build         - Build for Windows"
	@echo "  make build-darwin  - Build for macOS"
	@echo "  make build-linux   - Build for Linux"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make package       - Create distribution package"
	@echo "  make help          - Show this help"
