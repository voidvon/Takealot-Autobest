# Makefile for Takealot AutoBest (Wails 跨平台桌面端)

.PHONY: all help dev dev-server build release clean bump

BINARY_NAME := takealot
DIST_DIR := dist
SCRIPTS_DIR := scripts
DEFAULT_VERSION := 0.2.0

WAILS := $(shell which wails 2>/dev/null || echo $(HOME)/go/bin/wails)

# 自动计算下一个版本号
CALC_VERSION := $(shell python3 $(SCRIPTS_DIR)/bump_version.py 2>/dev/null || echo $(DEFAULT_VERSION))
VERSION ?= $(CALC_VERSION)
TAG := v$(VERSION)

all: build

help:
	@echo "========================================================"
	@echo "🛠️  Takealot 自动化桌面应用开发与发布帮助"
	@echo "========================================================"
	@echo "  make dev          启动 Wails 桌面开发模式 (热重载 React UI 与 Go 后端)"
	@echo "  make dev-server   以纯命令行/服务端模式启动开发 (不弹出桌面窗口)"
	@echo "  make build        编译当前平台原生桌面应用 (macOS 输出 Takealot.app)"
	@echo "  make bump         查看当前计算出的下一个版本号"
	@echo "  make release      自动升级版本号、全平台跨端打包并发布至 GitHub Release"
	@echo "                    (支持手动覆盖版本: make release VERSION=0.2.0)"
	@echo "  make clean        清理构建产物与临时文件"
	@echo "========================================================"

bump:
	@echo "📌 当前版本计算结果: $(VERSION) (Git Tag: $(TAG))"

dev: build-frontend
	@echo "🚀 正在启动 Wails 桌面热开发模式..."
	@lsof -ti :8000 | xargs kill -9 2>/dev/null || true
	@$(WAILS) dev

dev-server: build-frontend
	@echo "🚀 正在以服务端模式启动开发..."
	@lsof -ti :8000 | xargs kill -9 2>/dev/null || true
	@go run -ldflags="-X main.Version=dev" . -server-only

build-frontend:
	@echo "🎨 正在构建现代化 Vite React 前端 (Base UI Nova 风格)..."
	@cd web && ([ -d node_modules ] || npm install) && npm run build

build: build-frontend
	@echo "🔨 正在使用 Wails 构建本地原生桌面应用 (v$(VERSION))..."
	@$(WAILS) build -s -clean -ldflags="-s -w -X main.Version=$(VERSION)"
	@rm -rf Takealot.app $(BINARY_NAME)
	@cp -R build/bin/Takealot.app . 2>/dev/null || true
	@cp build/bin/Takealot.app/Contents/MacOS/Takealot ./$(BINARY_NAME) 2>/dev/null || true
	@chmod +x $(BINARY_NAME) 2>/dev/null || true
	@echo "✅ 构建完成: ./Takealot.app (双击无黑框直接运行) 及单文件 ./$(BINARY_NAME)"

