package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"takealot/pkg/account"
	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
)

type memberStatusChecker struct {
	mu     sync.Mutex
	status account.Status
}

func TestMembershipRecoveryPreservesUserAndBrowserPauses(t *testing.T) {
	for _, mode := range []string{"manual pause", "manual stop", "browser verification", "signed_out", "membership_required"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				checker := &memberStatusChecker{}
				checker.set("active")
				eng := NewEngine(nil, nil, nil, checker)
				eng.StartReprice("shop")
				defer eng.StopReprice("shop")
				synctest.Wait()
				lastRun := *eng.GetStatus("shop").LastRunTime
				if mode == "manual pause" {
					eng.PauseReprice("shop")
				} else if mode == "browser verification" {
					eng.SetVerificationRequired("shop", true, "waiting")
				}
				checker.set("unavailable")
				eng.CheckMembership()
				if s := eng.GetAllStatus(); !s.IsRunning || !s.IsPaused || !s.MembershipVerificationRequired || s.NextRunTime != nil {
					t.Fatalf("outage not visible in aggregate status: %+v", s)
				}
				switch mode {
				case "manual stop":
					eng.StopReprice("shop")
				case "signed_out", "membership_required":
					checker.set(mode)
					eng.CheckMembership()
					if eng.GetStatus("shop").IsRunning {
						t.Fatal("confirmed invalid membership did not stop task")
					}
				}
				checker.set("active")
				eng.CheckMembership()
				time.Sleep(time.Minute)
				synctest.Wait()
				s := eng.GetStatus("shop")
				wantRunning := mode == "manual pause" || mode == "browser verification"
				if s.IsRunning != wantRunning || s.IsPaused != wantRunning || s.MembershipVerificationRequired || *s.LastRunTime != lastRun {
					t.Fatalf("membership recovery overwrote user/browser state: %+v", s)
				}
			})
		})
	}
}

func TestMembershipOutageCancelsOldQuoteAndRecoveryFetchesNewPrice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		database, err := db.New(filepath.Join(t.TempDir(), "membership.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer database.Close()
		store, err := database.EnsureDefaultStore("fake-key", 1, 1, 5, 1, 1000, 120)
		if err != nil {
			t.Fatal(err)
		}
		if err := database.SaveStoreTargets(store.ID, map[string]config.Target{"111/222": {Selected: true, MinPrice: 100}}); err != nil {
			t.Fatal(err)
		}
		checker := &memberStatusChecker{}
		checker.set("active")
		pool := api.NewClientPool()
		eng := NewEngine(nil, pool, database, checker)
		var quotes, reads int
		var prices []int
		var interrupted context.Context
		pool.SetRequestTransport(func(_ string, _ string, req *http.Request, _ func(*http.Request) (*http.Response, error)) (*http.Response, error) {
			var body any
			switch req.URL.Path {
			case "/v2/seller":
				body = map[string]any{"seller_id": 123, "display_name": "Test store"}
			case "/v2/offers/detailed":
				reads++
				body = map[string]any{"offers": []api.OfferItem{{OfferID: 987, TSINID: 111, TSIN: api.TSINInfo{ProductlineID: 222, Title: "Test item"}, SellingPrice: 300}}}
			case "/1/catalogue/mpv/search":
				body = map[string]any{"results": []map[string]any{{"tsinId": 111, "bestPrice": 120, "competingOffers": 2}}}
			case "/rest/v-1-16-0/product-details/PLID222":
				quotes++
				competitor := 280
				if quotes == 1 {
					competitor = 120
					interrupted = req.Context()
					checker.set("unavailable")
					eng.CheckMembership()
				}
				body = map[string]any{"buybox": api.PublicBuybox{TSINID: 111, ItemsType: "multi", Items: []api.PublicBuyboxItem{
					{Price: 300, SellerID: "123"}, {Price: float64(competitor), SellerID: "456"},
				}}}
			case "/v2/offers/offer/987":
				if req.Method != http.MethodPatch {
					return nil, fmt.Errorf("unexpected method: %s", req.Method)
				}
				var payload struct {
					Price int `json:"selling_price"`
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					return nil, err
				}
				prices = append(prices, payload.Price)
				return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL)
			}
			encoded, err := json.Marshal(body)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(encoded)))}, err
		})
		if ok, msg := eng.StartReprice(store.ID); !ok {
			t.Fatal(msg)
		}
		defer eng.StopReprice(store.ID)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if interrupted == nil || interrupted.Err() != context.Canceled || len(prices) != 0 || !eng.GetStatus(store.ID).MembershipVerificationRequired {
			t.Fatalf("outage failed to cancel pending decision: quotes=%d prices=%v status=%+v", quotes, prices, eng.GetStatus(store.ID))
		}
		checker.set("active")
		eng.CheckMembership()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if quotes != 2 || reads != 2 || len(prices) != 1 || prices[0] != 279 {
			t.Fatalf("recovery used stale data: reads=%d quotes=%d submitted=%v, want fresh R279", reads, quotes, prices)
		}
	})
}

func (c *memberStatusChecker) GetStatus() account.Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *memberStatusChecker) IsValid() bool { return c.GetStatus().Eligible }

func (c *memberStatusChecker) set(state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = account.Status{State: state, Eligible: state == "active"}
}

func TestRepriceLoopWaitsForMemberServiceAndResumes(t *testing.T) {
	for _, state := range []string{"unavailable", "verification_required"} {
		t.Run(state, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				checker := &memberStatusChecker{}
				checker.set("active")
				eng := NewEngine(nil, nil, nil, checker)
				if ok, msg := eng.StartReprice("shop"); !ok {
					t.Fatal(msg)
				}
				defer eng.StopReprice("shop")
				synctest.Wait()
				lastRun := *eng.GetStatus("shop").LastRunTime
				checker.set(state)
				time.Sleep(5 * time.Minute)
				synctest.Wait()
				if s := eng.GetStatus("shop"); !s.IsRunning || !s.IsPaused || s.NextRunTime != nil || *s.LastRunTime != lastRun {
					t.Fatalf("temporary verification failure stopped or ran the task: %+v", s)
				}
				checker.set("active")
				time.Sleep(300 * time.Millisecond)
				synctest.Wait()
				if s := eng.GetStatus("shop"); !s.IsRunning || s.IsPaused || s.NextRunTime == nil || *s.LastRunTime == lastRun {
					t.Fatalf("verified recovery did not start a fresh cycle: %+v", s)
				}
			})
		})
	}
}
