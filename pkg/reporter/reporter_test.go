package reporter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadConfigFromDotKey(t *testing.T) {
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, ".api.key")
	content := `
# 注释行
XIAN_URL=https://test.0122.vip/
XIAN_SITE_ID=99
XIAN_API_KEY=cms_live_test_key_123
`
	if err := os.WriteFile(keyFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	LoadConfigFromDotKey(keyFile, &cfg)

	if cfg.BaseURL != "https://test.0122.vip" {
		t.Errorf("expected BaseURL https://test.0122.vip, got %s", cfg.BaseURL)
	}
	if cfg.SiteID != "99" {
		t.Errorf("expected SiteID 99, got %s", cfg.SiteID)
	}
	if cfg.APIKey != "cms_live_test_key_123" {
		t.Errorf("expected APIKey cms_live_test_key_123, got %s", cfg.APIKey)
	}
}

func TestCMSReporterPostContent(t *testing.T) {
	var receivedHeaderKey, receivedSiteID string
	var receivedPayload contentPayload

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaderKey = r.Header.Get("X-API-Key")
		receivedSiteID = r.Header.Get("X-Site-Id")

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:              ts.URL,
		SiteID:               "2",
		APIKey:               "cms_live_secret",
		CloudflareCategoryID: 1235,
		PriceFetchCategoryID: 1236,
		RateLimitDuration:    time.Minute,
	}
	r := New(cfg)

	err := r.postContent(1235, "测试标题", "测试摘要", "<b>测试内容</b>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedHeaderKey != "cms_live_secret" {
		t.Errorf("expected header cms_live_secret, got %s", receivedHeaderKey)
	}
	if receivedSiteID != "2" {
		t.Errorf("expected site id 2, got %s", receivedSiteID)
	}
	if receivedPayload.CategoryID != 1235 || receivedPayload.Title != "测试标题" {
		t.Errorf("unexpected payload: %+v", receivedPayload)
	}
}

func TestCMSReporterRateLimit(t *testing.T) {
	var requestCount int32
	done := make(chan struct{}, 5)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		done <- struct{}{}
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:              ts.URL,
		SiteID:               "2",
		APIKey:               "cms_live_secret",
		CloudflareCategoryID: 1235,
		PriceFetchCategoryID: 1236,
		RateLimitDuration:    500 * time.Millisecond,
	}
	r := New(cfg)

	// 第一次上报，应该触发
	r.ReportCloudflareBlock("店铺A", "1001", "PLID123", "403 Forbidden")

	// 紧接着第二次同 PLID+TSIN 上报，应被频率限制过滤
	r.ReportCloudflareBlock("店铺A", "1001", "PLID123", "403 Forbidden")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for first report")
	}

	time.Sleep(50 * time.Millisecond)
	if count := atomic.LoadInt32(&requestCount); count != 1 {
		t.Fatalf("expected 1 request due to rate limit, got %d", count)
	}

	// 等待限流窗口过期
	time.Sleep(550 * time.Millisecond)

	// 第三次上报，应该再次触发
	r.ReportCloudflareBlock("店铺A", "1001", "PLID123", "403 Forbidden")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for report after rate limit expiry")
	}

	if count := atomic.LoadInt32(&requestCount); count != 2 {
		t.Fatalf("expected 2 requests after expiry, got %d", count)
	}
}
