package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"
	"takealot/pkg/api"
	"takealot/pkg/config"
	"takealot/pkg/engine"
)

type Server struct {
	cfgMgr     *config.Manager
	api        *api.Client
	eng        *engine.Engine
	mux        *http.ServeMux
	staticHTML []byte
	version    string
}

func NewServer(cfgMgr *config.Manager, apiClient *api.Client, eng *engine.Engine, staticHTML []byte, version string) *Server {
	s := &Server{
		cfgMgr:     cfgMgr,
		api:        apiClient,
		eng:        eng,
		mux:        http.NewServeMux(),
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
	s.mux.HandleFunc("/api/follow/upload", s.handleFollowUpload)
	s.mux.HandleFunc("/api/follow/start", s.handleFollowStart)
	s.mux.HandleFunc("/api/logs/stream", s.handleLogsStream)
	s.mux.HandleFunc("/api/logs/history", s.handleLogsHistory)
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.staticHTML)
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
}

func (s *Server) handleOffers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := s.cfgMgr.Get()
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

	// Fetch MPV BestPrice and Competing Offers concurrently
	type mpvData struct {
		bestPrice int
		competing int
	}
	mpvMap := make(map[string]mpvData)
	var mpvMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)

	for _, item := range rawOffers {
		wg.Add(1)
		go func(tsinStr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			mpv, err := s.api.GetMPVByTSIN(tsinStr)
			if err == nil && mpv != nil {
				mpvMu.Lock()
				mpvMap[tsinStr] = mpvData{
					bestPrice: int(mpv.BestPrice),
					competing: api.AnyToInt(mpv.CompetingOffers),
				}
				mpvMu.Unlock()
			}
		}(strconv.FormatInt(item.TSINID, 10))
	}
	wg.Wait()

	targets := cfg.Targets
	parsed := make([]OfferViewModel, 0, len(rawOffers))

	for _, item := range rawOffers {
		tsinStr := strconv.FormatInt(item.TSINID, 10)
		plidStr := api.AnyToString(item.TSIN.ProductlineID)
		key := fmt.Sprintf("%s/%s", tsinStr, plidStr)
		targetInfo := targets[key]

		myPrice := int(item.SellingPrice)
		mpvInfo := mpvMap[tsinStr]
		bestPrice := mpvInfo.bestPrice
		competing := mpvInfo.competing
		priority := "solo"
		diff := 0

		if competing > 1 {
			if bestPrice > 0 {
				if myPrice <= bestPrice {
					priority = "winning"
				} else {
					priority = "losing"
					diff = myPrice - bestPrice
				}
			} else {
				priority = "winning"
			}
		} else {
			priority = "solo"
		}

		parsed = append(parsed, OfferViewModel{
			Key:             key,
			TSINID:          tsinStr,
			PLID:            plidStr,
			Title:           item.TSIN.Title,
			SellingPrice:    myPrice,
			RRP:             int(item.RRP),
			Stock:           item.TotalMerchantStock,
			DateModified:    item.DateModified,
			Selected:        targetInfo.Selected,
			MinPrice:        targetInfo.MinPrice,
			MaxPrice:        targetInfo.MaxPrice,
			BestPrice:       bestPrice,
			CompetingOffers: competing,
			PriorityStatus:  priority,
			PriceDiff:       diff,
		})
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"success": true,
		"total":   len(parsed),
		"offers":  parsed,
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
	s.eng.Log(fmt.Sprintf("💾 已保存 %d 个监控商品配置", len(req.Targets)), "INFO")
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
