package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
	"takealot/pkg/engine"
)

func TestMultiStoreServerAPI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "takealot_server_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "server_test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer database.Close()

	// Initialize default store
	defStore, err := database.EnsureDefaultStore("Key test_token_1", 1, 1, 5, 1, 1000, 120.0)
	if err != nil {
		t.Fatalf("EnsureDefaultStore 失败: %v", err)
	}

	cfgMgr := config.NewManager(tempDir)
	clientPool := api.NewClientPool()
	clientPool.GetOrCreate(defStore.ID, defStore.Authorization, defStore.ProxyURL)

	eng := engine.NewEngine(cfgMgr, clientPool, database, nil)
	srv := NewServer(cfgMgr, clientPool, eng, database, nil, nil, nil, "0.2.0")

	// 1. Test GET /api/stores
	req := httptest.NewRequest(http.MethodGet, "/api/stores", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/stores 返回状态码异常: %d, body: %s", w.Code, w.Body.String())
	}
	var storesResp struct {
		Success bool       `json:"success"`
		Stores  []db.Store `json:"stores"`
	}
	if err := json.NewDecoder(w.Body).Decode(&storesResp); err != nil || len(storesResp.Stores) != 1 {
		t.Fatalf("GET /api/stores 数据异常: %+v", storesResp)
	}

	// 2. Test POST /api/stores (Create Store 2)
	newStorePayload := db.Store{
		ID:                "store_branch_2",
		Name:              "南非2号直营店",
		Authorization:     "Key test_token_branch_2",
		IsActive:          true,
		PriceDecreaseStep: 2,
		PriceIncreaseStep: 3,
		RRPPercentage:     125.0,
		IntervalMinutes:   10,
		ProxyURL:          "http://127.0.0.1:8888",
	}
	body, _ := json.Marshal(newStorePayload)
	req2 := httptest.NewRequest(http.MethodPost, "/api/stores", bytes.NewReader(body))
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("POST /api/stores 创建店铺失败: %d, body: %s", w2.Code, w2.Body.String())
	}

	// Verify store 2 was added
	storesAfter, _ := database.GetStores()
	if len(storesAfter) != 2 {
		t.Fatalf("期望 2 个店铺，实际: %d", len(storesAfter))
	}

	// 3. Test Target isolation between stores via /api/targets
	targetMap1 := map[string]config.Target{
		"tsin1/plid1": {Selected: true, MinPrice: 100, MaxPrice: 200},
	}
	bodyTarget1, _ := json.Marshal(targetMap1)
	reqT1 := httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewReader(bodyTarget1))
	reqT1.Header.Set("X-Store-Id", "default")
	wT1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wT1, reqT1)
	if wT1.Code != http.StatusOK {
		t.Fatalf("POST /api/targets default 失败: %d", wT1.Code)
	}

	targetMap2 := map[string]config.Target{
		"tsin1/plid1": {Selected: true, MinPrice: 180, MaxPrice: 350},
	}
	bodyTarget2, _ := json.Marshal(targetMap2)
	reqT2 := httptest.NewRequest(http.MethodPost, "/api/targets", bytes.NewReader(bodyTarget2))
	reqT2.Header.Set("X-Store-Id", "store_branch_2")
	wT2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wT2, reqT2)
	if wT2.Code != http.StatusOK {
		t.Fatalf("POST /api/targets store_branch_2 失败: %d", wT2.Code)
	}

	// Fetch targets for default
	reqG1 := httptest.NewRequest(http.MethodGet, "/api/targets", nil)
	reqG1.Header.Set("X-Store-Id", "default")
	wG1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wG1, reqG1)
	var loadedT1 map[string]config.Target
	_ = json.NewDecoder(wG1.Body).Decode(&loadedT1)
	if loadedT1["tsin1/plid1"].MinPrice != 100 {
		t.Errorf("店铺 1 底价不符合预期: %+v", loadedT1)
	}

	// Fetch targets for store_branch_2
	reqG2 := httptest.NewRequest(http.MethodGet, "/api/targets", nil)
	reqG2.Header.Set("X-Store-Id", "store_branch_2")
	wG2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wG2, reqG2)
	var loadedT2 map[string]config.Target
	_ = json.NewDecoder(wG2.Body).Decode(&loadedT2)
	if loadedT2["tsin1/plid1"].MinPrice != 180 {
		t.Errorf("店铺 2 底价不符合预期: %+v", loadedT2)
	}

	// 4. Test DELETE /api/stores
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/stores?id=store_branch_2", nil)
	wDel := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("DELETE /api/stores 失败: %d, body: %s", wDel.Code, wDel.Body.String())
	}

	storesFinal, _ := database.GetStores()
	if len(storesFinal) != 1 {
		t.Fatalf("删除后应剩下 1 个店铺，实际: %d", len(storesFinal))
	}
}
