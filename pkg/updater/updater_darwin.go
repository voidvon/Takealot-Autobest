//go:build darwin

package updater

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func (m *Manager) applyAndRestart(unpackDir string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取当前可执行文件路径失败: %w", err)
	}
	realExe, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		realExe = exePath
	}

	// 1. 查找当前运行的 Takealot.app 路径
	var targetAppPath string
	cur := realExe
	for i := 0; i < 6; i++ {
		if strings.HasSuffix(cur, ".app") {
			targetAppPath = cur
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	// 如果当前未在 .app bundle 内部运行，检查当前工作目录下是否存在 Takealot.app
	if targetAppPath == "" {
		if fi, err := os.Stat("Takealot.app"); err == nil && fi.IsDir() {
			abs, _ := filepath.Abs("Takealot.app")
			targetAppPath = abs
		}
	}

	// 2. 查找解压出的新版本 .app 目录
	var newAppPath string
	_ = filepath.Walk(unpackDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() && strings.HasSuffix(path, ".app") {
			newAppPath = path
			return filepath.SkipDir
		}
		return nil
	})

	if newAppPath == "" {
		return fmt.Errorf("解压产物中未找到 Takealot.app 目录")
	}

	if targetAppPath == "" {
		return fmt.Errorf("未检测到当前安装的 Takealot.app 路径，请直接运行打包后的桌面应用 (当前执行文件: %s)", realExe)
	}

	pid := os.Getpid()
	log.Printf("[Updater] 准备执行 macOS 自动更新与重启: PID=%d, 目标=%s, 新版本=%s", pid, targetAppPath, newAppPath)

	// 3. 预先清除新 App 的 Gatekeeper 隔离标记
	_ = exec.Command("xattr", "-dr", "com.apple.quarantine", newAppPath).Run()

	// 4. 调用退出前清理钩子 (停止调度引擎、安全关闭 SQLite 数据库)
	if m.onBeforeRestart != nil {
		m.onBeforeRestart()
	}

	// 5. 启动后台脱离脚本，等待当前进程结束后执行文件覆盖并重新拉起
	shScript := fmt.Sprintf(`
pid=%d
target_app=%q
new_app=%q
unpack_dir=%q

# 循环等待当前进程完全退出
while kill -0 "$pid" 2>/dev/null; do
    sleep 0.2
done

# 替换旧应用目录
rm -rf "$target_app"
cp -R "$new_app" "$target_app"

# 清除隔离标记避免首次启动安全阻拦
xattr -dr com.apple.quarantine "$target_app" 2>/dev/null || true

# 重新启动新应用
open -n "$target_app"

# 清理解压临时目录
rm -rf "$unpack_dir"
`, pid, targetAppPath, newAppPath, unpackDir)

	cmd := exec.Command("sh", "-c", shScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 macOS 更新守护脚本失败: %w", err)
	}

	log.Println("[Updater] macOS 更新脚本已就绪，当前程序即将平滑退出并重启新版...")
	os.Exit(0)
	return nil
}
