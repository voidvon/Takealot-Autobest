package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLogExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "运行日志.txt")
	content := "[2026-10-22T16:59:45+08:00] [WARN] [店铺: Huihengxin / store-1] 前台报价访问被拒绝\r\n"
	saved, err := saveLogExport(content, func() (string, error) { return path, nil })
	if err != nil || saved != path {
		t.Fatalf("save export: path=%q error=%v", saved, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "\xef\xbb\xbf"+content {
		t.Fatalf("UTF-8 log content changed: %q, %v", raw, err)
	}
	if saved, err := saveLogExport(content, func() (string, error) { return "", nil }); err != nil || saved != "" {
		t.Fatalf("cancelling the dialog should not report a saved file: %q, %v", saved, err)
	}
	for _, tc := range []struct {
		name   string
		choose func() (string, error)
		want   string
	}{
		{"dialog failed", func() (string, error) { return "", errors.New("dialog failed") }, "选择日志保存位置失败"},
		{"write failed", func() (string, error) { return t.TempDir(), nil }, "保存日志失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if saved, err := saveLogExport(content, tc.choose); err == nil || saved != "" || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("export error was lost: %q, %v", saved, err)
			}
		})
	}
}
