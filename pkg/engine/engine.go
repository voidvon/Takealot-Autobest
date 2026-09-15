package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
)

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type Status struct {
	IsRunning        bool      `json:"is_running"`
	IsPaused         bool      `json:"is_paused"`
	FollowRunning    bool      `json:"follow_running"`
	LastRunTime      *string   `json:"last_run_time"`
	NextRunTime      *string   `json:"next_run_time"`
	CountdownSeconds int       `json:"countdown_seconds"`
	TotalChecked     int       `json:"total_checked"`
	TotalRepriced    int       `json:"total_repriced"`
	TotalFollowed    int       `json:"total_followed"`
}

type Engine struct {
	cfgMgr *config.Manager
	api    *api.Client
	db     *db.DB

	mu            sync.RWMutex
	isRunning     bool
	isPaused      bool
	followRunning bool
	cancelReprice context.CancelFunc

	lastRunTime   time.Time
	nextRunTime   time.Time
	totalChecked  int
	totalRepriced int
	totalFollowed int

	logMu       sync.RWMutex
	logs        []LogEntry
	subscribers map[chan LogEntry]struct{}
}

func NewEngine(cfgMgr *config.Manager, apiClient *api.Client, database *db.DB) *Engine {
	return &Engine{
		cfgMgr:      cfgMgr,
		api:         apiClient,
		db:          database,
		logs:        make([]LogEntry, 0, 500),
		subscribers: make(map[chan LogEntry]struct{}),
	}
}

func (e *Engine) Log(message, level string) {
	entry := LogEntry{
		Time:    time.Now().Format("15:04:05"),
		Level:   level,
		Message: message,
	}

	e.logMu.Lock()
	if len(e.logs) >= 500 {
		e.logs = e.logs[1:]
	}
	e.logs = append(e.logs, entry)
	subs := make([]chan LogEntry, 0, len(e.subscribers))
	for ch := range e.subscribers {
		subs = append(subs, ch)
	}
	e.logMu.Unlock()

	// Broadcast non-blocking
	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
}

func (e *Engine) SubscribeLogs() chan LogEntry {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	ch := make(chan LogEntry, 100)
	e.subscribers[ch] = struct{}{}
	return ch
}

func (e *Engine) UnsubscribeLogs(ch chan LogEntry) {
	e.logMu.Lock()
	defer e.logMu.Unlock()
	delete(e.subscribers, ch)
	close(ch)
}

func (e *Engine) GetRecentLogs() []LogEntry {
	e.logMu.RLock()
	defer e.logMu.RUnlock()
	copied := make([]LogEntry, len(e.logs))
	copy(copied, e.logs)
	return copied
}

func (e *Engine) StartReprice() (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.isRunning {
		if e.isPaused {
			e.isPaused = false
			e.Log("▶️ 自动改价监控已从暂停状态恢复", "INFO")
			return true, "已恢复运行"
		}
		return false, "监控已在运行中"
	}

	e.isRunning = true
	e.isPaused = false
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelReprice = cancel

	go e.repriceLoop(ctx)
	e.Log("🚀 启动自动调价监控任务...", "INFO")
	return true, "监控已成功启动"
}

func (e *Engine) PauseReprice() (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isRunning {
		return false, "监控未在运行"
	}
	e.isPaused = !e.isPaused
	statusStr := "已暂停"
	if !e.isPaused {
		statusStr = "已恢复"
	}
	e.Log(fmt.Sprintf("⏸️ 监控状态变更: %s", statusStr), "WARN")
	return true, fmt.Sprintf("监控%s", statusStr)
}

func (e *Engine) StopReprice() (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.isRunning {
		return false, "监控当前未运行"
	}
	e.isRunning = false
	e.isPaused = false
	if e.cancelReprice != nil {
		e.cancelReprice()
	}
	e.nextRunTime = time.Time{}
	e.Log("⏹️ 监控已停止", "WARN")
	return true, "已停止监控"
}

func (e *Engine) GetStatus() Status {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var lastStr, nextStr *string
	countdown := 0
	if !e.lastRunTime.IsZero() {
		s := e.lastRunTime.Format("2006-01-02 15:04:05")
		lastStr = &s
	}
	if !e.nextRunTime.IsZero() && e.isRunning {
		s := e.nextRunTime.Format("2006-01-02 15:04:05")
		nextStr = &s
		diff := time.Until(e.nextRunTime).Seconds()
		if diff > 0 {
			countdown = int(diff)
		}
	}

	return Status{
		IsRunning:        e.isRunning,
		IsPaused:         e.isPaused,
		FollowRunning:    e.followRunning,
		LastRunTime:      lastStr,
		NextRunTime:      nextStr,
		CountdownSeconds: countdown,
		TotalChecked:     e.totalChecked,
		TotalRepriced:    e.totalRepriced,
		TotalFollowed:    e.totalFollowed,
	}
}

