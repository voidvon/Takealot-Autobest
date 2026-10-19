package api

import (
	"encoding/json"
	"testing"
)

// 真实前台响应片段：PLID97129870 有 2 个卖家报价（buybox_items_type = multi）。
const multiBuyboxFixture = `{
  "buybox": {
    "buybox_items_type": "multi",
    "items": [
      {"is_selected": true,  "price": 170, "sku": 223468279, "is_add_to_cart_available": true},
      {"is_selected": false, "price": 113, "sku": 265591416, "is_add_to_cart_available": true}
    ]
  }
}`

// 多变体商品未选定变体时只返回聚合价（buybox_items_type = summary）。
const summaryBuyboxFixture = `{
  "buybox": {
    "buybox_items_type": "summary",
    "items": [
      {"is_selected": true, "price": 514}
    ]
  }
}`

func decodeBuybox(t *testing.T, raw string) *PublicBuybox {
	t.Helper()
	var data struct {
		Buybox PublicBuybox `json:"buybox"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal buybox: %v", err)
	}
	return &data.Buybox
}

func TestCompetitorLowestPrice_ExcludesOwnBySKU(t *testing.T) {
	bb := decodeBuybox(t, multiBuyboxFixture)
	// 我方是 R170 的报价，最低竞品应为 R113。
	if got := bb.CompetitorLowestPrice("223468279", 170); got != 113 {
		t.Fatalf("CompetitorLowestPrice = %d, want 113", got)
	}
	// 我方是 R113 的报价，最低竞品应为 R170（这正是"跟随涨价"要用的数据）。
	if got := bb.CompetitorLowestPrice("265591416", 113); got != 170 {
		t.Fatalf("CompetitorLowestPrice = %d, want 170", got)
	}
}

func TestCompetitorLowestPrice_FallsBackToPriceWhenSKUUnknown(t *testing.T) {
	bb := decodeBuybox(t, multiBuyboxFixture)
	// 没有 SKU 时按当前售价剔除自身报价。
	if got := bb.CompetitorLowestPrice("", 170); got != 113 {
		t.Fatalf("CompetitorLowestPrice = %d, want 113", got)
	}
	if got := bb.CompetitorLowestPrice("", 113); got != 170 {
		t.Fatalf("CompetitorLowestPrice = %d, want 170", got)
	}
}

func TestCompetitorLowestPrice_SKUUnknownButNotOwnPriceDoesNotDropCompetitor(t *testing.T) {
	// 我方 SKU 不在列表中，但价格 R170；最低竞品 R113 不能被当成自己剔除。
	bb := decodeBuybox(t, multiBuyboxFixture)
	if got := bb.CompetitorLowestPrice("999999", 170); got != 113 {
		t.Fatalf("CompetitorLowestPrice = %d, want 113", got)
	}
}

func TestCompetitorLowestPrice_SummaryReturnsZero(t *testing.T) {
	bb := decodeBuybox(t, summaryBuyboxFixture)
	if got := bb.CompetitorLowestPrice("", 514); got != 0 {
		t.Fatalf("CompetitorLowestPrice = %d, want 0 for summary", got)
	}
}

func TestCompetitorLowestPrice_NilSafe(t *testing.T) {
	var bb *PublicBuybox
	if got := bb.CompetitorLowestPrice("", 100); got != 0 {
		t.Fatalf("CompetitorLowestPrice = %d, want 0", got)
	}
}

func TestIsSolo_TrueOnlyWhenSingleOwnOffer(t *testing.T) {
	bb := decodeBuybox(t, multiBuyboxFixture)
	if bb.IsSolo("223468279", 170) {
		t.Fatal("IsSolo should be false when there are 2 offers")
	}

	solo := &PublicBuybox{ItemsType: "single", Items: []PublicBuyboxItem{{Price: 170, SKU: float64(223468279)}}}
	if !solo.IsSolo("223468279", 170) {
		t.Fatal("IsSolo should be true for a single own offer matched by SKU")
	}
	if !solo.IsSolo("", 170) {
		t.Fatal("IsSolo should be true when matched by price")
	}
	if solo.IsSolo("111", 999) {
		t.Fatal("IsSolo should be false when the single offer is not ours")
	}
}

func TestPublicBuyboxJSONDecodesOfferDetail(t *testing.T) {
	bb := decodeBuybox(t, multiBuyboxFixture)
	if len(bb.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(bb.Items))
	}
	if !bb.Items[0].IsSelected {
		t.Fatal("first item should be selected")
	}
	if AnyToString(bb.Items[1].SKU) != "265591416" {
		t.Fatalf("SKU = %q, want 265591416", AnyToString(bb.Items[1].SKU))
	}
}

const catalogueMPVFixture = `[
  {
    "productlineId": 60569699,
    "title": "Mens Short Sleeve Serengeti 2-tone Bush Shirt",
    "variants": [
      {"tsinId": 63718424, "size": "M", "colorName": "Green", "competingOffers": 2, "bestPrice": 517},
      {"tsinId": 63718432, "size": "M", "colorName": "Stone", "competingOffers": 1, "bestPrice": 514}
    ]
  }
]`

func TestFindVariant(t *testing.T) {
	var products []CatalogueMPVProduct
	if err := json.Unmarshal([]byte(catalogueMPVFixture), &products); err != nil {
		t.Fatalf("unmarshal catalogue: %v", err)
	}

	v, ok := FindVariant(products, "63718424")
	if !ok {
		t.Fatal("FindVariant should locate tsin 63718424")
	}
	if v.ColorName != "Green" || v.Size != "M" {
		t.Fatalf("variant = %+v, want Green/M", v)
	}

	if _, ok := FindVariant(products, "999"); ok {
		t.Fatal("FindVariant should not match an unknown tsin")
	}
	if _, ok := FindVariant(products, ""); ok {
		t.Fatal("FindVariant should reject an empty tsin")
	}
}

func TestHasOwnOffer(t *testing.T) {
	bb := decodeBuybox(t, multiBuyboxFixture)
	if !bb.HasOwnOffer("223468279", 0) {
		t.Fatal("HasOwnOffer should match by SKU")
	}
	if !bb.HasOwnOffer("", 113) {
		t.Fatal("HasOwnOffer should match by price when SKU is unknown")
	}
	if bb.HasOwnOffer("999999", 777) {
		t.Fatal("HasOwnOffer should be false for an unrelated offer")
	}

	var nilBB *PublicBuybox
	if nilBB.HasOwnOffer("1", 1) {
		t.Fatal("HasOwnOffer should be nil-safe")
	}
}
