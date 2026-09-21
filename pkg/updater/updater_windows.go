//go:build windows

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
		return fmt.Errorf("获取当前程序路径失败: %w", err)
	}
	realExe, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		realExe = exePath
	}

	// 1. 查找解压出的新 Takealot.exe
	var newExePath string
	_ = filepath.Walk(unpackDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".exe") {
			newExePath = path
			return filepath.SkipDir
		}
		return nil
	})

	if newExePath == "" {
		return fmt.Errorf("解压产物中未找到可执行文件 (.exe)")
	}

	pid := os.Getpid()
	log.Printf("[Updater] 准备执行 Windows 自动更新与重启: PID=%d, 目标=%s, 新版本=%s", pid, realExe, newExePath)

	// 2. 尝试使用 Windows 特有的在位重命名 (Windows 允许重命名正在运行的 .exe，但不允许直接覆盖)
	oldBackupPath := realExe + ".old"
	_ = os.Remove(oldBackupPath) // 清理上次旧的 .old 文件

	renamedSuccessfully := false
	if err := os.Rename(realExe, oldBackupPath); err == nil {
		// 重命名成功后，直接把新 exe 移动或复制到目标路径
		if err := copyOrMoveFile(newExePath, realExe); err == nil {
			renamedSuccessfully = true
		} else {
			// 如果移动失败，恢复原状
			_ = os.Rename(oldBackupPath, realExe)
		}
	}

	// 3. 执行退出前清理钩子 (停止引擎与关闭数据库)
	if m.onBeforeRestart != nil {
		m.onBeforeRestart()
	}

	// 4. 生成后台更新批处理脚本
	batchPath := filepath.Join(os.TempDir(), fmt.Sprintf("takealot_update_%d.bat", pid))
	var batchContent string

	if renamedSuccessfully {
		// 在位重命名已成功：脚本只需等待老进程完全结束，然后直接启动新程序并清理
		batchContent = fmt.Sprintf(`@echo off
chcp 65001 >nul
set PID=%d
set TARGET_EXE=%s
set UNPACK_DIR=%s

:wait_loop
tasklist /fi "PID eq %%PID%%" 2>nul | findstr /i "%%PID%%" >nul
if not errorlevel 1 (
    timeout /t 1 /nobreak >nul
    goto wait_loop
)

timeout /t 1 /nobreak >nul
start "" "%%TARGET_EXE%%"
rd /s /q "%%UNPACK_DIR%%" 2>nul
del "%%~f0" 2>nul
`, pid, realExe, unpackDir)
	} else {
		// 在位重命名未生效时的传统替换流程：等待老进程释放文件锁后删除并覆盖
		batchContent = fmt.Sprintf(`@echo off
chcp 65001 >nul
set PID=%d
set TARGET_EXE=%s
set NEW_EXE=%s
set UNPACK_DIR=%s

:wait_loop
tasklist /fi "PID eq %%PID%%" 2>nul | findstr /i "%%PID%%" >nul
if not errorlevel 1 (
    timeout /t 1 /nobreak >nul
    goto wait_loop
)

timeout /t 1 /nobreak >nul

del /f /q "%%TARGET_EXE%%" 2>nul
copy /y "%%NEW_EXE%%" "%%TARGET_EXE%%" >nul 2>&1
if errorlevel 1 (
    move /y "%%NEW_EXE%%" "%%TARGET_EXE%%" >nul 2>&1
)

start "" "%%TARGET_EXE%%"
rd /s /q "%%UNPACK_DIR%%" 2>nul
del "%%~f0" 2>nul
`, pid, realExe, newExePath, unpackDir)
	}

	if err := os.WriteFile(batchPath, []byte(batchContent), 0755); err != nil {
		return fmt.Errorf("写入 Windows 守护脚本失败: %w", err)
	}

	// CREATE_NEW_PROCESS_GROUP (0x00000200) | CREATE_NO_WINDOW (0x08000000)
	cmd := exec.Command("cmd.exe", "/c", batchPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200 | 0x08000000,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Windows 更新批处理失败: %w", err)
	}

	log.Println("[Updater] Windows 更新脚本已就绪，当前程序即将平滑退出并重启新版...")
	os.Exit(0)
	return nil
}

func copyOrMoveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Fallback to copy
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}
