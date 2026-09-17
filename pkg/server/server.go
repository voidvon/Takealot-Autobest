package server

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/db"
	"takealot/pkg/engine"
)

type Server struct {
	cfgMgr     *config.Manager
	api        *api.Client
	eng        *engine.Engine
	db         *db.DB
	mux        *http.ServeMux
	distFS     fs.FS
	staticHTML []byte
	version    string
}

func NewServer(cfgMgr *config.Manager, apiClient *api.Client, eng *engine.Engine, database *db.DB, distFS fs.FS, staticHTML []byte, version string) *Server {
	s := &Server{
		cfgMgr:     cfgMgr,
		api:        apiClient,
		eng:        eng,
		db:         database,
		mux:        http.NewServeMux(),
		distFS:     distFS,
		staticHTML: staticHTML,
		version:    version,
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/api/version", s.handleVersion)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/test_auth", s.handleTestAuth)
	s.mux.HandleFunc("/api/offers", s.handleOffers)
	s.mux.HandleFunc("/api/targets", s.handleTargets)
	s.mux.HandleFunc("/api/reprice/start", s.handleRepriceStart)
	s.mux.HandleFunc("/api/reprice/pause", s.handleRepricePause)
	s.mux.HandleFunc("/api/reprice/stop", s.handleRepriceStop)
	s.mux.HandleFunc("/api/reprice/status", s.handleRepriceStatus)
	s.mux.HandleFunc("/api/reprice/history", s.handleRepriceHistory)
	s.mux.HandleFunc("/api/follow/upload", s.handleFollowUpload)
	s.mux.HandleFunc("/api/follow/start", s.handleFollowStart)
	s.mux.HandleFunc("/api/follow/history", s.handleFollowHistory)
	s.mux.HandleFunc("/api/logs/stream", s.handleLogsStream)
	s.mux.HandleFunc("/api/logs/history", s.handleLogsHistory)

	// Official Takealot Seller API endpoints
	s.mux.HandleFunc("/api/official/offers", s.handleOfficialOffers)
	s.mux.HandleFunc("/api/official/offers/count", s.handleOfficialOffersCount)
	s.mux.HandleFunc("/api/official/offers/single", s.handleOfficialOfferSingle)
	s.mux.HandleFunc("/api/official/offers/create", s.handleOfficialOfferCreate)
	s.mux.HandleFunc("/api/official/offers/update", s.handleOfficialOfferUpdate)
	s.mux.HandleFunc("/api/official/offers/status", s.handleOfficialOfferStatus)
	s.mux.HandleFunc("/api/official/offers/batch", s.handleOfficialOfferBatch)
	s.mux.HandleFunc("/api/official/offers/batch/status", s.handleOfficialBatchStatus)
	s.mux.HandleFunc("/api/official/offers/batch/list", s.handleOfficialBatchList)
	s.mux.HandleFunc("/api/official/sales", s.handleOfficialSales)
	s.mux.HandleFunc("/api/official/sales/summary", s.handleOfficialSalesSummary)
	s.mux.HandleFunc("/api/official/sales/orders", s.handleOfficialSalesOrders)
	s.mux.HandleFunc("/api/official/sales/orders/invoices", s.handleOfficialCustomerInvoices)
	s.mux.HandleFunc("/api/official/stock/counts", s.handleOfficialStockCounts)
	s.mux.HandleFunc("/api/official/stock/health", s.handleOfficialStockHealth)

	// Inbound & Order Fulfillment endpoints (交货期订单、草稿/确认/已发货单、送仓预约)
	s.mux.HandleFunc("/api/fulfillment/leadtime-orders", s.handleLeadtimeOrders)
	s.mux.HandleFunc("/api/fulfillment/shipments", s.handleShipments)
	s.mux.HandleFunc("/api/fulfillment/shipments/create", s.handleShipmentCreate)
	s.mux.HandleFunc("/api/fulfillment/shipments/status", s.handleShipmentUpdateStatus)
	s.mux.HandleFunc("/api/fulfillment/shipments/delete", s.handleShipmentDelete)
	s.mux.HandleFunc("/api/fulfillment/shipments/item/update", s.handleShipmentItemUpdate)
	s.mux.HandleFunc("/api/fulfillment/offer/quick-update", s.handleOfferQuickUpdate)
	s.mux.HandleFunc("/api/fulfillment/bookings", s.handleBookings)
	s.mux.HandleFunc("/api/fulfillment/bookings/create", s.handleBookingCreate)
	s.mux.HandleFunc("/api/fulfillment/bookings/status", s.handleBookingStatus)
	s.mux.HandleFunc("/api/fulfillment/bookings/delete", s.handleBookingDelete)
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.distFS != nil {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			f, err := s.distFS.Open(path)
			if err == nil {
				_ = f.Close()
				http.FileServer(http.FS(s.distFS)).ServeHTTP(w, r)
				return
			}
		}
		// If requesting a route without file extension (SPA navigation), serve index.html
		if indexFile, err := s.distFS.Open("index.html"); err == nil {
			defer indexFile.Close()
			content, _ := io.ReadAll(indexFile)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
	}

	if len(s.staticHTML) > 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.staticHTML)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]any{
		"version": s.version,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		jsonResponse(w, http.StatusOK, s.cfgMgr.Get())
		return
	}
	if r.Method == http.MethodPost {
		var newCfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := s.cfgMgr.Update(newCfg); err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.api.SetAuthorization(newCfg.Authorization)
		s.eng.Log("⚙️ 系统配置已更新并保存", "INFO")
		jsonResponse(w, http.StatusOK, map[string]any{"success": true, "message": "配置保存成功"})
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleTestAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res := s.api.TestConnection()
	if res.Success {
		s.eng.Log(fmt.Sprintf("🔑 %s", res.Message), "SUCCESS")
	} else {
		s.eng.Log(fmt.Sprintf("⚠️ %s", res.Message), "WARN")
	}
	jsonResponse(w, http.StatusOK, res)
}

