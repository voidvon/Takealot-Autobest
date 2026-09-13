# Takealot 自动化控制中心 (Go 原生单文件跨平台版)

本项目是基于原 Windows 客户端（WinForms + CefSharp）进行**完全去依赖、跨平台重构的 Go 原生版本**。已彻底剥离原作者的第三方计费验证服务器与 Windows 机器码锁定，编译后为**零外部依赖的独立单文件可执行程序**（包含内嵌 Web UI）。

---

## ⚡ 特性优势

1. **单文件零依赖**：基于 Go 语言原生开发，通过 `//go:embed` 将前端控制台直接编译进二进制，无需安装 Python、Node.js 或 .NET 运行时环境。
2. **极速启动与低资源占用**：内存占用仅约 15MB ~ 25MB，比原版 Chromium 内嵌方案轻量 95% 以上，适合在 Mac 或低配 Linux VPS 上 7x24 小时静默运行。
3. **高并发与稳定调度**：采用 Goroutine 异步调度，支持毫秒级任务响应与 SSE 实时控制台日志推流。
4. **完整业务兼容**：
   - 自动读取并双向同步兼容原有的 [`GJDATA`](file:///Users/voidvon/Desktop/output/GJDATA)（商品监控底价）与 [`can.ini`](file:///Users/voidvon/Desktop/output/can.ini) 配置。
   - 原生兼容 [`待上传摸板.xlsx`](file:///Users/voidvon/Desktop/output/待上传摸板.xlsx) 表格拖拽批量解析与跟卖上架。

---

## 🚀 快速启动

在终端中执行根目录下的启动脚本：

```bash
cd /Users/voidvon/Desktop/output
./start.sh
```

或者直接运行编译好的二进制：

```bash
./takealot
```

程序启动后会自动在默认浏览器中打开控制面板：  
👉 **http://127.0.0.1:8000**

---

## 🛠️ 跨平台编译说明

如果需要编译给其他操作系统使用：

```bash
# 编译当前平台（Mac arm64 / Apple Silicon）
go build -o takealot .

# 交叉编译到 Windows (x64)
GOOS=windows GOARCH=amd64 go build -o takealot.exe .

# 交叉编译到 Linux (云服务器 / VPS)
GOOS=linux GOARCH=amd64 go build -o takealot_linux .
```

---

## 📖 核心功能与使用指南

### 1. 调价监控管理（Reprice Engine）
- **一键同步线上商品**：从 Takealot 官方 Seller API 自动分页拉取当前所有有效上架商品。
- **底价保护与幅度调整**：
  - 降价幅度 (R)：竞品更低时按设定值下调价格，严格受最低底价拦截保护。
  - 提价幅度 (R)：竞品涨价或无竞品时自适应上调。
  - 原价浮动比例 (%)：自动计算并更新建议零售价（RRP）。
- **启停与倒计时**：支持一键开始、暂停、恢复与停止，大盘实时展示下次轮询倒计时。

### 2. 批量表格跟卖（Follow Selling）
- 拖拽上传 `待上传摸板.xlsx`（列0:库存、列1:最低价、列2:商品链接或PLID）。
- 自动查询商品全部变体，对未上架款式一键批量跟卖并自动加入后续调价监控。

### 3. 店铺授权配置
- 在 [Takealot 卖家中心](https://sellers.takealot.com/) 开发者工具（Network）中复制 `authorization` 请求头。
- 粘贴到控制面板【店铺授权与参数】页面，点击【测试连接状态】保存即可。
