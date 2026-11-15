package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMPVSearchRequiresUniqueExactTSIN(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"match is second", `{"results":[{"tsinId":222,"bestPrice":10},{"tsinId":111,"bestPrice":210}]}`, true},
		{"different variant", `{"results":[{"tsinId":222,"bestPrice":10}]}`, false},
		{"missing identity", `{"results":[{"bestPrice":10}]}`, false},
		{"duplicate identity", `{"results":[{"tsinId":111,"bestPrice":10},{"tsinId":111,"bestPrice":210}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient("test")
			c.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			result, err := c.GetMPVByTSIN("111")
			if tc.valid {
				if err != nil || result.BestPrice != 210 {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			} else if err == nil {
				t.Fatalf("unverified identity accepted: %+v", result)
			}
		})
	}
}
