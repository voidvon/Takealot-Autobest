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
	Time      string `json:"time"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	StoreID   string `json:"store_id,omitempty"`
	StoreName string `json:"store_name,omitempty"`
}

type Status struct {
	StoreID          string  `json:"store_id"`
	StoreName        string  `json:"store_name"`
	IsRunning        bool    `json:"is_running"`
	IsPaused         bool    `json:"is_paused"`
	FollowRunning    bool    `json:"follow_running"`
	LastRunTime      *string `json:"last_run_time"`
	NextRunTime      *string `json:"next_run_time"`
	CountdownSeconds int     `json:"countdown_seconds"`
	TotalChecked     int     `json:"total_checked"`
	TotalRepriced    int     `json:"total_repriced"`
	TotalFollowed    int     `json:"total_followed"`
}

type StoreWorker struct {
	storeID       string
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
	storeName     string
}

type LicenseChecker interface {
	IsValid() bool
}

type Engine struct {
	cfgMgr     *config.Manager
	clientPool *api.ClientPool
	db         *db.DB
	licChecker LicenseChecker

	workerMu sync.RWMutex
	workers  map[string]*StoreWorker

	logMu       sync.RWMutex
	logs        []LogEntry
	subscribers map[chan LogEntry]struct{}
}

func NewEngine(cfgMgr *config.Manager, clientPool *api.ClientPool, database *db.DB, licChecker LicenseChecker) *Engine {
	return &Engine{
		cfgMgr:      cfgMgr,
		clientPool:  clientPool,
		db:          database,
		licChecker:  licChecker,
		workers:     make(map[string]*StoreWorker),
		logs:        make([]LogEntry, 0, 500),
		subscribers: make(map[chan LogEntry]struct{}),
	}
}

func (e *Engine) getOrCreateWorker(storeID string) *StoreWorker {
	if storeID == "" {
		storeID = "default"
	}
	e.workerMu.Lock()
	defer e.workerMu.Unlock()

	if w, ok := e.workers[storeID]; ok {
		return w
	}
	w := &StoreWorker{
		storeID: storeID,
	}
	if e.db != nil {
		if st, err := e.db.GetStore(storeID); err == nil && st != nil {
			w.storeName = st.Name
		}
	}
	e.workers[storeID] = w
	return w
}

func (e *Engine) Log(message, level string, storeInfo ...string) {
	entry := LogEntry{
		Time:    time.Now().Format("15:04:05"),
		Level:   level,
		Message: message,
	}
	if len(storeInfo) > 0 && storeInfo[0] != "" {
		entry.StoreID = storeInfo[0]
	}
	if len(storeInfo) > 1 && storeInfo[1] != "" {
		entry.StoreName = storeInfo[1]
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

func (e *Engine) StartReprice(storeID string) (bool, string) {
	if e.licChecker != nil && !e.licChecker.IsValid() {
		e.Log("❌ 启动失败: 软件未激活或授权已过期，请前往控制面板激活", "ERROR", storeID, "")
		return false, "软件未激活或授权已过期，请前往控制面板激活"
	}

	w := e.getOrCreateWorker(storeID)
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.isRunning {
		if w.isPaused {
			w.isPaused = false
			e.Log("▶️ 自动改价监控已从暂停状态恢复", "INFO", w.storeID, w.storeName)
			return true, "已恢复运行"
		}
		return false, "监控已在运行中"
	}

	w.isRunning = true
	w.isPaused = false
	ctx, cancel := context.WithCancel(context.Background())
	w.cancelReprice = cancel

	go e.repriceLoop(ctx, w)
	e.Log(fmt.Sprintf("🚀 启动店铺 [%s] 的自动调价监控任务...", w.storeName), "INFO", w.storeID, w.storeName)
	return true, "监控已成功启动"
}

func (e *Engine) PauseReprice(storeID string) (bool, string) {
	w := e.getOrCreateWorker(storeID)
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isRunning {
		return false, "监控未在运行"
	}
	w.isPaused = !w.isPaused
	statusStr := "已暂停"
	if !w.isPaused {
		statusStr = "已恢复"
	}
	e.Log(fmt.Sprintf("⏸️ 店铺 [%s] 监控状态变更: %s", w.storeName, statusStr), "WARN", w.storeID, w.storeName)
	return true, fmt.Sprintf("监控%s", statusStr)
}

func (e *Engine) StopReprice(storeID string) (bool, string) {
	w := e.getOrCreateWorker(storeID)
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.isRunning {
		return false, "监控当前未运行"
	}
	w.isRunning = false
	w.isPaused = false
	if w.cancelReprice != nil {
		w.cancelReprice()
	}
	w.nextRunTime = time.Time{}
	e.Log(fmt.Sprintf("⏹️ 店铺 [%s] 监控已停止", w.storeName), "WARN", w.storeID, w.storeName)
	return true, "已停止监控"
}

func (e *Engine) StartAll() []string {
	if e.licChecker != nil && !e.licChecker.IsValid() {
		e.Log("❌ 启动失败: 软件未激活或授权已过期，请前往控制面板激活", "ERROR", "", "")
		return []string{"软件未激活或授权已过期，请前往控制面板激活"}
	}

	var results []string
	if e.db == nil {
		return results
	}
	stores, err := e.db.GetStores()
	if err != nil {
		return results
	}
	for _, st := range stores {
		if st.IsActive {
			ok, msg := e.StartReprice(st.ID)
			if ok {
				results = append(results, fmt.Sprintf("[%s]: %s", st.Name, msg))
			}
		}
	}
	return results
}

func (e *Engine) StopAll() []string {
	var results []string
	if e.db == nil {
		return results
	}
	stores, err := e.db.GetStores()
	if err != nil {
		return results
	}
	for _, st := range stores {
		w := e.getOrCreateWorker(st.ID)
		w.mu.RLock()
		running := w.isRunning
		w.mu.RUnlock()
		if running {
			_, msg := e.StopReprice(st.ID)
			results = append(results, fmt.Sprintf("[%s]: %s", st.Name, msg))
		}
	}
	return results
}

func (e *Engine) PauseAll() []string {
	var results []string
	if e.db == nil {
		return results
	}
	stores, err := e.db.GetStores()
	if err != nil {
		return results
	}
	for _, st := range stores {
		w := e.getOrCreateWorker(st.ID)
		w.mu.RLock()
		running := w.isRunning
		w.mu.RUnlock()
		if running {
			_, msg := e.PauseReprice(st.ID)
			results = append(results, fmt.Sprintf("[%s]: %s", st.Name, msg))
		}
	}
	return results
}

func (e *Engine) GetAllStatus() Status {
	e.workerMu.RLock()
	defer e.workerMu.RUnlock()

	totalChecked := 0
	totalRepriced := 0
	totalFollowed := 0
	anyRunning := false
	allPaused := true
	minCountdown := 999999
	var latestLast *time.Time
	var earliestNext *time.Time

	activeCount := 0
	for _, w := range e.workers {
		w.mu.RLock()
		activeCount++
		totalChecked += w.totalChecked
		totalRepriced += w.totalRepriced
		totalFollowed += w.totalFollowed

		if w.isRunning {
			anyRunning = true
			if !w.isPaused {
				allPaused = false
			}
			if !w.nextRunTime.IsZero() {
				diff := int(time.Until(w.nextRunTime).Seconds())
				if diff > 0 && diff < minCountdown {
					minCountdown = diff
				}
				if earliestNext == nil || w.nextRunTime.Before(*earliestNext) {
					t := w.nextRunTime
					earliestNext = &t
				}
			}
		}
		if !w.lastRunTime.IsZero() {
			if latestLast == nil || w.lastRunTime.After(*latestLast) {
				t := w.lastRunTime
				latestLast = &t
			}
		}
		w.mu.RUnlock()
	}

	if activeCount == 0 || !anyRunning {
		allPaused = false
	}
	if minCountdown == 999999 {
		minCountdown = 0
	}

	var lastStr, nextStr *string
	if latestLast != nil {
		s := latestLast.Format("2006-01-02 15:04:05")
		lastStr = &s
	}
	if earliestNext != nil && anyRunning {
		s := earliestNext.Format("2006-01-02 15:04:05")
		nextStr = &s
	}

	return Status{
		StoreID:          "all",
		StoreName:        "全部店铺",
		IsRunning:        anyRunning,
		IsPaused:         allPaused,
		CountdownSeconds: minCountdown,
		TotalChecked:     totalChecked,
		TotalRepriced:    totalRepriced,
		TotalFollowed:    totalFollowed,
		LastRunTime:      lastStr,
		NextRunTime:      nextStr,
	}
}

func (e *Engine) GetStatus(storeID string) Status {
	if storeID == "all" {
		return e.GetAllStatus()
	}

	w := e.getOrCreateWorker(storeID)
	w.mu.RLock()
	defer w.mu.RUnlock()

	var lastStr, nextStr *string
	countdown := 0
	if !w.lastRunTime.IsZero() {
		s := w.lastRunTime.Format("2006-01-02 15:04:05")
		lastStr = &s
	}
	if !w.nextRunTime.IsZero() && w.isRunning {
		s := w.nextRunTime.Format("2006-01-02 15:04:05")
		nextStr = &s
		diff := time.Until(w.nextRunTime).Seconds()
		if diff > 0 {
			countdown = int(diff)
		}
	}

	name := w.storeName
	if name == "" && e.db != nil {
		if st, err := e.db.GetStore(w.storeID); err == nil && st != nil {
			name = st.Name
		}
	}

	return Status{
		StoreID:          w.storeID,
		StoreName:        name,
		IsRunning:        w.isRunning,
		IsPaused:         w.isPaused,
		FollowRunning:    w.followRunning,
		LastRunTime:      lastStr,
		NextRunTime:      nextStr,
		CountdownSeconds: countdown,
		TotalChecked:     w.totalChecked,
		TotalRepriced:    w.totalRepriced,
		TotalFollowed:    w.totalFollowed,
	}
}

func (e *Engine) repriceLoop(ctx context.Context, w *StoreWorker) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		interval := 5
		if e.db != nil {
			if st, err := e.db.GetStore(w.storeID); err == nil && st != nil && st.IntervalMinutes > 0 {
				interval = st.IntervalMinutes
			}
		}

		if e.licChecker != nil && !e.licChecker.IsValid() {
			e.Log(fmt.Sprintf("⚠️ 店铺 [%s] 检测到软件授权失效或已到期，已自动停止调价任务", w.storeName), "ERROR", w.storeID, w.storeName)
			e.StopReprice(w.storeID)
			return
		}

		w.mu.Lock()
		w.lastRunTime = time.Now()
		w.nextRunTime = w.lastRunTime.Add(time.Duration(interval) * time.Minute)
		isPaused := w.isPaused
		w.mu.Unlock()

		if !isPaused {
			e.executeRepriceCycle(ctx, w)
		}

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

func (e *Engine) executeRepriceCycle(ctx context.Context, w *StoreWorker) {
	if e.db == nil {
		return
	}

	store, err := e.db.GetStore(w.storeID)
	if err != nil || store == nil {
		e.Log(fmt.Sprintf("❌ 未找到店铺 [%s] 的配置信息", w.storeID), "ERROR", w.storeID, w.storeName)
		return
	}

	w.storeName = store.Name
	apiClient := e.clientPool.GetOrCreate(store.ID, store.Authorization, store.ProxyURL)

	targetsMap, err := e.db.LoadStoreTargets(w.storeID)
	if err != nil {
		return
	}

	activeTargets := make(map[string]config.Target)
	for k, v := range targetsMap {
		if v.Selected {
			activeTargets[k] = v
		}
	}

	if len(activeTargets) == 0 {
		e.Log(fmt.Sprintf("ℹ️ 店铺 [%s] 当前未勾选任何监控商品，跳过本轮改价", store.Name), "INFO", w.storeID, store.Name)
		return
	}

	e.Log(fmt.Sprintf("🔍 店铺 [%s] 开始执行新一轮调价巡检，已勾选监控商品: %d 个...", store.Name, len(activeTargets)), "INFO", w.storeID, store.Name)
	decreaseStep := store.PriceDecreaseStep
	increaseStep := store.PriceIncreaseStep
	rrpRatio := store.RRPPercentage / 100.0

	// 缓存店铺在线名称
	if w.storeName == "默认店铺" || w.storeName == "" {
		if seller, err := apiClient.GetSellerInfo(); err == nil && seller != nil && seller.DisplayName != "" {
			w.storeName = seller.DisplayName
			store.Name = seller.DisplayName
			_ = e.db.SaveStore(*store)
		}
	}

	maxFetch := store.MaxFetchOffers
	if maxFetch < len(activeTargets) {
		maxFetch = len(activeTargets) + 200
	}
	if maxFetch <= 0 {
		maxFetch = 1500
	}

	offerMap := make(map[string]api.OfferItem)
	rawOffers, err := apiClient.GetAllOffers(maxFetch)
	if err == nil && len(rawOffers) > 0 {
		cachedItems := make([]db.CachedOffer, 0, len(rawOffers))
		for _, o := range rawOffers {
			tsinStr := strconv.FormatInt(o.TSINID, 10)
			offerMap[tsinStr] = o
			plidStr := api.AnyToString(o.TSIN.ProductlineID)
			sku := o.MerchantSKU
			if sku == "" {
				sku = o.SKU
			}
			cachedItems = append(cachedItems, db.CachedOffer{
				StoreID:         w.storeID,
				Key:             fmt.Sprintf("%s/%s", tsinStr, plidStr),
				TSINID:          tsinStr,
				SKU:             sku,
				PLID:            plidStr,
				Title:           o.TSIN.Title,
				SellingPrice:    int(o.SellingPrice),
				RRP:             int(o.RRP),
				Stock:           o.TotalMerchantStock,
				DateModified:    o.DateModified,
				BestPrice:       0,
				CompetingOffers: 1,
				PriorityStatus:  "solo",
				PriceDiff:       0,
				ImageURL:        o.TSIN.ImageURL,
				ImageLargeURL:   api.GetLargeImageURL(o.TSIN.ImageURL),
			})
		}
		_ = e.db.SaveStoreCachedOffers(w.storeID, cachedItems)
	}

	for key, target := range activeTargets {
		select {
		case <-ctx.Done():
			return
		default:
		}

		w.mu.RLock()
		isPaused := w.isPaused
		w.mu.RUnlock()
		for isPaused {
			time.Sleep(1 * time.Second)
			w.mu.RLock()
			isPaused = w.isPaused
			w.mu.RUnlock()
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

		var curPrice int
		var offerID string
		var title string
		var sku string
		var imageURL string

		if o, ok := offerMap[tsinID]; ok {
			if id := api.AnyToString(o.TSIN.ProductlineID); id != "" && id != "0" {
				plid = id
			}
			offerID = strconv.FormatInt(o.OfferID, 10)
			curPrice = int(o.SellingPrice)
			title = o.TSIN.Title
			sku = o.MerchantSKU
			if sku == "" {
				sku = o.SKU
			}
			imageURL = o.TSIN.ImageURL
		} else {
			offerResp, err := apiClient.GetOffersPage(1, 1, tsinID)
			if err != nil || len(offerResp.Offers) == 0 {
				continue
			}
			curOffer := offerResp.Offers[0]
			if id := api.AnyToString(curOffer.TSIN.ProductlineID); id != "" && id != "0" {
				plid = id
			}
			offerID = strconv.FormatInt(curOffer.OfferID, 10)
			curPrice = int(curOffer.SellingPrice)
			title = curOffer.TSIN.Title
			sku = curOffer.MerchantSKU
			if sku == "" {
				sku = curOffer.SKU
			}
			imageURL = curOffer.TSIN.ImageURL
		}

		owner, err := e.db.ClaimRepriceProduct(w.storeID, tsinID, plid)
		if err != nil {
			e.Log(fmt.Sprintf("同链接防竞争校验失败，跳过商品 (TSIN: %s): %v", tsinID, err), "ERROR", w.storeID, w.storeName)
			continue
		}
		if owner != w.storeID {
			ownerName := owner
			if ownerStore, err := e.db.GetStore(owner); err == nil && ownerStore != nil {
				ownerName = ownerStore.Name
			}
			e.Log(fmt.Sprintf("同链接防竞争：商品 (TSIN: %s, PLID: %s) 由店铺 [%s] 负责改价，当前店铺自动跳过", tsinID, plid, ownerName), "INFO", w.storeID, w.storeName)
			continue
		}

		w.mu.Lock()
		w.totalChecked++
		w.mu.Unlock()

		mpv, err := apiClient.GetMPVByTSIN(tsinID)
		bestPrice := 0
		competing := 1
		if err == nil && mpv != nil {
			bestPrice = int(mpv.BestPrice)
			competing = api.AnyToInt(mpv.CompetingOffers)
		}

		if bestPrice <= 0 && plid != "" {
			bestPrice = apiClient.GetPublicPrice(plid)
		}

		priority := "solo"
		diff := 0
		if competing > 1 {
			if bestPrice > 0 {
				if curPrice <= bestPrice {
					priority = "winning"
				} else {
					priority = "losing"
					diff = curPrice - bestPrice
				}
			} else {
				priority = "winning"
			}
		}
		if bestPrice > 0 {
			_ = e.db.UpdateStoreSingleMPV(w.storeID, tsinID, bestPrice, competing, priority, diff)
		}

		if bestPrice <= 0 {
			e.Log(fmt.Sprintf("[%s] 未获取到竞品价格 (TSIN: %s)", truncate(title, 20), tsinID), "DEBUG", w.storeID, w.storeName)
			time.Sleep(300 * time.Millisecond)
			continue
		}

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

		if newPrice > 0 && newPrice != curPrice {
			rrp := int(float64(newPrice) * rrpRatio)
			if err := apiClient.UpdateOfferPrice(offerID, newPrice, rrp); err == nil {
				w.mu.Lock()
				w.totalRepriced++
				w.mu.Unlock()

				actionType := "跟降"
				if strings.Contains(actionDesc, "底价") {
					actionType = "底价保护"
				} else if newPrice > curPrice {
					actionType = "跟涨"
				}
				_ = e.db.RecordStoreReprice(w.storeID, key, tsinID, sku, title, imageURL, w.storeName, actionType, curPrice, newPrice, bestPrice, actionDesc)
				_ = e.db.UpdateStoreSingleMPV(w.storeID, tsinID, bestPrice, competing, "winning", 0)

				e.Log(fmt.Sprintf("✅ [调价成功] %s... (TSIN:%s) | 原价: R%d -> 新价: R%d (RRP: R%d) | 原因: %s",
					truncate(title, 22), tsinID, curPrice, newPrice, rrp, actionDesc), "SUCCESS", w.storeID, w.storeName)
			} else {
				e.Log(fmt.Sprintf("❌ [调价失败] %s...: %v", truncate(title, 22), err), "ERROR", w.storeID, w.storeName)
			}
		} else {
			e.Log(fmt.Sprintf("✓ [%s] 价格保持 R%d (竞品 R%d, 底价保护 R%d)",
				truncate(title, 20), curPrice, bestPrice, minPrice), "DEBUG", w.storeID, w.storeName)
		}

		time.Sleep(300 * time.Millisecond)
	}

	w.mu.RLock()
	checked := w.totalChecked
	repriced := w.totalRepriced
	w.mu.RUnlock()
	e.Log(fmt.Sprintf("🏁 店铺 [%s] 本轮巡检完成！累计巡检: %d 次，累计改价: %d 次", w.storeName, checked, repriced), "INFO", w.storeID, w.storeName)
}

type FollowItem struct {
	URL      string `json:"url"`
	Stock    int    `json:"stock"`
	MinPrice int    `json:"min_price"`
}

func (e *Engine) RunFollowBatch(storeID string, items []FollowItem) (bool, string) {
	w := e.getOrCreateWorker(storeID)
	w.mu.Lock()
	if w.followRunning {
		w.mu.Unlock()
		return false, "跟卖任务正在运行中"
	}
	w.followRunning = true
	w.mu.Unlock()

	go e.executeFollowBatch(w, items)
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
		e.Log("❌ 未找到店铺配置", "ERROR", w.storeID, w.storeName)
		return
	}
	apiClient := e.clientPool.GetOrCreate(store.ID, store.Authorization, store.ProxyURL)

	e.Log(fmt.Sprintf("🛒 店铺 [%s] 开始执行批量跟卖任务，共 %d 条目标...", store.Name, len(items)), "INFO", w.storeID, store.Name)
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

		e.Log(fmt.Sprintf("[%d/%d] 正在查询 PLID: %s 的商品变体...", idx+1, len(items), cleanPLID), "INFO", w.storeID, store.Name)
		results, err := apiClient.GetMPVByPLID(cleanPLID)
		if err != nil || len(results) == 0 {
			e.Log(fmt.Sprintf("⚠️ 未找到 PLID %s 对应商品或无返回", cleanPLID), "WARN", w.storeID, store.Name)
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

				if err := apiClient.CreateOffer(gtin, offerPrice, rrp, -1); err == nil {
					followedInPLID++
					successCount++
					w.mu.Lock()
					w.totalFollowed++
					w.mu.Unlock()

					key := fmt.Sprintf("%s/%s", tsinID, productlineID)
					newTargets[key] = config.Target{
						Selected: true,
						MinPrice: minPrice,
						MaxPrice: 0,
					}
					if e.db != nil {
						_ = e.db.RecordStoreFollow(w.storeID, item.URL, tsinID, productlineID, gtin, item.Stock, minPrice, "SUCCESS", fmt.Sprintf("售价: R%d, RRP: R%d", offerPrice, rrp))
					}
					e.Log(fmt.Sprintf("🎉 成功跟卖 GTIN:%s (TSIN:%s) | 售价: R%d, RRP: R%d", gtin, tsinID, offerPrice, rrp), "SUCCESS", w.storeID, store.Name)
				} else {
					if e.db != nil {
						_ = e.db.RecordStoreFollow(w.storeID, item.URL, tsinID, productlineID, gtin, item.Stock, minPrice, "FAILED", err.Error())
					}
					e.Log(fmt.Sprintf("❌ 跟卖失败 GTIN:%s: %v", gtin, err), "ERROR", w.storeID, store.Name)
				}
				time.Sleep(1500 * time.Millisecond)
			}
		}

		if followedInPLID == 0 {
			e.Log(fmt.Sprintf("ℹ️ PLID %s 全部变体均已有 Offer 或不可跟卖", cleanPLID), "DEBUG", w.storeID, store.Name)
		}
		time.Sleep(2 * time.Second)
	}

	if len(newTargets) > 0 && e.db != nil {
		_ = e.db.SaveStoreTargets(w.storeID, newTargets)
		e.Log(fmt.Sprintf("💾 已将新跟卖的 %d 个商品自动加入店铺 [%s] 监控列表并存入数据库", len(newTargets), store.Name), "SUCCESS", w.storeID, store.Name)
	}

	e.Log(fmt.Sprintf("🏁 店铺 [%s] 批量跟卖完成！成功上架 Offer: %d 个", store.Name, successCount), "SUCCESS", w.storeID, store.Name)
}

func truncate(str string, maxLen int) string {
	runes := []rune(str)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return str
}
