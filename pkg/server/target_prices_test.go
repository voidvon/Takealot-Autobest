package server

import (
	"encoding/json"
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

func TestVariantPriceConfigurationIsIndependentAndStoreScoped(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "prices.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.EnsureDefaultStore("test", 1, 1, 5, 1, 1000, 120); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveStore(db.Store{ID: "other", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	for _, store := range []string{"default", "other"} {
		if err := database.SaveStoreCachedOffers(store, []db.CachedOffer{
			{Key: "111/500", TSINID: "111", PLID: "500", SellingPrice: 120},
			{Key: "222/500", TSINID: "222", PLID: "500", SellingPrice: 300},
			{Key: "333/500", TSINID: "333", PLID: "500", SellingPrice: 150},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.SaveStoreCachedOffers("other", []db.CachedOffer{{Key: "999/900", TSINID: "999", PLID: "900"}}); err != nil {
		t.Fatal(err)
	}
	initial := map[string]config.Target{
		"111/500": {Selected: true, MinPrice: 80, MaxPrice: 150},
		"222":     {Selected: false, MinPrice: 210, MaxPrice: 400},
		"444/600": {Selected: true, MinPrice: 444},
	}
	if err := database.SaveStoreTargets("default", initial); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveStoreTargets("other", map[string]config.Target{"111/500": {Selected: true, MinPrice: 999}}); err != nil {
		t.Fatal(err)
	}
	pool := api.NewClientPool()
	pool.SetRequestTransport(func(_, _ string, req *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		t.Fatalf("local price configuration unexpectedly called upstream: %s %s", req.Method, req.URL)
		return nil, nil
	})
	eng := engine.NewEngine(nil, pool, database, nil)
	srv := NewServer(config.NewManager(t.TempDir()), pool, eng, database, nil, nil, nil, nil, "test")
	post := func(store, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8000/api/targets/prices", strings.NewReader(payload))
		req.Header.Set("X-Store-Id", store)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}
	w := post("default", `{"items":[{"key":"111/500","min_price":210,"max_price":400},{"key":"222/500","min_price":250,"max_price":450},{"key":"333/500","min_price":100,"max_price":0}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	targets, err := database.LoadStoreTargets("default")
	if err != nil || len(targets) != 4 || targets["111/500"].MinPrice != 210 || targets["111/500"].MaxPrice != 400 || !targets["111/500"].Selected ||
		targets["222/500"].MinPrice != 250 || targets["222/500"].MaxPrice != 450 || targets["222/500"].Selected ||
		targets["333/500"].MinPrice != 100 || targets["333/500"].Selected || targets["444/600"].MinPrice != 444 {
		t.Fatalf("limits, siblings, or selection changed incorrectly: %+v %v", targets, err)
	}
	other, _ := database.LoadStoreTarget("other", "111/500")
	if other.MinPrice != 999 || !other.Selected {
		t.Fatalf("other store overwritten: %+v", other)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8000/api/offers", nil)
	req.Header.Set("X-Store-Id", "default")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var response struct {
		Offers []OfferViewModel `json:"offers"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil || w.Code != 200 || len(response.Offers) != 3 {
		t.Fatalf("reload: %d %s %v", w.Code, w.Body.String(), err)
	}
	for _, offer := range response.Offers {
		wantPrice, wantMin := 120, 210
		if offer.TSINID == "222" {
			wantPrice, wantMin = 300, 250
		}
		if offer.TSINID == "333" {
			wantPrice, wantMin = 150, 100
		}
		if offer.SellingPrice != wantPrice || offer.MinPrice != wantMin {
			t.Fatalf("reloaded limits or platform price incorrect: %+v", offer)
		}
	}
	for _, invalid := range []struct{ store, body string }{
		{"all", `{"items":[{"key":"111/500","min_price":100}]}`},
		{"missing", `{"items":[{"key":"111/500","min_price":100}]}`},
		{"default", `{"items":[]}`},
		{"default", `{"items":[{"key":"111/500","min_price":100},{"key":"999/900","min_price":300}]}`},
		{"default", `{"items":[{"key":"111/500","min_price":100},{"key":"222/500","min_price":300,"max_price":200}]}`},
		{"default", `{"items":[{"key":"111/500","min_price":100},{"key":"111/500","min_price":300}]}`},
		{"default", `{"items":[{"key":"111/500","min_price":210.5}]}`},
		{"default", `{"items":[{"key":"111/500","min_price":0}]}`},
	} {
		w := post(invalid.store, invalid.body)
		current, _ := database.LoadStoreTarget("default", "111/500")
		if w.Code != http.StatusBadRequest || current.MinPrice != 210 || current.MaxPrice != 400 {
			t.Fatalf("invalid batch accepted or partially saved: %d %s target=%+v", w.Code, w.Body.String(), current)
		}
	}
}

func TestOfferLimitsNeverFallBackToAnotherStore(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "isolation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.EnsureDefaultStore("test", 1, 1, 5, 1, 1000, 120); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveStore(db.Store{ID: "other", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	for _, store := range []string{"default", "other"} {
		if err := database.SaveStoreCachedOffers(store, []db.CachedOffer{
			{Key: "111/500", TSINID: "111", PLID: "500"}, {Key: "222/500", TSINID: "222", PLID: "500"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Only default has a canonical limit; only other has a legacy TSIN limit.
	if err := database.SaveStoreTargets("default", map[string]config.Target{"111/500": {Selected: true, MinPrice: 210}}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveStoreTargets("other", map[string]config.Target{"222": {Selected: true, MinPrice: 350}}); err != nil {
		t.Fatal(err)
	}
	pool := api.NewClientPool()
	pool.SetRequestTransport(func(_, _ string, r *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		t.Errorf("cached offer read called upstream: %s", r.URL)
		return nil, http.ErrNotSupported
	})
	eng := engine.NewEngine(nil, pool, database, nil)
	srv := NewServer(config.NewManager(t.TempDir()), pool, eng, database, nil, nil, nil, nil, "test")
	for _, store := range []string{"default", "other", "all"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8000/api/offers", nil)
		r.Header.Set("X-Store-Id", store)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		var response struct {
			Offers []OfferViewModel `json:"offers"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 {
			t.Fatalf("read %s: %s %v", store, w.Body.String(), err)
		}
		for _, offer := range response.Offers {
			want := 0
			if offer.StoreID == "default" && offer.TSINID == "111" {
				want = 210
			}
			if offer.StoreID == "other" && offer.TSINID == "222" {
				want = 350
			}
			if offer.MinPrice != want || offer.Selected != (want > 0) {
				t.Errorf("store %s leaked or lost limits: %+v", store, offer)
			}
		}
	}
	// Saving a legacy monitor under its canonical identity must remove the old row.
	if err := database.SaveStoreTargets("other", map[string]config.Target{"222/500": {Selected: true, MinPrice: 400}}); err != nil {
		t.Fatal(err)
	}
	targets, err := database.LoadStoreTargets("other")
	if err != nil || len(targets) != 1 || targets["222/500"].MinPrice != 400 {
		t.Fatalf("duplicate legacy monitors: %+v %v", targets, err)
	}
	if err := database.SaveStoreTargets("other", map[string]config.Target{"222": {Selected: true, MinPrice: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveStoreTargetPrices("other", map[string]config.Target{"222/500": {MinPrice: 410}}); err != nil {
		t.Fatal(err)
	}
	targets, err = database.LoadStoreTargets("other")
	if err != nil || len(targets) != 1 || targets["222/500"].MinPrice != 410 {
		t.Fatalf("price dialog retained a stale legacy monitor: %+v %v", targets, err)
	}
	for _, tc := range []struct{ store, body string }{
		{"default", `{"111/500":{"selected":true,"min_price":0}}`},
		{"all", `{"default:111/500":{"selected":true,"min_price":0}}`},
		{"default", `{"111/500":{"selected":true,"min_price":300,"max_price":200}}`},
		{"default", `{"111/500":{"selected":true,"min_price":210,"store_id":"other"}}`},
		{"all", `{"default:111/500":{"selected":true,"min_price":210,"store_id":"other"}}`},
		{"all", `{"111/500":{"selected":true,"min_price":210}}`},
		{"all", `{"missing:111/500":{"selected":true,"min_price":210}}`},
		{"default", `{invalid}`},
	} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8000/api/targets", strings.NewReader(tc.body))
		r.Header.Set("X-Store-Id", tc.store)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)
		target, _ := database.LoadStoreTarget("default", "111/500")
		if w.Code != 400 || target.MinPrice != 210 {
			t.Fatalf("invalid targets changed floor: %d %s %+v", w.Code, w.Body.String(), target)
		}
	}
}
