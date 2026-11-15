package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"takealot/pkg/workspace"
	"testing"
)

func TestPortablePreferencesRemainLocalAndPersistWithoutMembership(t *testing.T) {
	root := t.TempDir()
	preferences, err := workspace.NewPreferencesStore(root)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(nil, nil, nil, nil, nil, nil, nil, nil, "test")
	srv.SetPreferences(preferences)
	call := func(method, body, origin string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://127.0.0.1:8123/api/preferences", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("got %d, want %d: %s", rec.Code, want, rec.Body.String())
		}
		return rec
	}
	call(http.MethodPut, `{"theme":"dark","active_store_id":"copy-a"}`, "http://127.0.0.1:8123", 200)
	call(http.MethodPut, `{"theme":"light"}`, "https://unrelated.example", 403)
	call(http.MethodPut, `{"access_token":"not-an-allowed-field"}`, "", 400)
	call(http.MethodPut, `{"theme":"light"} {}`, "", 400)
	got := call(http.MethodGet, "", "", 200)
	if got.Header().Get("Cache-Control") != "no-store" || !strings.Contains(got.Body.String(), `"theme":"dark"`) {
		t.Fatal(got.Body.String())
	}
	reopened, err := workspace.NewPreferencesStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Get().ActiveStoreID != "copy-a" || reopened.Get().Theme != "dark" {
		t.Fatal("preferences did not survive restart")
	}
}
