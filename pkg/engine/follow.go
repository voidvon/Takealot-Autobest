package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
)

// An imported link supplies defaults; a submitted item must identify one TSIN.
type FollowItem struct {
	URL          string `json:"url"`
	Stock        int    `json:"stock"`
	MinPrice     int    `json:"min_price"`
	MaxPrice     int    `json:"max_price"`
	SellingPrice int    `json:"selling_price"` // 0: calculate from this variant's latest market price
	TSINID       string `json:"tsin_id"`
}

type FollowVariant struct {
	FollowItem
	PLID       string `json:"plid"`
	Title      string `json:"title"`
	Colour     string `json:"colour"`
	Size       string `json:"size"`
	BestPrice  int    `json:"best_price"`
	CanFollow  bool   `json:"can_follow"`
	SkipReason string `json:"skip_reason"`
}

type FollowPreviewError struct {
	Row     int    `json:"row"`
	URL     string `json:"url"`
	Message string `json:"message"`
}

type FollowPreview struct {
	Variants []FollowVariant      `json:"variants"`
	Errors   []FollowPreviewError `json:"errors"`
}

func followPLID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if pos := strings.LastIndex(strings.ToUpper(id), "PLID"); pos >= 0 {
		id = id[pos+4:]
		if end := strings.IndexAny(id, "?&#/"); end >= 0 {
			id = id[:end]
		}
	}
	if !positiveFollowID(id) {
		return "", fmt.Errorf("请填写有效商品链接或 PLID；指定规格请另填 TSIN 列")
	}
	return id, nil
}

func positiveFollowID(raw string) bool {
	if raw == "" {
		return false
	}
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && id > 0
}

func validateFollowPrices(item FollowItem) error {
	if item.MinPrice <= 0 {
		return fmt.Errorf("请为每个勾选规格设置大于 0 的防亏底价")
	}
	if item.Stock <= 0 || item.MaxPrice < 0 || item.SellingPrice < 0 {
		return fmt.Errorf("库存必须大于 0，售价和封顶价不能为负数")
	}
	if item.MaxPrice > 0 && item.MaxPrice < item.MinPrice {
		return fmt.Errorf("封顶价不能低于防亏底价")
	}
	if item.SellingPrice > 0 && (item.SellingPrice < item.MinPrice || (item.MaxPrice > 0 && item.SellingPrice > item.MaxPrice)) {
		return fmt.Errorf("初始售价必须在该规格的底价与封顶价范围内")
	}
	return nil
}

func ValidateFollowItems(items []FollowItem) error {
	if len(items) == 0 {
		return fmt.Errorf("请先解析规格并勾选要上架的规格")
	}
	seen := make(map[string]bool)
	for i, item := range items {
		if _, err := followPLID(item.URL); err != nil {
			return fmt.Errorf("第 %d 个规格: %w", i+1, err)
		}
		if !positiveFollowID(item.TSINID) {
			return fmt.Errorf("第 %d 项缺少有效 TSIN，请先解析规格后再提交", i+1)
		}
		if seen[item.TSINID] {
			return fmt.Errorf("规格 TSIN %s 重复，请只保留一条配置", item.TSINID)
		}
		seen[item.TSINID] = true
		if err := validateFollowPrices(item); err != nil {
			return fmt.Errorf("TSIN %s: %w", item.TSINID, err)
		}
	}
	return nil
}

func followPrice(item FollowItem, bestPrice int) (int, error) {
	if err := validateFollowPrices(item); err != nil {
		return 0, err
	}
	if item.SellingPrice > 0 {
		return item.SellingPrice, nil
	}
	price := bestPrice - 1
	if price < item.MinPrice {
		price = item.MinPrice
	}
	if item.MaxPrice > 0 && price > item.MaxPrice {
		price = item.MaxPrice
	}
	return price, nil
}

func followEligibility(mpv api.MPVResult, plid string) string {
	if !positiveFollowID(api.AnyToString(mpv.TSINID)) || api.AnyToString(mpv.ProductlineID) != plid {
		return "规格身份不完整或不属于当前商品链接"
	}
	if strings.ToLower(api.AnyToString(mpv.HasOffer)) == "true" {
		return "本店已跟卖"
	}
	if strings.ToLower(api.AnyToString(mpv.HasOffer)) != "false" {
		return "无法确认本店是否已跟卖"
	}
	if strings.TrimSpace(mpv.GTIN) == "" {
		return "规格缺少 GTIN，无法上架"
	}
	return ""
}

