package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSubmitFeedbackUsesPublicFormAndCurrentMember(t *testing.T) {
	// 旧后台配置不能影响反馈的目标站点或携带的凭证。
	t.Setenv("XIAN_URL", "https://unused.invalid")
	t.Setenv("XIAN_API_KEY", "admin-secret-must-not-be-sent")
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/feedback" || r.URL.RawQuery != "" {
			t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Site-Id") != "" {
			t.Error("feedback must not carry admin credentials or a hardcoded site selector")
		}
		cookie, err := r.Cookie("gocms_user")
		if err != nil || cookie.Value != "current-member-session" {
			t.Errorf("missing current member cookie: %v", err)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Error("GoCMS feedback requires a URL-encoded form")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		for key, want := range map[string]string{
			"title": "[抓取失败] 测试 & 店铺", "content": "HTTP 403\nPLID123 + TSIN456",
			"name": "Alice", "username": "alice", "member_id": "42", "source": "Takealot-Autobest",
			"phone": "未提供（客户端自动反馈）",
		} {
			if got := r.Form.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		for _, key := range []string{"category_id", "class_id", "visible", "site_id"} {
			if r.Form.Has(key) {
				t.Errorf("unexpected field %s", key)
			}
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	m, err := NewWithStore(srv.URL, "takealot-vip", nil)
	if err != nil {
		t.Fatal(err)
	}
	m.token = "current-member-session"
	m.status = Status{Authenticated: true, User: &User{ID: 42, Username: "alice", DisplayName: "Alice"}}
	if err := m.SubmitFeedback(context.Background(), "[抓取失败] 测试 & 店铺", "HTTP 403\nPLID123 + TSIN456"); err != nil {
		t.Fatal(err)
	}
	// 退出失败时可能仍保留 token，但不得继续以该会话自动反馈。
	m.signedOut = true
	if err := m.SubmitFeedback(context.Background(), "title", "content"); err == nil {
		t.Fatal("signed-out feedback was allowed")
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected requests: %d", calls.Load())
	}
}

func TestSubmitFeedbackRejectsFailedOrUnconfirmedResponses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"validation", 400, "captcha is required", "HTTP 400: captcha is required"},
		{"server", 503, "unavailable", "HTTP 503"},
		{"challenge", 200, "<html>challenge</html>", "未确认提交成功"},
		{"rejected", 200, `{"ok":false}`, "未确认提交成功"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			m, err := NewWithStore(srv.URL, "takealot-vip", nil)
			if err != nil {
				t.Fatal(err)
			}
			m.token = "member-session"
			m.status = Status{Authenticated: true, User: &User{Username: "alice"}}
			err = m.SubmitFeedback(context.Background(), "title", "content")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSubmitFeedbackDoesNotFollowRedirect(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	m, err := NewWithStore(srv.URL, "takealot-vip", nil)
	if err != nil {
		t.Fatal(err)
	}
	m.token = "member-session"
	m.status = Status{Authenticated: true, User: &User{Username: "alice"}}
	if err := m.SubmitFeedback(context.Background(), "title", "content"); err == nil {
		t.Fatal("redirect accepted")
	}
	if leaked.Load() != 0 {
		t.Fatal("member cookie sent to redirected endpoint")
	}
}
