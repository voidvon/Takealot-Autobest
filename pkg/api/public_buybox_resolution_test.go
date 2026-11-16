package api

import (
	"encoding/json"
	"fmt"
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
		{"valid JSON with invalid field type", 200, `{"buybox":{"items":"unexpected"}}`, "字段格式无法解析"},
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

func TestResolvePublicBuyboxWithStringSizeValues(t *testing.T) {
	client := NewClient("")
	client.minInterval = time.Nanosecond
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"buybox":{"buybox_items_type":"summary"},"variants":{"selectors":[{"selector_type":"colour_variant","options":[{"is_enabled":true,"value":{"value":"Black"}}]},{"selector_type":"size","options":[{"is_enabled":true,"value":"XS"}]}]}}`
		if r.URL.Query().Get("colour_variant") == "Black" && r.URL.Query().Get("size") == "XS" {
			body = `{"buybox":{"tsin":123,"buybox_items_type":"single","items":[{"price":225,"sponsored_ads_seller_id":"M456"}]},"variants":{"selectors":[{"selector_type":"size","options":[{"is_enabled":true,"is_selected":true,"value":"XS"}]}]}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	initial, err := client.GetPublicBuybox("98958583")
	if err != nil {
		t.Fatal(err)
	}
	bb, err := client.ResolvePublicBuyboxByTSIN("98958583", "123", initial, nil)
	if err != nil || !bb.MatchesTSIN("123") {
		t.Fatalf("string size did not resolve exact TSIN: %+v, %v", bb, err)
	}
}

func TestPublicVariantValueFormats(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"value":"Black"}`, "Black"}, {`"XS"`, "XS"},
		{`42`, "42"}, {`null`, ""}, {`{}`, ""},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var value PublicVariantValue
			if err := json.Unmarshal([]byte(tc.raw), &value); err != nil || AnyToString(value.Value) != tc.want {
				t.Fatalf("value=%+v error=%v, want %s", value, err, tc.want)
			}
		})
	}
}

// Reduced response captured for PLID98958583 on 2026-11-13: sizes are
// strings and colours are objects in the same otherwise valid JSON response.
func TestPublicBuyboxParsesCapturedSizeSelectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/customer_size_selectors.json")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient("")
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})
	bb, err := client.GetPublicBuybox("98958583")
	if err != nil {
		t.Fatal(err)
	}
	if bb.HasOfferPrices() || len(bb.Selectors) != 2 || bb.Selectors[0].Options[0].Value.Value != "Black" || bb.Selectors[1].Options[0].Value.Value != "XS" {
		t.Fatalf("captured selectors were not preserved: %+v", bb)
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
	for i := 1; i <= maxPublicVariantRequests+10; i++ {
		option := PublicVariantOption{Enabled: true}
		option.Value.Value = i
		initial.Selectors[0].Options = append(initial.Selectors[0].Options, option)
	}
	cache := make(map[string]*PublicBuybox)
	bb, err := client.ResolvePublicBuyboxByTSIN("1", "123", initial, cache)
	if bb != nil || err == nil || !strings.Contains(err.Error(), "上限") || calls > maxPublicVariantRequests {
		t.Fatalf("unbounded or wrong TSIN accepted: %+v, %v, calls=%d", bb, err, calls)
	}
	before := calls
	if bb, err := client.ResolvePublicBuyboxByTSIN("1", "456", initial, cache); bb != nil || err == nil || calls != before {
		t.Fatalf("another TSIN bypassed the shared product request budget: bb=%+v err=%v calls %d -> %d", bb, err, before, calls)
	}
}

// Reduced public responses captured 2026-11-16 for the reported customer
// product. All requests are replayed locally; no seller credentials are used.
func TestReportedBraVariantResolvesWithoutWastingBudgetOnPartialSelections(t *testing.T) {
	raw, err := os.ReadFile("testdata/customer_bra_selectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	client := NewClient("")
	client.minInterval = time.Nanosecond
	calls := 0
	price := 0
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		colour, size := r.URL.Query().Get("colour_variant"), r.URL.Query().Get("size")
		key := "initial"
		if colour != "" {
			key = colour
			if size != "" {
				key += "/" + size
			}
		}
		body := fixtures[key]
		if key == "Nude/34B" && price > 0 {
			body = json.RawMessage(fmt.Sprintf(`{"buybox":{"tsin":93975231,"buybox_items_type":"multi","items":[{"price":%d,"sponsored_ads_seller_id":"M123"}]}}`, price))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	initial, err := client.GetPublicBuybox("93415081")
	if err != nil {
		t.Fatal(err)
	}
	cache := make(map[string]*PublicBuybox)
	bb, err := client.ResolvePublicBuyboxByTSIN("93415081", "93975231", initial, cache)
	if err != nil || !bb.MatchesTSIN("93975231") {
		t.Fatalf("available Nude/34B variant could not be located: %+v %v (calls=%d)", bb, err, calls)
	}
	if calls > 24 {
		t.Fatalf("too many requests to locate the reported variant: %d", calls)
	}
	before := calls
	if bb, err := client.ResolvePublicBuyboxByTSIN("93415081", "93975231", initial, cache); err != nil || !bb.MatchesTSIN("93975231") || calls != before {
		t.Fatalf("same-cycle exact TSIN cache was not reused: calls %d -> %d, err=%v", before, calls, err)
	}
	// The next cycle must fetch a fresh price using the remembered selector.
	price = 321
	bb, err = client.ResolvePublicBuyboxByTSIN("93415081", "93975231", initial, make(map[string]*PublicBuybox))
	if err != nil || !bb.MatchesTSIN("93975231") || len(bb.Items) != 1 || bb.Items[0].Price != 321 || calls != before+1 {
		t.Fatalf("next cycle did not reuse identity and refresh price: %+v err=%v calls=%d", bb, err, calls)
	}
}

func TestPublicVariantResolutionCoversLargerProductsAndRejectsStaleMapping(t *testing.T) {
	client := NewClient("")
	client.minInterval = time.Nanosecond
	initial := &PublicBuybox{ItemsType: "summary", Selectors: []PublicVariantSelector{{Type: "colour_variant"}, {Type: "size"}}}
	for i := 1; i <= 3; i++ {
		option := PublicVariantOption{Enabled: true}
		option.Value.Value = fmt.Sprintf("Colour%d", i)
		initial.Selectors[0].Options = append(initial.Selectors[0].Options, option)
	}
	for i := 1; i <= 21; i++ {
		option := PublicVariantOption{Enabled: true}
		option.Value.Value = fmt.Sprintf("Size%d", i)
		initial.Selectors[1].Options = append(initial.Selectors[1].Options, option)
	}
	calls := 0
	changed := false
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		colour, size := r.URL.Query().Get("colour_variant"), r.URL.Query().Get("size")
		if colour == "" {
			t.Fatal("wasted request on a size-only summary")
		}
		body := `{"buybox":{"buybox_items_type":"summary"},"variants":{"selectors":[{"selector_type":"size","options":[`
		for i := 1; i <= 21; i++ {
			if i > 1 {
				body += ","
			}
			body += fmt.Sprintf(`{"is_enabled":true,"value":"Size%d"}`, i)
		}
		body += `]}]}}`
		if size != "" {
			tsin := "999"
			if colour == "Colour3" && size == "Size21" && !changed || colour == "Colour1" && size == "Size1" && changed {
				tsin = "123"
			}
			body = fmt.Sprintf(`{"buybox":{"tsin":%s,"buybox_items_type":"single","items":[{"price":100,"sponsored_ads_seller_id":"M456"}]}}`, tsin)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	bb, err := client.ResolvePublicBuyboxByTSIN("1", "123", initial, nil)
	if err != nil || !bb.MatchesTSIN("123") || calls != 66 {
		t.Fatalf("63 valid combinations were not covered: bb=%+v err=%v calls=%d", bb, err, calls)
	}
	// A platform mapping change must invalidate our remembered selector.
	changed = true
	before := calls
	bb, err = client.ResolvePublicBuyboxByTSIN("1", "123", initial, nil)
	if err != nil || !bb.MatchesTSIN("123") || calls != before+3 {
		t.Fatalf("stale selector accepted or fresh selector missed: bb=%+v err=%v calls=%d", bb, err, calls-before)
	}
}

func TestPublicVariantResolutionCanLeaveDefaultConcreteVariant(t *testing.T) {
	client := NewClient("")
	client.minInterval = time.Nanosecond
	initial := &PublicBuybox{TSINID: 999, ItemsType: "single", Items: []PublicBuyboxItem{{Price: 100}}, Selectors: []PublicVariantSelector{{Type: "colour_variant", Options: []PublicVariantOption{
		{Enabled: true, Selected: true, Value: PublicVariantValue{Value: "Black"}},
		{Enabled: true, Value: PublicVariantValue{Value: "Nude"}},
	}}}}
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		tsin := 999
		if r.URL.Query().Get("colour_variant") == "Nude" {
			tsin = 123
		}
		body := fmt.Sprintf(`{"buybox":{"tsin":%d,"buybox_items_type":"single","items":[{"price":100}]}}`, tsin)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	bb, err := client.ResolvePublicBuyboxByTSIN("1", "123", initial, nil)
	if err != nil || !bb.MatchesTSIN("123") {
		t.Fatalf("default quote prevented checking other variants: %+v %v", bb, err)
	}
}
