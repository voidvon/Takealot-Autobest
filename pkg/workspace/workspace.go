// Package workspace defines the single filesystem boundary for an application instance.
package workspace

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"takealot/pkg/localstate"
)

type Layout struct {
	Root       string
	LegacyRoot string
	lock       *os.File
}

// Resolve never infers data ownership from the caller's current directory.
// A macOS bundle uses a sibling data directory, outside the replaceable bundle.
func Resolve(executable, override string) (Layout, error) {
	exe, err := filepath.Abs(executable)
	if err != nil {
		return Layout{}, err
	}
	if real, e := filepath.EvalSymlinks(exe); e == nil {
		exe = real
	}
	base := filepath.Dir(exe)
	if filepath.Base(base) == "MacOS" && filepath.Base(filepath.Dir(base)) == "Contents" && strings.HasSuffix(filepath.Dir(filepath.Dir(base)), ".app") {
		base = filepath.Dir(filepath.Dir(filepath.Dir(base)))
	}
	root := filepath.Join(base, "data")
	legacy := base
	if override != "" {
		root = override
		legacy = ""
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Layout{}, err
	}
	return Layout{Root: root, LegacyRoot: legacy}, nil
}

func (l Layout) Database() string        { return filepath.Join(l.Root, "takealot.db") }
func (l Layout) Accounts() string        { return filepath.Join(l.Root, "accounts") }
func (l Layout) BrowserSessions() string { return filepath.Join(l.Root, "browser-sessions") }
func (l Layout) Webview() string         { return filepath.Join(l.Root, "webview") }
func (l Layout) Updates() string         { return filepath.Join(l.Root, "updates") }
func (l Layout) Logs() string            { return filepath.Join(l.Root, "logs") }

func (l *Layout) Open() error {
	if err := os.MkdirAll(l.Root, 0700); err != nil {
		return fmt.Errorf("无法创建程序数据目录: %w", err)
	}
	root, err := filepath.EvalSymlinks(l.Root)
	if err != nil {
		return err
	}
	l.Root = root
	file, err := os.OpenFile(filepath.Join(l.Root, ".instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("无法打开程序目录锁: %w", err)
	}
	if err = lockFile(file); err != nil {
		file.Close()
		return fmt.Errorf("这个数据文件夹已被其他进程使用，请复制软件到另一个文件夹后多开: %w", err)
	}
	l.lock = file
	for _, dir := range []string{l.Accounts(), l.BrowserSessions(), l.Webview(), l.Updates(), l.Logs()} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			l.Close()
			return err
		}
	}
	return nil
}

func (l *Layout) Close() {
	if l.lock == nil {
		return
	}
	_ = os.Remove(filepath.Join(l.Root, "runtime.json"))
	_ = unlockFile(l.lock)
	_ = l.lock.Close()
	l.lock = nil
}

func (l Layout) RecordRuntime(address string) error {
	data, err := json.Marshal(struct {
		PID int    `json:"pid"`
		URL string `json:"url"`
	}{os.Getpid(), "http://" + address})
	if err != nil {
		return err
	}
	return localstate.WriteFile(filepath.Join(l.Root, "runtime.json"), data, 0600)
}

// Port zero reserves a free port atomically. Explicit ports fail before business
// startup instead of silently connecting a second app to the first app's server.
func Listen(port int) (net.Listener, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("端口必须在 0–65535 之间")
	}
	return net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
}
