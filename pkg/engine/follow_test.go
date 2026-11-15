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

const followMPVFixture = `{"results":[
	{"tsinId":111,"productlineId":500,"bestPrice":121,"hasOffer":false,"gtin":"gtin-single"},
	{"tsinId":222,"productlineId":500,"bestPrice":119,"hasOffer":false,"gtin":"gtin-triple"},
	{"tsinId":333,"productlineId":500,"bestPrice":450,"hasOffer":true,"gtin":"gtin-owned"},
	{"tsinId":444,"productlineId":500,"bestPrice":900,"hasOffer":false,"gtin":"gtin-unselected"}
]}`

const followLabelFixture = `[{"productlineId":500,"title":"Head Strap","variants":[
	{"tsinId":111,"size":"1pc","colorName":"Black"},
	{"tsinId":222,"size":"3pcs","colorName":"Black"}
]}]`

func followTestEngine(t *testing.T) (*Engine, *db.DB, *api.ClientPool) {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "follow.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.EnsureDefaultStore("test", 1, 1, 5, 1, 1000, 120); err != nil {
		t.Fatal(err)
	}
	pool := api.NewClientPool()
	return NewEngine(nil, pool, database, nil), database, pool
}

func jsonFollowResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

func TestFollowPreviewExpandsOrTargetsExactVariants(t *testing.T) {
	eng, _, pool := followTestEngine(t)
	pool.SetRequestTransport(func(_, _ string, req *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("preview attempted a write: %s", req.Method)
		}
		if req.URL.Path == "/1/catalogue/mpv/search" {
			return jsonFollowResponse(followMPVFixture), nil
		}
		if req.URL.Path == "/1/catalogue/mpv/PLID500" {
			return jsonFollowResponse(followLabelFixture), nil
		}
		t.Fatalf("unexpected preview request: %s", req.URL)
		return nil, nil
	})
	preview, err := eng.PreviewFollowBatch(context.Background(), "default", []FollowItem{{URL: "PLID500", Stock: 1, MinPrice: 80}})
	if err != nil || len(preview.Errors) != 0 || len(preview.Variants) != 4 {
		t.Fatalf("preview = %+v, err = %v", preview, err)
	}
	if preview.Variants[0].Size != "1pc" || preview.Variants[1].Size != "3pcs" || preview.Variants[2].CanFollow {
		t.Fatalf("variant labels or eligibility incorrect: %+v", preview.Variants)
	}
	items := []FollowItem{
		{URL: "https://www.takealot.com/straps/PLID500", TSINID: "111", Stock: 1, MinPrice: 80, MaxPrice: 150, SellingPrice: 100},
		{URL: "PLID500", TSINID: "222", Stock: 3, MinPrice: 210, MaxPrice: 300},
	}
	preview, err = eng.PreviewFollowBatch(context.Background(), "default", items)
	if err != nil || len(preview.Errors) != 0 || len(preview.Variants) != 2 {
		t.Fatalf("targeted preview = %+v, err = %v", preview, err)
	}
	for i, variant := range preview.Variants {
		if variant.FollowItem != items[i] {
			t.Fatalf("variant %d inherited another variant's prices: %+v", i, variant)
		}
	}
	preview, err = eng.PreviewFollowBatch(context.Background(), "default", []FollowItem{
		{URL: "PLID500", TSINID: "999"}, {URL: "PLID500", TSINID: "111"}, {URL: "PLID500", TSINID: "111"},
	})
	if err != nil || len(preview.Errors) != 2 || len(preview.Variants) != 1 {
		t.Fatalf("missing or duplicate TSIN not reported: %+v %v", preview, err)
	}
}

