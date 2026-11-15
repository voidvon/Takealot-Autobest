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

func TestPriceChangesPreserveRRPAndSavedLimits(t *testing.T) {
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "prices.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveStore(db.Store{ID: "own", Name: "Own", Authorization: "test", RRPPercentage: 300, MinPriceRatio: 10}); err != nil {
		t.Fatal(err)
	}
	const key = "123/456"
	if err := database.SaveStoreTargets("own", map[string]config.Target{key: {Selected: true, MinPrice: 210, MaxPrice: 400}}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveStoreCachedOffers("own", []db.CachedOffer{{Key: key, TSINID: "123", PLID: "456", SellingPrice: 250, RRP: 500}}); err != nil {
		t.Fatal(err)
	}
	pool := api.NewClientPool()
	writes := 0
	pool.SetRequestTransport(func(_, _ string, r *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		if r.Method == http.MethodGet {
			if r.URL.Path == "/v2/offers" || r.URL.Path == "/v2/offers/detailed" {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"offers":[{"offer_id":987,"tsin_id":123}]}`))}, nil
			}
			if r.URL.Path == "/v2/offers/offer/987" {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"offer_id":987,"tsin_id":123,"rrp":500}`))}, nil
			}
			t.Fatalf("unexpected read: %s", r.URL)
		}
		writes++
		var fields map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			t.Fatal(err)
		}
		if _, found := fields["rrp"]; found {
			t.Fatal("manual price changes must not submit RRP")
		}
		if r.Method == "POST" {
			if r.URL.Path == "/v2/offers/batch" {
				t.Fatal("batch requests use an array, handled separately")
			}
			return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"offer_id":1001}`))}, nil
		}
		if r.Method != "PATCH" || r.URL.Path != "/v2/offers/offer/987" || len(fields) != 1 {
			t.Fatalf("unexpected price update: %s %s %v", r.Method, r.URL, fields)
		}
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})
	srv := NewServer(config.NewManager(dir), pool, engine.NewEngine(nil, pool, database, nil), database, nil, nil, nil, nil, "test")
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/api/official/offers/update", `{"tsin_id":"123","target_key":"123/456","selling_price":240}`, 200},
		{"/api/fulfillment/offer/quick-update", `{"tsin":"123","selling_price":260}`, 200},
		{"/api/official/offers/update", `{"tsin_id":"123","selling_price":120}`, 400},
		{"/api/fulfillment/offer/quick-update", `{"tsin":"123","selling_price":120}`, 400},
		{"/api/official/offers/update", `{"offer_id":"987","selling_price":120}`, 400},
		{"/api/official/offers/update", `{"tsin_id":"123","selling_price":401}`, 400},
		{"/api/official/offers/update", `{"tsin_id":"123","rrp":100}`, 400},
		{"/api/official/offers/update", `{"tsin_id":"123","selling_price":120,"rrp":144}`, 400},
		{"/api/official/offers/update", `{"tsin_id":"123","extra":{"rrp":0}}`, 400},
		{"/api/official/offers/update", `{"tsin_id":"123","extra":{"selling_price":120}}`, 400},
		{"/api/fulfillment/offer/quick-update", `{"tsin":"123","selling_price":240,"rrp":0}`, 400},
		{"/api/official/offers/create", `{"barcode":"new","selling_price":240}`, 200},
		{"/api/official/offers/create", `{"barcode":"new","selling_price":240,"rrp":288}`, 400},
		{"/api/official/offers/batch", `{"offers":[{"offer_id":987,"selling_price":240,"rrp":288}]}`, 400},
	} {
		before := writes
		r := httptest.NewRequest("POST", "http://127.0.0.1:8000"+tc.path, strings.NewReader(tc.body))
		r.Header.Set("X-Store-Id", "own")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		if w.Code != tc.status || (tc.status == 400 && writes != before) {
			t.Fatalf("%s %s: code %d writes %d body %s", tc.path, tc.body, w.Code, writes-before, w.Body.String())
		}
	}
	if writes != 3 {
		t.Fatalf("unexpected upstream writes: %d", writes)
	}
	target, err := database.LoadStoreTarget("own", key)
	if err != nil || target.MinPrice != 210 || target.MaxPrice != 400 || !target.Selected {
		t.Fatalf("manual price changes altered protection: %+v %v", target, err)
	}
	cached, err := database.LoadStoreCachedOffers("own")
	if err != nil || len(cached) != 1 || cached[0].RRP != 500 || cached[0].SellingPrice != 260 {
		t.Fatalf("RRP or accepted selling price changed: %+v %v", cached, err)
	}
}
