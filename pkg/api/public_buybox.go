package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// PublicVariantSelector uses the same choices as the public product page.
// Pack sizes such as 1pc/2pcs are exposed as colour_variant, not necessarily
// as the colour name supplied by the seller catalogue.
type PublicVariantSelector struct {
	Type    string                `json:"selector_type"`
	Options []PublicVariantOption `json:"options"`
}

type PublicVariantOption struct {
	Enabled  bool `json:"is_enabled"`
	Selected bool `json:"is_selected"`
	Value    struct {
		Value any `json:"value"`
	} `json:"value"`
	LinkData struct {
		Fields map[string]any `json:"fields"`
	} `json:"link_data"`
}

// HasOfferPrices rejects aggregate prices and unknown response formats.
func (b *PublicBuybox) HasOfferPrices() bool {
	return b != nil && (b.ItemsType == "single" || b.ItemsType == "multi") && len(b.Items) > 0
}

func (b *PublicBuybox) MatchesTSIN(tsinID string) bool {
	return b.HasOfferPrices() && tsinID != "" && AnyToString(b.TSINID) == tsinID
}

// PricesForSeller excludes our offers by seller identity, even while the public
// page still shows our previous price. Seller API SKUs are merchant-defined and
// are not the numeric SKU used by the public page. Never identify us by price.
func (b *PublicBuybox) PricesForSeller(sellerID int64) (competitor int, ownFound bool, identified bool) {
	if !b.HasOfferPrices() || sellerID <= 0 {
		return 0, false, false
	}
	for _, item := range b.Items {
		if item.Price <= 0 {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimPrefix(item.SellerID, "M"), 10, 64)
		if err != nil || id <= 0 {
			return 0, false, false
		}
		if id == sellerID {
			ownFound = true
			continue
		}
		price := int(item.Price)
		if competitor == 0 || price < competitor {
			competitor = price
		}
	}
	return competitor, ownFound, true
}

// ResolvePublicBuyboxByTSIN finds the requested variant using public selectors.
// Never infer a variant's identity from its price. The cache lives for one
// repricing cycle so another TSIN can reuse responses without keeping old prices.
func (c *Client) ResolvePublicBuyboxByTSIN(plid, tsinID string, initial *PublicBuybox, cache map[string]*PublicBuybox) (*PublicBuybox, error) {
	if strings.TrimSpace(tsinID) == "" || initial == nil {
		return nil, fmt.Errorf("缺少 TSIN 或前台商品数据")
	}
	if initial.MatchesTSIN(tsinID) {
		return initial, nil
	}
	type selection struct{ colour, size string }
	type candidate struct {
		selection
		buybox *PublicBuybox
	}
	queue := []candidate{{buybox: initial}}
	seen := map[selection]bool{{}: true}
	const maxVariants = 24
	checked := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, selector := range current.buybox.Selectors {
			if selector.Type != "colour_variant" && selector.Type != "size" {
				continue
			}
			for _, option := range selector.Options {
				if !option.Enabled || option.Selected {
					continue
				}
				next := current.selection
				value := AnyToString(option.LinkData.Fields[selector.Type])
				if value == "" {
					value = AnyToString(option.Value.Value)
				}
				if value == "" {
					continue
				}
				if selector.Type == "colour_variant" {
					next.colour = value
				} else {
					next.size = value
				}
				if seen[next] {
					continue
				}
				if checked >= maxVariants {
					return nil, fmt.Errorf("公开规格查询达到 %d 次上限，未定位 TSIN %s", maxVariants, tsinID)
				}
				seen[next] = true
				checked++
				key := plid + "?" + url.Values{"colour_variant": {next.colour}, "size": {next.size}}.Encode()
				bb := cache[key]
				if bb == nil {
					var err error
					bb, err = c.GetPublicBuyboxForVariant(plid, next.colour, next.size)
					if err != nil {
						return nil, fmt.Errorf("读取公开规格报价失败: %w", err)
					}
					if cache != nil {
						cache[key] = bb
					}
				}
				if bb.MatchesTSIN(tsinID) {
					return bb, nil
				}
				queue = append(queue, candidate{selection: next, buybox: bb})
			}
		}
	}
	return nil, fmt.Errorf("未找到 TSIN %s 对应的独立报价（已检查 %d 个公开规格）", tsinID, checked)
}
