package reporter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type feedbackCall struct{ title, content string }
type feedbackStub struct {
	calls chan feedbackCall
	err   error
}

func (s *feedbackStub) SubmitFeedback(ctx context.Context, title, content string) error {
	s.calls <- feedbackCall{title, content}
	return s.err
}
func receiveCall(t *testing.T, calls <-chan feedbackCall) feedbackCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("feedback was not submitted")
		return feedbackCall{}
	}
}

func TestReportsSubmitFeedbackAndRateLimitPerStoreAndType(t *testing.T) {
	s := &feedbackStub{calls: make(chan feedbackCall, 10)}
	r := New(s, nil)
	r.ReportCloudflareBlock("店铺A", "1001", "PLID123", "HTTP 403")
	call := receiveCall(t, s.calls)
	for _, want := range []string{"Cloudflare拦截", "店铺A", "1001", "PLID123"} {
		if !strings.Contains(call.title, want) {
			t.Errorf("title missing %s: %s", want, call.title)
		}
	}
	for _, want := range []string{"HTTP 403", "https://api.takealot.com/rest/v-1-16-0/product-details/PLID123", "本轮已暂停该商品调价"} {
		if !strings.Contains(call.content, want) {
			t.Errorf("content missing %s", want)
		}
	}
	r.ReportCloudflareBlock("店铺A", "1001", "PLID123", "duplicate")
	// 同一商品在其他店铺或另一种异常类别应单独反馈。
	r.ReportCloudflareBlock("店铺B", "1001", "PLID123", "other store")
	r.ReportPriceFetchError("店铺A", "1001", "PLID123", "HTTP 500")
	for i := 0; i < 2; i++ {
		call := receiveCall(t, s.calls)
		if strings.Contains(call.content, "duplicate") {
			t.Fatal("duplicate feedback sent")
		}
	}
}

func TestRateLimitExpires(t *testing.T) {
	r := New(nil, nil)
	if r.shouldRateLimit("product") || !r.shouldRateLimit("product") {
		t.Fatal("duplicate not suppressed")
	}
	r.lastReport["product"] = time.Now().Add(-16 * time.Minute)
	if r.shouldRateLimit("product") {
		t.Fatal("expired window suppressed report")
	}
}

func TestFeedbackFailureIsReported(t *testing.T) {
	s := &feedbackStub{calls: make(chan feedbackCall, 1), err: errors.New("HTTP 400: captcha is required")}
	failures := make(chan error, 1)
	r := New(s, func(err error) { failures <- err })
	r.ReportPriceFetchError("店铺A", "1001", "PLID123", "HTTP 500")
	select {
	case err := <-failures:
		for _, want := range []string{"信息反馈提交失败", "店铺A", "1001", "PLID123", "captcha is required"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("failure missing %s: %v", want, err)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("feedback error was discarded")
	}
}
