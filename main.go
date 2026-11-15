package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"context"
	"takealot/pkg/account"
	"takealot/pkg/api"
	"takealot/pkg/engine"
	"takealot/pkg/reporter"
	"takealot/pkg/server"
	"takealot/pkg/updater"
	"takealot/pkg/workspace"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:web/dist
var distEmbedFS embed.FS

//go:embed web/static/index.html
var staticHTML []byte

var (
	Version    = "0.2.0"
	BuildMode  = "development" // "development" 或 "production" (构建时通过 ldflags 注入)
	ProdCMSURL = "https://takealot.0122.vip"
	DevCMSURL  = "https://takealot.0122.vip"
)

func resolveDefaultCMSURL() string {
	// 1. 最高优先级：环境变量显式指定完整 URL
	if v := os.Getenv("TAKEALOT_GOCMS_URL"); v != "" {
		return v
	}
	// 2. 次高优先级：环境变量显式指定运行模式 (如 TAKEALOT_ENV=production 或 dev)
	mode := strings.ToLower(os.Getenv("TAKEALOT_ENV"))
	if mode == "" {
		mode = strings.ToLower(BuildMode)
	}
	if mode == "production" || mode == "prod" {
		return ProdCMSURL
	}
	return DevCMSURL
}

func main() {
	if err := run(); err != nil {
		log.Print(err)
		if option := flag.Lookup("server-only"); option == nil || option.Value.String() != "true" {
			showStartupError(err.Error())
		}
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 0, "Web 控制台监听端口；0 自动选择空闲端口")
	dataDir := flag.String("data-dir", "", "数据目录；默认使用软件旁的 data 文件夹")
	serverOnly := flag.Bool("server-only", false, "以纯命令行/服务端模式运行 (不打开桌面 GUI)")
	noOpen := flag.Bool("no-open", false, "服务端模式下启动后不自动打开浏览器")
	cmsURL := flag.String("gocms-url", resolveDefaultCMSURL(), "GoCMS 会员服务地址")
	memberGroup := flag.String("member-group", envDefault("TAKEALOT_MEMBER_GROUP", "takealot-vip"), "所需会员组标识")
	flag.Parse()

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	layout, err := workspace.Resolve(exe, *dataDir)
	if err != nil {
		return err
	}
	if err := layout.Open(); err != nil {
		return err
	}
	defer layout.Close()
	logFile, err := os.OpenFile(filepath.Join(layout.Logs(), "application.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("无法写入软件日志: %w", err)
	}
	oldOutput := log.Writer()
	log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	defer func() { log.SetOutput(oldOutput); logFile.Close() }()
	migrated, err := layout.MigrateLegacy()
	if err != nil {
		return err
	}
	listener, err := workspace.Listen(*port)
	if err != nil {
		return fmt.Errorf("无法启动本地服务: %w", err)
	}
	defer listener.Close()
	if err := layout.RecordRuntime(listener.Addr().String()); err != nil {
		return err
	}
	updater.Cleanup()

	log.Println("========================================================")
	log.Printf("🚀 正在启动 Takealot 掌柜 (v%s - Wails 跨平台桌面版)", Version)
	log.Printf("🌐 运行环境: %s | 会员服务: %s", BuildMode, *cmsURL)
	log.Println("========================================================")

	// 1. Initialize SQLite. Store records are created only by an explicit add.
	database, cfgMgr, err := openApplicationData(layout.Root)
	if err != nil {
		return fmt.Errorf("SQLite 初始化失败: %w", err)
	}
	defer database.Close()

	// 3. Load only stores already saved in this data directory. Never create a
	// placeholder or adopt credentials/targets from neighbouring legacy files.
	clientPool := api.NewClientPool()
	if database != nil {
		// 加载全部店铺到 ClientPool
		if stores, err := database.GetStores(); err == nil {
			for _, st := range stores {
				clientPool.GetOrCreate(st.ID, st.Authorization, st.ProxyURL)
			}
			log.Printf("🏪 已成功就绪 %d 个店铺实例", len(stores))
		}
	}

	// Online member authorization. No offline activation or machine fingerprint.
	credentialStore := account.NewFileStore(layout.Accounts(), *cmsURL, *memberGroup)
	if migrated {
		if err := account.ImportLegacySession(*cmsURL, *memberGroup, credentialStore); err != nil {
			log.Print("旧会员会话未迁入，请在新版中重新登录（原凭据保持不变）")
		}
	}
	accountMgr, err := account.NewWithStore(*cmsURL, *memberGroup, credentialStore)
	if err != nil {
		return err
	}
	accountCtx, cancelAccount := context.WithCancel(context.Background())
	defer cancelAccount()

	// 5. Initialize Automation Engine
	eng := engine.NewEngine(cfgMgr, clientPool, database, accountMgr)
	defer eng.StopAll()
	eng.SetReporter(reporter.New(accountMgr, func(err error) {
		eng.Log(err.Error(), "WARN")
	}))

	go accountMgr.Run(accountCtx, func() { eng.StopAll() })

	// 6. Initialize Automatic Updater Manager
	updaterMgr := updater.NewManager("voidvon", "Takealot-Autobest", Version, func() {
		log.Println("🛑 收到更新重启指令，正在保存数据并停止后台服务...")
		eng.StopAll()
		if database != nil {
			_ = database.Close()
		}
	})

	updaterMgr.SetWorkDir(layout.Updates())
	updaterMgr.SetRestartArgs(append(os.Args[1:], "--data-dir", layout.Root))

	// 7. Initialize HTTP Server & Handler
	distSubFS, err := fs.Sub(distEmbedFS, "web/dist")
	if err != nil {
		log.Printf("⚠️ 提取前端静态文件失败: %v", err)
	}
	srv := server.NewServer(cfgMgr, clientPool, eng, database, accountMgr, updaterMgr, distSubFS, staticHTML, Version)

	preferences, err := workspace.NewPreferencesStore(layout.Root)
	if err != nil {
		return fmt.Errorf("读取界面配置失败: %w", err)
	}
	srv.SetPreferences(preferences)
	app := NewApp()
	if err := app.configureBrowserSessions(layout.BrowserSessions(), database, eng, accountMgr); err != nil {
		return fmt.Errorf("初始化访问会话失败: %w", err)
	}
	clientPool.SetRequestTransport(app.browserSessions.Do)

	url := "http://" + listener.Addr().String()
	httpServer := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	defer httpServer.Close()
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("本地 HTTP 服务监听异常: %v", err)
		}
	}()
	log.Printf("数据文件夹: %s", layout.Root)

	log.Printf("🌐 本地后台服务地址: %s", url)

	// 如果指定了仅以纯命令行/服务端模式运行
	if *serverOnly {
		log.Println("💻 当前以纯命令行模式运行 (未启动桌面 GUI 窗口)")
		log.Println("💡 提示: 按 Ctrl + C 可安全退出服务")
		log.Println("--------------------------------------------------------")

		if !*noOpen {
			go func() {
				time.Sleep(800 * time.Millisecond)
				openBrowser(url)
			}()
		}

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		signal.Stop(quit)
		log.Println("🛑 服务已退出")
		return nil
	}

	// 7. Desktop GUI 模式 (Wails 原生桌面窗口)

	err = wails.Run(&options.App{
		Title:           "Takealot 掌柜",
		Width:           1360,
		Height:          860,
		MinWidth:        1024,
		MinHeight:       700,
		Frameless:       runtime.GOOS == "windows",
		CSSDragProperty: "--wails-draggable",
		CSSDragValue:    "drag",
		AssetServer: &assetserver.Options{
			Assets:  distSubFS,
			Handler: srv.Handler(),
		},
		BackgroundColour: &options.RGBA{R: 248, G: 250, B: 252, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			NonPersistentWebview: true,
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		Windows: &windows.Options{
			WebviewUserDataPath:               layout.Webview(),
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
		},
		Linux: &linux.Options{NonPersistentWebview: true, WebviewGpuPolicy: linux.WebviewGpuPolicyNever},
	})

	if err != nil {
		return fmt.Errorf("桌面应用启动失败: %w", err)
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = cmd.Start()
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
