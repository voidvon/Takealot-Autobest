package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBrowserTransportCoversExistingAndNewClientsAndCancellation(t *testing.T) {
	pool := NewClientPool()
	first := pool.GetOrCreate("a", "test", "")
	var stores []string
	pool.SetRequestTransport(func(store, proxy string, req *http.Request, direct func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		stores = append(stores, store)
		if req.Method != "PATCH" || req.Header.Get("Authorization") != "Key test" {
			t.Fatalf("bad routed request: %+v", req)
		}
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	})
	second := pool.GetOrCreate("b", "test", "")
	for _, client := range []*Client{first, second} {
		if err := client.UpdateOfferPrice("1", 200, 240); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(stores, ",") != "a,b" {
		t.Fatalf("wrong store routing: %v", stores)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := first.WithContext(ctx).UpdateOfferPrice("1", 200, 240); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled write not stopped: %v", err)
	}
	if len(stores) != 2 {
		t.Fatal("cancelled write reached browser")
	}
}
func TestUncertainWritesAndVerificationAreNotRetried(t *testing.T) {
	for _, result := range []string{"transport-error", "server-error", "verification"} {
		t.Run(result, func(t *testing.T) {
			client := NewClient("test")
			calls := 0
			client.requestTransport = func(_, _ string, req *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
				calls++
				if result == "verification" {
					return nil, &VerificationRequiredError{Message: "verification"}
				}
				if result == "transport-error" {
					return nil, errors.New("lost response")
				}
				return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
			}
			if err := client.UpdateOfferPrice("1", 200, 240); err == nil || calls != 1 {
				t.Fatalf("write was silently replayed: calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestOfferListToleratesOptionalQualityScoreFormats(t *testing.T) {
	for _, score := range []string{`85`, `"85"`, `"N/A"`, `null`} {
		var offer OfficialOfferItem
		if err := json.Unmarshal([]byte(`{"offer_id":224068784,"tsin_id":100392696,"selling_price":225,"catalogue_quality_score":`+score+`}`), &offer); err != nil {
			t.Fatalf("score %s prevented offers loading: %v", score, err)
		}
		if offer.OfferID != 224068784 || offer.SellingPrice != 225 {
			t.Fatalf("offer identity/price changed: %+v", offer)
		}
		expected := 0
		if score == `85` || score == `"85"` {
			expected = 85
		}
		if offer.CatalogueQualityScore != expected {
			t.Fatalf("score %s = %d", score, offer.CatalogueQualityScore)
		}
	}
}
