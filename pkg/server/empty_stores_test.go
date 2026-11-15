package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
	"takealot/pkg/engine"
)

func TestEmptyInstallationRequiresExplicitStoreCreation(t *testing.T) {
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	pool := api.NewClientPool()
	calls := 0
	pool.SetRequestTransport(func(store, _ string, r *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		calls++
		if store != "own-shop" || r.Method != "GET" || r.URL.Path != "/v2/seller" {
			t.Errorf("unexpected upstream access: %s %s store=%s", r.Method, r.URL, store)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"seller_id":123,"display_name":"My shop"}`))}, nil
	})
	cfg := config.NewManager(dir)
	// Ambient legacy credentials must never become a new store via an API read.
	legacy := cfg.Get()
	legacy.Authorization = "Key unrelated-legacy-key"
	if err := cfg.Update(legacy); err != nil {
		t.Fatal(err)
	}
	eng := engine.NewEngine(cfg, pool, database, nil)
	srv := NewServer(cfg, pool, eng, database, nil, nil, nil, nil, "test")
	request := func(method, path, store, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:8000"+path, strings.NewReader(body))
		if store != "" {
			r.Header.Set("X-Store-Id", store)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s store=%s: %d %s", method, path, store, w.Code, w.Body.String())
		}
		return w
	}
	assertStoreCount := func(count int) {
		t.Helper()
		var result struct {
			Stores []db.Store `json:"stores"`
			Active string     `json:"active_store_id"`
		}
		if err := json.Unmarshal(request("GET", "/api/stores", "", "", 200).Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Stores) != count || (count == 0 && result.Active != "") {
			t.Fatalf("unexpected stores/active identity: %+v", result)
		}
	}
	assertStoreCount(0)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/reprice/status", "", 200},
		{"GET", "/api/config", "", 400},
		{"POST", "/api/config", `{"authorization":"Key unrelated-legacy-key"}`, 400},
		{"GET", "/api/offers", "", 400},
		{"GET", "/api/offers?sync=true", "", 400},
		{"GET", "/api/targets", "", 400},
		{"POST", "/api/targets", `{"111/500":{"selected":true,"min_price":210}}`, 400},
		{"POST", "/api/reprice/start", "", 400},
		{"POST", "/api/reprice/start_all", "", 400},
		{"POST", "/api/reprice/pause", "", 400},
		{"POST", "/api/reprice/stop", "", 400},
		{"GET", "/api/reprice/history", "", 400},
		{"GET", "/api/follow/history", "", 400},
		{"GET", "/api/fulfillment/shipments", "", 400},
		{"GET", "/api/fulfillment/bookings", "", 400},
		{"POST", "/api/fulfillment/shipments/create", `{}`, 400},
		{"POST", "/api/fulfillment/bookings/create", `{}`, 400},
		{"POST", "/api/stores", `{"id":"default","name":"Default"}`, 400},
		{"PUT", "/api/stores", `{"id":"default","name":"Default"}`, 400},
	} {
		request(tc.method, tc.path, "", tc.body, tc.status)
		assertStoreCount(0)
	}
	if ok, _ := eng.StartReprice("default"); ok {
		t.Fatal("engine accepted a nonexistent store")
	}
	if calls != 0 {
		t.Fatalf("empty installation made %d upstream calls", calls)
	}
	request("POST", "/api/reprice/start", "all", "", 400)
	request("POST", "/api/stores", "", `{"id":"own-shop","authorization":"Key own-test-key","is_active":true}`, 200)
	assertStoreCount(1)
	if calls != 1 {
		t.Fatalf("explicit store add did not use its own credentials: calls=%d", calls)
	}
	for _, path := range []string{"/api/offers", "/api/config", "/api/targets"} {
		request("GET", path, "deleted-or-foreign-store", "", 400)
	}
	request("POST", "/api/reprice/start", "deleted-or-foreign-store", "", 400)
	if calls != 1 {
		t.Fatal("unknown store silently accessed another shop")
	}
	for _, path := range []string{"/api/fulfillment/shipments?status=draft", "/api/fulfillment/bookings"} {
		w := request("GET", path, "own-shop", "", 200)
		var rows []any
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || len(rows) != 0 {
			t.Fatalf("read generated demo records: %s %s", path, w.Body.String())
		}
	}
	request("PUT", "/api/stores", "", `{"id":"own-shop","is_active":false}`, 200)
	store, err := database.GetStore("own-shop")
	if err != nil || store.Name != "My shop" || store.Authorization != "Key own-test-key" || store.IsActive {
		t.Fatalf("partial update lost store identity: %+v %v", store, err)
	}
	request("DELETE", "/api/stores?id=own-shop", "", "", 200)
	request("GET", "/api/reprice/status", "", "", 200)
	assertStoreCount(0)
	targets, err := database.LoadStoreTargets("all")
	if err != nil || len(targets) != 0 {
		t.Fatalf("empty installation acquired monitors: %+v %v", targets, err)
	}
}
