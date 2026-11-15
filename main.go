package main

import (
	"embed"
	"flag"
	"fmt"
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

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:web/dist
var distEmbedFS embed.FS

//go:embed web/static/index.html
var staticHTML []byte

var (
	Version    = "0.1.20"
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
	port := flag.Int("port", 8000, "Web 控制台监听端口")
	serverOnly := flag.Bool("server-only", false, "以纯命令行/服务端模式运行 (不打开桌面 GUI)")
	noOpen := flag.Bool("no-open", false, "服务端模式下启动后不自动打开浏览器")
	cmsURL := flag.String("gocms-url", resolveDefaultCMSURL(), "GoCMS 会员服务地址")
	memberGroup := flag.String("member-group", envDefault("TAKEALOT_MEMBER_GROUP", "takealot-vip"), "所需会员组标识")
	flag.Parse()

	setupWorkingDir()
	updater.Cleanup()

	log.Println("========================================================")
	log.Printf("🚀 正在启动 Takealot 掌柜 (v%s - Wails 跨平台桌面版)", Version)
	log.Printf("🌐 运行环境: %s | 会员服务: %s", BuildMode, *cmsURL)
	log.Println("========================================================")

	// 1. Initialize SQLite. Store records are created only by an explicit add.
	database, cfgMgr, err := openApplicationData(".")
	if err != nil {
		log.Fatalf("SQLite 初始化失败: %v", err)
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
	accountMgr, err := account.New(*cmsURL, *memberGroup)
	if err != nil {
		log.Fatal(err)
	}
	accountCtx, cancelAccount := context.WithCancel(context.Background())
	defer cancelAccount()

	// 5. Initialize Automation Engine
	eng := engine.NewEngine(cfgMgr, clientPool, database, accountMgr)
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

	// 7. Initialize HTTP Server & Handler
	distSubFS, err := fs.Sub(distEmbedFS, "web/dist")
	if err != nil {
		log.Printf("⚠️ 提取前端静态文件失败: %v", err)
	}
	srv := server.NewServer(cfgMgr, clientPool, eng, database, accountMgr, updaterMgr, distSubFS, staticHTML, Version)

	app := NewApp()
	if err := app.configureBrowserSessions(database, eng, accountMgr); err != nil {
		log.Fatalf("初始化访问会话失败: %v", err)
	}
	clientPool.SetRequestTransport(app.browserSessions.Do)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	url := fmt.Sprintf("http://%s", addr)

	// 后台启动 HTTP 服务，方便外部浏览器或自动化脚本访问
	go func() {
		if err := http.ListenAndServe(addr, srv.Handler()); err != nil && err != http.ErrServerClosed {
			log.Printf("⚠️ 后台 HTTP 服务监听异常: %v", err)
		}
	}()

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
		log.Println("🛑 服务已退出")
		return
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
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
		},
	})

	if err != nil {
		log.Fatalf("❌ 桌面应用启动失败: %v", err)
	}
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

func setupWorkingDir() {
	// If database or config already exists in current working dir, use it directly
	if _, err := os.Stat("takealot.db"); err == nil {
		return
	}
	if _, err := os.Stat("config.json"); err == nil {
		return
	}
	// Otherwise, resolve directory of executable
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	realPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		realPath = exePath
	}
	dir := filepath.Dir(realPath)
	// If inside macOS .app bundle (Takealot.app/Contents/MacOS/takealot)
	if strings.Contains(dir, ".app/Contents/MacOS") {
		dir = filepath.Dir(filepath.Dir(filepath.Dir(dir)))
	}
	_ = os.Chdir(dir)
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
