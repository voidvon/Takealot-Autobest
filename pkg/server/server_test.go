package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

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
	srv := NewServer(cfgMgr, clientPool, eng, database, nil, nil, nil, nil, "0.2.0")

	// 1. Test GET /api/stores
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8000/api/stores", nil)
	w := httptest.NewRecorder()
	req.Host = "127.0.0.1:8000"
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
	req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/stores", bytes.NewReader(body))
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
	reqT1 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/targets", bytes.NewReader(bodyTarget1))
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
	reqT2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/targets", bytes.NewReader(bodyTarget2))
	reqT2.Header.Set("X-Store-Id", "store_branch_2")
	wT2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wT2, reqT2)
	if wT2.Code != http.StatusOK {
		t.Fatalf("POST /api/targets store_branch_2 失败: %d", wT2.Code)
	}

	// Fetch targets for default
	reqG1 := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8000/api/targets", nil)
	reqG1.Header.Set("X-Store-Id", "default")
	wG1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wG1, reqG1)
	var loadedT1 map[string]config.Target
	_ = json.NewDecoder(wG1.Body).Decode(&loadedT1)
	if loadedT1["tsin1/plid1"].MinPrice != 100 {
		t.Errorf("店铺 1 底价不符合预期: %+v", loadedT1)
	}

	// Fetch targets for store_branch_2
	reqG2 := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8000/api/targets", nil)
	reqG2.Header.Set("X-Store-Id", "store_branch_2")
	wG2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wG2, reqG2)
	var loadedT2 map[string]config.Target
	_ = json.NewDecoder(wG2.Body).Decode(&loadedT2)
	if loadedT2["tsin1/plid1"].MinPrice != 180 {
		t.Errorf("店铺 2 底价不符合预期: %+v", loadedT2)
	}

	// 4. Test DELETE /api/stores
	reqDel := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:8000/api/stores?id=store_branch_2", nil)
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

func TestFollowTemplateAndUpload(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "takealot_follow_test_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "follow_test.db")
	database, _ := db.New(dbPath)
	defer database.Close()

	cfgMgr := config.NewManager(tempDir)
	clientPool := api.NewClientPool()
	eng := engine.NewEngine(cfgMgr, clientPool, database, nil)
	srv := NewServer(cfgMgr, clientPool, eng, database, nil, nil, nil, nil, "0.2.0")

	// 1. Test GET /api/follow/template
	reqTpl := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8000/api/follow/template", nil)
	wTpl := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wTpl, reqTpl)

	if wTpl.Code != http.StatusOK {
		t.Fatalf("GET /api/follow/template 失败: %d", wTpl.Code)
	}
	tplBytes := wTpl.Body.Bytes()
	if len(tplBytes) == 0 {
		t.Fatalf("下载的模板内容为空")
	}

	// 2. Test POST /api/follow/upload using the downloaded template bytes
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "test_template.xlsx")
	if err != nil {
		t.Fatalf("创建 FormFile 失败: %v", err)
	}
	if _, err := part.Write(tplBytes); err != nil {
		t.Fatalf("写入模板字节失败: %v", err)
	}
	mw.Close()

	reqUp := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/follow/upload", &body)
	reqUp.Header.Set("Content-Type", mw.FormDataContentType())
	wUp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wUp, reqUp)

	if wUp.Code != http.StatusOK {
		t.Fatalf("POST /api/follow/upload 失败: %d, body: %s", wUp.Code, wUp.Body.String())
	}

	var upRes struct {
		Success bool                `json:"success"`
		Total   int                 `json:"total"`
		Items   []engine.FollowItem `json:"items"`
	}
	if err := json.NewDecoder(wUp.Body).Decode(&upRes); err != nil {
		t.Fatalf("解析 upload 响应失败: %v", err)
	}

	if !upRes.Success || upRes.Total != 1 || len(upRes.Items) != 1 {
		t.Fatalf("上传解析结果不符合预期: %+v", upRes)
	}
	if upRes.Items[0].Stock != 10 || upRes.Items[0].MinPrice != 150 {
		t.Errorf("解析的库存或保底价不匹配: %+v", upRes.Items[0])
	}
}

