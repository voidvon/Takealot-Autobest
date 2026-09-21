package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"takealot/pkg/account"
	"takealot/pkg/engine"
	"testing"
)

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
		{"GET", "/api/stores", "", 403},
		{"POST", "/api/account/login", "https://evil.example", 403},
		{"POST", "/api/account/login", "http://127.0.0.1:8000", 503},
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
}
