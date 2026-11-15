package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
)

type membershipCheckFunc func() bool

func (f membershipCheckFunc) IsValid() bool { return f() }

func TestRepriceCycleHonorsSavedFloor(t *testing.T) {
	for _, tc := range []struct {
		name              string
		current, floor    int
		competitor, max   int
		newFloor          int
		deselect          bool
		unidentifiedQuote bool
		editAtSubmit      bool
		legacyDuplicate   bool
		wantPrice         int
	}{
		{name: "platform price change preserves floor and recovers", current: 120, floor: 210, competitor: 119, wantPrice: 210},
		{name: "increase cannot remain below floor", current: 120, floor: 210, competitor: 150, wantPrice: 210},
		{name: "floor changed during quote fetch", current: 300, floor: 100, competitor: 121, newFloor: 210, wantPrice: 210},
		{name: "monitoring disabled during quote fetch", current: 300, floor: 210, competitor: 119, deselect: true},
		{name: "floor changed during membership check", current: 300, floor: 100, competitor: 121, newFloor: 210, editAtSubmit: true},
		{name: "monitoring disabled during membership check", current: 300, floor: 210, competitor: 119, deselect: true, editAtSubmit: true},
		{name: "floor recovery does not require competitor identity", current: 120, floor: 210, competitor: 119, unidentifiedQuote: true, wantPrice: 210},
		{name: "unidentified quote cannot drive ordinary repricing", current: 300, floor: 210, competitor: 119, unidentifiedQuote: true},
		{name: "conflicting limits prevent write", current: 120, floor: 210, competitor: 119, max: 200},
		{name: "missing floor prevents write", current: 300, floor: 0, competitor: 119},
		{name: "stale legacy floor cannot override canonical floor", current: 300, floor: 210, competitor: 119, legacyDuplicate: true, wantPrice: 210},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			database, err := db.New(filepath.Join(t.TempDir(), "floor.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			store, err := database.EnsureDefaultStore("test", 1, 1, 5, 1, 1000, 120)
			if err != nil {
				t.Fatal(err)
			}
			const key = "100368112/98946292"
			target := config.Target{Selected: true, MinPrice: tc.floor, MaxPrice: tc.max}
			saveEdit := func() {
				target.Selected = !tc.deselect
				if tc.newFloor > 0 {
					target.MinPrice = tc.newFloor
				}
				if err := database.SaveStoreTargets(store.ID, map[string]config.Target{key: target}); err != nil {
					t.Fatal(err)
				}
			}
			if err := database.SaveStoreTargets(store.ID, map[string]config.Target{key: target}); err != nil {
				t.Fatal(err)
			}
			if tc.legacyDuplicate {
				if err := database.SaveStoreTargets(store.ID, map[string]config.Target{"100368112": {Selected: true, MinPrice: 1}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := database.SaveStoreCachedOffers(store.ID, []db.CachedOffer{{Key: key, TSINID: "100368112", PLID: "98946292", SellingPrice: 300}}); err != nil {
				t.Fatal(err)
			}
			pool := api.NewClientPool()
			var prices []int
			pool.SetRequestTransport(func(storeID, _ string, req *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
				if storeID != store.ID {
					t.Fatalf("wrong store: %s", storeID)
				}
				var body any
				switch req.URL.Path {
				case "/v2/seller":
					body = map[string]any{"seller_id": 123, "display_name": "Test store"}
				case "/v2/offers/detailed":
					body = map[string]any{"offers": []api.OfferItem{{OfferID: 987, TSINID: 100368112, TSIN: api.TSINInfo{ProductlineID: 98946292, Title: "Head Strap"}, SellingPrice: float64(tc.current)}}}
				case "/1/catalogue/mpv/search":
					body = map[string]any{"results": []map[string]any{{"tsinId": 100368112, "bestPrice": tc.competitor, "competingOffers": 2}}}
				case "/rest/v-1-16-0/product-details/PLID98946292":
					// Simulate a saved edit while the worker is waiting on the network.
					if !tc.editAtSubmit && (tc.newFloor > 0 || tc.deselect) {
						saveEdit()
					}
					ownSeller, competitorSeller := "123", "456"
					if tc.unidentifiedQuote {
						ownSeller, competitorSeller = "", ""
					}
					body = map[string]any{"buybox": api.PublicBuybox{TSINID: 100368112, ItemsType: "multi", Items: []api.PublicBuyboxItem{
						{Price: float64(tc.current), SellerID: ownSeller},
						{Price: float64(tc.competitor), SellerID: competitorSeller},
					}}}
				case "/v2/offers/offer/987":
					if req.Method != http.MethodPatch {
						t.Fatalf("unexpected write method: %s", req.Method)
					}
					var payload struct {
						SellingPrice int `json:"selling_price"`
					}
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					prices = append(prices, payload.SellingPrice)
					return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
				default:
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				encoded, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(encoded)))}, nil
			})
			eng := NewEngine(nil, pool, database, nil)
			if tc.editAtSubmit {
				eng.membershipChecker = membershipCheckFunc(func() bool {
					saveEdit()
					return true
				})
			}
			eng.executeRepriceCycle(context.Background(), eng.getOrCreateWorker(store.ID))
			if tc.wantPrice == 0 {
				if len(prices) != 0 {
					t.Fatalf("unexpected price writes: %v", prices)
				}
			} else if len(prices) != 1 || prices[0] != tc.wantPrice {
				t.Fatalf("submitted prices = %v, want [%d]", prices, tc.wantPrice)
			}
			targets, err := database.LoadStoreTargets(store.ID)
			if err != nil || targets[key].MinPrice != target.MinPrice {
				t.Fatalf("saved floor changed unexpectedly: %+v, %v", targets, err)
			}
			cached, err := database.LoadStoreCachedOffers(store.ID)
			if err != nil || len(cached) != 1 {
				t.Fatalf("cache: %+v, %v", cached, err)
			}
			if tc.wantPrice > tc.competitor && !tc.unidentifiedQuote && cached[0].PriorityStatus != "losing" {
				t.Fatalf("floor protection must not claim Buybox victory: %+v", cached[0])
			}
		})
	}
}
