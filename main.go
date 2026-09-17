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
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
	"takealot/pkg/engine"
	"takealot/pkg/server"
)

//go:embed all:web/dist
var distEmbedFS embed.FS

//go:embed web/static/index.html
var staticHTML []byte

var Version = "0.2.0"

func main() {
	port := flag.Int("port", 8000, "Web 控制台监听端口")
	noOpen := flag.Bool("no-open", false, "启动后不自动打开浏览器")
	flag.Parse()

	setupWorkingDir()

	log.Println("========================================================")
	log.Printf("🚀 正在启动 Takealot 自动化控制中心 (v%s - Go 原生跨平台版)", Version)
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

	// 4. Initialize Automation Engine
	eng := engine.NewEngine(cfgMgr, clientPool, database)

	// 5. Initialize HTTP Server
	distSubFS, _ := fs.Sub(distEmbedFS, "web/dist")
	srv := server.NewServer(cfgMgr, clientPool, eng, database, distSubFS, staticHTML, Version)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	url := fmt.Sprintf("http://%s", addr)
	log.Printf("🌐 本地控制面板地址: %s", url)
	log.Println("💡 提示: 按 Ctrl + C 可安全退出服务")
	log.Println("--------------------------------------------------------")

	// Open browser automatically in a separate goroutine
	if !*noOpen {
		go func() {
			time.Sleep(800 * time.Millisecond)
			openBrowser(url)
		}()
	}

	// Start server
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("❌ 服务启动失败: %v", err)
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
	// If config.json already exists in current working dir, use it directly
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
