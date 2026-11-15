// Package account integrates the online GoCMS member API. It never stores passwords.
package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}
type Membership struct {
	Slug      string  `json:"slug"`
	Status    string  `json:"status"`
	ExpiresAt *string `json:"expires_at"`
}
type Status struct {
	Authenticated bool    `json:"authenticated"`
	Eligible      bool    `json:"eligible"`
	State         string  `json:"state"`
	Message       string  `json:"message"`
	User          *User   `json:"user"`
	ExpiresAt     *string `json:"expires_at"`
	Persistent    bool    `json:"persistent"`
}
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *APIError) Error() string { return e.Message }

type Store interface {
	Load() (string, error)
	Save(string) error
	Delete() error
}

type Manager struct {
	signedOut bool
	mu        sync.Mutex
	group     string
	api       *memberClient
	store     Store
	token     string
	status    Status
	checked   time.Time
	expiry    time.Time
}

func New(base, group string) (*Manager, error) {
	return NewWithStore(base, group, nil)
}
func NewWithStore(base, group string, store Store) (*Manager, error) {
	api, err := newMemberClient(base)
	if err != nil {
		return nil, err
	}
	m := &Manager{api: api, group: group, store: store, status: Status{State: "signed_out", Message: "请登录会员账号"}}
	if base == "" {
		m.status = Status{State: "not_configured", Message: "尚未配置会员服务，请联系软件管理员"}
		return m, nil
	}
	if group == "" {
		return nil, errors.New("会员组标识不能为空")
	}
	if store != nil {
		// The old client stored the raw GoCMS session credential. The same value
		// is accepted as Bearer, so upgrades keep the existing session and slot.
		if token, e := store.Load(); e == nil && validBearerToken(token) {
			m.token = token
			m.status.Persistent = true
		}
	}
	return m, nil
}
func (m *Manager) saveToken(token string) {
	m.token = token
	m.status.Persistent = false
	if m.store != nil {
		m.status.Persistent = m.store.Save(token) == nil
		if !m.status.Persistent {
			// A rotated-out credential must not survive a failed replacement.
			_ = m.store.Delete()
		}
	}
}

func (m *Manager) forgetStoredToken() {
	m.status.Persistent = false
	if m.store != nil {
		_ = m.store.Delete()
	}
}

