# 持久 WebView 会话只读原型

这个实验验证：用户手动完成 Cloudflare 挑战后，后续 GET 是否能在同一浏览器会话中连续成功，以及重启后能否复用已保存的会话。

目前是 **macOS 14+ / WKWebView 原型**，尚未接入正式调价任务，也没有实现 Windows WebView2 适配。主程序已有修复不依赖此实验。

## 启动

需要本机 Xcode Command Line Tools、Python 3，以及已配置店铺凭证的 SQLite 数据库：

```sh
python3 experiments/browser-session-probe/launch.py \
  --database /absolute/path/to/takealot.db --store default
```

构建产物放在已忽略的 `build/bin/TakealotSessionProbe.app`。必须通过启动脚本运行；凭证通过标准输入交给进程，不放入命令行、URL、配置文件或日志。脚本只读数据库，不修改店铺配置。原型不读取、导出或转移验证 Cookie，后续请求都在 WKWebView 内执行。

每个数据库路径和店铺 ID 映射到固定的 WebKit 数据存储标识，两类接口使用两个保留的页面，并共享该店铺的持久存储。它不使用普通 Safari 浏览器的个人资料。关闭窗口后再次执行相同命令复用原型资料；Cookie 是否仍被站点接受需要实际验证。

原型使用 WebView 的系统网络路径，可配合 TUN。它目前没有应用 Go 客户端的店铺代理设置；数据库配置了店铺代理时会明确拒绝启动，防止误认为测试走了该代理。

## 操作

1. 分别进入“卖家后台接口”和“前台商品接口”，由用户手动完成网页验证。
2. 验证后的页面应显示 JSON。“重新打开验证页”会在保留会话的前提下重新载入接口，并为卖家接口附带原店铺 Key；若完成验证后出现缺少授权，也可使用此按钮。
3. 点击“连续测试 3 轮”。每个请求之间间隔 5 秒，共 9 个只读请求：
   - `GET https://seller-api.takealot.com/v2/offers/count`
   - `GET https://seller-api.takealot.com/v2/seller`
   - `GET https://api.takealot.com/rest/v-1-16-0/product-details/PLID98958583?platform=desktop`
4. 再次遇到挑战会停止连续测试，切换到对应页面，由用户完成后手动重新开始。
5. 关闭原型，再使用相同命令启动，检查能否直接得到 JSON，并再次运行三轮。

“停止测试”取消排队的后续请求，已发出的 GET 最长等待 20 秒。原型不进行自动解题、自动点击验证或循环重试，不包含价格修改请求。

## 持续观察

默认增加“持续观察 60 分钟 / 恢复”按钮；可用 `--duration-minutes`（1–1440）和 `--round-interval`（至少 30 秒，默认 60）调整。每轮请求至少间隔一分钟，每次请求间隔至少 5 秒，除上述三个接口，还读取以下接口：

- `/v2/offers/detailed?page_number=1&page_size=20&status_ids=1&status_ids=2`
- `/1/catalogue/mpv/search?search_by=tsin&search_query=<TSIN>`

目录和前台详情轮换数据库中已选中的最多 20 个商品，每轮共 5 次 GET。429/420 和服务器错误会记录失败并冷却至少 60 秒（遵守数字型 Retry-After）；浏览器执行错误记录后等待至少 30 秒。再次验证或授权错误会暂停，人工处理后点击观察按钮恢复，保留统计与原定结束时间。暂停期间的时间也计入观察，不能算作正常连续运行。

`browser-session-metrics.json` 每 15 秒及每次请求完成时更新，包含实际请求结果、延迟、阶段和结束标记。独立监控进程可以检查进程退出、超过 60 秒未更新心跳、超过 180 秒无响应及验证暂停：

```sh
python3 experiments/browser-session-probe/monitor.py \
  --metrics /absolute/path/to/browser-session-metrics.json --pid <Prototype-PID>
```

监控写入同目录的 `browser-session-watchdog.json`；心跳正常本身不代表接口成功。它不会发出业务请求或自动通过验证。

日志在数据库所在目录的 `browser-session-probe.log`（权限 0600），仅记录时间、HTTP 状态、数据是否有效和挑战状态，不输出 Key、Cookie 或响应正文。访问页面和业务测试均绑定固定 Takealot 主机；不把正式应用的 Go 绑定暴露给第三方页面。

## 接入主程序前仍需验证

- 多轮访问、跨进程持久化、代理切换、过期后再次验证，以及失败/关闭窗口时的暂停恢复。
- Windows WebView2 环境下的人工作答可行性和资料持久化。
- 主程序需要可替换的请求通道和人工验证状态，避免认证前重复请求；保留现有速率限制、商品身份校验与底价保护。
- 真实 PATCH 改价是否被平台接受。本原型只读，GET 成功不代表写入权限已验证。
