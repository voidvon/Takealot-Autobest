package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Fixtures are reduced public responses captured on 2026-10-22 for the two
// customer PLIDs. No credentials or seller account data are included.
func TestResolvePublicBuyboxByTSINCustomerPackSizes(t *testing.T) {
	raw, err := os.ReadFile("testdata/customer_buyboxes.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ plid, tsin string }{
		{"PLID100616050", "102263013"},
		{"PLID100929476", "102632376"},
	} {
		t.Run(tc.plid, func(t *testing.T) {
			client := NewClient("")
			client.minInterval = time.Nanosecond
			calls := 0
			client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "api.takealot.com" || r.Method != http.MethodGet {
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
				}
				key := tc.plid
				if colour := r.URL.Query().Get("colour_variant"); colour != "" {
					key += "_" + colour
				}
				body, ok := fixtures[key]
				if !ok {
					t.Fatalf("unexpected variant: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})
			initial, err := client.GetPublicBuybox(tc.plid)
			if err != nil {
				t.Fatal(err)
			}
			if initial.HasOfferPrices() || initial.HasOwnOffer("", int(initial.Items[0].Price)) || initial.IsSolo("", int(initial.Items[0].Price)) {
				t.Fatal("summary must not establish offer identity or solo status")
			}
			if initial.CompetitorLowestPrice("", 85) != 0 {
				t.Fatal("summary must not be used as a competitor price")
			}
			cache := make(map[string]*PublicBuybox)
			resolved, err := client.ResolvePublicBuyboxByTSIN(tc.plid, tc.tsin, initial, cache)
			if err != nil || !resolved.MatchesTSIN(tc.tsin) {
				t.Fatalf("resolve TSIN: %+v, %v", resolved, err)
			}
			if resolved.MatchesTSIN("999") {
				t.Fatal("resolved quote matched an unrelated TSIN")
			}
			before := calls
			if _, err := client.ResolvePublicBuyboxByTSIN(tc.plid, tc.tsin, initial, cache); err != nil || calls != before {
				t.Fatalf("cycle cache not reused: calls %d -> %d, %v", before, calls, err)
			}
		})
	}
}

func TestPublicBuyboxRejectsBlockedOrInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"forbidden", 403, "<html>Verify you are human</html>", "HTTP 403"},
		{"HTML challenge", 200, "<html>Just a moment</html>", "不是有效 JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient("")
			calls := 0
			client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			if bb, err := client.GetPublicBuybox("100616050"); bb != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("blocked quote: %+v, %v", bb, err)
			}
			if calls != 1 {
				t.Fatalf("blocked response should not be retried immediately: %d requests", calls)
			}
		})
	}
}

func TestPublicBuyboxNeverIdentifiesAnOfferByPriceAlone(t *testing.T) {
	bb := &PublicBuybox{ItemsType: "multi", Items: []PublicBuyboxItem{
		{Price: 84, SellerID: "M123"},
		{Price: 150, SellerID: "M456"},
	}}
	competitor, own, identified := bb.PricesForSeller(123)
	if competitor != 150 || !own || !identified {
		t.Fatalf("own old price was not excluded: %d, %v, %v", competitor, own, identified)
	}
	bb.Items[0].SellerID = ""
	if _, _, identified := bb.PricesForSeller(123); identified {
		t.Fatal("missing seller ID must not fall back to matching price")
	}
}

func TestPublicVariantResolutionPreservesColourWhenSelectingSize(t *testing.T) {
	client := NewClient("")
	client.minInterval = time.Nanosecond
	calls := 0
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("colour_variant") != "Green" {
			t.Fatal("colour was lost while selecting size:", r.URL)
		}
		body := `{"buybox":{"buybox_items_type":"summary","items":[{"price":85}]},"variants":{"selectors":[{"selector_type":"size","options":[{"is_enabled":true,"value":{"value":"M"}}]}]}}`
		if r.URL.Query().Get("size") == "M" {
			body = `{"buybox":{"tsin":123,"buybox_items_type":"single","items":[{"price":85,"sponsored_ads_seller_id":"M456"}]}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	initial := &PublicBuybox{ItemsType: "summary", Selectors: []PublicVariantSelector{{Type: "colour_variant", Options: []PublicVariantOption{{Enabled: true}}}}}
	initial.Selectors[0].Options[0].Value.Value = "Green"
	bb, err := client.ResolvePublicBuyboxByTSIN("1", "123", initial, nil)
	if err != nil || !bb.MatchesTSIN("123") || calls != 2 {
		t.Fatalf("nested selection failed: %+v, %v, calls=%d", bb, err, calls)
	}
}

func TestPublicVariantResolutionIsBounded(t *testing.T) {
	client := NewClient("")
	client.minInterval = time.Nanosecond
	calls := 0
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"buybox":{"tsin":999,"buybox_items_type":"single","items":[{"price":85}]}}`))}, nil
	})
	initial := &PublicBuybox{ItemsType: "summary", Selectors: []PublicVariantSelector{{Type: "colour_variant"}}}
	for i := 1; i <= 30; i++ {
		option := PublicVariantOption{Enabled: true}
		option.Value.Value = i
		initial.Selectors[0].Options = append(initial.Selectors[0].Options, option)
	}
	bb, err := client.ResolvePublicBuyboxByTSIN("1", "123", initial, nil)
	if bb != nil || err == nil || !strings.Contains(err.Error(), "上限") || calls > 24 {
		t.Fatalf("unbounded or wrong TSIN accepted: %+v, %v, calls=%d", bb, err, calls)
	}
}
