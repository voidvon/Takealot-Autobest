package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
	"takealot/pkg/reporter"
)

type LogEntry struct {
	Time      string `json:"time"`
	Timestamp string `json:"timestamp,omitempty"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	StoreID   string `json:"store_id,omitempty"`
	StoreName string `json:"store_name,omitempty"`
}

type Status struct {
	StoreID              string  `json:"store_id"`
	StoreName            string  `json:"store_name"`
	IsRunning            bool    `json:"is_running"`
	IsPaused             bool    `json:"is_paused"`
	VerificationRequired bool    `json:"verification_required"`
	FollowRunning        bool    `json:"follow_running"`
	LastRunTime          *string `json:"last_run_time"`
	NextRunTime          *string `json:"next_run_time"`
	CountdownSeconds     int     `json:"countdown_seconds"`
	TotalChecked         int     `json:"total_checked"`
	TotalRepriced        int     `json:"total_repriced"`
	TotalFollowed        int     `json:"total_followed"`
}

type StoreWorker struct {
	storeID              string
	mu                   sync.RWMutex
	isRunning            bool
	isPaused             bool
	followRunning        bool
	cancelReprice        context.CancelFunc
	cancelCycle          context.CancelFunc
	verificationRequired bool
	wake                 chan struct{}

	lastRunTime   time.Time
	nextRunTime   time.Time
	totalChecked  int
	totalRepriced int
	totalFollowed int
	storeName     string
}

type MembershipChecker interface {
	IsValid() bool
}

func (w *StoreWorker) name() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.storeName
}

type Engine struct {
	cfgMgr            *config.Manager
	clientPool        *api.ClientPool
	db                *db.DB
	membershipChecker MembershipChecker
	reporter          reporter.Reporter

	workerMu sync.RWMutex
	workers  map[string]*StoreWorker

	logMu       sync.RWMutex
	logs        []LogEntry
	subscribers map[chan LogEntry]struct{}
}

func NewEngine(cfgMgr *config.Manager, clientPool *api.ClientPool, database *db.DB, membershipChecker MembershipChecker) *Engine {
	e := &Engine{
		cfgMgr:            cfgMgr,
		clientPool:        clientPool,
		db:                database,
		membershipChecker: membershipChecker,
		workers:           make(map[string]*StoreWorker),
		logs:              make([]LogEntry, 0, 500),
		subscribers:       make(map[chan LogEntry]struct{}),
	}
	if clientPool != nil {
		clientPool.SetLogger(func(storeID, message, level string) {
			e.Log(message, level, storeID)
		})
	}
	return e
}

func (e *Engine) SetReporter(r reporter.Reporter) {
	e.reporter = r
}

func (e *Engine) reportPriceFetchFailure(storeName, tsinID, plid string, err error) {
	if e.reporter == nil || err == nil {
		return
	}
	errStr := err.Error()
	if strings.Contains(errStr, "Cloudflare") || strings.Contains(errStr, "403") || strings.Contains(errStr, "验证页面") {
		e.reporter.ReportCloudflareBlock(storeName, tsinID, plid, errStr)
	} else {
		e.reporter.ReportPriceFetchError(storeName, tsinID, plid, errStr)
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
		wake:    make(chan struct{}, 1),
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
	now := time.Now()
	entry := LogEntry{
		Time:      now.Format("15:04:05"),
		Timestamp: now.Format(time.RFC3339),
		Level:     level,
		Message:   message,
	}
	if len(storeInfo) > 0 && storeInfo[0] != "" {
		entry.StoreID = storeInfo[0]
	}
	if len(storeInfo) > 1 && storeInfo[1] != "" {
		entry.StoreName = storeInfo[1]
	}

	e.logMu.Lock()
	defer e.logMu.Unlock()
	if len(e.logs) >= 500 {
		e.logs = e.logs[1:]
	}
	e.logs = append(e.logs, entry)
	for ch := range e.subscribers {
		// Sending is nonblocking and uses the same lock as channel closure.
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
	if _, subscribed := e.subscribers[ch]; subscribed {
		delete(e.subscribers, ch)
		close(ch)
	}
}

func (e *Engine) GetRecentLogs() []LogEntry {
	e.logMu.RLock()
	defer e.logMu.RUnlock()
	copied := make([]LogEntry, len(e.logs))
	copy(copied, e.logs)
	return copied
}

func (e *Engine) StartReprice(storeID string) (bool, string) {
	if e.db != nil {
		if store, err := e.db.GetStore(storeID); err != nil || store == nil {
			return false, "请先添加并选择自己的店铺"
		}
	}
	if e.membershipChecker != nil && !e.membershipChecker.IsValid() {
		e.Log("❌ 启动失败: 未登录、VIP 已失效或无法在线验证，请检查会员账号", "ERROR", storeID, "")
		return false, "未登录、VIP 已失效或无法在线验证，请检查会员账号"
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
	if w.verificationRequired {
		return false, "请先在访问会话中完成验证并检查连接"
	}
	w.isPaused = !w.isPaused
	statusStr := "已暂停"
	if !w.isPaused {
		statusStr = "已恢复"
	}
	e.Log(fmt.Sprintf("⏸️ 店铺 [%s] 监控状态变更: %s", w.storeName, statusStr), "WARN", w.storeID, w.storeName)
	return true, fmt.Sprintf("监控%s", statusStr)
}

// A verification pause cancels the current calculation. Clearing it only
// wakes an already-running worker; it never restarts a user-stopped task.
func (e *Engine) SetVerificationRequired(storeID string, required bool, message string) {
	w := e.getOrCreateWorker(storeID)
	w.mu.Lock()
	changed := w.verificationRequired != required
	w.verificationRequired = required
	cancel := w.cancelCycle
	w.nextRunTime = time.Time{}
	w.mu.Unlock()
	if required && cancel != nil {
		cancel()
	}
	if changed {
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
	if changed {
		e.Log(message, "INFO", storeID, w.name())
	}
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
	if e.membershipChecker != nil && !e.membershipChecker.IsValid() {
		e.Log("❌ 启动失败: 未登录、VIP 已失效或无法在线验证，请检查会员账号", "ERROR", "", "")
		return []string{"未登录、VIP 已失效或无法在线验证，请检查会员账号"}
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
	anyChecking := false
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
			if !w.isPaused && !w.verificationRequired {
				allPaused = false
				if w.nextRunTime.IsZero() {
					anyChecking = true
				}
			}
			if !w.nextRunTime.IsZero() {
				diff := int(time.Until(w.nextRunTime).Seconds())
				if diff >= 0 && diff < minCountdown {
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
	if earliestNext != nil && anyRunning && !anyChecking {
		s := earliestNext.Format("2006-01-02 15:04:05")
		nextStr = &s
	}
	if anyChecking {
		minCountdown = 0
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
		StoreID:              w.storeID,
		StoreName:            name,
		IsRunning:            w.isRunning,
		IsPaused:             w.isPaused || w.verificationRequired,
		VerificationRequired: w.verificationRequired,
		FollowRunning:        w.followRunning,
		LastRunTime:          lastStr,
		NextRunTime:          nextStr,
		CountdownSeconds:     countdown,
		TotalChecked:         w.totalChecked,
		TotalRepriced:        w.totalRepriced,
		TotalFollowed:        w.totalFollowed,
	}
}

func (e *Engine) repriceLoop(ctx context.Context, w *StoreWorker) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		w.mu.RLock()
		paused := w.isPaused || w.verificationRequired
		w.mu.RUnlock()
		if paused {
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		select {
		case <-w.wake:
		default:
		}

		interval := 5
		if e.db != nil {
			if st, err := e.db.GetStore(w.storeID); err == nil && st != nil && st.IntervalMinutes > 0 {
				interval = st.IntervalMinutes
			}
		}

		if e.membershipChecker != nil && !e.membershipChecker.IsValid() {
			name := w.name()
			e.Log(fmt.Sprintf("⚠️ 店铺 [%s] 检测到会员资格失效或无法在线验证，已自动停止调价任务", name), "ERROR", w.storeID, name)
			e.StopReprice(w.storeID)
			return
		}

		w.mu.Lock()
		w.lastRunTime = time.Now()
		w.nextRunTime = time.Time{} // No countdown while this cycle is still running.
		isPaused := w.isPaused
		cycleCtx, cancelCycle := context.WithCancel(ctx)
		w.cancelCycle = cancelCycle
		if w.verificationRequired {
			cancelCycle()
		}
		w.mu.Unlock()

		if !isPaused {
			e.executeRepriceCycle(cycleCtx, w)
		}
		cycleCanceled := cycleCtx.Err() != nil
		cancelCycle()

		w.mu.Lock()
		w.cancelCycle = nil
		if ctx.Err() != nil {
			w.mu.Unlock()
			return
		}
		if w.verificationRequired || cycleCanceled {
			w.mu.Unlock()
			continue
		}
		w.nextRunTime = time.Now().Add(time.Duration(interval) * time.Minute)
		nextRun, name := w.nextRunTime, w.storeName
		w.mu.Unlock()
		e.Log(fmt.Sprintf("⏳ 店铺 [%s] 等待下一轮巡检，计划时间: %s", name, nextRun.Format("15:04:05")), "INFO", w.storeID, name)
		timer := time.NewTimer(time.Until(nextRun))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-w.wake:
			timer.Stop()
		}
	}
}

func (e *Engine) executeRepriceCycle(ctx context.Context, w *StoreWorker) {
	if e.db == nil {
		return
	}

	store, err := e.db.GetStore(w.storeID)
	if err != nil || store == nil {
		e.Log(fmt.Sprintf("❌ 未找到店铺 [%s] 的配置信息", w.storeID), "ERROR", w.storeID, w.name())
		return
	}

	w.mu.Lock()
	w.storeName = store.Name
	w.mu.Unlock()
	apiClient := e.clientPool.GetOrCreate(store.ID, store.Authorization, store.ProxyURL).WithContext(ctx)

	targetsMap, err := e.db.LoadStoreTargets(w.storeID)
	if err != nil {
		e.Log(fmt.Sprintf("读取监控商品失败: %v", err), "ERROR", store.ID, store.Name)
		return
	}

	activeTargets := make(map[string]config.Target)
	canonicalTSINs := make(map[string]bool)
	for key := range targetsMap {
		if parts := strings.SplitN(key, "/", 2); len(parts) == 2 {
			canonicalTSINs[parts[0]] = true
		}
	}
	for k, v := range targetsMap {
		if canonicalTSINs[k] {
			continue // A stale TSIN-only alias must not bypass canonical limits/selection.
		}
		if v.Selected {
			if v.MinPrice <= 0 || v.MaxPrice < 0 || (v.MaxPrice > 0 && v.MaxPrice < v.MinPrice) {
				e.Log(fmt.Sprintf("🛡️ 商品 %s 未设置有效底价或价格范围冲突，跳过自动调价", k), "WARN", store.ID, store.Name)
				continue
			}
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
	// 卖家后台的 merchant SKU 与前台数字 SKU 不同，必须获取真实卖家 ID。
	var sellerID int64
	seller, sellerErr := apiClient.GetSellerInfo()
	if ctx.Err() != nil {
		return
	}
	if sellerErr == nil && seller != nil {
		sellerID = seller.SellerID
	}
	if sellerID <= 0 {
		e.Log(fmt.Sprintf("⚠️ 店铺 [%s] 未获取到卖家 ID，本轮暂停调价，避免误认自身报价（请求错误: %v）", store.Name, sellerErr), "WARN", w.storeID, store.Name)
		return
	}

	// 缓存店铺在线名称
	if store.Name == "默认店铺" || store.Name == "" {
		if seller.DisplayName != "" {
			w.mu.Lock()
			w.storeName = seller.DisplayName
			w.mu.Unlock()
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
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		e.Log(fmt.Sprintf("⚠️ 店铺 [%s] 商品列表未完整获取（已读取 %d 条）: %v", store.Name, len(rawOffers), err), "WARN", store.ID, store.Name)
	}
	if len(rawOffers) > 0 {
		// 平台售价可能延迟或由其他工具修改，不能据此推断商家意图并覆盖防亏底价。
		cachedItems := make([]db.CachedOffer, 0, len(rawOffers))
		for _, o := range rawOffers {
			tsinStr := strconv.FormatInt(o.TSINID, 10)
			offerMap[tsinStr] = o
			plidStr := api.AnyToString(o.TSIN.ProductlineID)
			sku := o.MerchantSKU
			if sku == "" {
				sku = o.SKU
			}
			curPlatformPrice := int(o.SellingPrice)
			key := fmt.Sprintf("%s/%s", tsinStr, plidStr)

			cachedItems = append(cachedItems, db.CachedOffer{
				StoreID:         w.storeID,
				Key:             key,
				TSINID:          tsinStr,
				SKU:             sku,
				PLID:            plidStr,
				Title:           o.TSIN.Title,
				SellingPrice:    curPlatformPrice,
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

	// 多变体商品的颜色/尺码解析缓存（按 PLID 缓存，避免同一轮内重复调用卖家目录接口）。
	variantCache := make(map[string][]api.CatalogueMPVProduct)
	publicVariantCache := make(map[string]*api.PublicBuybox)
	publicErrors := make(map[string]error)
	mpvRateLimited := false

	for key := range activeTargets {
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
			if ctx.Err() != nil {
				return
			}
			if err != nil || len(offerResp.Offers) == 0 {
				e.Log(fmt.Sprintf("⚠️ 无法读取监控商品报价，跳过 TSIN %s: %v", tsinID, err), "WARN", store.ID, store.Name)
				continue
			}
			if curOfferTSIN := api.AnyToString(offerResp.Offers[0].TSINID); curOfferTSIN != tsinID {
				e.Log(fmt.Sprintf("⚠️ 返回报价的 TSIN %s 与监控 TSIN %s 不符，跳过调价", curOfferTSIN, tsinID), "WARN", store.ID, store.Name)
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
			e.Log(fmt.Sprintf("同链接防竞争校验失败，跳过商品 (TSIN: %s): %v", tsinID, err), "ERROR", w.storeID, store.Name)
			continue
		}
		if owner != w.storeID {
			ownerName := owner
			if ownerStore, err := e.db.GetStore(owner); err == nil && ownerStore != nil {
				ownerName = ownerStore.Name
			}
			e.Log(fmt.Sprintf("同链接防竞争：商品 (TSIN: %s, PLID: %s) 由店铺 [%s] 负责改价，当前店铺自动跳过", tsinID, plid, ownerName), "INFO", w.storeID, store.Name)
			continue
		}

		w.mu.Lock()
		w.totalChecked++
		w.mu.Unlock()

		bestPrice := 0
		competing := 0 // 0 表示卖家接口未确认报价数量，不能据此判定独家。
		if !mpvRateLimited {
			mpv, err := apiClient.GetMPVByTSIN(tsinID)
			if err == nil && mpv != nil {
				bestPrice = int(mpv.BestPrice)
				competing = api.AnyToInt(mpv.CompetingOffers)
			} else if ctx.Err() == nil {
				var limited *api.RateLimitError
				if errors.As(err, &limited) {
					mpvRateLimited = true
					e.Log("⚠️ 卖家目录查询持续限流，本轮暂停后续 MPV 搜索；继续核对前台报价，无法确认报价完整或独家时跳过调价", "WARN", store.ID, store.Name)
				} else {
					e.Log(fmt.Sprintf("⚠️ 卖家目录查询失败 (TSIN: %s): %v", tsinID, err), "WARN", store.ID, store.Name)
				}
			}
		}
		if ctx.Err() != nil {
			return
		}

		// 通过前台商品详情页获取"不含自己"的最低竞品价。
		// 卖家 API 的 bestPrice 是市场最低价（自己就是最低价时它等于我方售价），
		// 仅凭它无法判断竞品是否涨价，因此必须补充前台买盒报价列表才能实现跟随涨价。
		competitorPrice := 0
		soloConfirmed := false
		quoteConfirmed := false
		if plid != "" {
			bb, bbErr := publicVariantCache[plid], publicErrors[plid]
			if bb == nil && bbErr == nil {
				bb, bbErr = apiClient.GetPublicBuybox(plid)
				if bbErr == nil {
					publicVariantCache[plid] = bb
				} else {
					publicErrors[plid] = bbErr // 同一轮不重复请求获取失败的商品主页。
				}
			}
			if bbErr == nil && bb != nil {
				if !bb.MatchesTSIN(tsinID) {
					if resolved, ok := e.resolveVariantBuybox(apiClient, plid, tsinID, variantCache); ok {
						bb = resolved
					} else {
						// 卖家目录的颜色名不一定等于前台的规格值，例如 1pc/2pcs/3pcs。
						bb, bbErr = apiClient.ResolvePublicBuyboxByTSIN(plid, tsinID, bb, publicVariantCache)
					}
				}
				if bbErr == nil {
					var quoteReason string
					competitorPrice, soloConfirmed, quoteReason = buyboxPrices(bb, tsinID, sellerID, curPrice, competing)
					quoteConfirmed = quoteReason == ""
					if quoteReason != "" {
						e.Log(fmt.Sprintf("ℹ️ [%s] %s (TSIN: %s)", truncate(title, 20), quoteReason, tsinID), "WARN", w.storeID, store.Name)
					}
					// 只允许具体 TSIN 的真实报价补充最低价，不采用跨规格的汇总起价。
					if bestPrice <= 0 && bb.MatchesTSIN(tsinID) {
						bestPrice = bb.CompetitorLowestPrice("", 0)
					}
				}
			}
			if ctx.Err() != nil {
				return
			}
			if bbErr != nil {
				e.Log(fmt.Sprintf("⚠️ [%s] 获取具体规格竞品报价失败 (TSIN: %s, PLID: %s): %v", truncate(title, 20), tsinID, plid, bbErr), "WARN", w.storeID, store.Name)
				e.reportPriceFetchFailure(store.Name, tsinID, plid, bbErr)
			}
		} else {
			e.Log(fmt.Sprintf("⚠️ [%s] 缺少 PLID，无法获取竞品报价 (TSIN: %s)", truncate(title, 20), tsinID), "WARN", w.storeID, store.Name)
		}
		// 抓价可能耗时较长，计算前读取最新底价及勾选状态，避免整轮沿用旧配置。
		target, err := e.db.LoadStoreTarget(w.storeID, key)
		if err != nil {
			e.Log(fmt.Sprintf("⚠️ 读取最新监控配置失败，跳过调价 (TSIN: %s): %v", tsinID, err), "WARN", w.storeID, store.Name)
			continue
		}
		if !target.Selected || target.MinPrice <= 0 || target.MaxPrice < 0 || (target.MaxPrice > 0 && target.MaxPrice < target.MinPrice) {
			continue
		}
		minPrice := target.MinPrice
		floorRecovery := curPrice > 0 && curPrice < minPrice && (target.MaxPrice <= 0 || minPrice <= target.MaxPrice)
		if !quoteConfirmed && !floorRecovery {
			// bestPrice 也包含自身报价且可能延迟更新，不能用它追降自己的旧价，
			// 也不把它写成已经确认的竞品价覆盖历史数据。
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if !quoteConfirmed {
			// 已确认本店具体 TSIN 的售价，恢复商家明确设置的底价不依赖竞品报价。
			// 未确认的市场数据不能驱动其他调价或写入竞品缓存。
			competitorPrice, bestPrice, competing = 0, 0, 0
			soloConfirmed = false
			e.Log(fmt.Sprintf("🛡️ 当前售价 R%d 低于已保存底价 R%d，仅执行底价回调 (TSIN: %s)", curPrice, minPrice, tsinID), "WARN", w.storeID, store.Name)
		}

		// 展示用的竞品参考价：优先使用真实竞品价，其次退回市场最低价。
		refPrice := bestPrice
		if competitorPrice > 0 {
			refPrice = competitorPrice
		}

		priority := "solo"
		diff := 0
		if competing > 1 || competitorPrice > 0 {
			if refPrice > 0 {
				if curPrice <= refPrice {
					priority = "winning"
				} else {
					priority = "losing"
					diff = curPrice - refPrice
				}
			} else {
				priority = "winning"
			}
		}
		if refPrice > 0 {
			_ = e.db.UpdateStoreSingleMPV(w.storeID, tsinID, refPrice, competing, priority, diff)
		}
		if bestPrice <= 0 && competitorPrice <= 0 && !soloConfirmed && !floorRecovery {
			e.Log(fmt.Sprintf("[%s] 未获取到竞品价格 (TSIN: %s)", truncate(title, 20), tsinID), "DEBUG", w.storeID, store.Name)
			time.Sleep(300 * time.Millisecond)
			continue
		}

		decision := decidePrice(curPrice, competitorPrice, bestPrice, minPrice, target.MaxPrice, decreaseStep, increaseStep, soloConfirmed)
		newPrice := decision.NewPrice
		actionDesc := decision.Reason

		if newPrice > 0 && newPrice != curPrice {
			if ctx.Err() != nil {
				return
			}
			rrp := int(float64(newPrice) * rrpRatio)
			if e.membershipChecker != nil && !e.membershipChecker.IsValid() {
				return
			}
			// 会员校验也可能等待网络，提交前再次确认，配置变化后丢弃本次旧决策。
			latest, err := e.db.LoadStoreTarget(w.storeID, key)
			if err != nil || !latest.Selected || latest.MinPrice != minPrice || latest.MaxPrice != target.MaxPrice || newPrice < latest.MinPrice {
				e.Log(fmt.Sprintf("⚠️ 提交前监控配置已变化或无法读取，跳过旧调价决策 (TSIN: %s)", tsinID), "WARN", w.storeID, store.Name)
				continue
			}
			if err := apiClient.UpdateOfferPrice(offerID, newPrice, rrp); err == nil {
				w.mu.Lock()
				w.totalRepriced++
				w.mu.Unlock()

				actionType := decision.Action
				if actionType == "" {
					actionType = "调价"
				}
				_ = e.db.RecordStoreReprice(w.storeID, key, tsinID, sku, title, imageURL, store.Name, actionType, curPrice, newPrice, refPrice, actionDesc)
				if quoteConfirmed && refPrice > 0 {
					priority, diff = "winning", 0
					if newPrice > refPrice {
						priority, diff = "losing", newPrice-refPrice
					}
					_ = e.db.UpdateStoreSingleMPV(w.storeID, tsinID, refPrice, competing, priority, diff)
				}
				_ = e.db.UpdateCachedOfferPrice(w.storeID, tsinID, newPrice, rrp)

				e.Log(fmt.Sprintf("✅ [调价成功] %s... (TSIN:%s) | 原价: R%d -> 新价: R%d (RRP: R%d) | 原因: %s",
					truncate(title, 22), tsinID, curPrice, newPrice, rrp, actionDesc), "SUCCESS", w.storeID, store.Name)
			} else {
				e.Log(fmt.Sprintf("❌ [调价失败] %s...: %v", truncate(title, 22), err), "ERROR", w.storeID, store.Name)
			}
		} else {
			e.Log(fmt.Sprintf("✓ [%s] 价格保持 R%d (TSIN: %s, 竞品 R%d, 市场最低 R%d, 底价 R%d, 封顶 R%d) | 原因: %s",
				truncate(title, 20), curPrice, tsinID, competitorPrice, bestPrice, minPrice, target.MaxPrice, actionDesc), "DEBUG", w.storeID, store.Name)
		}

		time.Sleep(300 * time.Millisecond)
	}

	w.mu.RLock()
	checked := w.totalChecked
	repriced := w.totalRepriced
	w.mu.RUnlock()
	e.Log(fmt.Sprintf("🏁 店铺 [%s] 本轮巡检完成！累计巡检: %d 次，累计改价: %d 次", store.Name, checked, repriced), "INFO", w.storeID, store.Name)
}

// resolveVariantBuybox 针对多变体商品，先通过卖家目录接口把我方 TSIN 映射到颜色/尺码，
// 再按具体变体查询前台买盒报价。
// 只接受 TSIN 一致的报价，不能通过价格推断规格。
func (e *Engine) resolveVariantBuybox(apiClient *api.Client, plid, tsinID string, cache map[string][]api.CatalogueMPVProduct) (*api.PublicBuybox, bool) {
	products, cached := cache[plid]
	if !cached {
		var err error
		products, err = apiClient.GetCatalogueMPVByPLID(plid)
		if err != nil {
			cache[plid] = nil // 本轮不再重试，避免反复打接口
			return nil, false
		}
		cache[plid] = products
	}
	if len(products) == 0 {
		return nil, false
	}

	variant, found := api.FindVariant(products, tsinID)
	if !found {
		return nil, false
	}
	colour := variant.ColorName
	if colour == "" {
		colour = variant.BasicColors
	}
	size := variant.Size
	if colour == "" && size == "" {
		return nil, false
	}

	bb, err := apiClient.GetPublicBuyboxForVariant(plid, colour, size)
	if err != nil || bb == nil {
		return nil, false
	}
	if !bb.MatchesTSIN(tsinID) {
		return nil, false
	}
	return bb, true
}

func truncate(str string, maxLen int) string {
	runes := []rune(str)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return str
}
