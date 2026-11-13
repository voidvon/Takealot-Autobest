// Package browsersession keeps Takealot requests inside an isolated native
// browser profile. It never exports cookies or exposes Wails bindings remotely.
package browsersession

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"takealot/pkg/api"
)

const seller = "https://seller-api.takealot.com"
const public = "https://api.takealot.com"

var origins = []string{seller, public}
var initialPaths = []string{"/v2/offers/count", "/rest/v-1-16-0/product-details/PLID98958583?platform=desktop"}

type Bounds struct {
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Width          float64 `json:"width"`
	Height         float64 `json:"height"`
	ViewportWidth  float64 `json:"viewport_width"`
	ViewportHeight float64 `json:"viewport_height"`
}

type Driver interface {
	Start() error
	Create(id, profile, proxy, origin, initialURL string) error
	Eval(context.Context, string, string) (string, error)
	Show(string, Bounds) error
	Hide() error
	Destroy(string) error
	Close()
}

type OriginStatus struct {
	Origin string `json:"origin"`
	Phase  string `json:"phase"`
}
type Status struct {
	StoreID   string         `json:"store_id"`
	Available bool           `json:"available"`
	Enabled   bool           `json:"enabled"`
	Phase     string         `json:"phase"`
	Message   string         `json:"message"`
	Revision  uint64         `json:"revision"`
	Origins   []OriginStatus `json:"origins"`
}

type session struct {
	mu          sync.Mutex
	requests    chan struct{}
	once        sync.Once
	initialized chan struct{}
	initErr     error
	proxy       string
	ids         [2]string
	phase       string
	message     string
	confirmed   bool
	revision    uint64
	origins     [2]string
}

func (s *session) lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.requests <- struct{}{}:
		return nil
	}
}
func (s *session) unlock() { <-s.requests }

type Manager struct {
	mu              sync.Mutex
	configuration   sync.Mutex
	lifecycle       sync.Mutex
	root            string
	driver          Driver
	available       bool
	enabled         map[string]bool
	sessions        map[string]*session
	directBlocked   map[string]bool
	changed         func(string, bool, string)
	closed          bool
	presentation    sync.Mutex
	visibleRevision atomic.Uint64
}

func New(root string, driver Driver, changed func(string, bool, string)) (*Manager, error) {
	m := &Manager{root: root, driver: driver, enabled: map[string]bool{}, sessions: map[string]*session{}, directBlocked: map[string]bool{}, changed: changed}
	data, err := os.ReadFile(filepath.Join(root, "enabled.json"))
	if err == nil {
		if err = json.Unmarshal(data, &m.enabled); err != nil {
			return nil, fmt.Errorf("读取浏览器会话设置失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if m.enabled == nil {
		m.enabled = map[string]bool{}
	}
	return m, nil
}

func (m *Manager) Start() error {
	m.mu.Lock()
	started := m.available
	m.mu.Unlock()
	if started {
		return nil
	}
	if err := m.driver.Start(); err != nil {
		return err
	}
	m.mu.Lock()
	m.available = true
	m.mu.Unlock()
	return nil
}

func profileID(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:16])
}