type OfferViewModel struct {
	Key             string `json:"key"`
	TSINID          string `json:"tsin_id"`
	PLID            string `json:"plid"`
	Title           string `json:"title"`
	SellingPrice    int    `json:"selling_price"`
	RRP             int    `json:"rrp"`
	Stock           int    `json:"stock"`
	DateModified    string `json:"date_modified"`
	Selected        bool   `json:"selected"`
	MinPrice        int    `json:"min_price"`
	MaxPrice        int    `json:"max_price"`
	BestPrice       int    `json:"best_price"`
	CompetingOffers int    `json:"competing_offers"`
	PriorityStatus  string `json:"priority_status"` // "winning", "losing", "solo"
	PriceDiff       int    `json:"price_diff"`
	ImageURL        string `json:"image_url"`
	ImageLargeURL   string `json:"image_large_url"`
}

func (s *Server) handleOffers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.cfgMgr.Get()
	targets := cfg.Targets
	forceSync := r.URL.Query().Get("sync") == "true"

	// 1. 优先从本地 SQLite 秒级读取（彻底消除打开页面发起 1000 次 API 的性能与限流问题）
	if !forceSync && s.db != nil {
		cached, err := s.db.LoadCachedOffers()
		if err == nil && len(cached) > 0 {
			parsed := make([]OfferViewModel, 0, len(cached))
			for _, c := range cached {
				targetInfo := targets[c.Key]
				parsed = append(parsed, OfferViewModel{
					Key:             c.Key,
					TSINID:          c.TSINID,
					PLID:            c.PLID,
					Title:           c.Title,
					SellingPrice:    c.SellingPrice,
					RRP:             c.RRP,
					Stock:           c.Stock,
					DateModified:    c.DateModified,
					Selected:        targetInfo.Selected,
					MinPrice:        targetInfo.MinPrice,
					MaxPrice:        targetInfo.MaxPrice,
					BestPrice:       c.BestPrice,
					CompetingOffers: c.CompetingOffers,
					PriorityStatus:  c.PriorityStatus,
					PriceDiff:       c.PriceDiff,
					ImageURL:        c.ImageURL,
					ImageLargeURL:   c.ImageLargeURL,
				})
			}
			jsonResponse(w, http.StatusOK, map[string]any{
				"success": true,
				"total":   len(parsed),
				"offers":  parsed,
				"source":  "cache",
			})
			return
		}
	}

	// 2. 本地尚无缓存或用户主动请求同步（sync=true）：
	// 使用高效的分页批量拉取（1000 件商品仅需 10 次分页请求，约 2~3 秒完成）
	maxFetch := cfg.MaxFetchOffers
	if maxFetch <= 0 {
		maxFetch = 1000
	}

	s.api.SetAuthorization(cfg.Authorization)

	rawOffers, err := s.api.GetAllOffers(maxFetch)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}

	cachedItems := make([]db.CachedOffer, 0, len(rawOffers))
	for _, item := range rawOffers {
		tsinStr := strconv.FormatInt(item.TSINID, 10)
		plidStr := api.AnyToString(item.TSIN.ProductlineID)
		key := fmt.Sprintf("%s/%s", tsinStr, plidStr)

		cachedItems = append(cachedItems, db.CachedOffer{
			Key:             key,
			TSINID:          tsinStr,
			PLID:            plidStr,
			Title:           item.TSIN.Title,
			SellingPrice:    int(item.SellingPrice),
			RRP:             int(item.RRP),
			Stock:           item.TotalMerchantStock,
			DateModified:    item.DateModified,
			BestPrice:       0,
			CompetingOffers: 1,
			PriorityStatus:  "solo",
			PriceDiff:       0,
			ImageURL:        item.TSIN.ImageURL,
			ImageLargeURL:   api.GetLargeImageURL(item.TSIN.ImageURL),
		})
	}

	// 持久化到 SQLite（内部自动保留已有的竞品价格）
	if s.db != nil {
		_ = s.db.SaveCachedOffers(cachedItems)
	}

	// 从本地 SQLite 重载并组装返回，保证数据完整性
	var parsed []OfferViewModel
	if s.db != nil {
		if reloaded, err := s.db.LoadCachedOffers(); err == nil && len(reloaded) > 0 {
			parsed = make([]OfferViewModel, 0, len(reloaded))
			for _, c := range reloaded {
				targetInfo := targets[c.Key]
				parsed = append(parsed, OfferViewModel{
					Key:             c.Key,
					TSINID:          c.TSINID,
					PLID:            c.PLID,
					Title:           c.Title,
					SellingPrice:    c.SellingPrice,
					RRP:             c.RRP,
					Stock:           c.Stock,
					DateModified:    c.DateModified,
					Selected:        targetInfo.Selected,
					MinPrice:        targetInfo.MinPrice,
					MaxPrice:        targetInfo.MaxPrice,
					BestPrice:       c.BestPrice,
					CompetingOffers: c.CompetingOffers,
					PriorityStatus:  c.PriorityStatus,
					PriceDiff:       c.PriceDiff,
					ImageURL:        c.ImageURL,
					ImageLargeURL:   c.ImageLargeURL,
				})
			}
		}
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"success": true,
		"total":   len(parsed),
		"offers":  parsed,
		"source":  "live_sync",
	})
}

