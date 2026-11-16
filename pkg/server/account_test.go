package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"takealot/pkg/account"
	"takealot/pkg/api"
	"takealot/pkg/db"
	"takealot/pkg/engine"
)

func TestAccountRefreshOutageWaitsAndRecoveryDoesNotRestartStoppedTasks(t *testing.T) {
	var failure atomic.Int32
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failure.Load() != 0 {
			w.WriteHeader(int(failure.Load()))
			_, _ = w.Write([]byte(`{"error":{"code":"test_failure"}}`))
			return
		}
		body := `{"data":{"id":1,"username":"alice"}}`
		switch r.URL.Path {
		case "/api/v1/auth/token":
			body = `{"data":{"user":{"id":1,"username":"alice"},"access_token":"test-token","token_type":"Bearer","expires_in":3600}}`
		case "/api/v1/me/memberships":
			body = `{"data":[{"slug":"vip","status":"active"}]}`
		}
		_, _ = w.Write([]byte(body))
	}))
	defer cms.Close()
	mgr, err := account.New(cms.URL, "vip")
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.New(filepath.Join(t.TempDir(), "account.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store, err := database.EnsureDefaultStore("fake-key", 1, 1, 5, 1, 1000, 120)
	if err != nil {
		t.Fatal(err)
	}
	pool := api.NewClientPool()
	eng := engine.NewEngine(nil, pool, database, mgr)
	defer eng.StopAll()
	srv := NewServer(nil, pool, eng, database, mgr, nil, nil, nil, "test")
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:8000"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Store-Id", store.ID)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: got %d, want %d: %s", path, w.Code, want, w.Body.String())
		}
		return w
	}
	check := func(running, pending bool) {
		t.Helper()
		w := call("GET", "/api/reprice/status", "", 200)
		var status engine.Status
		if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || status.IsRunning != running || status.IsPaused != pending || status.MembershipVerificationRequired != pending {
			t.Fatalf("unexpected task status: %s (%v)", w.Body.String(), err)
		}
	}
	credentials := `{"identifier":"alice","password":"password123"}`
	call("POST", "/api/account/login", credentials, 200)
	call("POST", "/api/reprice/start", "{}", 200)
	check(true, false)
	failure.Store(503)
	call("POST", "/api/account/refresh", "{}", 200)
	check(true, true)
	call("GET", "/api/stores", "", 403) // Business APIs still fail closed.
	failure.Store(0)
	call("POST", "/api/account/refresh", "{}", 200)
	check(true, false)
	failure.Store(503)
	call("POST", "/api/account/refresh", "{}", 200)
	call("POST", "/api/reprice/stop", "{}", 200)
	failure.Store(0)
	call("POST", "/api/account/refresh", "{}", 200)
	check(false, false)
	call("POST", "/api/reprice/start", "{}", 200)
	failure.Store(401)
	call("POST", "/api/account/refresh", "{}", 200)
	check(false, false)
	failure.Store(0)
	call("POST", "/api/account/login", credentials, 200)
	check(false, false)
}