func (m *Manager) Configure(storeID, proxy string, enabled bool) error {
	m.configuration.Lock()
	defer m.configuration.Unlock()
	if !enabled {
		m.lifecycle.Lock()
		defer m.lifecycle.Unlock()
	}
	m.mu.Lock()
	if m.closed || !m.available {
		m.mu.Unlock()
		return errors.New("持久会话需要 Windows/macOS 桌面版，当前环境未就绪")
	}
	m.mu.Unlock()
	if enabled {
		if err := validateProxy(proxy); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(m.root, 0700); err != nil {
		return err
	}
	m.mu.Lock()
	settings := make(map[string]bool, len(m.enabled)+1)
	for id, value := range m.enabled {
		settings[id] = value
	}
	settings[storeID] = enabled
	data, _ := json.Marshal(settings)
	temp := filepath.Join(m.root, "enabled.json.tmp")
	err := os.WriteFile(temp, data, 0600)
	if err == nil {
		err = os.Rename(temp, filepath.Join(m.root, "enabled.json"))
	}
	if err != nil {
		m.mu.Unlock()
		return err
	}
	m.enabled = settings
	delete(m.directBlocked, storeID)
	old := m.sessions[storeID]
	if !enabled {
		delete(m.sessions, storeID)
	}
	m.mu.Unlock()
	if !enabled {
		if old != nil {
			for _, id := range old.ids {
				_ = m.driver.Destroy(id)
			}
		}
		m.notify(storeID, false, "已切换为直接请求")
		return nil
	}
	_, err = m.ensure(storeID, proxy)
	return err
}

func validateProxy(proxy string) error {
	if proxy == "" {
		return nil
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") || strings.ContainsAny(proxy, "\"\r\n \t") {
		return errors.New("浏览器会话仅支持无认证的 HTTP/HTTPS/SOCKS5 代理；带账号密码的代理请改用系统代理或 TUN")
	}
	return nil
}

func (m *Manager) ensure(storeID, proxy string) (*session, error) {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()
	if err := validateProxy(proxy); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed || !m.available {
		m.mu.Unlock()
		return nil, errors.New("浏览器会话尚未就绪，请在桌面版打开访问会话")
	}
	s := m.sessions[storeID]
	if s != nil && s.proxy != proxy {
		// Do not reuse clearance across a changed egress/proxy.
		delete(m.sessions, storeID)
		for _, id := range s.ids {
			_ = m.driver.Destroy(id)
		}
		s = nil
	}
	if s == nil {
		s = &session{proxy: proxy, initialized: make(chan struct{}), requests: make(chan struct{}, 1), phase: "loading", message: "正在准备访问会话，请检查两个接口", revision: 1, origins: [2]string{"loading", "loading"}}
		for i := range origins {
			s.ids[i] = profileID(storeID+"#"+proxy) + fmt.Sprint(i)
		}
		m.sessions[storeID] = s
	}
	m.mu.Unlock()
	s.once.Do(func() {
		defer close(s.initialized)
		profile := filepath.Join(m.root, profileID(storeID+"#"+proxy))
		if err := os.MkdirAll(profile, 0700); err != nil {
			s.initErr = err
			return
		}
		for i, origin := range origins {
			if err := m.driver.Create(s.ids[i], profile, proxy, origin, origin+initialPaths[i]); err != nil {
				s.initErr = err
				break
			}
		}
		if s.initErr != nil {
			m.pause(storeID, s, "error", "无法创建浏览器会话，请检查 WebView2 运行时或代理")
		} else {
			m.pause(storeID, s, "verification_required", "请在访问会话中检查页面，完成验证后点击检查并恢复")
		}
	})
	<-s.initialized
	return s, s.initErr
}

func (m *Manager) notify(id string, required bool, message string) {
	if m.changed != nil {
		m.changed(id, required, message)
	}
}
func (m *Manager) pause(id string, s *session, phase, message string) {
	s.mu.Lock()
	changed := s.phase != phase || s.confirmed
	s.confirmed = false
	s.phase = phase
	s.message = message
	if changed {
		s.revision++
	}
	s.mu.Unlock()
	m.notify(id, true, message)
}

func (m *Manager) Status(storeID string) Status {
	m.mu.Lock()
	enabled, available, s := m.enabled[storeID], m.available, m.sessions[storeID]
	m.mu.Unlock()
	status := Status{StoreID: storeID, Enabled: enabled, Available: available, Phase: "disabled", Message: "使用直接请求"}
	if !available {
		status.Message = "当前平台或运行模式尚未提供内嵌会话"
	}
	if !enabled {
		m.mu.Lock()
		blocked := m.directBlocked[storeID]
		m.mu.Unlock()
		if blocked {
			status.Phase = "direct_blocked"
			status.Message = "直接请求遇到网页验证，可停止任务后启用持久会话"
		}
		return status
	}
	status.Phase = "verification_required"
	status.Message = "请打开访问会话并检查连接"
	if s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		status.Phase = s.phase
		status.Message = s.message
		status.Revision = s.revision
		for i, origin := range origins {
			status.Origins = append(status.Origins, OriginStatus{origin, s.origins[i]})
		}
	}
	return status
}

