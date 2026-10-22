package engine

import (
	"fmt"
	"strings"

	"takealot/pkg/api"
)

// buyboxPrices separates a verified variant price from an aggregate price.
// Buybox cards can show only fastest/cheapest offers; an incomplete set cannot
// establish the second-lowest price or prove that our store is the only seller.
func buyboxPrices(bb *api.PublicBuybox, tsinID string, sellerID int64, curPrice, competing int) (competitor int, solo bool, reason string) {
	if !bb.MatchesTSIN(tsinID) {
		return 0, false, "未确认具体 TSIN 的卖家报价，不使用汇总起价调价"
	}
	competitor, ownFound, identified := bb.PricesForSeller(sellerID)
	if !identified {
		return 0, false, "前台报价缺少卖家身份，本轮暂停调价，避免将自身旧价当成竞品价"
	}
	if !ownFound {
		return 0, false, fmt.Sprintf("前台未找到卖家 %d 的报价，本轮暂停调价，避免将自身旧价当成竞品价", sellerID)
	}
	solo = competing == 1 && bb.ItemsType == "single" && len(bb.Items) == 1
	if competitor > curPrice {
		sellers := make(map[string]struct{})
		for _, item := range bb.Items {
			if item.Price > 0 {
				sellers[strings.TrimPrefix(item.SellerID, "M")] = struct{}{}
			}
		}
		if competing <= 0 || competing > len(sellers) {
			return 0, false, fmt.Sprintf("卖家接口报价数 %d，前台确认 %d 个卖家，无法确认第二低价，本轮暂停调价", competing, len(sellers))
		}
	}
	if competitor == 0 && !solo {
		return 0, false, "未获取到独立竞品价，且未确认独家在售，本轮暂停调价"
	}
	return competitor, solo, ""
}
