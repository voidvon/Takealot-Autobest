package api

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// Read-only check of public TSIN resolution; never accesses a seller account or
// submits a price change. Assertions deliberately do not depend on live prices.
func TestLiveCustomerPublicBuybox(t *testing.T) {
	if os.Getenv("TAKEALOT_LIVE_CHECK") != "1" {
		t.Skip("live check disabled")
	}
	c := NewClient("")
	for _, tc := range []struct{ plid, tsin string }{
		{"100616050", "102263013"},
		{"100929476", "102632376"},
	} {
		t.Run(tc.plid, func(t *testing.T) {
			initial, err := c.GetPublicBuybox(tc.plid)
			if err != nil {
				t.Fatal(err)
			}
			bb, err := c.ResolvePublicBuyboxByTSIN(tc.plid, tc.tsin, initial, nil)
			if err != nil {
				t.Fatal(err)
			}
			id, err := strconv.ParseInt(strings.TrimPrefix(bb.Items[0].SellerID, "M"), 10, 64)
			if err != nil {
				t.Fatal("public seller ID missing:", err)
			}
			competitor, own, identified := bb.PricesForSeller(id)
			if !own || !identified {
				t.Fatal("cannot distinguish sellers in resolved variant")
			}
			t.Logf("TSIN=%s type=%s displayed_offers=%d first_seller=%d other_price=%d", AnyToString(bb.TSINID), bb.ItemsType, len(bb.Items), id, competitor)
		})
	}
}

// 临时联网校验：仅在设置 TAKEALOT_LIVE_CHECK=1 时运行。
func TestLivePublicBuybox(t *testing.T) {
	if os.Getenv("TAKEALOT_LIVE_CHECK") != "1" {
		t.Skip("live check disabled")
	}
	c := NewClient("")

	bb, err := c.GetPublicBuybox("PLID97129870")
	if err != nil {
		t.Fatalf("GetPublicBuybox(multi): %v", err)
	}
	t.Logf("PLID97129870 type=%s items=%d", bb.ItemsType, len(bb.Items))
	for _, it := range bb.Items {
		t.Logf("   price=%v sku=%v selected=%v", it.Price, AnyToString(it.SKU), it.IsSelected)
	}
	if bb.ItemsType != "multi" || len(bb.Items) < 2 {
		t.Fatalf("expected multi with >=2 items")
	}

	bb2, err := c.GetPublicBuybox("PLID60569699")
	if err != nil {
		t.Fatalf("GetPublicBuybox(summary): %v", err)
	}
	t.Logf("PLID60569699 type=%s items=%d", bb2.ItemsType, len(bb2.Items))

	bb3, err := c.GetPublicBuyboxForVariant("PLID60569699", "Green", "M")
	if err != nil {
		t.Fatalf("GetPublicBuyboxForVariant: %v", err)
	}
	t.Logf("PLID60569699 Green/M type=%s items=%d", bb3.ItemsType, len(bb3.Items))
	for _, it := range bb3.Items {
		t.Logf("   price=%v sku=%v selected=%v", it.Price, AnyToString(it.SKU), it.IsSelected)
	}
	if bb3.ItemsType == "summary" {
		t.Fatal("variant resolution failed: still summary")
	}
	if !bb3.HasOwnOffer("223345270", 517) {
		t.Fatalf("variant buybox should contain the Green/M offer; items=%+v", bb3.Items)
	}

	if _, err := c.GetCatalogueMPVByPLID("60569699"); err == nil {
		t.Log("GetCatalogueMPVByPLID succeeded without auth (unexpected but fine)")
	} else {
		t.Logf("GetCatalogueMPVByPLID without auth -> %v (endpoint reachable, auth required)", err)
	}
}