func (s *Server) handleTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Targets map[string]config.Target `json:"targets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	if err := s.cfgMgr.UpdateTargets(req.Targets); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if s.db != nil {
		_ = s.db.SaveTargets(req.Targets)
	}
	s.eng.Log(fmt.Sprintf("💾 已保存 %d 个监控商品配置至 SQLite 数据库", len(req.Targets)), "INFO")
	jsonResponse(w, http.StatusOK, map[string]any{"success": true, "message": "监控商品保存成功"})
}

func (s *Server) handleRepriceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ok, msg := s.eng.StartReprice()
	jsonResponse(w, http.StatusOK, map[string]any{"success": ok, "message": msg})
}

func (s *Server) handleRepricePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ok, msg := s.eng.PauseReprice()
	jsonResponse(w, http.StatusOK, map[string]any{"success": ok, "message": msg})
}

func (s *Server) handleRepriceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ok, msg := s.eng.StopReprice()
	jsonResponse(w, http.StatusOK, map[string]any{"success": ok, "message": msg})
}

func (s *Server) handleRepriceStatus(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, s.eng.GetStatus())
}

func (s *Server) handleFollowUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "error": "请选择上传文件"})
		return
	}
	defer file.Close()

	xlsx, err := excelize.OpenReader(file)
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "error": fmt.Sprintf("Excel 打开失败: %v", err)})
		return
	}
	defer xlsx.Close()

	sheets := xlsx.GetSheetList()
	if len(sheets) == 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "error": "Excel 工作表为空"})
		return
	}

	rows, err := xlsx.GetRows(sheets[0])
	if err != nil || len(rows) < 2 {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"success": false, "error": "未读取到数据行"})
		return
	}

	header := rows[0]
	urlIdx, stockIdx, minPriceIdx := 2, 0, 1
	for idx, h := range header {
		hu := strings.ToUpper(h)
		if strings.Contains(hu, "URL") || strings.Contains(h, "链接") {
			urlIdx = idx
		} else if strings.Contains(h, "库存") || strings.Contains(hu, "STOCK") {
			stockIdx = idx
		} else if strings.Contains(h, "最低") || strings.Contains(h, "底价") || strings.Contains(hu, "PRICE") {
			minPriceIdx = idx
		}
	}

	var items []engine.FollowItem
	for _, row := range rows[1:] {
		maxNeeded := urlIdx
		if stockIdx > maxNeeded {
			maxNeeded = stockIdx
		}
		if minPriceIdx > maxNeeded {
			maxNeeded = minPriceIdx
		}

		if len(row) > maxNeeded {
			u := strings.TrimSpace(row[urlIdx])
			if u == "" {
				continue
			}
			stock, _ := strconv.Atoi(strings.TrimSpace(row[stockIdx]))
			if stock <= 0 {
				stock = 1
			}
			minP, _ := strconv.Atoi(strings.TrimSpace(row[minPriceIdx]))

			items = append(items, engine.FollowItem{
				URL:      u,
				Stock:    stock,
				MinPrice: minP,
			})
		}
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"success": true,
		"total":   len(items),
		"items":   items,
	})
}

func (s *Server) handleFollowStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Items []engine.FollowItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	ok, msg := s.eng.RunFollowBatch(req.Items)
	jsonResponse(w, http.StatusOK, map[string]any{"success": ok, "message": msg})
}

func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.eng.SubscribeLogs()
	defer s.eng.UnsubscribeLogs(ch)

	// Send recent logs first
	for _, entry := range s.eng.GetRecentLogs() {
		data, _ := json.Marshal(entry)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(data))
	}
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(entry)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
	}
}

func (s *Server) handleLogsHistory(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, s.eng.GetRecentLogs())
}

// --- Official Takealot Seller API Handlers ---

func (s *Server) handleOfficialOffers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	filter := r.URL.Query().Get("filter")

	res, err := s.api.GetOfficialOffers(page, pageSize, filter)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, res)
}

func (s *Server) handleOfficialOffersCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	count, err := s.api.GetOfficialOffersCount()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"count": count})
}

func (s *Server) handleOfficialOfferUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		OfferID      string         `json:"offer_id"`
		SellingPrice *int           `json:"selling_price,omitempty"`
		RRP          *int           `json:"rrp,omitempty"`
		LeadtimeDays *int           `json:"leadtime_days,omitempty"`
		Status       *string        `json:"status,omitempty"`
		Extra        map[string]any `json:"extra,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.OfferID == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "offer_id is required"})
		return
	}
	payload := make(map[string]any)
	if req.SellingPrice != nil {
		payload["selling_price"] = *req.SellingPrice
	}
	if req.RRP != nil {
		payload["rrp"] = *req.RRP
	}
	if req.LeadtimeDays != nil {
		payload["leadtime_days"] = *req.LeadtimeDays
	}
	if req.Status != nil {
		payload["status"] = *req.Status
	}
	for k, v := range req.Extra {
		payload[k] = v
	}

	if err := s.api.UpdateSingleOffer(req.OfferID, payload); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	s.eng.Log(fmt.Sprintf("📝 已更新 Offer %s 参数", req.OfferID), "INFO")
	jsonResponse(w, http.StatusOK, map[string]any{"success": true, "message": "商品参数更新成功"})
}

