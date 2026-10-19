package api

import (
	"os"
	"testing"
)

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
