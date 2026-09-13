# Makefile for Takealot AutoBest

.PHONY: all help dev build release clean bump

BINARY_NAME := takealot
DIST_DIR := dist
SCRIPTS_DIR := scripts
DEFAULT_VERSION := 0.1.0

# 自动计算下一个版本号：首个版本0.1.0，直至0.1.20递增至0.2.0，0.20.0递增至1.0.0
CALC_VERSION := $(shell python3 $(SCRIPTS_DIR)/bump_version.py 2>/dev/null || echo $(DEFAULT_VERSION))
VERSION ?= $(CALC_VERSION)
TAG := v$(VERSION)

all: build

help:
	@echo "========================================================"
	@echo "🛠️  Takealot 自动化控制中心开发与发布帮助"
	@echo "========================================================"
	@echo "  make dev          启动本地开发模式 (自动释放端口、热启动并打开浏览器)"
	@echo "  make build        编译当前机器本地单文件二进制"
	@echo "  make bump         查看当前计算出的下一个版本号"
	@echo "  make release      自动升级版本号、全平台打包(ZIP格式)并发布至 GitHub Release"
	@echo "                    (支持手动覆盖版本: make release VERSION=0.1.0)"
	@echo "  make clean        清理构建产物与临时日志"
	@echo "========================================================"

bump:
	@echo "📌 当前版本计算结果: $(VERSION) (Git Tag: $(TAG))"

dev:
	@echo "🚀 正在启动开发模式..."
	@lsof -ti :8000 | xargs kill -9 2>/dev/null || true
	@go run -ldflags="-X main.Version=dev" .

build:
	@echo "🔨 正在构建本地可执行程序 (v$(VERSION))..."
	@go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BINARY_NAME) .
	@chmod +x $(BINARY_NAME)
	@echo "✅ 构建完成: ./$(BINARY_NAME)"

release:
	@echo "========================================================"
	@echo "📦 正在执行自动化发布: $(TAG)"
	@echo "========================================================"
	@which gh >/dev/null 2>&1 || (echo "❌ 错误: 未安装 GitHub CLI (gh)" && exit 1)
	@gh auth status >/dev/null 2>&1 || (echo "❌ 错误: GitHub CLI 未登录，请先运行 gh auth login" && exit 1)
	@echo "1. 清理并初始化构建目录..."
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@echo "2. 开始跨平台编译与打包 (统一 ZIP 格式)..."
	@echo "   -> [macOS] 编译 Apple Silicon (arm64)..."
	@GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/Takealot-mac .
	@chmod +x $(DIST_DIR)/Takealot-mac
	@echo "   -> [macOS] 组装 Takealot.app Bundle..."
	@mkdir -p $(DIST_DIR)/Takealot.app/Contents/MacOS
	@cp $(DIST_DIR)/Takealot-mac $(DIST_DIR)/Takealot.app/Contents/MacOS/Takealot
	@printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>' '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' '<plist version="1.0"><dict><key>CFBundleExecutable</key><string>Takealot</string><key>CFBundleIdentifier</key><string>com.takealot.autobest</string><key>CFBundleName</key><string>Takealot</string><key>CFBundleDisplayName</key><string>Takealot</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleShortVersionString</key><string>$(VERSION)</string><key>CFBundleVersion</key><string>$(VERSION)</string><key>LSMinimumSystemVersion</key><string>11.0</string><key>NSHighResolutionCapable</key><true/></dict></plist>' > $(DIST_DIR)/Takealot.app/Contents/Info.plist
	@chmod +x $(DIST_DIR)/Takealot.app/Contents/MacOS/Takealot
	@cp -f Takealot.command $(DIST_DIR)/Takealot.command 2>/dev/null || true
	@cp -f config.example.json "待上传摸板.xlsx" README.md $(DIST_DIR)/
	@echo "   -> [macOS] 打包 macOS arm64 ZIP 压缩包..."
	@cd $(DIST_DIR) && zip -q -r Takealot-v$(VERSION)-macOS-arm64.zip Takealot.app Takealot-mac Takealot.command config.example.json "待上传摸板.xlsx" README.md
	@rm -f $(DIST_DIR)/Takealot-mac $(DIST_DIR)/Takealot.command
	@rm -rf $(DIST_DIR)/Takealot.app
	@echo "   -> [Windows] 编译 x86_64 二进制与打包 ZIP..."
	@GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/Takealot.exe .
	@cd $(DIST_DIR) && zip -q -r Takealot-v$(VERSION)-windows-amd64.zip Takealot.exe config.example.json "待上传摸板.xlsx" README.md
	@rm -f $(DIST_DIR)/Takealot.exe
	@echo "   -> [Linux] 编译 x86_64 二进制与打包 ZIP..."
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(DIST_DIR)/takealot .
	@cd $(DIST_DIR) && zip -q -r Takealot-v$(VERSION)-linux-amd64.zip takealot config.example.json "待上传摸板.xlsx" README.md
	@rm -f $(DIST_DIR)/takealot $(DIST_DIR)/config.example.json $(DIST_DIR)/"待上传摸板.xlsx" $(DIST_DIR)/README.md
	@echo "3. 生成 Release Notes..."
	@printf "## 🚀 %s 发布\n\n### ✨ 核心功能\n- **实时比价战况看板**：展示处于优先、失去优先、独家在售统计及竞品最优价差\n- **智能防亏底价保护**：针对单个商品配置独立保护底价，守住利润底线\n- **全自动跟价引擎**：毫秒级多协程轮询，自动下调/回调商品售价\n- **变体批量跟卖**：支持上传 Excel 自动识别 TSIN/PLID 变体并上架\n- **跨平台原生单文件**：零依赖运行，提供双击即用与 Web 控制台\n\n### 📦 资产下载 (统一 ZIP 格式)\n- **macOS (Apple Silicon arm64)**：\`Takealot-v%s-macOS-arm64.zip\` (解压即用)\n- **Windows (x64)**：\`Takealot-v%s-windows-amd64.zip\` (解压双击运行)\n- **Linux (x64)**：\`Takealot-v%s-linux-amd64.zip\` (解压运行)\n" "$(TAG)" "$(VERSION)" "$(VERSION)" "$(VERSION)" > $(DIST_DIR)/release_notes.md
	@echo "4. 更新 VERSION 文件与 Git 提交..."
	@echo $(VERSION) > VERSION
	@git add VERSION scripts/ Makefile main.go pkg/ web/ Takealot.command .gitignore README.md 2>/dev/null || true
	@git diff --cached --quiet || git commit -m "chore: release $(TAG)"
	@git push origin main
	@echo "5. 创建并推送 Git Tag $(TAG)..."
	@git tag -f -a "$(TAG)" -m "$(TAG)"
	@git push -f origin "$(TAG)"
	@echo "6. 调用 GitHub CLI 发布 Release 并上传打包资产..."
	@gh release create "$(TAG)" 		$(DIST_DIR)/Takealot-v$(VERSION)-macOS-arm64.zip 		$(DIST_DIR)/Takealot-v$(VERSION)-windows-amd64.zip 		$(DIST_DIR)/Takealot-v$(VERSION)-linux-amd64.zip 		--title "$(TAG)" 		--notes-file $(DIST_DIR)/release_notes.md
	@echo "========================================================"
	@echo "🎉 发布完成！"
	@gh release view "$(TAG)"
	@echo "========================================================"

clean:
	@echo "🧹 清理临时文件..."
	@rm -rf $(DIST_DIR) $(BINARY_NAME) server.log
	@echo "✅ 清理完成"