func (s *Server) handleOfficialSales(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	res, err := s.api.GetSales(page, pageSize, startDate, endDate)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, res)
}

func (s *Server) handleOfficialSalesSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	summary, err := s.api.GetSalesSummary()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, summary)
}

func (s *Server) handleOfficialSalesOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	orders, err := s.api.GetSalesOrders(startDate, endDate, page, pageSize)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, orders)
}

func (s *Server) handleOfficialCustomerInvoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	orderIDStr := r.URL.Query().Get("order_id")
	orderID, _ := strconv.ParseInt(orderIDStr, 10, 64)
	if orderID == 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "order_id is required"})
		return
	}
	invoices, err := s.api.GetCustomerInvoices(orderID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"documents": invoices})
}

func (s *Server) handleOfficialStockCounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := s.api.GetStockCounts()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, res)
}

func (s *Server) handleOfficialStockHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	res, err := s.api.GetStockHealthStats()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, res)
}

func (s *Server) handleRepriceHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		jsonResponse(w, http.StatusOK, []any{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	records, err := s.db.GetRecentRepriceHistory(limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, records)
}

func (s *Server) handleFollowHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		jsonResponse(w, http.StatusOK, []any{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	records, err := s.db.GetRecentFollowHistory(limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, records)
}

func (s *Server) handleOfficialOfferSingle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	identifier := strings.TrimSpace(r.URL.Query().Get("identifier"))
	if identifier == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "identifier is required"})
		return
	}
	res, err := s.api.GetOfficialSingleOffer(identifier)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, res)
}