release: build-frontend
	@echo "========================================================"
	@echo "📦 正在执行自动化发布: $(TAG)"
	@echo "========================================================"
	@which gh >/dev/null 2>&1 || (echo "❌ 错误: 未安装 GitHub CLI (gh)" && exit 1)
	@gh auth status >/dev/null 2>&1 || (echo "❌ 错误: GitHub CLI 未登录，请先运行 gh auth login" && exit 1)
	@echo "1. 清理并初始化构建目录..."
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@echo "2. 开始跨平台编译与打包 (统一 ZIP 格式)..."
	@echo "   -> [macOS] 编译 Apple Silicon (arm64) 原生桌面 App..."
	@$(WAILS) build -platform darwin/arm64 -s -clean -ldflags="-s -w -X main.Version=$(VERSION)"
	@cp -R build/bin/Takealot.app $(DIST_DIR)/Takealot.app
	@cp -f "批量导入模板.xlsx" README.md $(DIST_DIR)/
	@echo "   -> [macOS] 打包 macOS arm64 ZIP 压缩包..."
	@cd $(DIST_DIR) && zip -q -r Takealot-v$(VERSION)-macOS-arm64.zip Takealot.app "批量导入模板.xlsx" README.md
	@rm -rf $(DIST_DIR)/Takealot.app
	@echo "   -> [Windows] 编译 x86_64 原生桌面程序 (无黑框 GUI) 与打包 ZIP..."
	@$(WAILS) build -platform windows/amd64 -s -clean -ldflags="-s -w -X main.Version=$(VERSION)"
	@cp build/bin/Takealot.exe $(DIST_DIR)/Takealot.exe
	@cd $(DIST_DIR) && zip -q -r Takealot-v$(VERSION)-windows-amd64.zip Takealot.exe "批量导入模板.xlsx" README.md
	@rm -f $(DIST_DIR)/Takealot.exe
	@echo "   -> [Linux] 编译 x86_64 纯服务端/CLI 二进制与打包 ZIP..."
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/takealot .
	@cd $(DIST_DIR) && zip -q -r Takealot-v$(VERSION)-linux-amd64.zip takealot "批量导入模板.xlsx" README.md
	@rm -f $(DIST_DIR)/takealot $(DIST_DIR)/"批量导入模板.xlsx" $(DIST_DIR)/README.md
	@echo "3. 生成 Release Notes..."
	@printf "## 🚀 %s 发布\n\n### ✨ 核心功能\n- **Wails 现代化原生桌面端**：彻底告别控制台黑框终端，原生窗口启动，内置现代化 Web 交互\n- **多店铺聚合管理**：支持多店铺快捷切换、各店铺独立配置与聚合数据看板\n- **软件授权与安全体系**：支持机器硬件指纹绑定、离线激活码验证与授权生命周期管理\n- **实时比价战况看板**：展示处于优先、失去优先、独家在售统计及竞品最优价差\n- **智能防亏底价保护**：支持按倍数批量计算与单品设置最低保护底价，守住利润底线\n- **全自动跟价引擎**：毫秒级多协程轮询，自动下调/回调商品售价\n- **变体批量跟卖**：支持上传 Excel 自动识别 TSIN/PLID 变体并上架\n- **双模运行能力**：双击即可启动沉浸式桌面 GUI，亦支持通过 \`--server-only\` 以纯后端服务模式运行\n\n### 📦 资产下载 (统一 ZIP 格式)\n- **macOS (Apple Silicon arm64)**：\`Takealot-v%s-macOS-arm64.zip\` (解压后双击 Takealot.app 即用，无黑框)\n- **Windows (x64)**：\`Takealot-v%s-windows-amd64.zip\` (解压后双击 Takealot.exe，纯原生桌面窗口无 cmd 黑框)\n- **Linux (x64)**：\`Takealot-v%s-linux-amd64.zip\` (解压运行，纯服务端/CLI)\n" "$(TAG)" "$(VERSION)" "$(VERSION)" "$(VERSION)" > $(DIST_DIR)/release_notes.md
	@echo "4. 更新 VERSION 文件与 Git 提交..."
	@echo $(VERSION) > VERSION
	@git add VERSION scripts/ Makefile main.go app.go wails.json pkg/ web/ "批量导入模板.xlsx" .gitignore README.md 2>/dev/null || true
	@git diff --cached --quiet || git commit -m "chore: release $(TAG) with Wails cross-platform GUI"
	@git push origin main
	@echo "5. 创建并推送 Git Tag $(TAG)..."
	@git tag -f -a "$(TAG)" -m "$(TAG)"
	@git push -f origin "$(TAG)"
	@echo "6. 调用 GitHub CLI 发布 Release 并上传打包资产..."
	@gh release create "$(TAG)" \
		$(DIST_DIR)/Takealot-v$(VERSION)-macOS-arm64.zip \
		$(DIST_DIR)/Takealot-v$(VERSION)-windows-amd64.zip \
		$(DIST_DIR)/Takealot-v$(VERSION)-linux-amd64.zip \
		--title "$(TAG)" \
		--notes-file $(DIST_DIR)/release_notes.md
	@echo "========================================================"
	@echo "🎉 发布完成！"
	@gh release view "$(TAG)"
	@echo "========================================================"

clean:
	@echo "🧹 清理临时文件..."
	@rm -rf $(DIST_DIR) $(BINARY_NAME) Takealot.app build/bin server.log
	@echo "✅ 清理完成"
