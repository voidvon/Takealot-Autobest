package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
	"takealot/pkg/engine"
)

// Every upstream request is intercepted: this exercises the production HTTP
// handlers, SQLite persistence and background workers without a real shop write.
func TestImportedVariantsThroughAutomaticRepricing(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "workflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.EnsureDefaultStore("fake-workflow-key", 1, 1, 5, 1, 1000, 120); err != nil {
		t.Fatal(err)
	}
	// An existing cache must not hide newly followed variants.
	if err := database.SaveStoreCachedOffers("default", []db.CachedOffer{{Key: "999/900", TSINID: "999", PLID: "900", SellingPrice: 900}}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	prices := map[string]int{}
	competitors := map[string]int{"111": 121, "222": 301}
	writes := map[string]int{}
	rejectWrites := false
	pool := api.NewClientPool()
	pool.SetRequestTransport(func(store, _ string, r *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if store != "default" {
			return nil, fmt.Errorf("unexpected store %s", store)
		}
		status := http.StatusOK
		var body any
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v2/offers/offer/gtin-"):
			tsin := strings.TrimPrefix(r.URL.Path, "/v2/offers/offer/gtin-")
			var offer struct {
				Price int `json:"selling_price"`
			}
			if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
				return nil, err
			}
			prices[tsin] = offer.Price
			body = map[string]any{"offer_id": tsin}
		case r.Method == http.MethodPatch:
			tsin := strings.TrimPrefix(r.URL.Path, "/v2/offers/offer/9")
			if _, ok := prices[tsin]; !ok {
				return nil, fmt.Errorf("write used unknown offer ID: %s", r.URL)
			}
			var offer struct {
				Price int  `json:"selling_price"`
				RRP   *int `json:"rrp"`
			}
			if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
				return nil, err
			}
			if offer.RRP != nil {
				return nil, fmt.Errorf("repricer must omit RRP: %+v", offer)
			}
			writes[tsin]++
			if rejectWrites {
				status, body = http.StatusBadRequest, map[string]string{"message": "simulated rejection"}
			} else {
				prices[tsin] = offer.Price
				status = http.StatusNoContent
			}
		case r.Method != http.MethodGet:
			return nil, fmt.Errorf("unexpected write: %s %s", r.Method, r.URL)
		case r.URL.Path == "/v2/seller":
			body = map[string]any{"seller_id": 123, "display_name": "Workflow shop"}
		case r.URL.Path == "/v2/offers/detailed":
			offers := []api.OfferItem{}
			for _, tsin := range []string{"111", "222"} {
				if price, ok := prices[tsin]; ok {
					id, _ := strconv.ParseInt(tsin, 10, 64)
					offers = append(offers, api.OfferItem{OfferID: 9000 + id, TSINID: id, SellingPrice: float64(price), RRP: 500, TSIN: api.TSINInfo{ProductlineID: 500, Title: "Strap " + tsin}})
				}
			}
			body = map[string]any{"offers": offers}
		case r.URL.Path == "/1/catalogue/mpv/search":
			results := []map[string]any{}
			for _, tsin := range []string{"111", "222"} {
				if r.URL.Query().Get("search_by") == "tsin" && r.URL.Query().Get("search_query") != tsin {
					continue
				}
				_, owned := prices[tsin]
				results = append(results, map[string]any{"tsinId": tsin, "productlineId": 500, "hasOffer": owned, "gtin": "gtin-" + tsin, "bestPrice": competitors[tsin], "competingOffers": 2})
			}
			body = map[string]any{"results": results}
		case r.URL.Path == "/1/catalogue/mpv/PLID500":
			body = []map[string]any{{"productlineId": 500, "title": "Strap", "variants": []map[string]any{
				{"tsinId": 111, "colorName": "Blue"}, {"tsinId": 222, "colorName": "Orange"},
			}}}
		case r.URL.Path == "/rest/v-1-16-0/product-details/PLID500":
			tsin := "111"
			if r.URL.Query().Get("colour_variant") == "Orange" {
				tsin = "222"
			}
			body = map[string]any{"buybox": api.PublicBuybox{TSINID: tsin, ItemsType: "multi", Items: []api.PublicBuyboxItem{
				{Price: float64(prices[tsin]), SellerID: "123"}, {Price: float64(competitors[tsin]), SellerID: "456"},
			}}}
		default:
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
		encoded, err := json.Marshal(body)
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(encoded))}, err
	})
	eng := engine.NewEngine(nil, pool, database, nil)
	srv := NewServer(config.NewManager(t.TempDir()), pool, eng, database, nil, nil, nil, nil, "test")
	call := func(method, path string, value any) *httptest.ResponseRecorder {
		t.Helper()
		var body io.Reader
		if value != nil {
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(encoded)
		}
		r := httptest.NewRequest(method, "http://127.0.0.1:8000"+path, body)
		r.Header.Set("X-Store-Id", "default")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	wait := func(message string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if done() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timeout: %s; logs: %+v", message, eng.GetRecentLogs())
	}
	book := excelize.NewFile()
	for i, row := range [][]any{{"库存", "最低价", "URL", "TSIN", "初始售价", "封顶价"}, {1, 80, "PLID500", "111", 100, 150}, {3, 210, "PLID500", "222", 300, 400}} {
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
	var imported struct {
		Items []engine.FollowItem `json:"items"`
	}
	if err := json.Unmarshal(call("POST", "/api/follow/upload", map[string]any{"file_data": file.Bytes(), "file_size": file.Len()}).Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if len(imported.Items) != 2 {
		t.Fatalf("import: %+v", imported)
	}
	var preview engine.FollowPreview
	if err := json.Unmarshal(call("POST", "/api/follow/preview", imported).Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Errors) != 0 || len(preview.Variants) != 2 || !preview.Variants[0].CanFollow || !preview.Variants[1].CanFollow {
		t.Fatalf("preview: %+v", preview)
	}
	call("POST", "/api/follow/start", imported)
	wait("follow completion", func() bool { return !eng.GetStatus("default").FollowRunning })
	history, err := database.GetStoreFollowHistory("default", 10)
	if err != nil || len(history) != 2 || history[0].Status != "SUCCESS" || history[1].Status != "SUCCESS" {
		t.Fatalf("follow history: %+v %v", history, err)
	}
	readOffers := func(path string) map[string]OfferViewModel {
		t.Helper()
		var response struct {
			Offers []OfferViewModel `json:"offers"`
		}
		if err := json.Unmarshal(call("GET", path, nil).Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		result := map[string]OfferViewModel{}
		for _, offer := range response.Offers {
			result[offer.TSINID] = offer
		}
		return result
	}
	listed := readOffers("/api/offers")
	if len(listed) != 3 || listed["111"].MinPrice != 80 || listed["222"].MinPrice != 210 || !listed["111"].Selected || listed["222"].Stock != 0 {
		t.Fatalf("follow-to-reprice cache: %+v", listed)
	}
	call("POST", "/api/targets/prices", map[string]any{"items": []map[string]any{
		{"key": "111/500", "min_price": 90, "max_price": 150}, {"key": "222/500", "min_price": 250, "max_price": 400},
	}})
	listed = readOffers("/api/offers?sync=true")
	if listed["111"].MinPrice != 90 || listed["222"].MinPrice != 250 || listed["111"].RRP != 500 || listed["222"].RRP != 500 {
		t.Fatalf("sync lost limits: %+v", listed)
	}
	runCycle := func(want111, want222 int, fail bool) {
		t.Helper()
		previous := eng.GetStatus("default").LastRunTime
		call("POST", "/api/reprice/start", nil)
		wait("cycle completion", func() bool {
			s := eng.GetStatus("default")
			return s.NextRunTime != nil && s.LastRunTime != nil && (previous == nil || *previous != *s.LastRunTime)
		})
		call("POST", "/api/reprice/stop", nil)
		// NextRunTime is set only after executeRepriceCycle has returned.
		listed := readOffers("/api/offers")
		if listed["111"].SellingPrice != want111 || listed["222"].SellingPrice != want222 || listed["111"].MinPrice != 90 || listed["222"].MinPrice != 250 || listed["111"].RRP != 500 || listed["222"].RRP != 500 {
			t.Fatalf("cycle prices/limits: %+v", listed)
		}
		if eng.GetStatus("default").IsRunning {
			t.Fatal("stop failed")
		}
		mu.Lock()
		if !fail && (prices["111"] != want111 || prices["222"] != want222) {
			t.Errorf("upstream prices: %v", prices)
		}
		mu.Unlock()
	}
	// One sibling follows up while the other follows down.
	mu.Lock()
	competitors["111"], competitors["222"] = 131, 280
	mu.Unlock()
	runCycle(130, 279, false)
	// Both siblings then hit their own floor / ceiling.
	mu.Lock()
	competitors["111"], competitors["222"] = 50, 600
	mu.Unlock()
	runCycle(90, 400, false)
	reprices, err := database.GetStoreRepriceHistory("default", 20)
	if err != nil || len(reprices) != 4 {
		t.Fatalf("successful reprice history: %+v %v", reprices, err)
	}
	mu.Lock()
	rejectWrites = true
	competitors["111"], competitors["222"] = 140, 300
	mu.Unlock()
	runCycle(90, 400, true)
	after, err := database.GetStoreRepriceHistory("default", 20)
	mu.Lock()
	writeCount := writes["111"] + writes["222"]
	mu.Unlock()
	if err != nil || len(after) != 4 || writeCount != 6 {
		t.Fatalf("failed write updated history or retried: history=%d writes=%d err=%v", len(after), writeCount, err)
	}
}