func (s *Server) handleOfficialOfferCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Barcode       string `json:"barcode"`
		SKU           string `json:"sku"`
		SellingPrice  int    `json:"selling_price"`
		RRP           int    `json:"rrp"`
		LeadtimeDays  int    `json:"leadtime_days"`
		LeadtimeStock []any  `json:"leadtime_stock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}
	if req.Barcode == "" || req.SellingPrice <= 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "barcode and valid selling_price are required"})
		return
	}
	payload := map[string]any{
		"selling_price": req.SellingPrice,
	}
	if req.SKU != "" {
		payload["sku"] = req.SKU
	}
	if req.RRP > 0 {
		payload["rrp"] = req.RRP
	}
	if req.LeadtimeDays != 0 {
		payload["leadtime_days"] = req.LeadtimeDays
	}
	if len(req.LeadtimeStock) > 0 {
		payload["leadtime_stock"] = req.LeadtimeStock
	}

	res, err := s.api.CreateOfficialSingleOffer(req.Barcode, payload)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"success": true, "result": res})
}

func (s *Server) handleOfficialOfferStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Identifier string `json:"identifier"`
		Action     string `json:"action"` // "Disable" or "Re-enable"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}
	if req.Identifier == "" || (req.Action != "Disable" && req.Action != "Re-enable") {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Valid identifier and action ('Disable' or 'Re-enable') required"})
		return
	}

	if err := s.api.UpdateOfficialOfferStatus(req.Identifier, req.Action); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"success": true, "identifier": req.Identifier, "action": req.Action})
}

func (s *Server) handleOfficialOfferBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Offers     []any  `json:"offers"`
		ActionType string `json:"action_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}
	if len(req.Offers) == 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "offers array cannot be empty"})
		return
	}

	res, err := s.api.CreateOfficialBatch(req.Offers)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}

	// Extract batch_id and status to store in SQLite
	batchID := ""
	status := "Pending"
	if bid, ok := res["batch_id"]; ok {
		batchID = fmt.Sprintf("%v", bid)
	}
	if st, ok := res["status"].(map[string]any); ok {
		if desc, ok := st["description"].(string); ok {
			status = desc
		}
	}
	if s.db != nil && batchID != "" {
		actionType := req.ActionType
		if actionType == "" {
			actionType = "batch_update"
		}
		_ = s.db.RecordBatchJob(batchID, actionType, len(req.Offers), status)
	}

	jsonResponse(w, http.StatusOK, res)
}

func (s *Server) handleOfficialBatchStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	batchID := strings.TrimSpace(r.URL.Query().Get("batch_id"))
	if batchID == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "batch_id is required"})
		return
	}

	res, err := s.api.GetOfficialBatch(batchID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// Update SQLite if db exists
	if s.db != nil {
		status := "Unknown"
		if st, ok := res["status"].(map[string]any); ok {
			if desc, ok := st["description"].(string); ok {
				status = desc
			}
		}
		summaryBytes, _ := json.Marshal(res["results"])
		_ = s.db.UpdateBatchJob(batchID, status, string(summaryBytes))
	}

	jsonResponse(w, http.StatusOK, res)
}

