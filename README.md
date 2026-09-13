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

---

## 🛠️ 常用开发与发布命令 (Makefile)

本项目内置完整的 `Makefile`，支持一键开发与自动发布：

```bash
# 启动本地开发调试 (自动释放端口、热运行、并打开浏览器)
make dev

# 编译当前平台本地单文件
make build

# 查看按版本规则递增计算出的下一个版本号
make bump

# 🚀 自动按版本规则递增、全平台编译打包并发布到 GitHub Release
make release

# 手动指定特定版本发布
make release VERSION=0.1.0
```

### 🏷️ 自动化版本递增规则
* 初始版本：`0.1.0`
* Patch 递增：`0.1.0` -> `0.1.1` -> ... -> `0.1.20`
* Patch 逢 20 进位：`0.1.20` 下一个版本为 `0.2.0`
* Minor 逢 20 进位：`0.20.0` 下一个版本为 `1.0.0`
* `make release` 会全自动处理交叉编译、打 Zip 归档、打 Git Tag、推送并调用 `gh` 发布到 GitHub Releases。


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
