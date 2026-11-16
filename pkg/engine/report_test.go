package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"takealot/pkg/account"
	"takealot/pkg/api"
	"takealot/pkg/reporter"
)

type mockReporter struct{ cfCalls, fetchCalls, variantCalls int }

func (m *mockReporter) ReportCloudflareBlock(_, _, _, _ string)        { m.cfCalls++ }
func (m *mockReporter) ReportPriceFetchError(_, _, _, _ string)        { m.fetchCalls++ }
func (m *mockReporter) ReportVariantResolutionError(_, _, _, _ string) { m.variantCalls++ }

func TestEngineReporterTrigger(t *testing.T) {
	for _, tc := range []struct {
		message    string
		cloudflare bool
	}{
		{"HTTP 403: 前台报价访问被拒绝", true},
		{"Cloudflare challenge", true},
		{"前台报价响应不是有效 JSON（可能返回了验证页面）", true},
		{"HTTP 500: internal error", false},
		{"前台报价字段格式无法解析: unexpected field type", false},
		{"公开规格查询达到 24 次上限，未定位 TSIN 92040398", false},
		{`Get "https://api.takealot.com/product-details/PLID1040398": EOF`, false},
	} {
		t.Run(tc.message, func(t *testing.T) {
			mock := &mockReporter{}
			eng := NewEngine(nil, nil, nil, nil)
			eng.SetReporter(mock)
			eng.reportPriceFetchFailure("测试店铺", "1001", "PLID123", errors.New(tc.message))
			if tc.cloudflare && (mock.cfCalls != 1 || mock.fetchCalls != 0) || !tc.cloudflare && (mock.cfCalls != 0 || mock.fetchCalls != 1) {
				t.Fatalf("incorrect routing: %+v", mock)
			}
		})
	}
}

func TestVariantResolutionErrorHasItsOwnFeedbackCategory(t *testing.T) {
	mock := &mockReporter{}
	eng := NewEngine(nil, nil, nil, nil)
	eng.SetReporter(mock)
	eng.reportPriceFetchFailure("shop", "92040398", "123", fmt.Errorf("resolve: %w", &api.VariantResolutionError{TSINID: "92040398", Limited: true}))
	if mock.variantCalls != 1 || mock.cfCalls != 0 || mock.fetchCalls != 0 {
		t.Fatalf("resolution failure misclassified: %+v", mock)
	}
}

func TestPriceFetchFailureAutomaticallySubmitsMemberFeedback(t *testing.T) {
	feedback := make(chan url.Values, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("member flow sent Cookie")
		}
		if r.URL.Path != "/api/v1/auth/token" && r.Header.Get("Authorization") != "Bearer member-session" {
			t.Error("member Bearer token missing")
		}
		switch r.URL.Path {
		case "/api/v1/auth/token":
			_, _ = w.Write([]byte(`{"data":{"user":{"id":7,"username":"member"},"access_token":"member-session","token_type":"Bearer","expires_in":3600}}`))
		case "/api/v1/me":
			_, _ = w.Write([]byte(`{"data":{"id":7,"username":"member","display_name":"普通会员"}}`))
		case "/api/v1/me/memberships":
			_, _ = w.Write([]byte(`{"data":[{"slug":"takealot-vip","status":"active"}]}`))
		case "/api/feedback":
			if r.Method != http.MethodPost {
				t.Error("feedback must POST")
			}
			if r.Header.Get("X-API-Key") != "" {
				t.Error("admin key sent")
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			for _, key := range []string{"title", "name", "phone", "content"} {
				if r.Form.Get(key) == "" {
					t.Errorf("required feedback field missing: %s", key)
				}
			}
			feedback <- r.Form
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	member, err := account.NewWithStore(srv.URL, "takealot-vip", nil)
	if err != nil {
		t.Fatal(err)
	}
	status, err := member.Login(context.Background(), "member", "test-password")
	if err != nil || !status.Eligible {
		t.Fatalf("login failed: %+v %v", status, err)
	}
	eng := NewEngine(nil, nil, nil, member)
	eng.SetReporter(reporter.New(member, func(err error) { eng.Log(err.Error(), "WARN") }))
	for _, message := range []string{"HTTP 403: Cloudflare", "HTTP 500: unavailable"} {
		eng.reportPriceFetchFailure("店铺A", "1001", "PLID123", errors.New(message))
		select {
		case form := <-feedback:
			if form.Get("username") != "member" || !strings.Contains(form.Get("content"), message) {
				t.Fatalf("incorrect feedback: %v", form)
			}
		case <-time.After(time.Second):
			t.Fatal("engine failure did not automatically submit feedback")
		}
	}
}
