package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"takealot/pkg/account"
	"takealot/pkg/browsersession"
	"takealot/pkg/db"
	"takealot/pkg/engine"
	"time"
)

func browserProfileRoot() (string, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	database, err := filepath.Abs("takealot.db")
	if err != nil {
		return "", err
	}
	if canonical, e := filepath.EvalSymlinks(database); e == nil {
		database = canonical
	}
	digest := sha256.Sum256([]byte(database))
	return filepath.Join(configRoot, "Takealot", "browser-sessions", fmt.Sprintf("%x", digest[:12])), nil
}
func (a *App) configureBrowserSessions(database *db.DB, eng *engine.Engine, accountMgr *account.Manager) error {
	root, err := browserProfileRoot()
	if err != nil {
		return err
	}
	manager, err := browsersession.New(root, browsersession.NewNativeDriver(), eng.SetVerificationRequired)
	if err != nil {
		return err
	}
	a.browserSessions = manager
	a.database = database
	a.engine = eng
	a.account = accountMgr
	return nil
}
func (a *App) domReady(ctx context.Context) {
	if a.browserSessions != nil {
		if err := a.browserSessions.Start(); err != nil {
			log.Printf("内嵌访问会话不可用: %v", err)
		}
	}
}
func (a *App) browserStore(storeID string) (*db.Store, error) {
	if a.ctx == nil || a.browserSessions == nil || a.database == nil {
		return nil, fmt.Errorf("访问会话需要桌面版")
	}
	if a.account != nil && !a.account.IsValid() {
		return nil, fmt.Errorf("请登录并开通有效 VIP")
	}
	store, err := a.database.GetStore(storeID)
	if err != nil || store == nil {
		return nil, fmt.Errorf("店铺不存在")
	}
	return store, nil
}

// These bindings exist only on the local Wails application view. The embedded
// Takealot views are created without bindings or the application's URL handler.
func (a *App) BrowserSessionStatuses() []browsersession.Status {
	statuses := []browsersession.Status{}
	if a.browserSessions == nil || a.database == nil {
		return statuses
	}
	if a.account != nil && !a.account.IsValid() {
		return statuses
	}
	stores, err := a.database.GetStores()
	if err != nil {
		return statuses
	}
	for _, store := range stores {
		statuses = append(statuses, a.browserSessions.Status(store.ID))
	}
	return statuses
}
func (a *App) ConfigureBrowserSession(storeID string, enabled bool) error {
	store, err := a.browserStore(storeID)
	if err != nil {
		return err
	}
	status := a.engine.GetStatus(storeID)
	if status.IsRunning || status.FollowRunning {
		return fmt.Errorf("请先停止该店铺任务，再切换请求方式")
	}
	return a.browserSessions.Configure(storeID, store.ProxyURL, enabled)
}
func (a *App) ShowBrowserSession(storeID, origin string, bounds browsersession.Bounds) error {
	store, err := a.browserStore(storeID)
	if err != nil {
		return err
	}
	if !a.browserSessions.Status(storeID).Enabled {
		return fmt.Errorf("请先启用持久会话")
	}
	return a.browserSessions.Show(storeID, store.ProxyURL, origin, bounds)
}
func (a *App) HideBrowserSession() error {
	if a.browserSessions == nil {
		return nil
	}
	return a.browserSessions.Hide()
}
func (a *App) ShowBrowserSessionPanels(storeID string, sellerBounds, publicBounds browsersession.Bounds) error {
	store, err := a.browserStore(storeID)
	if err != nil {
		return err
	}
	if !a.browserSessions.Status(storeID).Enabled {
		return fmt.Errorf("请先启用持久会话")
	}
	return a.browserSessions.ShowPanels(storeID, store.ProxyURL, sellerBounds, publicBounds)
}
func (a *App) ReloadBrowserSession(storeID, origin string) error {
	store, err := a.browserStore(storeID)
	if err != nil {
		return err
	}
	if !a.browserSessions.Status(storeID).Enabled {
		return fmt.Errorf("请先启用持久会话")
	}
	return a.browserSessions.Reload(storeID, store.ProxyURL, origin)
}
func (a *App) CheckBrowserSession(storeID string) error {
	store, err := a.browserStore(storeID)
	if err != nil {
		return err
	}
	if !a.browserSessions.Status(storeID).Enabled {
		return fmt.Errorf("请先启用持久会话")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 65*time.Second)
	defer cancel()
	return a.browserSessions.Check(ctx, storeID, store.ProxyURL, store.Authorization)
}