func (e *Engine) repriceLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg := e.cfgMgr.Get()
		interval := cfg.IntervalMinutes
		if interval < 1 {
			interval = 5
		}

		e.mu.Lock()
		e.lastRunTime = time.Now()
		e.nextRunTime = e.lastRunTime.Add(time.Duration(interval) * time.Minute)
		isPaused := e.isPaused
		e.mu.Unlock()

		if !isPaused {
			e.executeRepriceCycle(ctx)
		}

		// Sleep in 1-second chunks for responsive cancellation
		sleepDuration := time.Duration(interval) * time.Minute
		ticker := time.NewTicker(1 * time.Second)
		startTime := time.Now()

	WaitLoop:
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				if time.Since(startTime) >= sleepDuration {
					ticker.Stop()
					break WaitLoop
				}
			}
		}
	}
}

func (e *Engine) executeRepriceCycle(ctx context.Context) {
	cfg := e.cfgMgr.Get()
	activeTargets := make(map[string]config.Target)
	for k, v := range cfg.Targets {
		if v.Selected {
			activeTargets[k] = v
		}
	}

	if len(activeTargets) == 0 {
		e.Log("ℹ️ 当前未勾选任何监控商品，跳过本轮改价（请在商品列表勾选监控项）", "INFO")
		return
	}

	e.Log(fmt.Sprintf("🔍 开始执行新一轮调价巡检，已勾选监控商品: %d 个...", len(activeTargets)), "INFO")
	decreaseStep := cfg.PriceDecreaseStep
	increaseStep := cfg.PriceIncreaseStep
	rrpRatio := cfg.RRPPercentage / 100.0

	for key, target := range activeTargets {
		select {
		case <-ctx.Done():
			return
		default:
		}

		e.mu.RLock()
		isPaused := e.isPaused
		e.mu.RUnlock()
		for isPaused {
			time.Sleep(1 * time.Second)
			e.mu.RLock()
			isPaused = e.isPaused
			e.mu.RUnlock()
			select {
			case <-ctx.Done():
				return
			default:
			}
		}

		parts := strings.Split(key, "/")
		tsinID := parts[0]
		plid := ""
		if len(parts) > 1 {
			plid = parts[1]
		}
		minPrice := target.MinPrice

		// 1. Fetch current offer status
		offerResp, err := e.api.GetOffersPage(1, 1, tsinID)
		if err != nil || len(offerResp.Offers) == 0 {
			continue
		}

		curOffer := offerResp.Offers[0]
		offerID := strconv.FormatInt(curOffer.OfferID, 10)
		curPrice := int(curOffer.SellingPrice)
		title := curOffer.TSIN.Title

		e.mu.Lock()
		e.totalChecked++
		e.mu.Unlock()

		// 2. Query competitor bestPrice from MPV catalog
		mpv, err := e.api.GetMPVByTSIN(tsinID)
		bestPrice := 0
		if err == nil && mpv != nil {
			bestPrice = int(mpv.BestPrice)
		}

		// Fallback to public price if MPV bestPrice is 0
		if bestPrice <= 0 && plid != "" {
			bestPrice = e.api.GetPublicPrice(plid)
		}

		if bestPrice <= 0 {
			e.Log(fmt.Sprintf("[%s] 未获取到竞品价格 (TSIN: %s)", truncate(title, 20), tsinID), "DEBUG")
			time.Sleep(1500 * time.Millisecond)
			continue
		}

		// 3. Repricing Decision
		newPrice := -1
		actionDesc := ""

		if bestPrice < curPrice {
			newPrice = bestPrice - decreaseStep
			if minPrice > 0 && newPrice < minPrice {
				newPrice = minPrice
				actionDesc = fmt.Sprintf("竞品低价 R%d，触碰底价保护 R%d", bestPrice, minPrice)
			} else {
				actionDesc = fmt.Sprintf("竞品低价 R%d，跟降 R%d 至 R%d", bestPrice, decreaseStep, newPrice)
			}
		} else if bestPrice >= curPrice {
			if bestPrice > curPrice+increaseStep {
				newPrice = bestPrice - increaseStep
				actionDesc = fmt.Sprintf("竞品价更高 R%d，跟涨 R%d 至 R%d", bestPrice, increaseStep, newPrice)
			}
		}

		// 4. Update if price changed
		if newPrice > 0 && newPrice != curPrice {
			rrp := int(float64(newPrice) * rrpRatio)
			if err := e.api.UpdateOfferPrice(offerID, newPrice, rrp); err == nil {
				e.mu.Lock()
				e.totalRepriced++
				e.mu.Unlock()
				if e.db != nil {
					_ = e.db.RecordReprice(key, tsinID, title, curPrice, newPrice, bestPrice, actionDesc)
				}
				e.Log(fmt.Sprintf("✅ [调价成功] %s... (TSIN:%s) | 原价: R%d -> 新价: R%d (RRP: R%d) | 原因: %s",
					truncate(title, 22), tsinID, curPrice, newPrice, rrp, actionDesc), "SUCCESS")
			} else {
				e.Log(fmt.Sprintf("❌ [调价失败] %s...: %v", truncate(title, 22), err), "ERROR")
			}
		} else {
			e.Log(fmt.Sprintf("✓ [%s] 价格保持 R%d (竞品 R%d, 底价保护 R%d)",
				truncate(title, 20), curPrice, bestPrice, minPrice), "DEBUG")
		}

		time.Sleep(2 * time.Second)
	}

	e.mu.RLock()
	checked := e.totalChecked
	repriced := e.totalRepriced
	e.mu.RUnlock()
	e.Log(fmt.Sprintf("🏁 本轮巡检完成！累计巡检: %d 次，累计改价: %d 次", checked, repriced), "INFO")
}