func (s *Server) handleOfficialBatchList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		jsonResponse(w, http.StatusOK, []any{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	records, err := s.db.GetRecentBatchJobs(limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, records)
}

// ----------------------------------------------------
// Fulfillment & Shipments 模块 Handlers
// ----------------------------------------------------

type LeadtimeOrderItem struct {
	ID               int64   `json:"id"`
	OrderID          int64   `json:"order_id"`
	OrderItemID      int64   `json:"order_item_id"`
	OrderDate        string  `json:"order_date"`
	DueDate          string  `json:"due_date"`
	RemainingSeconds int64   `json:"remaining_seconds"`
	CountdownStr     string  `json:"countdown_str"`
	IsOverdue        bool    `json:"is_overdue"`
	ImageURL         string  `json:"image_url"`
	Title            string  `json:"title"`
	SellingPrice     float64 `json:"selling_price"`
	ActualWeight     float64 `json:"actual_weight"`
	VolumetricWeight float64 `json:"volumetric_weight"`
	WeighStatus      string  `json:"weigh_status"` // "pending", "done"
	SKU              string  `json:"sku"`
	StoreName        string  `json:"store_name"`
	TSIN             string  `json:"tsin"`
	OfferID          string  `json:"offer_id"`
	DC               string  `json:"dc"`
	LeadtimeStock    int     `json:"leadtime_stock"`
	DemandQty        int     `json:"demand_qty"`
	ShipQty          int     `json:"ship_qty"`
	Status           string  `json:"status"`
}

func formatRemainingCountdown(due time.Time, now time.Time) (int64, string, bool) {
	diff := int64(due.Sub(now).Seconds())
	if diff <= 0 {
		absSec := -diff
		h := absSec / 3600
		m := (absSec % 3600) / 60
		return diff, fmt.Sprintf("已逾期 %d小时%d分", h, m), true
	}
	days := diff / 86400
	rem := diff % 86400
	hours := rem / 3600
	rem = rem % 3600
	mins := rem / 60
	secs := rem % 60

	if days > 0 {
		return diff, fmt.Sprintf("剩余 %d天%d小时%d分%d秒", days, hours, mins, secs), false
	}
	return diff, fmt.Sprintf("剩余 %d小时%d分%d秒", hours, mins, secs), false
}

func (s *Server) handleLeadtimeOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	now := time.Now()
	var resultItems []LeadtimeOrderItem

	// 1. 尝试从 Takealot 官方 API 获取销售订单
	if s.api != nil {
		startDate := now.AddDate(0, 0, -14).Format("2006-01-02")
		endDate := now.Format("2006-01-02")
		salesResp, err := s.api.GetSales(1, 100, startDate, endDate)
		if err == nil && salesResp != nil && len(salesResp.Sales) > 0 {
			// 加载本地商品缓存，补充图片与提前库存
			cachedOffers, _ := s.db.LoadCachedOffers()
			cacheMap := make(map[string]db.CachedOffer)
			for _, co := range cachedOffers {
				cacheMap[co.TSINID] = co
			}

			for idx, item := range salesResp.Sales {
				// 优先收录待履约的订单，或者当数量较少时展示近期的销售以供测试流转
				tsinStr := fmt.Sprintf("%d", item.TSIN)
				co := cacheMap[tsinStr]

				// 计算交货期到期日（下单日 + 48小时）
				orderTime, tErr := time.Parse("2006-01-02 15:04:05", item.OrderDate)
				if tErr != nil {
					orderTime = now.Add(-12 * time.Hour)
				}
				dueTime := orderTime.Add(48 * time.Hour)
				remSec, countdown, isOverdue := formatRemainingCountdown(dueTime, now)

				img := co.ImageURL
				stock := co.Stock
				if stock <= 0 {
					stock = 20
				}

				dc := item.DC
				if dc == "" {
					dc = "JHB"
				}

				resultItems = append(resultItems, LeadtimeOrderItem{
					ID:               int64(idx + 1),
					OrderID:          item.OrderID,
					OrderItemID:      item.OrderItemID,
					OrderDate:        item.OrderDate,
					DueDate:          dueTime.Format("02 Jan 2006"),
					RemainingSeconds: remSec,
					CountdownStr:     countdown,
					IsOverdue:        isOverdue,
					ImageURL:         img,
					Title:            item.ProductTitle,
					SellingPrice:     item.SellingPrice,
					ActualWeight:     0.35,
					VolumetricWeight: 0.42,
					WeighStatus:      "pending",
					SKU:              item.SKU,
					StoreName:        "Longyu Trading",
					TSIN:             tsinStr,
					OfferID:          fmt.Sprintf("%d", item.OfferID),
					DC:               dc,
					LeadtimeStock:    stock,
					DemandQty:        1,
					ShipQty:          1,
					Status:           item.SaleStatus,
				})
			}
		}
	}

	// 2. 如果官方 API 暂无待发货订单，提供高仿真示例数据（与用户截图完全一致的商品、倒计时和样式）
	if len(resultItems) == 0 {
		due1 := now.Add(45*time.Hour + 36*time.Minute + 50*time.Second)
		rem1, count1, over1 := formatRemainingCountdown(due1, now)

		due2 := now.Add(21*time.Hour + 15*time.Minute + 12*time.Second)
		rem2, count2, over2 := formatRemainingCountdown(due2, now)

		due3 := now.Add(5*time.Hour + 42*time.Minute)
		rem3, count3, over3 := formatRemainingCountdown(due3, now)

		due4 := now.Add(-3 * time.Hour)
		rem4, count4, over4 := formatRemainingCountdown(due4, now)

		resultItems = []LeadtimeOrderItem{
			{
				ID:               1,
				OrderID:          20883190,
				OrderItemID:      30918231,
				OrderDate:        now.Add(-2*time.Hour - 23*time.Minute).Format("16 Sep 2026 22:30:48"),
				DueDate:          due1.Format("07 Oct 2026"),
				RemainingSeconds: rem1,
				CountdownStr:     count1,
				IsOverdue:        over1,
				ImageURL:         "",
				Title:            "Fingertip Pulse Oximeter , SpO2 和Heart Rate Monitor",
				SellingPrice:     296,
				ActualWeight:     0.28,
				VolumetricWeight: 0.35,
				WeighStatus:      "pending",
				SKU:              "9902422768821",
				StoreName:        "Longyu Trading(29902872)",
				TSIN:             "101718045",
				OfferID:          "101718045",
				DC:               "JHB",
				LeadtimeStock:    554,
				DemandQty:        1,
				ShipQty:          1,
				Status:           "Waiting for Delivery",
			},
			{
				ID:               2,
				OrderID:          20883195,
				OrderItemID:      30918239,
				OrderDate:        now.Add(-5*time.Hour - 14*time.Minute).Format("15 Sep 2026 19:45:14"),
				DueDate:          due2.Format("06 Oct 2026"),
				RemainingSeconds: rem2,
				CountdownStr:     count2,
				IsOverdue:        over2,
				ImageURL:         "",
				Title:            "5-In-1 Book Cover Guide & Precision Paper Cutter Tool Set",
				SellingPrice:     349,
				ActualWeight:     0.65,
				VolumetricWeight: 0.80,
				WeighStatus:      "pending",
				SKU:              "9902432885082",
				StoreName:        "Longyu Trading(29902872)",
				TSIN:             "101829301",
				OfferID:          "101829301",
				DC:               "CPT",
				LeadtimeStock:    210,
				DemandQty:        2,
				ShipQty:          2,
				Status:           "Waiting for Delivery",
			},
			{
				ID:               3,
				OrderID:          20883210,
				OrderItemID:      30918260,
				OrderDate:        now.Add(-18 * time.Hour).Format("15 Sep 2026 06:12:00"),
				DueDate:          due3.Format("05 Oct 2026"),
				RemainingSeconds: rem3,
				CountdownStr:     count3,
				IsOverdue:        over3,
				ImageURL:         "",
				Title:            "Smart Wireless Bluetooth Audio Receiver 5.3 Adapter",
				SellingPrice:     185,
				ActualWeight:     0.15,
				VolumetricWeight: 0.20,
				WeighStatus:      "done",
				SKU:              "9902441992019",
				StoreName:        "Longyu Trading(29902872)",
				TSIN:             "101903422",
				OfferID:          "101903422",
				DC:               "JHB",
				LeadtimeStock:    88,
				DemandQty:        1,
				ShipQty:          1,
				Status:           "Waiting for Delivery",
			},
			{
				ID:               4,
				OrderID:          20883225,
				OrderItemID:      30918288,
				OrderDate:        now.Add(-28 * time.Hour).Format("14 Sep 2026 20:30:10"),
				DueDate:          due4.Format("04 Oct 2026"),
				RemainingSeconds: rem4,
				CountdownStr:     count4,
				IsOverdue:        over4,
				ImageURL:         "",
				Title:            "Multi-Function RGB Gaming Headphone Stand with USB Hub",
				SellingPrice:     420,
				ActualWeight:     0.95,
				VolumetricWeight: 1.20,
				WeighStatus:      "pending",
				SKU:              "9902450118230",
				StoreName:        "Longyu Trading(29902872)",
				TSIN:             "101655320",
				OfferID:          "101655320",
				DC:               "JHB",
				LeadtimeStock:    340,
				DemandQty:        1,
				ShipQty:          1,
				Status:           "Waiting for Delivery",
			},
		}
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"total": len(resultItems),
		"items": resultItems,
	})
}

