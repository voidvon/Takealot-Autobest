//go:build darwin

package updater

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	shScript := `
set -eu
pid=$1
target_app=$2
new_app=$3
unpack_dir=$4
shift 4
while kill -0 "$pid" 2>/dev/null; do sleep 0.2; done
staged_app="${target_app}.new"
backup_app="${target_app}.old"
cp -R "$new_app" "$staged_app"
mv "$target_app" "$backup_app"
if ! mv "$staged_app" "$target_app"; then mv "$backup_app" "$target_app"; exit 1; fi
xattr -dr com.apple.quarantine "$target_app" 2>/dev/null || true
open -n "$target_app" --args "$@"
rm -rf "$backup_app" "$unpack_dir"
`
	args := append([]string{"-c", shScript, "takealot-updater", strconv.Itoa(pid), targetAppPath, newAppPath, unpackDir}, m.restartArgs...)
	cmd := exec.Command("sh", args...)

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