type FollowItem struct {
	URL      string `json:"url"`
	Stock    int    `json:"stock"`
	MinPrice int    `json:"min_price"`
}

func (e *Engine) RunFollowBatch(items []FollowItem) (bool, string) {
	e.mu.Lock()
	if e.followRunning {
		e.mu.Unlock()
		return false, "跟卖任务正在运行中"
	}
	e.followRunning = true
	e.mu.Unlock()

	go e.executeFollowBatch(items)
	return true, "跟卖任务已在后台启动"
}

func (e *Engine) executeFollowBatch(items []FollowItem) {
	defer func() {
		e.mu.Lock()
		e.followRunning = false
		e.mu.Unlock()
	}()

	e.Log(fmt.Sprintf("🛒 开始执行批量跟卖任务，共 %d 条目标...", len(items)), "INFO")
	successCount := 0
	newTargets := make(map[string]config.Target)

	for idx, item := range items {
		urlOrPLID := strings.TrimSpace(item.URL)
		minPrice := item.MinPrice

		cleanPLID := urlOrPLID
		if strings.Contains(strings.ToUpper(urlOrPLID), "PLID") {
			pos := strings.Index(strings.ToUpper(urlOrPLID), "PLID")
			sub := urlOrPLID[pos+4:]
			for _, delim := range []string{"?", "&", "/", "#"} {
				if idx := strings.Index(sub, delim); idx != -1 {
					sub = sub[:idx]
				}
			}
			cleanPLID = sub
		}

		if cleanPLID == "" {
			continue
		}

		e.Log(fmt.Sprintf("[%d/%d] 正在查询 PLID: %s 的商品变体...", idx+1, len(items), cleanPLID), "INFO")
		results, err := e.api.GetMPVByPLID(cleanPLID)
		if err != nil || len(results) == 0 {
			e.Log(fmt.Sprintf("⚠️ 未找到 PLID %s 对应商品或无返回", cleanPLID), "WARN")
			time.Sleep(1500 * time.Millisecond)
			continue
		}

		followedInPLID := 0
		for _, mpv := range results {
			hasOfferStr := fmt.Sprintf("%v", mpv.HasOffer)
			if strings.ToLower(hasOfferStr) == "false" {
				gtin := mpv.GTIN
				tsinID := api.AnyToString(mpv.TSINID)
				productlineID := api.AnyToString(mpv.ProductlineID)
				bestPrice := int(mpv.BestPrice)

				offerPrice := bestPrice - 1
				if offerPrice < minPrice && minPrice > 0 {
					offerPrice = minPrice
				}
				if offerPrice < 1 {
					offerPrice = 100
				}
				rrp := int(float64(offerPrice) * 1.2)

				if err := e.api.CreateOffer(gtin, offerPrice, rrp, -1); err == nil {
					followedInPLID++
					successCount++
					e.mu.Lock()
					e.totalFollowed++
					e.mu.Unlock()

					key := fmt.Sprintf("%s/%s", tsinID, productlineID)
					newTargets[key] = config.Target{
						Selected: true,
						MinPrice: minPrice,
						MaxPrice: 0,
					}
					if e.db != nil {
						_ = e.db.RecordFollow(item.URL, tsinID, productlineID, gtin, item.Stock, minPrice, "SUCCESS", fmt.Sprintf("售价: R%d, RRP: R%d", offerPrice, rrp))
					}
					e.Log(fmt.Sprintf("🎉 成功跟卖 GTIN:%s (TSIN:%s) | 售价: R%d, RRP: R%d", gtin, tsinID, offerPrice, rrp), "SUCCESS")
				} else {
					if e.db != nil {
						_ = e.db.RecordFollow(item.URL, tsinID, productlineID, gtin, item.Stock, minPrice, "FAILED", err.Error())
					}
					e.Log(fmt.Sprintf("❌ 跟卖失败 GTIN:%s: %v", gtin, err), "ERROR")
				}
				time.Sleep(1500 * time.Millisecond)
			}
		}

		if followedInPLID == 0 {
			e.Log(fmt.Sprintf("ℹ️ PLID %s 全部变体均已有 Offer 或不可跟卖", cleanPLID), "DEBUG")
		}
		time.Sleep(2 * time.Second)
	}

	if len(newTargets) > 0 {
		_ = e.cfgMgr.UpdateTargets(newTargets)
		e.Log(fmt.Sprintf("💾 已将新跟卖的 %d 个商品自动加入监控列表", len(newTargets)), "SUCCESS")
	}

	e.Log(fmt.Sprintf("🏁 批量跟卖完成！成功上架 Offer: %d 个", successCount), "SUCCESS")
}

func truncate(str string, maxLen int) string {
	runes := []rune(str)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return str
}