func (m *Manager) clearToken() {
	m.token = ""
	m.forgetStoredToken()
}
func (m *Manager) Register(ctx context.Context, username, email, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.api.base == "" {
		return &APIError{Code: "not_configured", Message: m.status.Message, Status: 503}
	}
	return m.api.call(ctx, "POST", "/auth/register", "", map[string]string{"username": username, "email": email, "password": password}, nil)
}
func (m *Manager) ChangePassword(ctx context.Context, oldPassword, newPassword string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.api.base == "" {
		return &APIError{Code: "not_configured", Message: m.status.Message, Status: 503}
	}
	if m.signedOut || m.token == "" {
		return &APIError{Code: "unauthorized", Message: "未登录，请先登录账号", Status: 401}
	}
	if strings.TrimSpace(oldPassword) == "" {
		return &APIError{Code: "invalid", Message: "请输入原密码", Status: 400}
	}
	if len(oldPassword) > 1024 || len(newPassword) < 8 || len(newPassword) > 1024 {
		return &APIError{Code: "invalid", Message: "新密码长度须为 8–1024 字节，原密码不能超过 1024 字节", Status: 400}
	}
	var out struct {
		Reauthenticate bool `json:"reauthenticate"`
	}
	err := m.api.call(ctx, "PUT", "/me/password", m.token, map[string]string{
		"old_password": oldPassword,
		"new_password": newPassword,
	}, &out)
	if err != nil {
		// Wrong old passwords also return 401 (invalid_credentials). Only an
		// authentication rejection invalidates the current session here.
		var api *APIError
		if errors.As(err, &api) && api.Status == 401 && api.Code != "invalid_credentials" {
			m.refreshError(err)
		}
		return err
	}
	m.signedOut = true
	m.clearToken()
	m.checked, m.expiry = time.Time{}, time.Time{}
	m.status = Status{State: "signed_out", Message: "密码修改成功，请重新登录"}
	return nil
}
func (m *Manager) Login(ctx context.Context, identifier, password string) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.api.base == "" {
		return m.status, &APIError{Code: "not_configured", Message: m.status.Message, Status: 503}
	}
	// Pass the current Bearer credential so same-account login rotates the
	// existing session instead of consuming another GoCMS session slot.
	grant, e := m.api.login(ctx, m.token, identifier, password)
	if e != nil {
		return m.status, e
	}
	m.saveToken(grant.AccessToken)
	m.signedOut = false
	m.checked = time.Time{}
	return m.refresh(ctx), nil
}
func (m *Manager) refresh(ctx context.Context) Status {
	if m.signedOut {
		return m.status
	}
	persistent := m.status.Persistent
	m.status = Status{State: "signed_out", Message: "请登录会员账号", Persistent: persistent}
	m.expiry = time.Time{}
	if m.api.base == "" {
		m.status.State = "not_configured"
		m.status.Message = "尚未配置会员服务，请联系软件管理员"
		return m.status
	}
	if m.token == "" {
		return m.status
	}
	var u User
	if e := m.api.call(ctx, "GET", "/me", m.token, nil, &u); e != nil {
		return m.refreshError(e)
	}
	if u.ID <= 0 || u.Username == "" {
		return m.refreshError(invalidResponse())
	}
	m.status.User = &u
	m.status.Authenticated = true
	var memberships []Membership
	if e := m.api.call(ctx, "GET", "/me/memberships", m.token, nil, &memberships); e != nil {
		return m.refreshError(e)
	}
	m.status.State = "membership_required"
	m.status.Message = "账号尚无有效 VIP，请联系管理员开通或续期"
	for _, g := range memberships {
		if g.Slug != m.group || g.Status != "active" {
			continue
		}
		if g.ExpiresAt != nil {
			t, e := time.Parse(time.RFC3339, *g.ExpiresAt)
			if e != nil || !t.After(time.Now()) {
				continue
			}
			m.expiry = t
		}
		m.status.Eligible = true
		m.status.State = "active"
		m.status.Message = "VIP 有效"
		m.status.ExpiresAt = g.ExpiresAt
		break
	}
	m.checked = time.Now()
	return m.status
}

func (m *Manager) refreshError(err error) Status {
	var api *APIError
	if errors.As(err, &api) && api.Status == 401 {
		m.clearToken()
		m.status = Status{State: "signed_out", Message: "登录已失效，请重新登录"}
	} else {
		m.status.Eligible = false
		m.status.State = "unavailable"
		m.status.Message = err.Error()
	}
	m.expiry = time.Time{}
	m.checked = time.Now()
	return m.status
}
func (m *Manager) Refresh(ctx context.Context) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refresh(ctx)
}
func (m *Manager) GetStatus() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	if s.Eligible && (time.Since(m.checked) > 45*time.Second || (!m.expiry.IsZero() && !m.expiry.After(time.Now()))) {
		s.Eligible = false
		s.State = "verification_required"
		s.Message = "需要重新验证会员状态"
	}
	return s
}
func (m *Manager) IsValid() bool { return m.GetStatus().Eligible }
func (m *Manager) Logout(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Fail closed locally even if the remote logout cannot complete; preserve the
	// credential for retry so an unreachable service does not silently leak a slot.
	m.signedOut = true
	m.status = Status{State: "signed_out", Message: "正在退出登录"}
	m.checked, m.expiry = time.Time{}, time.Time{}
	m.forgetStoredToken()
	if m.token != "" {
		if e := m.api.call(ctx, "POST", "/auth/logout", m.token, map[string]string{}, nil); e != nil {
			m.status.Message = "退出未完成，请恢复网络后重试"
			return e
		}
	}
	m.clearToken()
	m.status = Status{State: "signed_out", Message: "已退出登录"}
	return nil
}
func (m *Manager) Run(ctx context.Context, onInvalid func()) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		state := m.Refresh(ctx)
		if !state.Eligible && onInvalid != nil {
			onInvalid()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (m *Manager) String() string { return fmt.Sprintf("GoCMS member group %s", m.group) }