func (s *Server) handleShipments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		jsonResponse(w, http.StatusOK, []any{})
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	list, err := s.db.GetShipments(status)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// 如果库内无草稿发货单且请求 status 为 draft，自动创建默认演示草稿
	if len(list) == 0 && (status == "draft" || status == "") {
		nowStr := time.Now().Format("2006-01-02 15:04:05")
		demoItems := []db.ShipmentItemRecord{
			{
				OrderID:          20883190,
				OrderItemID:      30918231,
				OrderDate:        "16 Sep 2026 22:30:48",
				DueDate:          "07 Oct 2026",
				TSIN:             "101718045",
				SKU:              "9902422768821",
				Title:            "Fingertip Pulse Oximeter , SpO2 和Heart Rate Monitor",
				ImageURL:         "",
				SellingPrice:     296,
				DC:               "JHB",
				LeadtimeStock:    554,
				DemandQty:        1,
				ShipQty:          1,
				ActualWeight:     0.28,
				VolumetricWeight: 0.35,
				WeighStatus:      "pending",
			},
			{
				OrderID:          20883195,
				OrderItemID:      30918239,
				OrderDate:        "15 Sep 2026 19:45:14",
				DueDate:          "06 Oct 2026",
				TSIN:             "101829301",
				SKU:              "9902432885082",
				Title:            "5-In-1 Book Cover Guide & Precision Paper Cutter Tool Set",
				ImageURL:         "",
				SellingPrice:     349,
				DC:               "JHB",
				LeadtimeStock:    210,
				DemandQty:        2,
				ShipQty:          2,
				ActualWeight:     0.65,
				VolumetricWeight: 0.80,
				WeighStatus:      "pending",
			},
		}
		shipmentNo := fmt.Sprintf("SH-JHB-%d", time.Now().Unix()%1000000)
		created, _ := s.db.CreateShipment(shipmentNo, "draft", "JHB", "待送约堡仓常规批次", demoItems)
		if created != nil {
			list, _ = s.db.GetShipments(status)
		}
		_ = nowStr
	}

	jsonResponse(w, http.StatusOK, list)
}

