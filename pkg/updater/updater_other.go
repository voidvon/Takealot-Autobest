//go:build !darwin && !windows

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

	// 查找解压出的二进制文件
	var newBinPath string
	_ = filepath.Walk(unpackDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && (info.Mode()&0111 != 0 || strings.Contains(strings.ToLower(path), "takealot")) {
			newBinPath = path
			return filepath.SkipDir
		}
		return nil
	})

	if newBinPath == "" {
		return fmt.Errorf("解压产物中未找到可执行二进制文件")
	}

	pid := os.Getpid()
	log.Printf("[Updater] 准备执行 Linux 自动更新与重启: PID=%d, 目标=%s, 新版本=%s", pid, realExe, newBinPath)

	if m.onBeforeRestart != nil {
		m.onBeforeRestart()
	}

	shScript := fmt.Sprintf(`
pid=%d
target_bin=%q
new_bin=%q
unpack_dir=%q

while kill -0 "$pid" 2>/dev/null; do
    sleep 0.2
done

cp -f "$new_bin" "$target_bin"
chmod +x "$target_bin"
"$target_bin" &
rm -rf "$unpack_dir"
`, pid, realExe, newBinPath, unpackDir)

	cmd := exec.Command("sh", "-c", shScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新守护脚本失败: %w", err)
	}

	log.Println("[Updater] Linux 更新脚本已就绪，当前程序即将平滑退出并重启新版...")
	os.Exit(0)
	return nil
}