// PreviewFollowBatch only reads catalogue data. Importing or previewing never creates offers.
func (e *Engine) PreviewFollowBatch(ctx context.Context, storeID string, items []FollowItem) (FollowPreview, error) {
	preview := FollowPreview{Variants: []FollowVariant{}, Errors: []FollowPreviewError{}}
	if e.db == nil || e.clientPool == nil {
		return preview, fmt.Errorf("店铺未连接")
	}
	store, err := e.db.GetStore(storeID)
	if err != nil || store == nil {
		return preview, fmt.Errorf("店铺不存在")
	}
	client := e.clientPool.GetOrCreate(store.ID, store.Authorization, store.ProxyURL).WithContext(ctx)
	type catalogue struct {
		results []api.MPVResult
		labels  []api.CatalogueMPVProduct
		err     error
	}
	cache := make(map[string]catalogue)
	seen := make(map[string]bool)
	for i, item := range items {
		if ctx.Err() != nil {
			return preview, ctx.Err()
		}
		addError := func(err error) {
			preview.Errors = append(preview.Errors, FollowPreviewError{Row: i + 1, URL: item.URL, Message: err.Error()})
		}
		plid, err := followPLID(item.URL)
		if err != nil {
			addError(err)
			continue
		}
		if item.TSINID != "" && !positiveFollowID(item.TSINID) {
			addError(fmt.Errorf("TSIN 必须是有效的数字规格编号"))
			continue
		}
		data, exists := cache[plid]
		if !exists {
			data.results, data.err = client.GetMPVByPLID(plid)
			if data.err == nil {
				// Labels are optional: a missing colour/size must not be inferred from price.
				data.labels, _ = client.GetCatalogueMPVByPLID(plid)
			}
			cache[plid] = data
		}
		if data.err != nil {
			addError(data.err)
			continue
		}
		matched := false
		for _, mpv := range data.results {
			tsin := api.AnyToString(mpv.TSINID)
			if item.TSINID != "" && item.TSINID != tsin {
				continue
			}
			matched = true
			if !positiveFollowID(tsin) || api.AnyToString(mpv.ProductlineID) != plid {
				addError(fmt.Errorf("返回规格 TSIN %s 的商品身份与 PLID %s 不符", tsin, plid))
				continue
			}
			if seen[tsin] {
				addError(fmt.Errorf("TSIN %s 重复，请删除重复导入行后重新解析", tsin))
				continue
			}
			seen[tsin] = true
			variant := FollowVariant{FollowItem: item, PLID: plid, BestPrice: int(mpv.BestPrice)}
			variant.TSINID = tsin
			variant.SkipReason = followEligibility(mpv, plid)
			variant.CanFollow = variant.SkipReason == ""
			if label, ok := api.FindVariant(data.labels, tsin); ok {
				variant.Title, variant.Colour, variant.Size = label.Title, label.ColorName, label.Size
				if variant.Colour == "" {
					variant.Colour = label.BasicColors
				}
			}
			if variant.Title == "" && len(data.labels) > 0 {
				variant.Title = data.labels[0].Title
			}
			preview.Variants = append(preview.Variants, variant)
		}
		if !matched {
			addError(fmt.Errorf("该链接未返回匹配的规格，请检查 PLID 和 TSIN"))
		}
	}
	return preview, nil
}

func (e *Engine) RunFollowBatch(storeID string, items []FollowItem) (bool, string) {
	if e.membershipChecker != nil && !e.membershipChecker.IsValid() {
		return false, "请登录并开通有效 VIP"
	}
	if err := ValidateFollowItems(items); err != nil {
		return false, err.Error()
	}
	if e.db == nil || e.clientPool == nil {
		return false, "店铺未连接"
	}
	if store, err := e.db.GetStore(storeID); err != nil || store == nil {
		return false, "店铺不存在"
	}
	w := e.getOrCreateWorker(storeID)
	w.mu.Lock()
	if w.followRunning {
		w.mu.Unlock()
		return false, "跟卖任务正在运行中"
	}
	w.followRunning = true
	w.mu.Unlock()
	go e.executeFollowBatch(w, append([]FollowItem(nil), items...))
	return true, "跟卖任务已在后台启动"
}