func TestFollowUploadJSON(t *testing.T) {
	workbook := excelize.NewFile()
	defer workbook.Close()
	sheet := workbook.GetSheetName(0)
	rows := [][]any{
		{"商品链接", "最低价", "库存"},
		{"https://www.takealot.com/测试商品/PLID123", 150, 10},
		{"https://www.takealot.com/another-product/PLID456", 275, 3},
	}
	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := workbook.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	file, err := workbook.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	// Marshaling []byte produces the same Base64 field as FileReader in the frontend.
	payload, err := json.Marshal(map[string]any{"file_data": file.Bytes(), "file_size": file.Len()})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(nil, nil, nil, nil, nil, nil, nil, nil, "test")
	for _, host := range []string{"127.0.0.1:8000", "wails.localhost"} {
		t.Run(host, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://"+host+"/api/follow/upload", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
			req.Header.Set("Origin", "http://"+host)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("upload returned %d: %s", w.Code, w.Body.String())
			}
			var result struct {
				Success bool                `json:"success"`
				Total   int                 `json:"total"`
				Items   []engine.FollowItem `json:"items"`
			}
			if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if !result.Success || result.Total != 2 || len(result.Items) != 2 {
				t.Fatalf("unexpected upload result: %+v", result)
			}
			for i, item := range result.Items {
				if item.URL != rows[i+1][0] || item.MinPrice != rows[i+1][1] || item.Stock != rows[i+1][2] {
					t.Errorf("row %d changed during upload: %+v", i+1, item)
				}
			}
		})
	}
}

func TestManualMinPriceUpdateUsesOwningStore(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "manual_prices.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.EnsureDefaultStore("", 1, 1, 5, 1, 1000, 120); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveStore(db.Store{ID: "branch", Name: "Branch"}); err != nil {
		t.Fatal(err)
	}
	for _, storeID := range []string{"default", "branch"} {
		if err := database.SaveStoreTargets(storeID, map[string]config.Target{
			"12345/67890": {Selected: true, MinPrice: 100, MaxPrice: 900},
		}); err != nil {
			t.Fatal(err)
		}
		if err := database.SaveStoreCachedOffers(storeID, []db.CachedOffer{{Key: "12345/67890", TSINID: "12345"}}); err != nil {
			t.Fatal(err)
		}
	}
	srv := NewServer(nil, api.NewClientPool(), nil, database, nil, nil, nil, nil, "test")
	for _, price := range []int{225, 0} {
		payload, _ := json.Marshal(map[string]any{"tsin_id": "12345", "store_id": "branch", "min_price": price})
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/official/offers/update", bytes.NewReader(payload))
		req.Header.Set("X-Store-Id", "all")
		w := httptest.NewRecorder()
		// Success without seller credentials proves a min-only edit makes no upstream request.
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("min-only update failed: %d %s", w.Code, w.Body.String())
		}
		for _, storeID := range []string{"default", "branch"} {
			targets, err := database.LoadStoreTargets(storeID)
			if err != nil {
				t.Fatal(err)
			}
			wantMin := 100
			if storeID == "branch" {
				wantMin = price
			}
			target := targets["12345/67890"]
			if target.MinPrice != wantMin || !target.Selected || target.MaxPrice != 900 {
				t.Fatalf("unexpected target for %s: %+v", storeID, target)
			}
		}
	}
	for _, storeID := range []string{"missing-store", "all"} {
		payload, _ := json.Marshal(map[string]any{"tsin_id": "12345", "store_id": storeID, "min_price": 500})
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/official/offers/update", bytes.NewReader(payload))
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid store %s was accepted: %d %s", storeID, w.Code, w.Body.String())
		}
	}
}

func TestFollowUploadInvalidJSON(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil, nil, nil, nil, nil, "test")
	for _, tc := range []struct {
		name    string
		payload string
		message string
	}{
		{"malformed JSON", `{`, "上传文件数据无效"},
		{"invalid Base64", `{"file_data":"%%%","file_size":3}`, "上传文件数据无效"},
		{"empty file", `{"file_data":"","file_size":0}`, "上传文件为空"},
		{"incomplete file", `{"file_data":"UEsDBA==","file_size":100}`, "上传文件内容不完整"},
		{"invalid XLSX", `{"file_data":"UEsDBA==","file_size":4}`, "Excel 打开失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/follow/upload", strings.NewReader(tc.payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.handleFollowUpload(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			var result struct {
				Success bool   `json:"success"`
				Error   string `json:"error"`
			}
			if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result.Success || !strings.Contains(result.Error, tc.message) {
				t.Fatalf("unexpected error response: %+v", result)
			}
		})
	}
}