func (m *Manager) Do(storeID, proxy string, req *http.Request, direct func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	m.mu.Lock()
	enabled := m.enabled[storeID]
	m.mu.Unlock()
	if !enabled {
		response, err := direct(req)
		if err == nil && strings.EqualFold(response.Header.Get("Cf-Mitigated"), "challenge") {
			m.mu.Lock()
			m.directBlocked[storeID] = true
			m.mu.Unlock()
		}
		return response, err
	}
	s, err := m.ensure(storeID, proxy)
	if err != nil {
		m.notify(storeID, true, err.Error())
		return nil, &api.VerificationRequiredError{Message: err.Error()}
	}
	if err := s.lock(req.Context()); err != nil {
		return nil, err
	}
	defer s.unlock()
	s.mu.Lock()
	confirmed, message := s.confirmed, s.message
	s.mu.Unlock()
	if !confirmed {
		return nil, &api.VerificationRequiredError{Message: message}
	}
	response, err := m.fetch(req, s)
	if err != nil {
		return nil, err
	}
	if response.Header.Get("Cf-Mitigated") == "challenge" {
		response.Body.Close()
		s.mu.Lock()
		for i, origin := range origins {
			if origin == req.URL.Scheme+"://"+req.URL.Host {
				s.origins[i] = "verification_required"
			}
		}
		s.mu.Unlock()
		m.pause(storeID, s, "verification_required", "访问验证已过期，任务已暂停，请完成验证后检查并恢复")
		for i, origin := range origins {
			if origin == req.URL.Scheme+"://"+req.URL.Host {
				_ = m.driver.Create(s.ids[i], filepath.Join(m.root, profileID(storeID+"#"+proxy)), proxy, origin, origin+initialPaths[i])
			}
		}
		return nil, &api.VerificationRequiredError{Message: "Cloudflare 需要人工验证，请打开访问会话；本轮价格计算已中止"}
	}
	if response.StatusCode == 401 {
		m.pause(storeID, s, "auth_required", "店铺授权已失效，请更新 Authorization 后检查连接")
	}
	return response, nil
}

func (m *Manager) fetch(req *http.Request, s *session) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	origin := req.URL.Scheme + "://" + req.URL.Host
	index := -1
	for i, value := range origins {
		if value == origin {
			index = i
		}
	}
	if index < 0 || req.URL.User != nil || req.URL.Fragment != "" {
		return nil, &api.VerificationRequiredError{Message: "会话拒绝非 Takealot API 地址"}
	}
	body := []byte(nil)
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(req.Body, 8*1024*1024+1))
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		if len(body) > 8*1024*1024 {
			return nil, errors.New("请求正文超过会话限制")
		}
	}
	auth := ""
	if index == 0 {
		auth = req.Header.Get("Authorization")
	}
	args, _ := json.Marshal(map[string]any{"origin": origin, "path": req.URL.RequestURI(), "method": req.Method, "body": string(body), "authorization": auth})
	script := fmt.Sprintf(fetchScript, string(args))
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()
	raw, err := m.driver.Eval(ctx, s.ids[index], script)
	if err != nil {
		return nil, errors.New("浏览器请求执行失败；请检查网络，写入结果需回读确认")
	}
	var result struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
		Error   string            `json:"error"`
	}
	if err = json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, errors.New("浏览器返回格式无效")
	}
	if result.Error != "" {
		return nil, errors.New("浏览器请求失败（" + result.Error + "），请检查连接")
	}
	if result.Status < 100 || result.Status > 599 {
		return nil, errors.New("浏览器未返回有效 HTTP 状态")
	}
	headers := make(http.Header)
	for name, value := range result.Headers {
		headers.Set(name, value)
	}
	return &http.Response{StatusCode: result.Status, Status: fmt.Sprintf("%d %s", result.Status, http.StatusText(result.Status)), Header: headers, Body: io.NopCloser(bytes.NewBufferString(result.Body)), ContentLength: int64(len(result.Body)), Request: req}, nil
}

// No response text or exception message enters logs. In particular, remote
// exceptions can contain Authorization/request bodies; only fixed error codes
// cross this boundary. fetch never follows redirects carrying credentials.
const fetchScript = `return await (async () => {
 const q = %s;
 if(location.origin !== q.origin) return JSON.stringify({error:'origin'});
 const controller = new AbortController(); const timer = setTimeout(()=>controller.abort(),25000);
 try {
  const headers={Accept:'application/json, text/plain, */*'};
  if(q.authorization) headers.Authorization=q.authorization;
  if(q.body) headers['Content-Type']='application/json';
  const response=await fetch(q.path,{method:q.method,headers,body:q.body||undefined,credentials:'include',cache:'no-store',redirect:'error',signal:controller.signal});
  const body=await response.text();
  const challenge=response.headers.get('cf-mitigated')==='challenge'||(/^\s*</.test(body)&&/Just a moment|cf-chl|challenge-platform/i.test(body));
  const h={'content-type':response.headers.get('content-type')||'', 'retry-after':response.headers.get('retry-after')||''};
  if(challenge) h['cf-mitigated']='challenge';
  return JSON.stringify({status:response.status,headers:h,body});
 } catch(e) { return JSON.stringify({error:e.name==='AbortError'?'timeout':'network'}); }
 finally {clearTimeout(timer);}
})();`

