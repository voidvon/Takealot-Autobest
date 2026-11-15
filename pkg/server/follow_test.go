package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
	"takealot/pkg/engine"
)

func TestFollowUploadReadsIndependentVariantColumns(t *testing.T) {
	book := excelize.NewFile()
	defer book.Close()
	rows := [][]any{
		{"初始售价", "封顶价", "TSIN", "商品链接", "最低价", "库存"},
		{100, 150, "111", "PLID500", 80, 1},
		{"", 300, "222", "PLID500", 210, 3},
	}
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := book.SetSheetRow(book.GetSheetName(0), cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	file, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"file_data": file.Bytes(), "file_size": file.Len()})
	srv := NewServer(nil, nil, nil, nil, nil, nil, nil, nil, "test")
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/follow/upload", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Items []engine.FollowItem `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	want := []engine.FollowItem{
		{URL: "PLID500", TSINID: "111", MinPrice: 80, MaxPrice: 150, SellingPrice: 100, Stock: 1},
		{URL: "PLID500", TSINID: "222", MinPrice: 210, MaxPrice: 300, Stock: 3},
	}
	if len(response.Items) != len(want) {
		t.Fatalf("unexpected items: %+v", response.Items)
	}
	for i := range want {
		if response.Items[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, response.Items[i], want[i])
		}
	}
}

func TestFollowPreviewAPIAndSubmissionValidation(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.EnsureDefaultStore("test", 1, 1, 5, 1, 1000, 120); err != nil {
		t.Fatal(err)
	}
	pool := api.NewClientPool()
	calls := 0
	pool.SetRequestTransport(func(_, _ string, req *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		calls++
		if req.Method != http.MethodGet {
			t.Fatalf("preview or invalid submission wrote upstream: %s", req.Method)
		}
		body := `[{"productlineId":500,"title":"Strap","variants":[{"tsinId":111,"size":"1pc"}]}]`
		if req.URL.Path == "/1/catalogue/mpv/search" {
			body = `{"results":[{"tsinId":111,"productlineId":500,"bestPrice":120,"hasOffer":false,"gtin":"gtin-111"}]}`
		} else if req.URL.Path != "/1/catalogue/mpv/PLID500" {
			t.Fatalf("unexpected request: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	eng := engine.NewEngine(nil, pool, database, nil)
	srv := NewServer(config.NewManager(t.TempDir()), pool, eng, database, nil, nil, nil, nil, "test")
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/follow/preview", strings.NewReader(`{"items":[{"url":"PLID500","stock":1,"min_price":80}]}`))
	req.Header.Set("X-Store-Id", "default")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var response engine.FollowPreview
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Variants) != 1 || response.Variants[0].TSINID != "111" || response.Variants[0].Size != "1pc" || calls != 2 {
		t.Fatalf("preview: %+v, requests=%d", response, calls)
	}
	for _, tc := range []struct{ path, store, body string }{
		{"preview", "all", `{"items":[{"url":"PLID500"}]}`},
		{"preview", "missing-store", `{"items":[{"url":"PLID500"}]}`},
		{"start", "all", `{"items":[{"url":"PLID500"}]}`},
		{"start", "missing-store", `{"items":[{"url":"PLID500"}]}`},
		{"start", "default", `{"items":[{"url":"PLID500","stock":1,"min_price":80}]}`},
		{"start", "default", `{"items":[{"url":"PLID500","tsin_id":"111","stock":1,"min_price":210,"selling_price":120}]}`},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/follow/"+tc.path, strings.NewReader(tc.body))
		req.Header.Set("X-Store-Id", tc.store)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest || calls != 2 {
			t.Errorf("invalid %s/%s accepted: %d %s calls=%d", tc.path, tc.store, w.Code, w.Body.String(), calls)
		}
	}
}

func TestFollowUploadRejectsInvalidMoney(t *testing.T) {
	for _, value := range []any{"210.5", "not a price", -210} {
		book := excelize.NewFile()
		rows := [][]any{{"库存", "最低价", "URL"}, {1, value, "PLID500"}}
		for i, row := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if err := book.SetSheetRow(book.GetSheetName(0), cell, &row); err != nil {
				t.Fatal(err)
			}
		}
		file, err := book.WriteToBuffer()
		book.Close()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(map[string]any{"file_data": file.Bytes(), "file_size": file.Len()})
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/follow/upload", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv := NewServer(nil, nil, nil, nil, nil, nil, nil, nil, "test")
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "第 2 行") {
			t.Errorf("unsafe money %v was silently converted: %d %s", value, w.Code, w.Body.String())
		}
	}
}
