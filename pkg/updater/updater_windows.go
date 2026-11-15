//go:build windows

package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// A JSON plan avoids interpreting installation paths or CLI arguments as batch
// commands. Both plan and script live in this workspace's update staging folder.
func (m *Manager) applyAndRestart(unpackDir string) error {
	target, err := os.Executable()
	if err != nil {
		return err
	}
	if real, e := filepath.EvalSymlinks(target); e == nil {
		target = real
	}
	var source string
	err = filepath.WalkDir(unpackDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), "Takealot.exe") {
			source = path
		}
		return nil
	})
	if err != nil {
		return err
	}
	if source == "" {
		return fmt.Errorf("更新包缺少 Takealot.exe")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	arguments := make([]string, len(m.restartArgs))
	for i, value := range m.restartArgs {
		arguments[i] = syscall.EscapeArg(value)
	}
	plan, err := json.Marshal(map[string]any{"pid": os.Getpid(), "target": target, "source": source,
		"unpack": unpackDir, "cwd": cwd, "arguments": strings.Join(arguments, " ")})
	if err != nil {
		return err
	}
	planPath := filepath.Join(unpackDir, "restart.json")
	scriptPath := filepath.Join(unpackDir, "restart.ps1")
	if err := os.WriteFile(planPath, plan, 0600); err != nil {
		return err
	}
	script := `param([string]$PlanPath)
$ErrorActionPreference = 'Stop'
$plan = Get-Content -LiteralPath $PlanPath -Raw -Encoding UTF8 | ConvertFrom-Json
try {
    Wait-Process -Id $plan.pid -ErrorAction SilentlyContinue
    $staged = $plan.target + '.new'
    $backup = $plan.target + '.old'
    Copy-Item -LiteralPath $plan.source -Destination $staged -Force
    if (Test-Path -LiteralPath $backup) { Remove-Item -LiteralPath $backup -Force }
    Move-Item -LiteralPath $plan.target -Destination $backup
    try { Move-Item -LiteralPath $staged -Destination $plan.target }
    catch { Move-Item -LiteralPath $backup -Destination $plan.target; throw }
    $start = @{ FilePath = $plan.target; WorkingDirectory = $plan.cwd }
    if ($plan.arguments.Length -gt 0) { $start.ArgumentList = $plan.arguments }
    Start-Process @start
    Remove-Item -LiteralPath $plan.unpack -Recurse -Force
} catch {
    $_.Exception.Message | Out-File -LiteralPath (Join-Path (Split-Path $plan.unpack) 'last-error.log') -Encoding UTF8
}
`
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath, planPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x08000000}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新进程失败: %w", err)
	}
	if m.onBeforeRestart != nil {
		m.onBeforeRestart()
	}
	os.Exit(0)
	return nil
}