func (e *Engine) executeFollowBatch(w *StoreWorker, items []FollowItem) {
	defer func() {
		w.mu.Lock()
		w.followRunning = false
		w.mu.Unlock()
	}()
	store, err := e.db.GetStore(w.storeID)
	if err != nil || store == nil {
		e.Log("❌ 未找到店铺配置", "ERROR", w.storeID, w.name())
		return
	}
	client := e.clientPool.GetOrCreate(store.ID, store.Authorization, store.ProxyURL)
	e.Log(fmt.Sprintf("🛒 店铺 [%s] 开始逐规格跟卖，共 %d 个 TSIN", store.Name, len(items)), "INFO", store.ID, store.Name)
	successCount := 0
	for _, item := range items {
		if e.membershipChecker != nil && !e.membershipChecker.IsValid() {
			e.Log("会员验证失效，已停止跟卖任务", "WARN", store.ID, store.Name)
			return
		}
		plid, err := followPLID(item.URL)
		if err != nil || !positiveFollowID(item.TSINID) {
			continue
		}
		// Revalidate identity and availability at execution time. Client-supplied GTINs are never used.
		results, err := client.GetMPVByPLID(plid)
		if err != nil {
			e.Log(fmt.Sprintf("⚠️ 读取规格 TSIN %s 失败: %v", item.TSINID, err), "WARN", store.ID, store.Name)
			continue
		}
		var matched *api.MPVResult
		for _, mpv := range results {
			if api.AnyToString(mpv.TSINID) == item.TSINID {
				if matched != nil { // Ambiguous upstream identity must not produce an offer.
					matched = nil
					break
				}
				copy := mpv
				matched = &copy
			}
		}
		if matched == nil {
			e.Log(fmt.Sprintf("⚠️ 链接中未确认唯一 TSIN %s，跳过上架", item.TSINID), "WARN", store.ID, store.Name)
			continue
		}
		if reason := followEligibility(*matched, plid); reason != "" {
			e.Log(fmt.Sprintf("ℹ️ 跳过 TSIN %s: %s", item.TSINID, reason), "INFO", store.ID, store.Name)
			continue
		}
		price, err := followPrice(item, int(matched.BestPrice))
		if err != nil {
			e.Log(fmt.Sprintf("⚠️ TSIN %s 配置无效: %v", item.TSINID, err), "WARN", store.ID, store.Name)
			continue
		}
		ratio := store.RRPPercentage / 100
		if ratio < 1 {
			ratio = 1.2
		}
		rrp := int(float64(price) * ratio)
		if e.membershipChecker != nil && !e.membershipChecker.IsValid() {
			return
		}
		if err := client.CreateOffer(matched.GTIN, price, rrp, -1); err != nil {
			_ = e.db.RecordStoreFollow(store.ID, item.URL, item.TSINID, plid, matched.GTIN, item.Stock, item.MinPrice, "FAILED", err.Error())
			e.Log(fmt.Sprintf("❌ TSIN %s 上架失败: %v", item.TSINID, err), "ERROR", store.ID, store.Name)
			continue
		}
		successCount++
		w.mu.Lock()
		w.totalFollowed++
		w.mu.Unlock()
		key := item.TSINID + "/" + plid
		// Persist each success immediately, so a later membership failure cannot lose earlier floors.
		if err := e.db.SaveStoreTargets(store.ID, map[string]config.Target{key: {Selected: true, MinPrice: item.MinPrice, MaxPrice: item.MaxPrice}}); err != nil {
			e.Log(fmt.Sprintf("❌ TSIN %s 已上架，但独立底价保存失败，请手动补录: %v", item.TSINID, err), "ERROR", store.ID, store.Name)
			return
		}
		// Make the accepted offer visible immediately, even while the platform's
		// detailed offer list is still catching up. Imported stock is a plan, not
		// confirmed platform inventory; only a later platform sync supplies stock.
		if err := e.db.SaveStoreCachedOffers(store.ID, []db.CachedOffer{{
			Key: key, TSINID: item.TSINID, PLID: plid, Title: "TSIN " + item.TSINID,
			SellingPrice: price, RRP: rrp, DateModified: time.Now().UTC().Format(time.RFC3339),
		}}); err != nil {
			e.Log(fmt.Sprintf("⚠️ TSIN %s 已上架并保存底价，但列表缓存保存失败，请同步商品: %v", item.TSINID, err), "WARN", store.ID, store.Name)
		}
		message := fmt.Sprintf("售价: R%d, 底价: R%d, 封顶价: R%d, RRP: R%d", price, item.MinPrice, item.MaxPrice, rrp)
		_ = e.db.RecordStoreFollow(store.ID, item.URL, item.TSINID, plid, matched.GTIN, item.Stock, item.MinPrice, "SUCCESS", message)
		e.Log(fmt.Sprintf("🎉 成功上架 TSIN %s | %s", item.TSINID, message), "SUCCESS", store.ID, store.Name)
		time.Sleep(1500 * time.Millisecond)
	}
	e.Log(fmt.Sprintf("🏁 店铺 [%s] 批量跟卖完成，成功 %d 个规格，已分别保存监控底价", store.Name, successCount), "SUCCESS", store.ID, store.Name)
}
