package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type offerTestTransport func(*http.Request) (*http.Response, error)

func (f offerTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResolveTSINAndUpdateOffer(t *testing.T) {
	client := NewClient("store-token")
	client.minInterval = time.Nanosecond
	calls := 0
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "Key store-token" {
			t.Fatal("request lost the owning store's credentials")
		}
		switch calls {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/v2/offers/detailed" || r.URL.Query().Get("tsin_id") != "12345" {
				t.Fatalf("unexpected lookup request: %s %s", r.Method, r.URL)
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"offers":[{"tsin_id":99999,"offer_id":11111},{"tsin_id":12345,"offer_id":76543}]}`))}, nil
		case 2:
			if r.Method != http.MethodPatch || r.URL.Path != "/v2/offers/offer/76543" {
				t.Fatalf("PATCH must use the seller Offer ID, got %s %s", r.Method, r.URL)
			}
			var payload map[string]int
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["selling_price"] != 450 || payload["rrp"] != 600 {
				t.Fatalf("unexpected price payload: %+v", payload)
			}
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
		default:
			t.Fatal("unexpected additional request")
			return nil, nil
		}
	})
	offerID, err := client.GetOfferIDByTSIN("12345")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.UpdateSingleOffer(offerID, map[string]any{"selling_price": 450, "rrp": 600}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected lookup and PATCH, got %d requests", calls)
	}
}

func TestResolveTSINRejectsMissingOffer(t *testing.T) {
	for _, tc := range []struct {
		name string
		tsin string
		code int
		body string
	}{
		{"empty identifier", "", 200, `{}`},
		{"invalid identifier", "12345&tsin_id=99999", 200, `{}`},
		{"empty list", "12345", 200, `{"offers":[]}`},
		{"different product", "12345", 200, `{"offers":[{"tsin_id":99999,"offer_id":76543}]}`},
		{"missing offer ID", "12345", 200, `{"offers":[{"tsin_id":12345,"offer_id":0}]}`},
		{"upstream error", "12345", 500, `{"message":"lookup failed"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient("store-token")
			client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet {
					t.Fatal("lookup sent a mutation request")
				}
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			if id, err := client.GetOfferIDByTSIN(tc.tsin); err == nil || id != "" {
				t.Fatalf("invalid lookup returned id=%q, error=%v", id, err)
			}
		})
	}
}
