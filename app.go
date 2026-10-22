package main

import (
	"context"
	"fmt"
	"os"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct manages desktop window lifecycle
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown is called at application termination
func (a *App) shutdown(ctx context.Context) {
}

// ExportLogs opens a native save dialog because embedded webviews do not
// consistently support downloads from blob URLs.
func (a *App) ExportLogs(content string) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("桌面应用尚未就绪")
	}
	return saveLogExport(content, func() (string, error) {
		return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
			Title:                "导出运行日志",
			DefaultFilename:      "takealot-logs-" + time.Now().Format("20060102-150405") + ".txt",
			Filters:              []wailsruntime.FileFilter{{DisplayName: "文本日志 (*.txt)", Pattern: "*.txt"}},
			CanCreateDirectories: true,
		})
	})
}

func saveLogExport(content string, choosePath func() (string, error)) (string, error) {
	path, err := choosePath()
	if err != nil {
		return "", fmt.Errorf("选择日志保存位置失败: %w", err)
	}
	if path == "" {
		return "", nil // The user cancelled the dialog.
	}
	// UTF-8 BOM keeps Chinese logs readable in Windows text editors.
	if err := os.WriteFile(path, []byte("\xef\xbb\xbf"+content), 0600); err != nil {
		return "", fmt.Errorf("保存日志失败: %w", err)
	}
	return path, nil
}