func TestLocalAccountAPIKeepsBearerCredentialInBackend(t *testing.T) {
	const token = "server-only-access-token"
	var active atomic.Bool
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("local account flow sent a Cookie to GoCMS")
		}
		switch r.URL.Path {
		case "/api/v1/auth/token":
			active.Store(true)
			_, _ = w.Write([]byte(`{"data":{"user":{"id":1,"username":"alice"},"access_token":"` + token + `","token_type":"Bearer","expires_in":3600}}`))
		case "/api/v1/auth/register":
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"data":{"user":{"id":1,"username":"alice"}}}`))
		default:
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Error("protected member request lost its Bearer token")
			}
			if r.URL.Path == "/api/v1/auth/logout" {
				active.Store(false)
				_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
				return
			}
			if !active.Load() {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"error":{"code":"authentication_required"}}`))
				return
			}
			if r.URL.Path == "/api/v1/me" {
				_, _ = w.Write([]byte(`{"data":{"id":1,"username":"alice"}}`))
			} else if r.URL.Path == "/api/v1/me/memberships" {
				_, _ = w.Write([]byte(`{"data":[{"slug":"vip","status":"active"}]}`))
			} else {
				t.Errorf("unexpected GoCMS path: %s", r.URL.Path)
				w.WriteHeader(404)
			}
		}
	}))
	defer cms.Close()
	mgr, err := account.NewWithStore(cms.URL, "vip", nil)
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.NewEngine(nil, nil, nil, mgr)
	srv := NewServer(nil, nil, eng, nil, mgr, nil, nil, nil, "test")
	// Exercise the real membership middleware without touching seller operations.
	srv.mux.HandleFunc("/api/test-member-only", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:8000"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: got %d, want %d: %s", path, w.Code, want, w.Body.String())
		}
		if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "access_token") || w.Header().Get("Set-Cookie") != "" {
			t.Fatal("GoCMS credential escaped to the frontend")
		}
		if strings.HasPrefix(path, "/api/account/") && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("account response is cacheable")
		}
		return w
	}
	checkStatus := func(method, path, body string, wantEligible bool) {
		t.Helper()
		w := call(method, path, body, 200)
		var status account.Status
		if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || status.Eligible != wantEligible || status.Authenticated != wantEligible {
			t.Fatalf("incorrect account status: %s %v", w.Body.String(), err)
		}
	}
	credentials := `{"identifier":"alice","password":"password123"}`
	checkStatus("GET", "/api/account/status", "", false)
	call("GET", "/api/test-member-only", "", 403)
	call("POST", "/api/account/register", `{"username":"alice","password":"password123"}`, 201)
	checkStatus("POST", "/api/account/login", credentials, true)
	checkStatus("GET", "/api/account/status", "", true)
	checkStatus("POST", "/api/account/refresh", "{}", true)
	call("GET", "/api/test-member-only", "", 204)
	active.Store(false) // Simulate revocation in the online GoCMS administration.
	checkStatus("POST", "/api/account/refresh", "{}", false)
	call("GET", "/api/test-member-only", "", 403)
	checkStatus("POST", "/api/account/login", credentials, true)
	checkStatus("POST", "/api/account/logout", "{}", false)
	checkStatus("POST", "/api/account/refresh", "{}", false)
	call("GET", "/api/test-member-only", "", 403)
	if active.Load() {
		t.Fatal("logout did not revoke remote session")
	}
}

func TestAccountGateAndLocalOrigins(t *testing.T) {
	mgr, err := account.New("", "takealot-vip")
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.NewEngine(nil, nil, nil, mgr)
	srv := NewServer(nil, nil, eng, nil, mgr, nil, nil, nil, "test")
	for _, tc := range []struct {
		method, path, origin string
		want                 int
	}{
		{"GET", "/api/account/status", "", 200},
		{"POST", "/api/account/login", "", 503},
		{"POST", "/api/account/change-password", "", 503},
		{"GET", "/api/stores", "", 403},
		{"POST", "/api/account/login", "https://evil.example", 403},
		{"POST", "/api/account/login", "http://127.0.0.1:8000", 503},
		{"POST", "/api/account/login", "wails://wails.localhost", 503},
		{"POST", "/api/account/login", "wails://wails", 503},
	} {
		r := httptest.NewRequest(tc.method, "http://127.0.0.1:8000"+tc.path, strings.NewReader(`{"identifier":"alice","password":"secret"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if w.Header().Get("Access-Control-Allow-Origin") == "*" {
			t.Fatal("wildcard CORS")
		}
	}
	if ok, _ := eng.StartReprice("store"); ok {
		t.Fatal("task started without membership")
	}
	if ok, _ := eng.RunFollowBatch("store", nil); ok {
		t.Fatal("follow started without membership")
	}
	r := httptest.NewRequest(http.MethodGet, "http://evil.example/api/account/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("DNS rebinding host allowed")
	}

	// Test macOS Wails host & origin
	rMac := httptest.NewRequest(http.MethodGet, "/api/account/status", nil)
	rMac.Host = "wails"
	rMac.Header.Set("Origin", "wails://wails")
	wMac := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wMac, rMac)
	if wMac.Code != 200 {
		t.Fatalf("macOS Wails host rejected: %d %s", wMac.Code, wMac.Body.String())
	}

	// Test Windows Wails host & origin
	rWin := httptest.NewRequest(http.MethodGet, "/api/account/status", nil)
	rWin.Host = "wails.localhost"
	rWin.Header.Set("Origin", "http://wails.localhost")
	wWin := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wWin, rWin)
	if wWin.Code != 200 {
		t.Fatalf("Windows Wails host rejected: %d %s", wWin.Code, wWin.Body.String())
	}
}
