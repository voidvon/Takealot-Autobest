package engine

import (
	"strings"
	"testing"

	"takealot/pkg/api"
)

func TestBuyboxPreventsFollowingOurOwnDelayedPrice(t *testing.T) {
	for _, tc := range []struct {
		cur, publicOwn, competitor, want int
	}{
		{85, 85, 190, 189},
		{84, 84, 150, 149},
		{149, 84, 150, -1},
		{149, 83, 150, -1},
		{149, 82, 150, -1},
		{149, 149, 140, 139},
	} {
		bb := &api.PublicBuybox{TSINID: "103477991", ItemsType: "multi", Items: []api.PublicBuyboxItem{
			{Price: float64(tc.publicOwn), SKU: "public-sku", SellerID: "M123"},
			{Price: float64(tc.competitor), SKU: "competitor", SellerID: "M456"},
		}}
		competitor, solo, reason := buyboxPrices(bb, "103477991", 123, tc.cur, 2)
		if reason != "" {
			t.Fatal(reason)
		}
		// bestPrice also still contains our old price. A confirmed competitor
		// must take precedence over that stale aggregate market minimum.
		decision := decidePrice(tc.cur, competitor, tc.publicOwn, 40, 300, 1, 1, solo)
		if decision.NewPrice != tc.want {
			t.Errorf("current %d public own %d competitor %d: %+v, want %d", tc.cur, tc.publicOwn, tc.competitor, decision, tc.want)
		}
	}
}

func TestBuyboxRequiresIdentityAndCompleteQuotesForRaises(t *testing.T) {
	for _, tc := range []struct {
		name, kind, tsin, ownSeller string
		sellerID                    int64
		competing                   int
	}{
		{"summary", "summary", "1", "M123", 123, 2},
		{"wrong variant", "multi", "2", "M123", 123, 2},
		{"missing seller identity", "multi", "1", "", 123, 2},
		{"own offer absent", "multi", "1", "M999", 123, 2},
		{"missing account identity", "multi", "1", "M123", 0, 2},
		{"incomplete buybox cards", "multi", "1", "M123", 123, 3},
		{"unknown offer count", "multi", "1", "M123", 123, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bb := &api.PublicBuybox{TSINID: tc.tsin, ItemsType: tc.kind, Items: []api.PublicBuyboxItem{
				{Price: 85, SellerID: tc.ownSeller}, {Price: 190, SellerID: "M456"},
			}}
			competitor, solo, reason := buyboxPrices(bb, "1", tc.sellerID, 85, tc.competing)
			if competitor != 0 || solo || reason == "" {
				t.Fatalf("unverified quote accepted: %d, %v, %q", competitor, solo, reason)
			}
		})
	}
	bb := &api.PublicBuybox{TSINID: "1", ItemsType: "single", Items: []api.PublicBuyboxItem{{Price: 85, SellerID: "M123"}}}
	if _, solo, reason := buyboxPrices(bb, "1", 123, 85, 2); solo || reason == "" {
		t.Fatal("a single buybox card does not prove that no competitors exist")
	}
	if _, solo, reason := buyboxPrices(bb, "1", 123, 85, 1); !solo || reason != "" {
		t.Fatal("seller API and public offer agree that we are solo")
	}
	bb.ItemsType = "multi"
	bb.Items = []api.PublicBuyboxItem{{Price: 85, SellerID: "M123"}, {Price: 85, SellerID: "M123"}, {Price: 190, SellerID: "M456"}}
	if _, _, reason := buyboxPrices(bb, "1", 123, 85, 3); reason == "" {
		t.Fatal("duplicated cards must not count as separate sellers")
	}
}

func TestPriceHoldExplainsCap(t *testing.T) {
	got := decidePrice(85, 190, 85, 40, 85, 1, 1, false)
	if got.NewPrice != -1 || !strings.Contains(got.Reason, "封顶价 R85") {
		t.Fatalf("cap hold was not explained: %+v", got)
	}
}

func TestBuyboxCanLowerWhenOnlyCheaperCompetitorsAreDisplayed(t *testing.T) {
	bb := &api.PublicBuybox{TSINID: "1", ItemsType: "multi", Items: []api.PublicBuyboxItem{
		{Price: 80, SellerID: "M456"}, {Price: 90, SellerID: "M789"},
	}}
	competitor, solo, reason := buyboxPrices(bb, "1", 123, 100, 0)
	if competitor != 80 || solo || reason != "" {
		t.Fatalf("known competitors were rejected because our card was absent: %d %v %s", competitor, solo, reason)
	}
	if decision := decidePrice(100, competitor, 80, 85, 150, 1, 1, solo); decision.NewPrice != 85 {
		t.Fatalf("a lower quote bypassed the floor: %+v", decision)
	}
	for _, price := range []int{75, 80} {
		if _, _, reason := buyboxPrices(bb, "1", 123, price, 2); reason == "" {
			t.Fatal("missing own card must not justify a raise or a tie-price change")
		}
	}
	bb.Items[0].SellerID = "" // Even a cheap quote is unusable without identity.
	if _, _, reason := buyboxPrices(bb, "1", 123, 100, 2); reason == "" {
		t.Fatal("lowering accepted unidentified sellers")
	}
}