func TestFollowBatchCreatesOnlySelectedVariantsWithIndependentFloors(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "all selected", true: "membership expires after first success"}[expire], func(t *testing.T) {
			t.Parallel()
			eng, database, pool := followTestEngine(t)
			if err := database.SaveStoreTargets("default", map[string]config.Target{"333/500": {Selected: true, MinPrice: 400, MaxPrice: 800}}); err != nil {
				t.Fatal(err)
			}
			if err := database.SaveStore(db.Store{ID: "other", Name: "Other store"}); err != nil {
				t.Fatal(err)
			}
			if err := database.SaveStoreTargets("other", map[string]config.Target{"111/500": {MinPrice: 999}}); err != nil {
				t.Fatal(err)
			}
			writes := map[string]int{}
			pool.SetRequestTransport(func(_, _ string, req *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
				if req.Method == http.MethodGet && req.URL.Path == "/1/catalogue/mpv/search" {
					return jsonFollowResponse(followMPVFixture), nil
				}
				if req.Method != http.MethodPost {
					t.Fatalf("unexpected follow request: %s %s", req.Method, req.URL)
				}
				var payload struct {
					Price int `json:"selling_price"`
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				writes[req.URL.Path] = payload.Price
				return &http.Response{StatusCode: 201, Body: http.NoBody}, nil
			})
			eng.membershipChecker = membershipCheckFunc(func() bool { return !expire || len(writes) == 0 })
			eng.executeFollowBatch(eng.getOrCreateWorker("default"), []FollowItem{
				{URL: "PLID500", TSINID: "111", Stock: 1, MinPrice: 80, MaxPrice: 150, SellingPrice: 100},
				{URL: "PLID500", TSINID: "222", Stock: 3, MinPrice: 210, MaxPrice: 300},
				{URL: "PLID500", TSINID: "333", Stock: 1, MinPrice: 1},
				{URL: "PLID500", TSINID: "999", Stock: 1, MinPrice: 1},
			})
			wantCount := 2
			if expire {
				wantCount = 1
			}
			if len(writes) != wantCount || writes["/v2/offers/offer/gtin-single"] != 100 || (!expire && writes["/v2/offers/offer/gtin-triple"] != 210) {
				t.Fatalf("incorrect per-variant prices or unselected variant created: %v", writes)
			}
			targets, err := database.LoadStoreTargets("default")
			if err != nil || targets["111/500"].MinPrice != 80 || targets["111/500"].MaxPrice != 150 || !targets["111/500"].Selected {
				t.Fatalf("first successful floor not saved: %+v %v", targets, err)
			}
			if !expire && (targets["222/500"].MinPrice != 210 || targets["222/500"].MaxPrice != 300) {
				t.Fatalf("second variant floor changed: %+v", targets)
			}
			if targets["333/500"].MinPrice != 400 || targets["333/500"].MaxPrice != 800 {
				t.Fatalf("existing sibling configuration overwritten: %+v", targets)
			}
			other, err := database.LoadStoreTargets("other")
			if err != nil || other["111/500"].MinPrice != 999 {
				t.Fatalf("other store floor overwritten: %+v %v", other, err)
			}
		})
	}
}

func TestFollowPriceProtectsVariantRange(t *testing.T) {
	for _, tc := range []struct{ min, max, price, best, want int }{
		{80, 150, 0, 121, 120}, {210, 300, 0, 119, 210},
		{210, 300, 0, 900, 300}, {210, 0, 0, 0, 210},
		{80, 150, 100, 121, 100},
	} {
		got, err := followPrice(FollowItem{Stock: 1, MinPrice: tc.min, MaxPrice: tc.max, SellingPrice: tc.price}, tc.best)
		if err != nil || got != tc.want {
			t.Errorf("case %+v: price %d, error %v", tc, got, err)
		}
	}
	for _, item := range []FollowItem{
		{Stock: 1}, {Stock: 1, MinPrice: 210, MaxPrice: 200},
		{Stock: 1, MinPrice: 210, SellingPrice: 120}, {Stock: 1, MinPrice: 80, MaxPrice: 150, SellingPrice: 200},
	} {
		if _, err := followPrice(item, 119); err == nil {
			t.Errorf("unsafe prices accepted: %+v", item)
		}
	}
}

func TestFollowStartRequiresResolvedUniqueTSINs(t *testing.T) {
	eng, _, _ := followTestEngine(t)
	for _, items := range [][]FollowItem{
		nil, {{URL: "PLID500", Stock: 1, MinPrice: 80}},
		{{URL: "PLID500", TSINID: "111", Stock: 1, MinPrice: 80}, {URL: "PLID500", TSINID: "111", Stock: 1, MinPrice: 210}},
	} {
		if ok, _ := eng.RunFollowBatch("default", items); ok {
			t.Errorf("unresolved or duplicate batch started: %+v", items)
		}
	}
}

func TestFollowExecutionRejectsUnverifiedVariantIdentity(t *testing.T) {
	for _, tc := range []struct{ name, response string }{
		{"different parent link", `{"results":[{"tsinId":111,"productlineId":600,"hasOffer":false,"gtin":"foreign"}]}`},
		{"unknown own offer status", `{"results":[{"tsinId":111,"productlineId":500,"gtin":"unknown"}]}`},
		{"duplicate TSIN in upstream response", `{"results":[{"tsinId":111,"productlineId":500,"hasOffer":false,"gtin":"one"},{"tsinId":111,"productlineId":500,"hasOffer":false,"gtin":"two"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			eng, database, pool := followTestEngine(t)
			pool.SetRequestTransport(func(_, _ string, req *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("unverified variant was created: %s", req.URL)
				}
				return jsonFollowResponse(tc.response), nil
			})
			eng.executeFollowBatch(eng.getOrCreateWorker("default"), []FollowItem{{URL: "PLID500", TSINID: "111", Stock: 1, MinPrice: 210}})
			targets, err := database.LoadStoreTargets("default")
			if err != nil || len(targets) != 0 {
				t.Fatalf("unverified variant added to monitors: %+v %v", targets, err)
			}
		})
	}
}
