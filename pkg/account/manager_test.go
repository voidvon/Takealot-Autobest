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
		if outage {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":{"code":"service_unavailable","message":"offline"}}`))
			return
		}
		cookie, _ := r.Cookie("gocms_user")
		switch r.URL.Path {
		case "/api/v1/auth/register":
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"data":{"user":{"id":1,"username":"alice"}}}`))
		case "/api/v1/auth/login":
			if limit {
				w.WriteHeader(409)
				_, _ = w.Write([]byte(`{"error":{"code":"session_limit_reached","message":"limit"}}`))
				return
			}
			if logins > 0 && (cookie == nil || cookie.Value != token) {
				t.Error("re-login dropped current cookie")
			}
			logins++
			token = strings.Repeat("t", logins+5)
			http.SetCookie(w, &http.Cookie{Name: "gocms_user", Value: token, Path: "/", HttpOnly: true})
			_, _ = w.Write([]byte(`{"data":{"user":{"id":1,"username":"alice"}}}`))
		case "/api/v1/auth/logout":
			token = ""
			http.SetCookie(w, &http.Cookie{Name: "gocms_user", Value: "", Path: "/", MaxAge: -1})
			_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
		default:
			if unauthorized || cookie == nil || cookie.Value != token {
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
		t.Fatal("persisted cookie not restored")
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
	outage = false
	if m.Refresh(ctx).Eligible {
		t.Fatal("failed logout silently restored session")
	}
	if err = m.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if store.token != "" {
		t.Fatal("logout failed to clear cookie")
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
