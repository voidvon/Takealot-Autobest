package server

import (
	"fmt"
	"strings"

	"takealot/pkg/api"
)

// Manual price changes must honor saved limits without recalculating them.
// Resolve Offer IDs only when limits exist and the caller did not supply a TSIN.
func (s *Server) validateSellingPrice(storeID string, client *api.Client, offerID, tsin string, price int) error {
	if price <= 0 {
		return fmt.Errorf("实际售价必须是大于 0 的整数")
	}
	if s.db == nil {
		return fmt.Errorf("无法读取已保存的价格保护配置")
	}
	targets, err := s.db.LoadStoreTargets(storeID)
	if err != nil {
		return fmt.Errorf("读取价格保护配置失败: %w", err)
	}
	if len(targets) == 0 {
		return nil
	}
	tsin = strings.TrimSpace(tsin)
	if tsin == "" {
		offer, err := client.GetOfficialSingleOffer(offerID)
		if err != nil {
			return fmt.Errorf("无法确认商品规格，未提交改价: %w", err)
		}
		tsin = api.AnyToString(offer["tsin_id"])
		if tsin == "" || tsin == "0" {
			return fmt.Errorf("无法确认商品 TSIN，未提交改价")
		}
	}
	canonical := false
	for key := range targets {
		if strings.HasPrefix(key, tsin+"/") {
			canonical = true
			break
		}
	}
	for key, target := range targets {
		if (!strings.HasPrefix(key, tsin+"/") && key != tsin) || (key == tsin && canonical) {
			continue
		}
		if target.MinPrice > 0 && price < target.MinPrice {
			return fmt.Errorf("实际售价 R%d 低于该规格已保存的防亏底价 R%d，请在自动竞价中核对价格配置", price, target.MinPrice)
		}
		if target.MaxPrice > 0 && price > target.MaxPrice {
			return fmt.Errorf("实际售价 R%d 高于该规格已保存的封顶价 R%d，请在自动竞价中核对价格配置", price, target.MaxPrice)
		}
	}
	return nil
}
