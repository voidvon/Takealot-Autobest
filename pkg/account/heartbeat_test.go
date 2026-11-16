package account

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type heartbeatTransport func(*http.Request) (*http.Response, error)

func (f heartbeatTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHeartbeatNotifiesOutageAndVerifiedRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &memoryStore{token: "saved-token"}
		m, err := NewWithStore("http://localhost", "vip", store)
		if err != nil {
			t.Fatal(err)
		}
		var outage, revoked atomic.Bool
		m.api.http.Transport = heartbeatTransport(func(r *http.Request) (*http.Response, error) {
			if outage.Load() {
				return nil, errors.New("simulated network timeout")
			}
			code, body := http.StatusOK, `{"data":{"id":1,"username":"alice"}}`
			if revoked.Load() {
				code, body = http.StatusUnauthorized, `{"error":{"code":"authentication_required"}}`
			} else if r.URL.Path == "/api/v1/me/memberships" {
				body = `{"data":[{"slug":"vip","status":"active"}]}`
			}
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		checks := make(chan Status, 4)
		go m.Run(ctx, func() { checks <- m.GetStatus() })
		next := func() Status {
			t.Helper()
			synctest.Wait()
			select {
			case state := <-checks:
				return state
			default:
				t.Fatal("heartbeat did not report the completed check")
				return Status{}
			}
		}
		storedToken := func() string {
			m.mu.Lock()
			defer m.mu.Unlock()
			return store.token
		}
		if s := next(); !s.Eligible {
			t.Fatalf("initial successful check was not reported: %+v", s)
		}
		outage.Store(true)
		time.Sleep(30 * time.Second)
		synctest.Wait()
		if s := m.GetStatus(); !s.Eligible {
			t.Fatalf("membership was rechecked before the daily interval: %+v", s)
		}
		select {
		case s := <-checks:
			t.Fatalf("unexpected check within the first day: %+v", s)
		default:
		}
		time.Sleep(24*time.Hour - 31*time.Second)
		synctest.Wait()
		if s := m.GetStatus(); !s.Eligible {
			t.Fatalf("verified membership did not remain valid until the next daily check: %+v", s)
		}
		select {
		case s := <-checks:
			t.Fatalf("unexpected check before 24 hours elapsed: %+v", s)
		default:
		}
		time.Sleep(time.Second)
		if s := next(); s.State != "unavailable" || s.Eligible || storedToken() == "" {
			t.Fatalf("network outage did not retain recoverable session: %+v", s)
		}
		outage.Store(false)
		time.Sleep(24 * time.Hour)
		if s := next(); !s.Eligible || storedToken() != "saved-token" {
			t.Fatalf("verified recovery was not reported: %+v", s)
		}
		revoked.Store(true)
		time.Sleep(24 * time.Hour)
		if s := next(); s.State != "signed_out" || s.Eligible || storedToken() != "" {
			t.Fatalf("revocation was not reported: %+v", s)
		}
	})
}

func TestKnownMembershipExpiryIsTerminal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &Manager{status: Status{Eligible: true, State: "active"}, checked: time.Now(), expiry: time.Now().Add(time.Second)}
		time.Sleep(2 * time.Second)
		if s := m.GetStatus(); s.Eligible || s.State != "membership_required" {
			t.Fatalf("known expiry treated as temporary verification failure: %+v", s)
		}
	})
}
