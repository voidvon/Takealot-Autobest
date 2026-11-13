import Cocoa
import WebKit

struct Sample: Decodable {
    let tsin: String
    let plid: String
}

struct Configuration: Decodable {
    let authorization: String
    let profile: UUID
    let logPath: String
    let metricsPath: String
    let durationMinutes: Int
    let roundInterval: Int
    let samples: [Sample]
}

// This experiment keeps both the challenge and subsequent GET requests inside
// WKWebView. No cookies are exported to Go and no challenge is solved by code.
@MainActor
final class Probe: NSObject, NSApplicationDelegate, WKNavigationDelegate {
    let config: Configuration
    var window: NSWindow!
    let tabs = NSTabView()
    let status = NSTextField(labelWithString: "正在打开两个接口，请在对应页面手动完成验证")
    let output = NSTextView()
    var views: [WKWebView] = []
    var ready = [false, false]
    var requestInFlight = false
    var generation = 0
    var started: Date?
    var deadline: Date?
    var roundStarted = Date()
    var total = 0
    var successes = 0
    var challenges = 0
    var rateLimits = 0
    var failures = 0
    var finished = false
    var phase = "idle"
    var lastResponse: Date?
    var endTimer: Timer?
    var heartbeat: Timer?
    var latencyTotal = 0.0
    var maxLatency = 0.0
    var ended: Date?
    var nextPosition = 0
    let origins = ["https://seller-api.takealot.com", "https://api.takealot.com"]
    let basicChecks = [(0, "/v2/offers/count", "商品数量"), (0, "/v2/seller", "卖家信息"),
                  (1, "/rest/v-1-16-0/product-details/PLID98958583?platform=desktop", "前台报价")]

    var checks: [(Int, String, String)] {
        guard config.durationMinutes > 0 else { return basicChecks }
        return [(0, "/v2/offers/count", "商品数量"), (0, "/v2/seller", "卖家信息"),
                (0, "/v2/offers/detailed?page_number=1&page_size=20&status_ids=1&status_ids=2", "在售商品"),
                (0, "/1/catalogue/mpv/search?search_by=tsin&search_query=", "目录报价"),
                (1, "/rest/v-1-16-0/product-details/PLID", "前台报价")]
    }

    init(_ configuration: Configuration) { config = configuration }