func originIndex(origin string) (int, error) {
	for i, value := range origins {
		if value == origin {
			return i, nil
		}
	}
	return 0, errors.New("未知会话接口")
}
func (m *Manager) Show(storeID, proxy, origin string, bounds Bounds) error {
	index, err := originIndex(origin)
	if err != nil {
		return err
	}
	return m.showPanels(storeID, proxy, []int{index}, []Bounds{bounds})
}

// ShowPanels presents both origins together, so resizing, hiding, and switching
// stores cannot leave a page from the previous presentation visible.
func (m *Manager) ShowPanels(storeID, proxy string, sellerBounds, publicBounds Bounds) error {
	return m.showPanels(storeID, proxy, []int{0, 1}, []Bounds{sellerBounds, publicBounds})
}

func (m *Manager) showPanels(storeID, proxy string, indices []int, bounds []Bounds) error {
	revision := m.visibleRevision.Add(1)
	for _, b := range bounds {
		if b.Width < 1 || b.Height < 1 || b.ViewportWidth < 1 || b.ViewportHeight < 1 {
			return errors.New("验证面板尺寸无效")
		}
	}
	s, err := m.ensure(storeID, proxy)
	if err != nil {
		return err
	}
	m.presentation.Lock()
	defer m.presentation.Unlock()
	if m.visibleRevision.Load() != revision {
		return nil
	}
	if err := m.driver.Hide(); err != nil {
		return err
	}
	for i, index := range indices {
		if err := m.driver.Show(s.ids[index], bounds[i]); err != nil {
			_ = m.driver.Hide()
			return err
		}
	}
	return nil
}
func (m *Manager) Hide() error {
	m.visibleRevision.Add(1)
	m.presentation.Lock()
	defer m.presentation.Unlock()
	return m.driver.Hide()
}
func (m *Manager) Reload(storeID, proxy, origin string) error {
	index, err := originIndex(origin)
	if err != nil {
		return err
	}
	s, err := m.ensure(storeID, proxy)
	if err != nil {
		return err
	}
	m.pause(storeID, s, "verification_required", "请完成页面验证后检查并恢复")
	return m.driver.Create(s.ids[index], filepath.Join(m.root, profileID(storeID+"#"+proxy)), proxy, origin, origin+initialPaths[index])
}
func (m *Manager) Check(ctx context.Context, storeID, proxy, auth string) error {
	s, err := m.ensure(storeID, proxy)
	if err != nil {
		return err
	}
	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()
	for i, origin := range origins {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, origin+initialPaths[i], nil)
		if i == 0 {
			req.Header.Set("Authorization", api.FormatAuth(auth))
		}
		resp, err := m.fetch(req, s)
		if err != nil {
			m.pause(storeID, s, "verification_required", "页面尚未就绪，请等待加载或重新打开验证页")
			return err
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		var data map[string]json.RawMessage
		valid := readErr == nil && json.Unmarshal(body, &data) == nil
		if i == 0 {
			var count int
			raw, ok := data["count"]
			valid = valid && ok && json.Unmarshal(raw, &count) == nil
		} else {
			raw, ok := data["buybox"]
			valid = valid && ok && string(raw) != "null"
		}
		if resp.StatusCode != 200 || !valid {
			phase, message := "verification_required", "请在两个接口页面完成人工验证，再检查连接"
			if resp.StatusCode == 401 {
				phase = "auth_required"
				message = "授权已失效，请更新 Authorization"
			}
			if resp.StatusCode == 429 || resp.StatusCode == 420 {
				phase = "rate_limited"
				message = "接口暂时限流，请稍后再检查"
			}
			s.mu.Lock()
			s.origins[i] = phase
			s.mu.Unlock()
			m.pause(storeID, s, phase, message)
			return fmt.Errorf("%s HTTP %d：%s", origin, resp.StatusCode, message)
		}
		s.mu.Lock()
		s.origins[i] = "ready"
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.confirmed = true
	s.phase = "ready"
	s.message = "会话可用，后续请求将复用当前会话"
	s.revision++
	s.mu.Unlock()
	m.notify(storeID, false, "访问会话已就绪，运行中的任务将重新比价")
	return nil
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.available = false
	m.mu.Unlock()
	m.driver.Close()
}
