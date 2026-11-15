package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSavedCookieSessionMigratesToBearerWithoutLogin(t *testing.T) {
	store := &memoryStore{token: "legacy-session"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer legacy-session" || r.Header.Get("Cookie") != "" {
			t.Error("saved session was not migrated to Bearer")
		}
		// Neither a replacement nor a deletion cookie may mutate native state.
		http.SetCookie(w, &http.Cookie{Name: "gocms_user", Value: "wrong-session"})
		http.SetCookie(w, &http.Cookie{Name: "gocms_user", MaxAge: -1})
		switch r.URL.Path {
		case "/api/v1/me":
			_, _ = w.Write([]byte(`{"data":{"id":1,"username":"alice"}}`))
		case "/api/v1/me/memberships":
			_, _ = w.Write([]byte(`{"data":[{"slug":"vip","status":"active"}]}`))
		default:
			t.Error("migration must not create or rotate a session")
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	m, err := NewWithStore(srv.URL, "vip", store)
	if err != nil {
		t.Fatal(err)
	}
	status := m.Refresh(context.Background())
	if !status.Eligible || !status.Persistent || store.token != "legacy-session" || m.token != store.token {
		t.Fatalf("migration failed: %+v", status)
	}
}

func TestRefreshSessionRejectionAndServiceFailure(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		status           int
		revoked          bool
	}{
		{"profile revoked", "/api/v1/me", `{"error":{"code":"authentication_required"}}`, 401, true},
		{"memberships revoked", "/api/v1/me/memberships", `{"error":{"code":"authentication_required"}}`, 401, true},
		{"non-JSON rejection", "/api/v1/me/memberships", "<html>Unauthorized</html>", 401, true},
		{"profile unavailable", "/api/v1/me", "<html>Unavailable</html>", 503, false},
		{"memberships unavailable", "/api/v1/me/memberships", `{"error":{"code":"internal_error"}}`, 500, false},
		{"invalid profile", "/api/v1/me", `{"data":{}}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tc.path {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				_, _ = w.Write([]byte(`{"data":{"id":1,"username":"alice"}}`))
			}))
			defer srv.Close()
			store := &memoryStore{token: "existing-session"}
			m, err := NewWithStore(srv.URL, "vip", store)
			if err != nil {
				t.Fatal(err)
			}
			status := m.Refresh(context.Background())
			if status.Eligible {
				t.Fatal("failed verification allowed business operations")
			}
			if tc.revoked {
				if status.Authenticated || status.Persistent || status.User != nil || status.State != "signed_out" || store.token != "" || m.token != "" {
					t.Fatalf("rejected session was retained: %+v", status)
				}
			} else if status.State != "unavailable" || !status.Persistent || store.token == "" || m.token == "" {
				t.Fatalf("service failure discarded recoverable session: %+v", status)
			}
		})
	}
}

func TestLoginRejectsInvalidGrantsWithoutReplacingCredential(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"Cookie only", `{"data":{"user":{"id":1,"username":"alice"}}}`},
		{"wrong scheme", `{"data":{"user":{"id":1,"username":"alice"},"access_token":"new-token","token_type":"Basic","expires_in":3600}}`},
		{"expired", `{"data":{"user":{"id":1,"username":"alice"},"access_token":"new-token","token_type":"Bearer","expires_in":0}}`},
		{"header injection", `{"data":{"user":{"id":1,"username":"alice"},"access_token":"bad\r\nX-Header:value","token_type":"Bearer","expires_in":3600}}`},
		{"missing user", `{"data":{"access_token":"new-token","token_type":"Bearer","expires_in":3600}}`},
		{"null grant", `{"data":null}`},
		{"malformed", `{"data":`},
		{"trailing JSON", `{"data":{}} {"data":{}}`},
		{"oversized", strings.Repeat(" ", 1<<20) + `{"data":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/api/v1/auth/token" || r.Header.Get("Authorization") != "Bearer previous-token" || r.Header.Get("Cookie") != "" {
					t.Error("unexpected token login request")
				}
				http.SetCookie(w, &http.Cookie{Name: "gocms_user", Value: "cookie-must-be-ignored"})
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			store := &memoryStore{token: "previous-token"}
			m, err := NewWithStore(srv.URL, "vip", store)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.Login(context.Background(), "alice", "password123"); err == nil {
				t.Fatal("invalid grant accepted")
			}
			if requests.Load() != 1 || m.token != "previous-token" || store.token != "previous-token" || m.GetStatus().Authenticated {
				t.Fatal("invalid grant replaced credential or continued verification")
			}
		})
	}
}

