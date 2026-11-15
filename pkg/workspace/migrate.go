package workspace

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "modernc.org/sqlite"

	"takealot/pkg/localstate"
)

// MigrateLegacy runs only for an existing database next to this executable, never
// a neighbouring project's CWD. Originals remain intact. VACUUM INTO includes WAL
// transactions in a consistent SQLite snapshot; copying only .db can lose data.
func (l Layout) MigrateLegacy() (bool, error) {
	if l.LegacyRoot == "" {
		return false, nil
	}
	if _, err := os.Stat(l.Database()); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	source := filepath.Join(l.LegacyRoot, "takealot.db")
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	stage, err := os.MkdirTemp(l.Root, ".migration-*")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(stage)
	sourcePath := filepath.ToSlash(source)
	if runtime.GOOS == "windows" && !strings.HasPrefix(sourcePath, "/") {
		sourcePath = "/" + sourcePath
	}
	u := &url.URL{Scheme: "file", Path: sourcePath, RawQuery: "mode=ro"}
	database, err := sql.Open("sqlite", u.String())
	if err != nil {
		return false, err
	}
	defer database.Close()
	snapshot := filepath.Join(stage, "takealot.db")
	if _, err := database.Exec("VACUUM INTO ?", snapshot); err != nil {
		return false, fmt.Errorf("旧数据库迁移失败，请先关闭旧版软件后重试: %w", err)
	}
	if err := os.Chmod(snapshot, 0600); err != nil {
		return false, err
	}
	// Keep legacy defaults, but never turn credentials/targets into new stores.
	for _, name := range []string{"config.json", "can.ini"} {
		data, err := os.ReadFile(filepath.Join(l.LegacyRoot, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if err := localstate.WriteFile(filepath.Join(l.Root, name), data, 0600); err != nil {
			return false, err
		}
	}
	if err := l.migrateBrowserProfiles(source, stage); err != nil {
		return false, err
	}
	// The database is the commit marker; a failed snapshot is never adopted.
	if err := os.Rename(snapshot, l.Database()); err != nil {
		return false, err
	}
	return true, nil
}

func (l Layout) migrateBrowserProfiles(databasePath, stage string) error {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(databasePath); err == nil {
		databasePath = real
	}
	digest := sha256.Sum256([]byte(databasePath))
	source := filepath.Join(configRoot, "Takealot", "browser-sessions", fmt.Sprintf("%x", digest[:12]))
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	// Never replace profiles created by a previous startup or manual import.
	entries, err := os.ReadDir(l.BrowserSessions())
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return nil
	}
	staged := filepath.Join(stage, "browser-sessions")
	if err := CopyTree(source, staged); err != nil {
		return fmt.Errorf("旧浏览器资料迁移失败，请关闭旧版软件: %w", err)
	}
	if err := os.Remove(l.BrowserSessions()); err != nil {
		return err
	}
	return os.Rename(staged, l.BrowserSessions())
}

// CopyTree imports legacy browser profiles while the old application is closed.
// Reject links instead of following them outside the explicitly selected root.
func CopyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("迁移目录包含符号链接: %s", entry.Name())
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("无法迁移非常规文件: %s", entry.Name())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