    func applicationDidFinishLaunching(_ notification: Notification) {
        let store = WKWebsiteDataStore(forIdentifier: config.profile)
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1180, height: 850),
                          styleMask: [.titled, .closable, .resizable, .miniaturizable], backing: .buffered, defer: false)
        window.title = "Takealot 持久 WebView 会话试验（只读）"
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 10
        stack.edgeInsets = NSEdgeInsets(top: 12, left: 12, bottom: 12, right: 12)
        stack.translatesAutoresizingMaskIntoConstraints = false
        window.contentView!.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: window.contentView!.leadingAnchor),
            stack.trailingAnchor.constraint(equalTo: window.contentView!.trailingAnchor),
            stack.topAnchor.constraint(equalTo: window.contentView!.topAnchor),
            stack.bottomAnchor.constraint(equalTo: window.contentView!.bottomAnchor)
        ])
        let note = NSTextField(wrappingLabelWithString: "只读试验：手动完成下方网页验证后，点击连续测试。两个页面持续保留 Cookie；关闭后再次启动使用同一配置。不会修改商品价格。")
        stack.addArrangedSubview(note)
        let buttons = NSStackView()
        for (title, action) in [("重新打开验证页", #selector(reopen)), ("连续测试 3 轮", #selector(test)), ("持续观察 \(config.durationMinutes) 分钟 / 恢复", #selector(soak)), ("停止测试", #selector(stop))] {
            let button = NSButton(title: title, target: self, action: action)
            buttons.addArrangedSubview(button)
        }
        stack.addArrangedSubview(buttons)
        status.lineBreakMode = .byWordWrapping
        status.maximumNumberOfLines = 3
        stack.addArrangedSubview(status)
        for index in 0...1 {
            let cfg = WKWebViewConfiguration()
            cfg.websiteDataStore = store
            // The isolated content world contains the key only for the duration
            // of each same-origin request. No app backend bindings are exposed.
            let view = WKWebView(frame: .zero, configuration: cfg)
            view.navigationDelegate = self
            views.append(view)
            let tab = NSTabViewItem(identifier: index)
            tab.label = index == 0 ? "卖家后台接口" : "前台商品接口"
            tab.view = view
            tabs.addTabViewItem(tab)
        }
        stack.addArrangedSubview(tabs)
        tabs.translatesAutoresizingMaskIntoConstraints = false
        tabs.widthAnchor.constraint(equalTo: stack.widthAnchor, constant: -24).isActive = true
        tabs.heightAnchor.constraint(greaterThanOrEqualToConstant: 460).isActive = true
        let scroll = NSScrollView()
        output.isEditable = false
        output.font = NSFont.monospacedSystemFont(ofSize: 11, weight: .regular)
        scroll.documentView = output
        scroll.hasVerticalScroller = true
        stack.addArrangedSubview(scroll)
        scroll.widthAnchor.constraint(equalTo: tabs.widthAnchor).isActive = true
        scroll.heightAnchor.constraint(equalToConstant: 160).isActive = true
        log("持久会话已启动；代理使用 macOS WebView 的网络设置/TUN；仅允许固定接口的 GET 请求")
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        load(0, checks[0].1)
        load(1, basicChecks[2].1)
        heartbeat = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.writeMetrics() }
        }
    }

    func log(_ message: String) {
        let line = ISO8601DateFormatter().string(from: Date()) + " " + message + "\n"
        output.textStorage?.append(NSAttributedString(string: line))
        output.scrollToEndOfDocument(nil)
        if let data = line.data(using: .utf8), let handle = FileHandle(forWritingAtPath: config.logPath) {
            defer { try? handle.close() }
            _ = try? handle.seekToEnd()
            try? handle.write(contentsOf: data)
        }
    }

    func load(_ index: Int, _ path: String) {
        ready[index] = false
        var req = URLRequest(url: URL(string: origins[index] + path)!)
        if index == 0 { req.setValue(config.authorization, forHTTPHeaderField: "Authorization") }
        req.setValue("application/json, text/plain, */*", forHTTPHeaderField: "Accept")
        views[index].load(req)
    }

    @objc func reopen() {
        stop()
        load(0, checks[0].1)
        load(1, basicChecks[2].1)
        status.stringValue = "请在两个标签页中手动完成验证，再点击连续测试"
    }

    @objc func stop() {
        generation += 1
        status.stringValue = "测试已停止；页面与会话保留"
        if started != nil && !finished { phase = "stopped"; finished = true; ended = Date() }
        endTimer?.invalidate()
        writeMetrics()
        log("停止后续测试（已发出的只读请求可能仍在结束）")
    }

    @objc func soak() {
        guard !requestInFlight else { status.stringValue = "请等待当前请求结束"; return }
        if started != nil && !finished {
            guard phase != "running" && phase != "waiting" else { return }
            generation += 1
            log("人工恢复持续观察；保留已有统计和原定结束时间")
            run(nextPosition, generation)
            return
        }
        generation += 1
        started = Date()
        ended = nil; nextPosition = 0; lastResponse = nil
        deadline = Date().addingTimeInterval(Double(max(1, config.durationMinutes)) * 60)
        total = 0; successes = 0; challenges = 0; rateLimits = 0; failures = 0
        latencyTotal = 0; maxLatency = 0; finished = false; phase = "running"
        endTimer?.invalidate()
        endTimer = Timer.scheduledTimer(withTimeInterval: Double(max(1, config.durationMinutes)) * 60, repeats: false) { [weak self] _ in
            MainActor.assumeIsolated { self?.finishObservation() }
        }
        log("开始持续观察：计划 \(max(1, config.durationMinutes)) 分钟，每轮间隔至少 \(config.roundInterval) 秒，轮换 \(config.samples.count) 个商品；仅 GET")
        writeMetrics()
        run(0, generation)
    }

    func finishObservation() {
        guard !finished else { return }
        generation += 1
        finished = true
        ended = Date()
        phase = phase == "running" || phase == "waiting" ? "completed" : phase
        let message = "观察结束：请求 \(total)，成功 \(successes)，失败 \(failures)，验证 \(challenges)，限流 \(rateLimits)"
        status.stringValue = message
        log(message)
        writeMetrics()
    }

    func writeMetrics() {
        guard let start = started else { return }
        let data: [String: Any] = ["started_at": ISO8601DateFormatter().string(from: start),
            "planned_end": ISO8601DateFormatter().string(from: deadline ?? start),
            "updated_at": ISO8601DateFormatter().string(from: Date()),
            "elapsed_seconds": (ended ?? Date()).timeIntervalSince(start), "phase": phase,
            "finished": finished, "in_flight": requestInFlight, "requests": total,
            "successes": successes, "failures": failures, "challenges": challenges, "rate_limits": rateLimits,
            "success_rate": total > 0 ? Double(successes) / Double(total) : 0,
            "mean_latency_ms": total > 0 ? latencyTotal / Double(total) : 0,
            "max_latency_ms": maxLatency,
            "last_response": lastResponse.map { ISO8601DateFormatter().string(from: $0) } ?? "",
            "pid": ProcessInfo.processInfo.processIdentifier]
        if let encoded = try? JSONSerialization.data(withJSONObject: data, options: [.prettyPrinted, .sortedKeys]) {
            try? encoded.write(to: URL(fileURLWithPath: config.metricsPath), options: .atomic)
            try? FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: config.metricsPath)
        }
    }

    func record(success: Bool, code: Int, challenge: Bool, latency: Double) {
        guard started != nil else { return }
        total += 1
        if success { successes += 1 } else { failures += 1 }
        if challenge { challenges += 1 }
        if code == 429 || code == 420 { rateLimits += 1 }
        latencyTotal += latency; maxLatency = max(maxLatency, latency)
        lastResponse = Date()
        writeMetrics()
    }

    func schedule(_ position: Int, _ token: Int, extraDelay: Double = 0) {
        let width = started == nil ? basicChecks.count : checks.count
        var delay = max(5, extraDelay)
        if started != nil && position % width == 0 {
            delay = max(delay, Double(config.roundInterval) - Date().timeIntervalSince(roundStarted))
            phase = "waiting"
            status.stringValue = "持续观察：已完成 \(position / width) 轮，成功 \(successes)/\(total)，等待下一轮"
            writeMetrics()
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + delay) { self.run(position, token) }
    }

    @objc func test() {
        guard !requestInFlight else { status.stringValue = "请等待当前只读请求结束"; return }
        guard started == nil || finished else { status.stringValue = "持续观察尚未结束，请先停止"; return }
        generation += 1
        started = nil; deadline = nil; endTimer?.invalidate()
        run(0, generation)
    }

    func run(_ position: Int, _ token: Int) {
        guard generation == token else { return }
        nextPosition = position
        if let deadline = deadline, Date() >= deadline { finishObservation(); return }
        let plan = started == nil ? basicChecks : checks
        let width = plan.count
        if position % width == 0 { roundStarted = Date() }
        guard started != nil || position < 9 else {
            status.stringValue = "3 轮共 9 个 GET 请求全部成功；会话复用验证通过"
            log("PASS：同一持久 WebView 会话完成 3 轮共 9 个 GET 请求")
            return
        }
        let (index, basePath, name) = plan[position % width]
        var path = basePath
        if started != nil && !config.samples.isEmpty {
            let sample = config.samples[(position / width) % config.samples.count]
            if name == "目录报价" { path += sample.tsin }
            if name == "前台报价" { path += sample.plid + "?platform=desktop" }
        }
        // A challenge page has the right origin too. The request is permitted
        // only after the actual top-level document is JSON, not merely loaded.
        guard ready[index], views[index].url?.host == URL(string: origins[index])!.host else {
            tabs.selectTabViewItem(at: index)
            status.stringValue = "\(name)：请先在下方网页完成人工验证，再点击连续测试"
            phase = "verification_required"; writeMetrics()
            log("暂停：\(name) 尚未返回正常 JSON；未发起后台重试")
            return
        }
        requestInFlight = true
        phase = "running"
        writeMetrics()
        let requestStarted = Date()
        status.stringValue = "第 \(position / width + 1) 轮：正在读取\(name)"
        let script = """
        if (location.origin !== origin) throw new Error('origin mismatch');
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 20000);
        try {
            const headers = {Accept: 'application/json, text/plain, */*'};
            if (authorization) headers.Authorization = authorization;
            const response = await fetch(path, {method: 'GET', headers, credentials: 'include',
                cache: 'no-store', redirect: 'error', signal: controller.signal});
            const body = await response.text();
            let data = null;
            try { data = JSON.parse(body); } catch (_) {}
            const valid = data && (path.includes('/offers/count') ? typeof data.count === 'number' :
                path === '/v2/seller' ? typeof data.seller_id === 'number' :
                path.includes('/offers/detailed') ? Array.isArray(data.offers) :
                path.includes('/catalogue/mpv/search') ? Array.isArray(data.results) : !!data.buybox);
            return {status: response.status, json: !!data, valid: !!valid,
                challenge: response.headers.get('cf-mitigated') === 'challenge' ||
                    (!data && /Just a moment|cf-chl|challenge-platform/i.test(body)),
                retryAfter: Number(response.headers.get('retry-after')) || 0};
        } finally { clearTimeout(timer); }
        """
        views[index].callAsyncJavaScript(script, arguments: ["origin": origins[index], "path": path,
            "authorization": index == 0 ? config.authorization : ""], in: nil, in: .defaultClient, completionHandler: { [weak self] result in
            guard let self = self else { return }
            self.requestInFlight = false
            guard self.generation == token else { self.writeMetrics(); return }
            switch result {
            case .failure(let error):
                // Do not log localized JS exceptions: a transport could embed
                // request details. Keys and response bodies never enter logs.
                let latency = Date().timeIntervalSince(requestStarted) * 1000
                self.record(success: false, code: 0, challenge: false, latency: latency)
                self.status.stringValue = "\(name)：浏览器请求失败，已记录"
                if self.started != nil { self.schedule(position + 1, token, extraDelay: 30) }
                self.log("FAIL：\(name) 浏览器执行错误 code=\((error as NSError).code)")
            case .success(let value):
                guard let data = value as? [String: Any] else {
                    self.record(success: false, code: 0, challenge: false, latency: Date().timeIntervalSince(requestStarted) * 1000)
                    self.phase = "invalid_result"; self.writeMetrics(); self.log("FAIL：浏览器结果格式异常"); return
                }
                let code = data["status"] as? Int ?? 0
                let challenge = data["challenge"] as? Bool ?? false
                let valid = data["valid"] as? Bool ?? false
                let latency = Date().timeIntervalSince(requestStarted) * 1000
                self.record(success: code == 200 && valid, code: code, challenge: challenge, latency: latency)
                self.log("第 \(position / width + 1) 轮 \(name)：HTTP \(code)，JSON=\(data["json"] ?? false)，数据有效=\(valid)，验证页面=\(challenge)，耗时=\(Int(latency))ms")
                guard code == 200 && valid else {
                    self.status.stringValue = "\(name)：HTTP \(code)，测试暂停。\(challenge ? "请手动完成验证" : "请检查接口返回")"
                    if challenge {
                        self.phase = "verification_required"; self.writeMetrics()
                        self.tabs.selectTabViewItem(at: index)
                        self.load(index, path)
                    } else if self.started != nil && (code == 429 || code == 420 || code >= 500) {
                        self.schedule(position + 1, token, extraDelay: max(60, data["retryAfter"] as? Double ?? 0))
                    } else {
                        self.phase = code == 401 ? "auth_required" : "http_failure"; self.writeMetrics()
                    }
                    return
                }
                self.schedule(position + 1, token)
            }
        })
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        guard let index = views.firstIndex(of: webView) else { return }
        let expected = origins[index]
        webView.callAsyncJavaScript("""
            if (location.origin !== origin) return false;
            try {
                const data = JSON.parse(document.body.innerText);
                return origin.includes('seller-api') ?
                    (typeof data.count === 'number' || typeof data.seller_id === 'number' || Array.isArray(data.offers) || Array.isArray(data.results)) : !!data.buybox;
            } catch (_) { return false; }
            """, arguments: ["origin": expected], in: nil, in: .defaultClient, completionHandler: { [weak self] result in
            guard let self = self else { return }
            let usable = (try? result.get()) as? Bool ?? false
            self.ready[index] = usable
            self.log("\(index == 0 ? "卖家" : "前台")页面载入：\(usable ? "正常 JSON，可开始连续测试" : "等待人工验证或接口未返回正常 JSON")")
            self.status.stringValue = self.ready.allSatisfy { $0 } ? "两个接口页面已就绪，可以点击连续测试" : "请在未就绪的标签页中手动完成验证"
        })
    }

    func webView(_ webView: WKWebView, decidePolicyFor navigationResponse: WKNavigationResponse,
                 decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void) {
        if navigationResponse.isForMainFrame, let response = navigationResponse.response as? HTTPURLResponse {
            let blocked = response.value(forHTTPHeaderField: "cf-mitigated") == "challenge"
            log("页面导航：\(response.url?.host ?? "未知主机") HTTP \(response.statusCode)，Cloudflare challenge=\(blocked)")
        }
        decisionHandler(.allow)
    }

    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
                 decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url, url.scheme == "https", let host = url.host,
              let index = views.firstIndex(of: webView) else { decisionHandler(.cancel); return }
        let ownHost = URL(string: origins[index])!.host!
        let mainFrame = navigationAction.targetFrame?.isMainFrame ?? true
        let allowed = host == ownHost || (!mainFrame && host == "challenges.cloudflare.com")
        decisionHandler(allowed ? .allow : .cancel)
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        log("页面加载失败 code=\((error as NSError).code)")
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}

@main
struct Launcher {
    @MainActor
    static func main() {
        let raw = FileHandle.standardInput.readDataToEndOfFile()
        guard let configuration = try? JSONDecoder().decode(Configuration.self, from: raw) else {
            fputs("请通过 launch.py 启动，并从本地店铺配置读取凭证。\n", stderr)
            exit(1)
        }
        let app = NSApplication.shared
        app.setActivationPolicy(.regular)
        let delegate = Probe(configuration)
        app.delegate = delegate
        app.run()
    }
}
