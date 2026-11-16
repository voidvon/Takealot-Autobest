package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type memoryStore struct{ token string }

func (s *memoryStore) Load() (string, error) { return s.token, nil }
func (s *memoryStore) Save(v string) error   { s.token = v; return nil }
func (s *memoryStore) Delete() error         { s.token = ""; return nil }

func TestOnlineAccountLifecycle(t *testing.T) {
	ctx := context.Background()
	group := "other"
	memberStatus := "active"
	var expiry *string
	unauthorized := false
	outage := false
	limit := false
	token := ""
	logins := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Cookie") != "" || r.URL.RawQuery != "" {
			t.Error("member credentials must only use the Authorization header")
		}
		if outage {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"code":"service_unavailable","message":"offline"}}`))
			return
		}
		authorization := r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/api/v1/auth/register":
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"data":{"user":{"id":1,"username":"alice"}}}`))
		case "/api/v1/auth/token":
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
				t.Error("token login must POST JSON")
			}
			var credentials map[string]string
			if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil || credentials["identifier"] != "alice" || credentials["password"] != "password123" {
				t.Error("incorrect token login payload")
			}
			if limit {
				w.WriteHeader(409)
				_, _ = w.Write([]byte(`{"error":{"code":"session_limit_reached","message":"limit"}}`))
				return
			}
			if logins > 0 && authorization != "Bearer "+token {
				t.Error("re-login dropped current Bearer token")
			}
			logins++
			token = strings.Repeat("t", logins+5)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": tokenGrant{User: User{ID: 1, Username: "alice"}, AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600}})
		case "/api/v1/auth/logout":
			if authorization != "Bearer "+token {
				t.Error("logout dropped current Bearer token")
			}
			token = ""
			_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
		default:
			if unauthorized || token == "" || authorization != "Bearer "+token {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"error":{"code":"authentication_required","message":"login"}}`))
				return
			}
			if r.URL.Path == "/api/v1/me" {
				_, _ = w.Write([]byte(`{"data":{"id":1,"username":"alice","display_name":"Alice"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []Membership{{Slug: group, Status: memberStatus, ExpiresAt: expiry}}})
		}
	}))
	defer srv.Close()
	store := &memoryStore{}
	m, err := NewWithStore(srv.URL, "takealot-vip", store)
	if err != nil {
		t.Fatal(err)
	}
	if m.IsValid() {
		t.Fatal("unauthenticated access")
	}
	if err := m.Register(ctx, "alice", "", "password123"); err != nil {
		t.Fatal(err)
	}
	status, err := m.Login(ctx, "alice", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Authenticated || status.Eligible {
		t.Fatal("wrong membership authorized")
	}
	group = "takealot-vip"
	if !m.Refresh(ctx).Eligible {
		t.Fatal("VIP denied")
	}
	first := store.token
	if _, err := m.Login(ctx, "alice", "password123"); err != nil || first == store.token {
		t.Fatal("rotation failed", err)
	}
	limit = true
	_, err = m.Login(ctx, "alice", "password123")
	var api *APIError
	if !errors.As(err, &api) || api.Code != "session_limit_reached" {
		t.Fatal(err)
	}
	limit = false
	restored, err := NewWithStore(srv.URL, "takealot-vip", store)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Refresh(ctx).Eligible {
		t.Fatal("persisted token not restored")
	}
	memberStatus = "expired"
	if m.Refresh(ctx).Eligible {
		t.Fatal("expired membership allowed")
	}
	memberStatus = "active"
	past := time.Now().Add(-time.Second).UTC().Format(time.RFC3339)
	expiry = &past
	if m.Refresh(ctx).Eligible {
		t.Fatal("past timestamp allowed")
	}
	expiry = nil
	if !m.Refresh(ctx).Eligible {
		t.Fatal("permanent VIP denied")
	}
	outage = true
	if m.Refresh(ctx).Eligible {
		t.Fatal("offline access allowed")
	}
	outage = false
	if !m.Refresh(ctx).Eligible {
		t.Fatal("online recovery failed")
	}
	m.mu.Lock()
	m.checked = time.Now().Add(-time.Minute)
	m.mu.Unlock()
	if !m.IsValid() {
		t.Fatal("valid membership expired within the daily verification interval")
	}
	m.mu.Lock()
	m.checked = time.Now().Add(-25 * time.Hour)
	m.mu.Unlock()
	if m.IsValid() {
		t.Fatal("stale state allowed")
	}
	unauthorized = true
	if m.Refresh(ctx).Authenticated || store.token != "" {
		t.Fatal("revocation not respected")
	}
	unauthorized = false
	logins = 0
	if _, err = m.Login(ctx, "alice", "password123"); err != nil {
		t.Fatal(err)
	}
	outage = true
	if err = m.Logout(ctx); err == nil {
		t.Fatal("offline logout reported success")
	}
	if m.GetStatus().Persistent || m.GetStatus().Authenticated || store.token != "" {
		t.Fatal("offline logout retained persisted or authenticated state")
	}
	if err := m.ChangePassword(ctx, "password123", "newpassword"); err == nil {
		t.Fatal("pending logout credential used for password change")
	}
	outage = false
	if m.Refresh(ctx).Eligible {
		t.Fatal("failed logout silently restored session")
	}
	if err = m.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if store.token != "" {
		t.Fatal("logout failed to clear token")
	}
}

