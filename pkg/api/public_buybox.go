package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Bound actual requests for one product line in a cycle. Colour-only summaries
// are included, while cached reads do not consume another request.
const maxPublicVariantRequests = 96

type publicVariantSelection struct{ colour, size string }

func (s publicVariantSelection) cacheKey(plid string) string {
	return plid + "?" + url.Values{"colour_variant": {s.colour}, "size": {s.size}}.Encode()
}

type VariantResolutionError struct {
	TSINID  string
	Checked int
	Limited bool
}

func (e *VariantResolutionError) Error() string {
	if e.Limited {
		return fmt.Sprintf("公开规格查询达到本轮 %d 次请求上限，未定位 TSIN %s", maxPublicVariantRequests, e.TSINID)
	}
	return fmt.Sprintf("未找到 TSIN %s 对应的可用独立报价（已检查 %d 个公开规格，可能已无货或前台未展示）", e.TSINID, e.Checked)
}

// PublicVariantSelector uses the same choices as the public product page.
// Pack sizes such as 1pc/2pcs are exposed as colour_variant, not necessarily
// as the colour name supplied by the seller catalogue.
type PublicVariantSelector struct {
	Type    string                `json:"selector_type"`
	Options []PublicVariantOption `json:"options"`
}

type PublicVariantOption struct {
	Enabled  bool               `json:"is_enabled"`
	Selected bool               `json:"is_selected"`
	Value    PublicVariantValue `json:"value"`
	LinkData struct {
		Fields map[string]any `json:"fields"`
	} `json:"link_data"`
}

// Colour choices contain {"value":"Black"}, while size choices can be "XS".
// Keep both forms usable by the TSIN resolver without guessing the variant.
type PublicVariantValue struct {
	Value any `json:"value"`
}

func (v *PublicVariantValue) UnmarshalJSON(raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if object, ok := value.(map[string]any); ok {
		value = object["value"]
	}
	switch value.(type) {
	case nil, string, float64:
		v.Value = value
		return nil
	default:
		return fmt.Errorf("不支持的前台规格值格式")
	}
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
	plid = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(plid)), "PLID")
	if cache == nil {
		cache = make(map[string]*PublicBuybox)
	}
	// Reuse any exact match already read for another monitored TSIN this cycle.
	prefix := plid + "?"
	requests := 0
	for key, bb := range cache {
		if strings.HasPrefix(key, prefix) {
			requests++
			if bb.MatchesTSIN(tsinID) {
				return bb, nil
			}
		}
	}
	remember := func(s publicVariantSelection, bb *PublicBuybox) {
		if !bb.HasOfferPrices() || AnyToString(bb.TSINID) == "" {
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.publicVariants == nil || len(c.publicVariants) >= 4096 {
			c.publicVariants = make(map[string]publicVariantSelection)
		}
		// Keep the selector only. Prices must always be read again next cycle.
		c.publicVariants[plid+"/"+AnyToString(bb.TSINID)] = s
	}
	fetch := func(s publicVariantSelection) (*PublicBuybox, error) {
		key := s.cacheKey(plid)
		if bb := cache[key]; bb != nil {
			return bb, nil
		}
		if requests >= maxPublicVariantRequests {
			return nil, &VariantResolutionError{TSINID: tsinID, Checked: requests, Limited: true}
		}
		requests++
		bb, err := c.GetPublicBuyboxForVariant(plid, s.colour, s.size)
		if err != nil {
			return nil, fmt.Errorf("读取公开规格报价失败: %w", err)
		}
		cache[key] = bb
		remember(s, bb)
		return bb, nil
	}
	identityKey := plid + "/" + tsinID
	c.mu.RLock()
	known, hasKnown := c.publicVariants[identityKey]
	c.mu.RUnlock()
	if hasKnown {
		bb, err := fetch(known)
		if err != nil {
			return nil, err
		}
		if bb.MatchesTSIN(tsinID) {
			return bb, nil
		}
		c.mu.Lock()
		delete(c.publicVariants, identityKey)
		c.mu.Unlock()
	}
	type candidate struct {
		selection publicVariantSelection
		buybox    *PublicBuybox
	}
	queue := []candidate{{buybox: initial}}
	seen := map[publicVariantSelection]bool{{}: true}
	checked := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.buybox == nil {
			var err error
			current.buybox, err = fetch(current.selection)
			if err != nil {
				return nil, err
			}
			checked++
		}
		if current.buybox.MatchesTSIN(tsinID) {
			return current.buybox, nil
		}
		// Follow one unchosen dimension at a time. A response's default
		// selected option must not hide the other sizes or colours.
		for _, selector := range current.buybox.Selectors {
			if selector.Type != "colour_variant" && selector.Type != "size" {
				continue
			}
			if selector.Type == "colour_variant" && current.selection.colour != "" || selector.Type == "size" && current.selection.size != "" {
				continue
			}
			var nextCandidates []candidate
			for _, option := range selector.Options {
				if !option.Enabled {
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
				if colour := AnyToString(option.LinkData.Fields["colour_variant"]); colour != "" {
					next.colour = colour
				}
				if size := AnyToString(option.LinkData.Fields["size"]); size != "" {
					next.size = size
				}
				if seen[next] {
					continue
				}
				seen[next] = true
				nextCandidates = append(nextCandidates, candidate{selection: next})
			}
			// Finish selecting this colour's sizes before visiting the next
			// colour. Do not request every size-only summary from the root.
			queue = append(nextCandidates, queue...)
			break
		}
	}
	return nil, &VariantResolutionError{TSINID: tsinID, Checked: checked}
}
