package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestRateLimitRetryLogsAndError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewClientPool()
		client := pool.GetOrCreate("shop", "test", "")
		var messages []string
		pool.SetLogger(func(storeID, message, level string) {
			if storeID != "shop" || level != "WARN" {
				t.Errorf("wrong log context: %s %s", storeID, level)
			}
			messages = append(messages, message)
		})
		calls := 0
		client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"2"}}, Body: http.NoBody}, nil
		})
		_, err := client.GetMPVByTSIN("123")
		var limited *RateLimitError
		if !errors.As(err, &limited) || limited.StatusCode != 429 || calls != 4 || len(messages) != 3 {
			t.Fatalf("calls=%d logs=%v error=%v", calls, messages, err)
		}
		for _, message := range messages {
			if !strings.Contains(message, "限流") || !strings.Contains(message, "HTTP 429") {
				t.Errorf("missing retry reason: %s", message)
			}
		}
	})
}

func TestCancelRateLimitWaitDoesNotCancelOtherRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient("test")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/1/catalogue/mpv/search" {
				return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"3600"}}, Body: http.NoBody}, nil
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"seller_id":123}`))}, nil
		})
		client.SetLogger(func(_, _ string) { cancel() })
		start := time.Now()
		if _, err := client.WithContext(ctx).GetMPVByTSIN("123"); !errors.Is(err, context.Canceled) {
			t.Fatalf("retry was not cancelled: %v", err)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("stopping waited for upstream cooldown: %s", elapsed)
		}
		if seller, err := client.GetSellerInfo(); err != nil || seller.SellerID != 123 {
			t.Fatalf("worker cancellation affected the control panel: %+v %v", seller, err)
		}
	})
}

func TestRetryAfterSupportsDateAndSeconds(t *testing.T) {
	now := time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		header string
		want   time.Duration
	}{
		{"5", 5 * time.Second},
		{now.Add(time.Minute).Format(http.TimeFormat), time.Minute},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{"bad", 0}, {"-1", 0},
	} {
		if got := retryAfterDelay(tc.header, now); got != tc.want {
			t.Errorf("header=%q got %v want %v", tc.header, got, tc.want)
		}
	}
}

func TestGetAllOffersReportsPartialFetchFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient("test")
		client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
			status, body := 200, `{"offers":[`+strings.TrimSuffix(strings.Repeat(`{"tsin_id":123},`, 100), ",")+`]}`
			if r.URL.Query().Get("page_number") == "2" {
				status, body = 401, `{"error":"expired"}`
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		offers, err := client.GetAllOffers(1000)
		if len(offers) != 100 || err == nil || !strings.Contains(err.Error(), "第 2 页") {
			t.Fatalf("partial fetch silently succeeded: count=%d error=%v", len(offers), err)
		}
	})
}

func TestRetryCancellationReachesInflightRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewClient("test")
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
			close(started)
			<-r.Context().Done()
			return nil, r.Context().Err()
		})
		done := make(chan error, 1)
		go func() {
			_, err := client.WithContext(ctx).GetSellerInfo()
			done <- err
		}()
		<-started
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("inflight request not cancelled: %v", err)
		}
	})
}

func TestCloudflareChallengeIsNotReportedAsInvalidKey(t *testing.T) {
	client := NewClient("test")
	calls := 0
	client.httpClient.Transport = offerTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 403, Header: http.Header{"Cf-Mitigated": {"challenge"}}, Body: io.NopCloser(strings.NewReader("<html>Just a moment</html>"))}, nil
	})
	result := client.TestConnection()
	if result.Success || !strings.Contains(result.Message, "Cloudflare") || strings.Contains(result.Message, "已失效") || calls != 1 {
		t.Fatalf("challenge misreported as expired credentials or retried: %+v calls=%d", result, calls)
	}
}
