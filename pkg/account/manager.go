// Package account integrates the online GoCMS member API. It never stores passwords.
package account

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
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
type keyringStore struct{ key string }

func (s keyringStore) Load() (string, error) { return keyring.Get("Takealot-GoCMS", s.key) }
func (s keyringStore) Save(v string) error   { return keyring.Set("Takealot-GoCMS", s.key, v) }
func (s keyringStore) Delete() error         { return keyring.Delete("Takealot-GoCMS", s.key) }

type Manager struct {
	signedOut   bool
	mu          sync.Mutex
	base, group string
	client      *http.Client
	store       Store
	token       string
	status      Status
	checked     time.Time
	expiry      time.Time
}

func New(base, group string) (*Manager, error) {
	sum := sha256.Sum256([]byte(strings.TrimRight(base, "/") + "|" + group))
	return NewWithStore(base, group, keyringStore{hex.EncodeToString(sum[:])})
}
func NewWithStore(base, group string, store Store) (*Manager, error) {
	m := &Manager{base: strings.TrimRight(base, "/"), group: group, store: store, status: Status{State: "signed_out", Message: "请登录会员账号"}}
	m.client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("会员服务不允许重定向") }}
	if base == "" {
		m.status = Status{State: "not_configured", Message: "尚未配置会员服务，请联系软件管理员"}
		return m, nil
	}
	u, e := url.Parse(m.base)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()))) {
		return nil, errors.New("会员服务地址必须为 HTTPS；仅本机开发允许 HTTP")
	}
	if group == "" {
		return nil, errors.New("会员组标识不能为空")
	}
	if store != nil {
		if token, e := store.Load(); e == nil {
			m.token = token
			m.status.Persistent = true
		}
	}
	return m, nil
}
func (m *Manager) call(ctx context.Context, method, path string, input, output any) error {
	var body []byte
	if input != nil {
		var e error
		body, e = json.Marshal(input)
		if e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, m.base+"/api/v1"+path, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	if m.token != "" {
		req.AddCookie(&http.Cookie{Name: "gocms_user", Value: m.token})
	}
	res, e := m.client.Do(req)
	if e != nil {
		return &APIError{Code: "service_unavailable", Message: "无法连接会员服务，请检查网络后重试", Status: 503}
	}
	defer res.Body.Close()
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error APIError        `json:"error"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope); e != nil {
		return &APIError{Code: "service_unavailable", Message: "会员服务响应异常", Status: 503}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		envelope.Error.Status = res.StatusCode
		if envelope.Error.Code == "" {
			envelope.Error.Code = "service_error"
			envelope.Error.Message = "会员请求失败"
		}
		return &envelope.Error
	}
	if output != nil {
		if e = json.Unmarshal(envelope.Data, output); e != nil {
			return &APIError{Code: "service_unavailable", Message: "会员服务数据异常", Status: 503}
		}
	}
	for _, cookie := range res.Cookies() {
		if cookie.Name == "gocms_user" {
			m.token = cookie.Value
			m.status.Persistent = false
			if m.store != nil {
				if m.token == "" || cookie.MaxAge < 0 {
					_ = m.store.Delete()
				} else {
					m.status.Persistent = m.store.Save(m.token) == nil
				}
			}
		}
	}
	return nil
}
func (m *Manager) Register(ctx context.Context, username, email, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.base == "" {
		return &APIError{Code: "not_configured", Message: m.status.Message, Status: 503}
	}
	return m.call(ctx, "POST", "/auth/register", map[string]string{"username": username, "email": email, "password": password}, nil)
}
func (m *Manager) Login(ctx context.Context, identifier, password string) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.base == "" {
		return m.status, &APIError{Code: "not_configured", Message: m.status.Message, Status: 503}
	}
	var data struct {
		User User `json:"user"`
	}
	// Always pass the current cookie: GoCMS rotates a live same-account session.
	if e := m.call(ctx, "POST", "/auth/login", map[string]string{"identifier": identifier, "password": password}, &data); e != nil {
		return m.status, e
	}
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
	if m.base == "" {
		m.status.State = "not_configured"
		m.status.Message = "尚未配置会员服务，请联系软件管理员"
		return m.status
	}
	if m.token == "" {
		return m.status
	}
	var u User
	if e := m.call(ctx, "GET", "/me", nil, &u); e != nil {
		var api *APIError
		if errors.As(e, &api) && api.Status == 401 {
			m.token = ""
			if m.store != nil {
				_ = m.store.Delete()
			}
			m.status.Message = "登录已失效，请重新登录"
		} else {
			m.status.State = "unavailable"
			m.status.Message = e.Error()
		}
		m.checked = time.Now()
		return m.status
	}
	m.status.User = &u
	m.status.Authenticated = true
	var memberships []Membership
	if e := m.call(ctx, "GET", "/me/memberships", nil, &memberships); e != nil {
		m.status.State = "unavailable"
		m.status.Message = e.Error()
		m.checked = time.Now()
		return m.status
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
	m.status.Eligible = false
	m.status.Authenticated = false
	m.status.User = nil
	m.status.State = "signed_out"
	if m.store != nil {
		_ = m.store.Delete()
	}
	if m.token != "" {
		if e := m.call(ctx, "POST", "/auth/logout", map[string]string{}, nil); e != nil {
			m.status.Message = "退出未完成，请恢复网络后重试"
			return e
		}
	}
	m.token = ""
	if m.store != nil {
		_ = m.store.Delete()
	}
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