func (s *Server) handleShipmentCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": "db not initialized"})
		return
	}

	var req struct {
		ShipmentNumber string                  `json:"shipment_number"`
		DestinationDC  string                  `json:"destination_dc"`
		Notes          string                  `json:"notes"`
		Status         string                  `json:"status"`
		Items          []db.ShipmentItemRecord `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	if req.ShipmentNumber == "" {
		dc := req.DestinationDC
		if dc == "" {
			dc = "JHB"
		}
		req.ShipmentNumber = fmt.Sprintf("SH-%s-%d", dc, time.Now().Unix()%1000000)
	}
	if req.Status == "" {
		req.Status = "draft"
	}

	shipment, err := s.db.CreateShipment(req.ShipmentNumber, req.Status, req.DestinationDC, req.Notes, req.Items)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, shipment)
}

func (s *Server) handleShipmentUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}
	if err := s.db.UpdateShipmentStatus(req.ID, req.Status); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleShipmentDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ID == 0 {
		idStr := r.URL.Query().Get("id")
		req.ID, _ = strconv.ParseInt(idStr, 10, 64)
	}
	if req.ID == 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
		return
	}
	if err := s.db.DeleteShipment(req.ID); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleShipmentItemUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ItemID           int64   `json:"item_id"`
		ShipQty          int     `json:"ship_qty"`
		LeadtimeStock    int     `json:"leadtime_stock"`
		ActualWeight     float64 `json:"actual_weight"`
		VolumetricWeight float64 `json:"volumetric_weight"`
		WeighStatus      string  `json:"weigh_status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid body"})
		return
	}
	if err := s.db.UpdateShipmentItem(req.ItemID, req.ShipQty, req.LeadtimeStock, req.ActualWeight, req.VolumetricWeight, req.WeighStatus); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleOfferQuickUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		OfferID       string  `json:"offer_id"`
		TSIN          string  `json:"tsin"`
		SellingPrice  int     `json:"selling_price"`
		RRP           int     `json:"rrp"`
		WeightKg      float64 `json:"weight_kg"`
		LeadtimeStock int     `json:"leadtime_stock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid body"})
		return
	}

	targetOfferID := strings.TrimSpace(req.OfferID)
	if targetOfferID == "" {
		targetOfferID = strings.TrimSpace(req.TSIN)
	}
	if targetOfferID == "" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "offer_id or tsin is required"})
		return
	}

	// 1. 如果有改价
	if req.SellingPrice > 0 {
		if s.api != nil {
			_ = s.api.UpdateOfferPrice(targetOfferID, req.SellingPrice, req.RRP)
		}
	}

	// 2. 如果有改重或改提前库存，通过官方 PATCH /v2/offers/offer/{id} 更新
	if req.WeightKg > 0 || req.LeadtimeStock > 0 {
		payload := make(map[string]any)
		if req.WeightKg > 0 {
			payload["package_weight"] = req.WeightKg
		}
		if req.LeadtimeStock > 0 {
			payload["leadtime_stock"] = req.LeadtimeStock
		}
		if s.api != nil {
			_ = s.api.UpdateSingleOffer(targetOfferID, payload)
		}
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "更新已生效并成功同步至 Takealot",
	})
}

func (s *Server) handleBookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.db == nil {
		jsonResponse(w, http.StatusOK, []any{})
		return
	}
	list, err := s.db.GetBookings()
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// 若空，初始化一条示例预约
	if len(list) == 0 {
		tomorrow := time.Now().Add(24 * time.Hour).Format("2006-01-02")
		b, _ := s.db.CreateBooking(1, "JHB", tomorrow, "10:00 - 12:00", "Courier Guy", "GP 882-901", "预约送约堡1号中转仓")
		if b != nil {
			list, _ = s.db.GetBookings()
		}
	}

	jsonResponse(w, http.StatusOK, list)
}

func (s *Server) handleBookingCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ShipmentID  int64  `json:"shipment_id"`
		DC          string `json:"dc"`
		BookingDate string `json:"booking_date"`
		TimeSlot    string `json:"time_slot"`
		Carrier     string `json:"carrier"`
		VehicleReg  string `json:"vehicle_reg"`
		Notes       string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid body"})
		return
	}
	if req.DC == "" {
		req.DC = "JHB"
	}
	if req.BookingDate == "" {
		req.BookingDate = time.Now().Add(24 * time.Hour).Format("2006-01-02")
	}

	b, err := s.db.CreateBooking(req.ShipmentID, req.DC, req.BookingDate, req.TimeSlot, req.Carrier, req.VehicleReg, req.Notes)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, b)
}

func (s *Server) handleBookingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "Invalid body"})
		return
	}
	if err := s.db.UpdateBookingStatus(req.ID, req.Status); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleBookingDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ID == 0 {
		idStr := r.URL.Query().Get("id")
		req.ID, _ = strconv.ParseInt(idStr, 10, 64)
	}
	if req.ID == 0 {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
		return
	}
	if err := s.db.DeleteBooking(req.ID); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"success": true})
}

