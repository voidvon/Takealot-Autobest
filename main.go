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

	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
	"takealot/pkg/engine"
	"takealot/pkg/license"
	"takealot/pkg/server"

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

var Version = "0.2.0"

func main() {
	port := flag.Int("port", 8000, "Web 控制台监听端口")
	serverOnly := flag.Bool("server-only", false, "以纯命令行/服务端模式运行 (不打开桌面 GUI)")
	noOpen := flag.Bool("no-open", false, "服务端模式下启动后不自动打开浏览器")
	flag.Parse()

	setupWorkingDir()

	log.Println("========================================================")
	log.Printf("🚀 正在启动 Takealot 自动化控制中心 (v%s - Wails 跨平台桌面版)", Version)
	log.Println("========================================================")

	// 1. Initialize SQLite Database & Migrate Legacy Data
	database, err := db.New("takealot.db")
	if err != nil {
		log.Printf("⚠️ SQLite 初始化提示: %v", err)
	} else {
		defer database.Close()

		// 自动检测并迁移旧版 GJDATA 到 SQLite
		if count, err := database.MigrateGJData("GJDATA"); err != nil {
			log.Printf("⚠️ 迁移旧版 GJDATA 失败: %v", err)
		} else if count > 0 {
			log.Printf("📦 成功将旧版 GJDATA 中的 %d 条商品监控数据迁移至 SQLite (takealot.db)，原文件已备份为 GJDATA.migrated.bak", count)
		}
	}

	// 2. Initialize Configuration Manager
	cfgMgr := config.NewManager(".")
	cfg := cfgMgr.Get()

	// 3. 确保店铺初始化与 ClientPool 就绪
	clientPool := api.NewClientPool()
	if database != nil {
		defStore, err := database.EnsureDefaultStore(
			cfg.Authorization,
			cfg.PriceDecreaseStep,
			cfg.PriceIncreaseStep,
			cfg.IntervalMinutes,
			cfg.BulkStock,
			cfg.MaxFetchOffers,
			cfg.RRPPercentage,
		)
		if err == nil && defStore != nil {
			clientPool.GetOrCreate(defStore.ID, defStore.Authorization, defStore.ProxyURL)
		}

		// 加载全部店铺到 ClientPool
		if stores, err := database.GetStores(); err == nil {
			for _, st := range stores {
				clientPool.GetOrCreate(st.ID, st.Authorization, st.ProxyURL)
			}
			log.Printf("🏪 已成功就绪 %d 个店铺实例", len(stores))
		}

		// 监控商品双向同步保证 SQLite 持久化
		dbTargets, err := database.LoadStoreTargets("default")
		if err == nil && len(dbTargets) > 0 {
			_ = cfgMgr.UpdateTargets(dbTargets)
		} else if len(cfg.Targets) > 0 {
			if err := database.SaveStoreTargets("default", cfg.Targets); err == nil {
				log.Printf("📦 已将现有配置中的 %d 条商品监控数据导入 SQLite", len(cfg.Targets))
			}
		}
	}

	// 4. Initialize License Manager
	licMgr := license.NewManager(database)
	licStatus := licMgr.GetStatus()
	if licStatus.Activated {
		log.Printf("🔑 软件授权状态: 已激活 [客户: %s | 有效期: %s]", licStatus.Customer, licStatus.ExpiresAtFormatted)
	} else {
		log.Printf("⚠️ 软件授权状态: 未激活 (本机机器识别码: %s)", licStatus.MachineID)
		log.Printf("💡 请在控制台输入激活码，或联系管理员获取离线授权")
	}

	// 5. Initialize Automation Engine
	eng := engine.NewEngine(cfgMgr, clientPool, database, licMgr)

	// 6. Initialize HTTP Server & Handler
	distSubFS, err := fs.Sub(distEmbedFS, "web/dist")
	if err != nil {
		log.Printf("⚠️ 提取前端静态文件失败: %v", err)
	}
	srv := server.NewServer(cfgMgr, clientPool, eng, database, licMgr, distSubFS, staticHTML, Version)

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
	app := NewApp()

	err = wails.Run(&options.App{
		Title:             "Takealot 自动化控制中心",
		Width:             1360,
		Height:            860,
		MinWidth:          1024,
		MinHeight:         700,
		Frameless:         runtime.GOOS == "windows",
		CSSDragProperty:   "--wails-draggable",
		CSSDragValue:      "drag",
		AssetServer: &assetserver.Options{
			Assets:  distSubFS,
			Handler: srv.Handler(),
		},
		BackgroundColour: &options.RGBA{R: 248, G: 250, B: 252, A: 1},
		OnStartup:        app.startup,
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
			WebviewIsTransparent:             false,
			WindowIsTranslucent:              false,
			DisableWindowIcon:                false,
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