func TestURLSecurity(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://user:password@example.com", "https://example.com/?token=secret"} {
		if _, err := NewWithStore(raw, "vip", &memoryStore{}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	m, err := NewWithStore("", "vip", &memoryStore{})
	if err != nil || m.IsValid() {
		t.Fatal("unconfigured not locked")
	}
}

func TestChangePassword(t *testing.T) {
	ctx := context.Background()
	var receivedOld, receivedNew string
	failPassword := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Cookie") != "" {
			t.Error("password flow sent Cookie")
		}
		if r.URL.Path == "/api/v1/auth/token" {
			_, _ = w.Write([]byte(`{"data":{"user":{"id":1,"username":"alice"},"access_token":"token-abc","token_type":"Bearer","expires_in":3600}}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer token-abc" {
			t.Error("password flow lost Bearer token")
		}
		if r.URL.Path == "/api/v1/me" {
			_, _ = w.Write([]byte(`{"data":{"id":1,"username":"alice"}}`))
			return
		}
		if r.URL.Path == "/api/v1/me/memberships" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		if r.URL.Path == "/api/v1/me/password" && r.Method == http.MethodPut {
			if failPassword {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"error":{"code":"invalid_credentials","message":"原密码错误"}}`))
				return
			}
			var in struct {
				Old string `json:"old_password"`
				New string `json:"new_password"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			receivedOld = in.Old
			receivedNew = in.New
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"data":{"reauthenticate":true}}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	store := &memoryStore{}
	m, err := NewWithStore(srv.URL, "takealot-vip", store)
	if err != nil {
		t.Fatal(err)
	}

	// 1. 未登录
	if err := m.ChangePassword(ctx, "old", "newpassword123"); err == nil {
		t.Fatal("expected unauthorized error")
	}

	// 登录
	if _, err := m.Login(ctx, "alice", "oldpwd"); err != nil {
		t.Fatal(err)
	}

	// 2. 校验参数
	if err := m.ChangePassword(ctx, "", "newpassword123"); err == nil {
		t.Fatal("expected empty old password error")
	}
	if err := m.ChangePassword(ctx, "old", "123"); err == nil {
		t.Fatal("expected short new password error")
	}
	for _, password := range []string{"1234567", strings.Repeat("a", 1025)} {
		if err := m.ChangePassword(ctx, "old", password); err == nil {
			t.Fatal("accepted password outside GoCMS limits")
		}
	}

	// 3. 原密码错误
	failPassword = true
	if err := m.ChangePassword(ctx, "wrong", "newpassword123"); err == nil {
		t.Fatal("expected error on wrong old password")
	}
	if !m.GetStatus().Authenticated || store.token == "" {
		t.Fatal("wrong password must not discard a valid login")
	}
	failPassword = false

	// 4. 成功修改密码
	if err := m.ChangePassword(ctx, "oldpwd", "newpassword123"); err != nil {
		t.Fatalf("unexpected change password error: %v", err)
	}
	if receivedOld != "oldpwd" || receivedNew != "newpassword123" {
		t.Fatalf("wrong payload received: %s, %s", receivedOld, receivedNew)
	}
	if m.IsValid() || store.token != "" {
		t.Fatal("session should be cleared after password change")
	}
}
