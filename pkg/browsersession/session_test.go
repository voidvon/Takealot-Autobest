package browsersession

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"takealot/pkg/api"
)

type browserCall struct {
	Origin        string `json:"origin"`
	Path          string `json:"path"`
	Method        string `json:"method"`
	Body          string `json:"body"`
	Authorization string `json:"authorization"`
}
type fakeDriver struct {
	mu             sync.Mutex
	calls          []browserCall
	views          map[string]string
	profiles       map[string]string
	created        int
	respond        func(browserCall) (string, error)
	visible        map[string]Bounds
	failShowOrigin string
}

func (d *fakeDriver) Start() error { return nil }
func (d *fakeDriver) Create(id, profile, proxy, origin, initialURL string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.views == nil {
		d.views = map[string]string{}
		d.profiles = map[string]string{}
	}
	d.views[id] = origin
	d.profiles[id] = profile
	d.created++
	return nil
}
func browserJSON(code int, body string, challenge bool) string {
	headers := map[string]string{"content-type": "application/json"}
	if challenge {
		headers["cf-mitigated"] = "challenge"
	}
	data, _ := json.Marshal(map[string]any{"status": code, "body": body, "headers": headers})
	return string(data)
}
func (d *fakeDriver) Eval(ctx context.Context, id, script string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	start := strings.Index(script, "const q = ") + len("const q = ")
	end := strings.Index(script[start:], ";\n") + start
	var call browserCall
	if err := json.Unmarshal([]byte(script[start:end]), &call); err != nil {
		return "", err
	}
	d.mu.Lock()
	if d.views[id] != call.Origin {
		d.mu.Unlock()
		return "", errors.New("wrong view origin")
	}
	d.calls = append(d.calls, call)
	respond := d.respond
	d.mu.Unlock()
	if respond != nil {
		return respond(call)
	}
	if call.Origin == seller {
		return browserJSON(200, `{"count":536}`, false), nil
	}
	return browserJSON(200, `{"buybox":{"items":[]}}`, false), nil
}
func (d *fakeDriver) Show(id string, bounds Bounds) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.failShowOrigin != "" && d.views[id] == d.failShowOrigin {
		return errors.New("view unavailable")
	}
	if d.visible == nil {
		d.visible = map[string]Bounds{}
	}
	d.visible[id] = bounds
	return nil
}
func (d *fakeDriver) Hide() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.visible = map[string]Bounds{}
	return nil
}
func (d *fakeDriver) Destroy(string) error { return nil }
func (d *fakeDriver) Close()               {}
func testManager(t *testing.T) (*Manager, *fakeDriver) {
	t.Helper()
	driver := &fakeDriver{}
	manager, err := New(t.TempDir(), driver, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Start(); err != nil {
		t.Fatal(err)
	}
	return manager, driver
}
func forbiddenFallback(*http.Request) (*http.Response, error) {
	return nil, errors.New("browser request fell back to Go")
}

func TestSplitPanelsSwitchStoresAndHideTogether(t *testing.T) {
	manager, driver := testManager(t)
	for _, storeID := range []string{"a", "b"} {
		if err := manager.Configure(storeID, "", true); err != nil {
			t.Fatal(err)
		}
	}
	left := Bounds{X: 20, Y: 100, Width: 480, Height: 500, ViewportWidth: 1040, ViewportHeight: 800}
	right := left
	right.X = 520
	for _, storeID := range []string{"a", "b"} {
		if err := manager.ShowPanels(storeID, "", left, right); err != nil {
			t.Fatal(err)
		}
		ids := manager.sessions[storeID].ids
		if len(driver.visible) != 2 || driver.visible[ids[0]] != left || driver.visible[ids[1]] != right {
			t.Fatal("split layout lost a page, its bounds, or left another store visible")
		}
	}
	if err := manager.Hide(); err != nil || len(driver.visible) != 0 {
		t.Fatal("closing panel did not hide both pages")
	}
	driver.failShowOrigin = public
	if err := manager.ShowPanels("b", "", left, right); err == nil || len(driver.visible) != 0 {
		t.Fatal("failed layout left a partially visible panel")
	}
}

func TestSessionBlocksUntilVerifiedAndRoutesWritesWithoutExportingCookies(t *testing.T) {
	manager, driver := testManager(t)
	if err := manager.Configure("shop-a", "", true); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("PATCH", seller+"/v2/offers/offer/123", strings.NewReader(`{"selling_price":200,"rrp":240}`))
	req.Header.Set("Authorization", "Key test-secret")
	if _, err := manager.Do("shop-a", "", req, forbiddenFallback); err == nil {
		t.Fatal("unverified session issued a write")
	}
	if len(driver.calls) != 0 {
		t.Fatal("unverified session dispatched fetch")
	}
	if err := manager.Check(context.Background(), "shop-a", "", "test-secret"); err != nil {
		t.Fatal(err)
	}
	resp, err := manager.Do("shop-a", "", req, forbiddenFallback)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(driver.calls) != 3 {
		t.Fatalf("got %d fetches", len(driver.calls))
	}
	if driver.calls[1].Authorization != "" {
		t.Fatal("seller credential leaked to public origin")
	}
	write := driver.calls[2]
	if write.Method != "PATCH" || write.Authorization != "Key test-secret" || write.Body != `{"selling_price":200,"rrp":240}` {
		t.Fatalf("write not preserved: %+v", write)
	}
	if status := manager.Status("shop-a"); status.Phase != "ready" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestChallengePausesWithoutReplayingWriteAndResumesAfterChecks(t *testing.T) {
	manager, driver := testManager(t)
	var paused []bool
	manager.changed = func(_ string, required bool, _ string) { paused = append(paused, required) }
	if err := manager.Configure("a", "", true); err != nil {
		t.Fatal(err)
	}
	if err := manager.Check(context.Background(), "a", "", "test"); err != nil {
		t.Fatal(err)
	}
	driver.respond = func(browserCall) (string, error) { return browserJSON(403, "<html>Just a moment</html>", true), nil }
	req, _ := http.NewRequest("PATCH", seller+"/v2/offers/offer/123", strings.NewReader(`{"selling_price":200}`))
	_, err := manager.Do("a", "", req, forbiddenFallback)
	var verification *api.VerificationRequiredError
	if !errors.As(err, &verification) {
		t.Fatalf("challenge did not pause: %v", err)
	}
	if manager.Status("a").Phase != "verification_required" {
		t.Fatal("challenge hidden")
	}
	_, err = manager.Do("a", "", req, forbiddenFallback)
	if err == nil || len(driver.calls) != 3 {
		t.Fatal("paused write replayed")
	}
	driver.respond = nil
	if err = manager.Check(context.Background(), "a", "", "test"); err != nil {
		t.Fatal(err)
	}
	if len(driver.calls) != 5 {
		t.Fatal("check replayed a price write")
	}
	if len(paused) != 4 || !paused[0] || paused[1] || !paused[2] || paused[3] {
		t.Fatalf("pause/resume events: %v", paused)
	}
}

func TestProfilesAndPreferencesAreIsolatedByStoreAndProxy(t *testing.T) {
	manager, driver := testManager(t)
	for _, id := range []string{"a", "b"} {
		if err := manager.Configure(id, "", true); err != nil {
			t.Fatal(err)
		}
	}
	a, b := manager.sessions["a"], manager.sessions["b"]
	if driver.profiles[a.ids[0]] == driver.profiles[b.ids[0]] || a.ids[0] == b.ids[0] {
		t.Fatal("stores shared profile")
	}
	if driver.profiles[a.ids[0]] != driver.profiles[a.ids[1]] {
		t.Fatal("same-store origins did not share profile")
	}
	newer, err := New(manager.root, &fakeDriver{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !newer.Status("a").Enabled || newer.Status("a").Available {
		t.Fatal("preference was lost or unavailable runtime was marked ready")
	}
	old := a.ids[0]
	a, err = manager.ensure("a", "http://127.0.0.1:8888")
	if err != nil {
		t.Fatal(err)
	}
	if a.ids[0] == old || a.confirmed {
		t.Fatal("changed proxy reused verified clearance")
	}
	if err = manager.Configure("bad", "http://user:pass@127.0.0.1:8888", true); err == nil {
		t.Fatal("unsupported authenticated proxy silently accepted")
	}
}

func TestQueuedRequestCanBeCanceledAndNeverSent(t *testing.T) {
	manager, driver := testManager(t)
	if err := manager.Configure("a", "", true); err != nil {
		t.Fatal(err)
	}
	if err := manager.Check(context.Background(), "a", "", "test"); err != nil {
		t.Fatal(err)
	}
	s := manager.sessions["a"]
	if err := s.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.unlock()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "PATCH", seller+"/v2/offers/offer/1", strings.NewReader(`{}`))
	done := make(chan error, 1)
	go func() { _, err := manager.Do("a", "", req, forbiddenFallback); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued browser write could not be stopped")
	}
	if len(driver.calls) != 2 {
		t.Fatal("cancelled queued write was sent")
	}
}

func TestNoCredentialOrWriteRedirectToOtherOrigins(t *testing.T) {
	manager, driver := testManager(t)
	if err := manager.Configure("a", "", true); err != nil {
		t.Fatal(err)
	}
	if err := manager.Check(context.Background(), "a", "", "test"); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("PATCH", "https://attacker.example/offers", strings.NewReader(`{}`))
	if _, err := manager.Do("a", "", req, forbiddenFallback); err == nil {
		t.Fatal("foreign endpoint accepted")
	}
	if len(driver.calls) != 2 {
		t.Fatal("foreign endpoint reached browser")
	}
	if err := manager.Configure("a", "", false); err != nil {
		t.Fatal(err)
	}
	resp, err := manager.Do("a", "", req, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("direct"))}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestEnabledSessionNeverFallsBackWhenDesktopRuntimeIsUnavailable(t *testing.T) {
	manager, _ := testManager(t)
	if err := manager.Configure("a", "", true); err != nil {
		t.Fatal(err)
	}
	paused := false
	headless, err := New(manager.root, &fakeDriver{}, func(_ string, required bool, _ string) { paused = required })
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("PATCH", seller+"/v2/offers/offer/1", strings.NewReader(`{}`))
	directCalled := false
	_, err = headless.Do("a", "", req, func(*http.Request) (*http.Response, error) {
		directCalled = true
		return nil, errors.New("unexpected direct request")
	})
	if err == nil || directCalled || !paused {
		t.Fatalf("unavailable runtime bypassed session: error=%v direct=%v paused=%v", err, directCalled, paused)
	}
}
func TestDirectChallengePromptsWithoutSilentlyEnablingBrowser(t *testing.T) {
	manager, driver := testManager(t)
	req, _ := http.NewRequest("GET", public+"/rest/product", nil)
	response, err := manager.Do("a", "", req, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{"Cf-Mitigated": []string{"challenge"}}, Body: http.NoBody}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	status := manager.Status("a")
	if status.Enabled || status.Phase != "direct_blocked" || driver.created != 0 {
		t.Fatalf("direct challenge did not remain user-controlled: %+v", status)
	}
}