type failingSaveStore struct{ memoryStore }

func (s *failingSaveStore) Save(string) error { return errors.New("credential store unavailable") }

func TestFailedPersistenceKeepsOnlyCurrentInMemoryToken(t *testing.T) {
	store := &failingSaveStore{memoryStore{token: "rotated-out-token"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/token":
			if r.Header.Get("Authorization") != "Bearer rotated-out-token" {
				t.Error("login failed to rotate persisted token")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": tokenGrant{
				User: User{ID: 1, Username: "alice"}, AccessToken: "current-token", TokenType: "Bearer", ExpiresIn: 3600,
			}})
		case "/api/v1/me":
			if r.Header.Get("Authorization") != "Bearer current-token" {
				t.Error("new in-memory credential not used")
			}
			_, _ = w.Write([]byte(`{"data":{"id":1,"username":"alice"}}`))
		case "/api/v1/me/memberships":
			_, _ = w.Write([]byte(`{"data":[{"slug":"vip","status":"active"}]}`))
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	m, err := NewWithStore(srv.URL, "vip", store)
	if err != nil {
		t.Fatal(err)
	}
	status, err := m.Login(context.Background(), "alice", "password123")
	if err != nil || !status.Eligible || status.Persistent || store.token != "" || m.token != "current-token" {
		t.Fatalf("incorrect memory-only session: %+v %v", status, err)
	}
}

func TestTokenLoginDoesNotFollowRedirect(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	store := &memoryStore{token: "existing-session"}
	m, err := NewWithStore(srv.URL, "vip", store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Login(context.Background(), "alice", "password123"); err == nil {
		t.Fatal("login redirect accepted")
	}
	if leaked.Load() != 0 || store.token != "existing-session" {
		t.Fatal("redirect leaked credentials or changed stored session")
	}
}

func TestChangePasswordClearsRejectedSession(t *testing.T) {
	for _, body := range []string{`{"error":{"code":"authentication_required"}}`, "Unauthorized"} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/api/v1/me/password" || r.Header.Get("Authorization") != "Bearer expired-token" {
					t.Error("unexpected password request")
				}
				w.WriteHeader(401)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			store := &memoryStore{token: "expired-token"}
			m, err := NewWithStore(srv.URL, "vip", store)
			if err != nil {
				t.Fatal(err)
			}
			m.status.Authenticated, m.status.Eligible = true, true
			if err := m.ChangePassword(context.Background(), "oldpassword", "newpassword"); err == nil {
				t.Fatal("revoked session changed password")
			}
			status := m.GetStatus()
			if status.Authenticated || status.Eligible || status.Persistent || store.token != "" || m.token != "" {
				t.Fatalf("password flow retained rejected session: %+v", status)
			}
		})
	}
}

func TestEmptyOrMalformedSavedSessionIsNotLoaded(t *testing.T) {
	for _, token := range []string{"", "token with spaces", "token\r\nInjected:value"} {
		m, err := NewWithStore("https://member.example", "vip", &memoryStore{token: token})
		if err != nil {
			t.Fatal(err)
		}
		if m.token != "" || m.GetStatus().Persistent {
			t.Fatal("empty or malformed session was restored")
		}
	}
}
