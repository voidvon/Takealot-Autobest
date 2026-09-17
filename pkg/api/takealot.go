package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SellerBaseURL = "https://seller-api.takealot.com"
	PublicBaseURL = "https://api.takealot.com"
	UserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

type Client struct {
	mu            sync.RWMutex
	authorization string
	proxyURL      string
	httpClient    *http.Client

	rateMu      sync.Mutex
	lastReqTime time.Time
	minInterval time.Duration
}

func FormatAuth(auth string) string {
	auth = strings.TrimSpace(auth)
	if auth == "" {
		return ""
	}
	if !strings.HasPrefix(auth, "Key ") && !strings.HasPrefix(auth, "Bearer ") {
		return "Key " + auth
	}
	return auth
}

func makeHTTPClient(proxyStr string) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}
	if proxyStr != "" {
		if u, err := url.Parse(proxyStr); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}
}

func NewClient(authorization string, proxyURL ...string) *Client {
	proxy := ""
	if len(proxyURL) > 0 && strings.TrimSpace(proxyURL[0]) != "" {
		proxy = strings.TrimSpace(proxyURL[0])
	}
	return &Client{
		authorization: FormatAuth(authorization),
		proxyURL:      proxy,
		httpClient:    makeHTTPClient(proxy),
		minInterval:   350 * time.Millisecond, // 默认全局请求间隔不少于 350ms，限制每秒发包量在安全区间
	}
}

func (c *Client) SetAuthorization(auth string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authorization = FormatAuth(auth)
}

func (c *Client) SetProxy(proxyURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	proxyURL = strings.TrimSpace(proxyURL)
	if c.proxyURL != proxyURL {
		c.proxyURL = proxyURL
		c.httpClient = makeHTTPClient(proxyURL)
	}
}

func (c *Client) GetProxy() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.proxyURL
}

func (c *Client) getAuth() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authorization
}

// ClientPool 提供按店铺 store_id 缓存与复用 Client 的管理器
type ClientPool struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewClientPool() *ClientPool {
	return &ClientPool{
		clients: make(map[string]*Client),
	}
}

func (p *ClientPool) Get(storeID string) *Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.clients[storeID]
}

func (p *ClientPool) Set(storeID string, client *Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients[storeID] = client
}

func (p *ClientPool) Remove(storeID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.clients, storeID)
}

func (p *ClientPool) GetOrCreate(storeID, auth, proxyURL string) *Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[storeID]; ok {
		if auth != "" {
			c.SetAuthorization(auth)
		}
		c.SetProxy(proxyURL)
		return c
	}
	c := NewClient(auth, proxyURL)
	p.clients[storeID] = c
	return c
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	auth := c.getAuth()
	if auth != "" {
		req.Header.Set("authorization", auth)
	}
}

// waitRateLimit 确保全局向 Takealot 发起的请求保持安全时间间隔，防止突发流量触发 420
func (c *Client) waitRateLimit() {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()

	interval := c.minInterval
	if interval <= 0 {
		interval = 350 * time.Millisecond
	}
	now := time.Now()
	elapsed := now.Sub(c.lastReqTime)
	if elapsed < interval {
		time.Sleep(interval - elapsed)
	}
	c.lastReqTime = time.Now()
}

// doRequest 封装了发包速率控制、请求头注入以及对 HTTP 420 (Rate Limit Exceeded) 的自动退避重试
func (c *Client) doRequest(method, urlStr string, body []byte) (*http.Response, error) {
	maxRetries := 3
	baseDelay := 1500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		c.waitRateLimit()

		var bodyReader io.Reader
		if len(body) > 0 {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequest(method, urlStr, bodyReader)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < maxRetries {
				time.Sleep(baseDelay * time.Duration(attempt+1))
				continue
			}
			return nil, err
		}

		// 重点处理 Takealot 官方特有的 HTTP 420 (Rate Limit Exceeded) 或标准 429
		if resp.StatusCode == 420 || resp.StatusCode == 429 {
			retryAfterSec := 0
			if h := resp.Header.Get("Retry-After"); h != "" {
				if s, err := strconv.Atoi(h); err == nil && s > 0 {
					retryAfterSec = s
				}
			}
			resp.Body.Close()

			if attempt < maxRetries {
				sleepDuration := baseDelay * time.Duration(attempt+1)
				if retryAfterSec > 0 {
					sleepDuration = time.Duration(retryAfterSec) * time.Second
				}
				log.Printf("⚠️ [API 限流保护] Takealot 触发频率超限 (HTTP %d)，正在休眠 %v 后进行第 %d 次自动重试: %s",
					resp.StatusCode, sleepDuration, attempt+1, urlStr)
				time.Sleep(sleepDuration)
				continue
			}
			return nil, fmt.Errorf("HTTP %d: Rate Limit Exceeded (请求频率超限，系统已自动重试 %d 次仍受限，请稍后刷新)", resp.StatusCode, maxRetries)
		}

		// 临时服务端故障 502/503/504 自动重试
		if resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 {
			resp.Body.Close()
			if attempt < maxRetries {
				time.Sleep(baseDelay * time.Duration(attempt+1))
				continue
			}
			return nil, fmt.Errorf("HTTP %d: Takealot 官方服务暂时不可用，请稍后再试", resp.StatusCode)
		}

		return resp, nil
	}
	return nil, fmt.Errorf("request failed after %d retries", maxRetries)
}

type TestResult struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	TotalOffers int    `json:"total_offers"`
}

func (c *Client) TestConnection() TestResult {
	auth := c.getAuth()
	if auth == "" {
		return TestResult{Success: false, Message: "请先配置 Authorization 凭证 (API Key 或 Token)"}
	}

	// 1. Try official /v2/offers/count
	countUrl := fmt.Sprintf("%s/v2/offers/count", SellerBaseURL)
	resp, err := c.doRequest("GET", countUrl, nil)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var data struct {
				Count int `json:"count"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
				return TestResult{
					Success:     true,
					Message:     fmt.Sprintf("连接成功！当前店铺有效商品数: %d", data.Count),
					TotalOffers: data.Count,
				}
			}
		}
	}

	// 2. Fallback to /v2/offers/detailed
	url := fmt.Sprintf("%s/v2/offers/detailed?page_number=1&page_size=1&status_ids=1&status_ids=2", SellerBaseURL)
	respDet, err := c.doRequest("GET", url, nil)
	if err != nil {
		return TestResult{Success: false, Message: fmt.Sprintf("网络连接失败: %v", err)}
	}
	defer respDet.Body.Close()

	if respDet.StatusCode == 200 {
		var data struct {
			Total  int   `json:"total"`
			Offers []any `json:"offers"`
		}
		_ = json.NewDecoder(respDet.Body).Decode(&data)
		total := data.Total
		if total == 0 {
			total = len(data.Offers)
		}
		return TestResult{
			Success:     true,
			Message:     fmt.Sprintf("连接成功！当前店铺有效商品数: %d", total),
			TotalOffers: total,
		}
	} else if respDet.StatusCode == 401 || respDet.StatusCode == 403 {
		return TestResult{Success: false, Message: fmt.Sprintf("授权认证失败 (HTTP %d)，Token 或 API Key 可能已失效", respDet.StatusCode)}
	}
	return TestResult{Success: false, Message: fmt.Sprintf("接口返回异常状态码: HTTP %d", respDet.StatusCode)}
}

type SellerInfo struct {
	SellerID    int64  `json:"seller_id"`
	DisplayName string `json:"display_name"`
	Slug        string `json:"slug"`
}

func (c *Client) GetSellerInfo() (*SellerInfo, error) {
	url := fmt.Sprintf("%s/v2/seller", SellerBaseURL)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		var info SellerInfo
		if err := json.NewDecoder(resp.Body).Decode(&info); err == nil {
			return &info, nil
		}
	}
	return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
}

func AnyToString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	default:
		return fmt.Sprintf("%v", val)
	}
}

var imageSizeRegex = regexp.MustCompile(`/s(-[a-zA-Z0-9_-]+|\{size\})?\.file`)

func GetLargeImageURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	rawURL = strings.Replace(rawURL, "http://", "https://", 1)
	if strings.Contains(rawURL, "covers_images") {
		return imageSizeRegex.ReplaceAllString(rawURL, "/s-pdpxl.file")
	}
	return rawURL
}

func GetZoomImageURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	rawURL = strings.Replace(rawURL, "http://", "https://", 1)
	if strings.Contains(rawURL, "covers_images") {
		return imageSizeRegex.ReplaceAllString(rawURL, "/s-zoom.file")
	}
	return rawURL
}

type TSINInfo struct {
	Title         string `json:"title"`
	ProductlineID any    `json:"productline_id"`
	ImageURL      string `json:"image_url"`
}

type OfferItem struct {
	OfferID            int64    `json:"offer_id"`
	TSINID             int64    `json:"tsin_id"`
	TSIN               TSINInfo `json:"tsin"`
	SellingPrice       float64  `json:"selling_price"`
	RRP                float64  `json:"rrp"`
	TotalMerchantStock int      `json:"total_merchant_stock"`
	DateModified       string   `json:"date_modified"`
	Status             any      `json:"status"`
	MerchantSKU        string   `json:"merchant_sku"`
	SKU                string   `json:"sku"`
}

type OffersResponse struct {
	Offers []OfferItem `json:"offers"`
	Total  int         `json:"total"`
}

func (c *Client) GetOffersPage(page, pageSize int, tsinID string) (*OffersResponse, error) {
	url := fmt.Sprintf("%s/v2/offers/detailed?page_number=%d&page_size=%d&sort_key=date_added&sort_dir=asc&status_ids=1&status_ids=2",
		SellerBaseURL, page, pageSize)
	if tsinID != "" {
		url += fmt.Sprintf("&tsin_id=%s", tsinID)
	}

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return &OffersResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var result OffersResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			return &result, nil
		} else {
			fmt.Printf("❌ JSON decode error: %v\n", err)
			return &OffersResponse{}, err
		}
	}
	respBytes, _ := io.ReadAll(resp.Body)
	fmt.Printf("⚠️ Offers page HTTP %d: %s\n", resp.StatusCode, string(respBytes))
	return &OffersResponse{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
}

func (c *Client) GetAllOffers(maxItems int) ([]OfferItem, error) {
	var allOffers []OfferItem
	page := 1
	pageSize := 100

	for len(allOffers) < maxItems {
		res, err := c.GetOffersPage(page, pageSize, "")
		if err != nil || len(res.Offers) == 0 {
			break
		}
		allOffers = append(allOffers, res.Offers...)
		if len(res.Offers) < pageSize {
			break
		}
		page++
		time.Sleep(300 * time.Millisecond)
	}

	if len(allOffers) > maxItems {
		allOffers = allOffers[:maxItems]
	}
	return allOffers, nil
}

func AnyToInt(v any) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	default:
		return 0
	}
}

type MPVResult struct {
	TSINID          any     `json:"tsinId"`
	ProductlineID   any     `json:"productlineId"`
	BestPrice       float64 `json:"bestPrice"`
	CompetingOffers any     `json:"competingOffers"`
	HasOffer        any     `json:"hasOffer"`
	GTIN            string  `json:"gtin"`
}

func (c *Client) GetMPVByTSIN(tsinID string) (*MPVResult, error) {
	url := fmt.Sprintf("%s/1/catalogue/mpv/search?search_by=tsin&search_query=%s", SellerBaseURL, tsinID)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data struct {
			Results []MPVResult `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && len(data.Results) > 0 {
			return &data.Results[0], nil
		}
	}
	return nil, fmt.Errorf("mpv not found")
}

func (c *Client) GetMPVByPLID(plid string) ([]MPVResult, error) {
	cleanPLID := strings.TrimSpace(strings.ReplaceAll(strings.ToUpper(plid), "PLID", ""))
	url := fmt.Sprintf("%s/1/catalogue/mpv/search?search_by=plid&search_query=%s", SellerBaseURL, cleanPLID)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data struct {
			Results []MPVResult `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			return data.Results, nil
		}
	}
	return nil, fmt.Errorf("mpv query failed")
}

func (c *Client) GetPublicPrice(plid string) int {
	cleanPLID := strings.TrimSpace(strings.ReplaceAll(strings.ToUpper(plid), "PLID", ""))
	url := fmt.Sprintf("%s/rest/v-1-16-0/product-details/PLID%s", PublicBaseURL, cleanPLID)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return 0
	}
	defer resp.Body.Close()

	var data struct {
		Buybox struct {
			Items []struct {
				Price float64 `json:"price"`
			} `json:"items"`
		} `json:"buybox"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
		if len(data.Buybox.Items) > 0 {
			return int(data.Buybox.Items[0].Price)
		}
	}
	return 0
}

func (c *Client) UpdateOfferPrice(offerID string, sellingPrice, rrp int) error {
	url := fmt.Sprintf("%s/v2/offers/offer/%s", SellerBaseURL, offerID)
	payload := map[string]int{
		"selling_price": sellingPrice,
		"rrp":           rrp,
	}
	body, _ := json.Marshal(payload)

	resp, err := c.doRequest("PATCH", url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		return nil
	}
	respBytes, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
}

func (c *Client) CreateOffer(gtin string, sellingPrice, rrp, leadtimeDays int) error {
	url := fmt.Sprintf("%s/v2/offers/offer/%s", SellerBaseURL, gtin)
	payload := map[string]int{
		"selling_price": sellingPrice,
		"rrp":           rrp,
		"leadtime_days": leadtimeDays,
	}
	body, _ := json.Marshal(payload)

	resp, err := c.doRequest("POST", url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		return nil
	}
	respBytes, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
}

// --- Official Takealot Seller API V2 / V1 Methods ---

type WarehouseStockDetail struct {
	Warehouse struct {
		WarehouseID int    `json:"warehouse_id"`
		Name        string `json:"name"`
	} `json:"warehouse"`
	QuantityAvailable int `json:"quantity_available"`
}

type OfficialOfferItem struct {
	OfferID               int64                  `json:"offer_id"`
	TSINID                int64                  `json:"tsin_id"`
	SKU                   string                 `json:"sku"`
	Barcode               string                 `json:"barcode"`
	ProductLabelNumber    string                 `json:"product_label_number"`
	SellingPrice          float64                `json:"selling_price"`
	RRP                   float64                `json:"rrp"`
	LeadtimeDays          int                    `json:"leadtime_days"`
	Status                string                 `json:"status"`
	Title                 string                 `json:"title"`
	OfferURL              string                 `json:"offer_url"`
	ImageURL              string                 `json:"image_url"`
	ImageLargeURL         string                 `json:"image_large_url"`
	StockAtTakealot       []WarehouseStockDetail `json:"stock_at_takealot"`
	StockOnWay            []WarehouseStockDetail `json:"stock_on_way"`
	TotalStockOnWay       int                    `json:"total_stock_on_way"`
	StockAtTakealotTotal  int                    `json:"stock_at_takealot_total"`
	CatalogueQualityScore int                    `json:"catalogue_quality_score"`
	DateCreated           string                 `json:"date_created"`
	Discount              string                 `json:"discount"`
	DiscountShown         bool                   `json:"discount_shown"`
}

type OfficialOffersResponse struct {
	PageSize     int                 `json:"page_size"`
	PageNumber   int                 `json:"page_number"`
	TotalResults int                 `json:"total_results"`
	Offers       []OfficialOfferItem `json:"offers"`
}

func (c *Client) GetOfficialOffers(page, pageSize int, filter string) (*OfficialOffersResponse, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	url := fmt.Sprintf("%s/v2/offers?page_number=%d&page_size=%d", SellerBaseURL, page, pageSize)
	if filter != "" {
		url += fmt.Sprintf("&filter=%s", filter)
	}

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result OfficialOffersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	for i := range result.Offers {
		result.Offers[i].ImageLargeURL = GetLargeImageURL(result.Offers[i].ImageURL)
	}
	return &result, nil
}

func (c *Client) GetOfficialOffersCount() (int, error) {
	url := fmt.Sprintf("%s/v2/offers/count", SellerBaseURL)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var data struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}
	return data.Count, nil
}

func (c *Client) UpdateSingleOffer(offerID string, payload map[string]any) error {
	url := fmt.Sprintf("%s/v2/offers/offer/%s", SellerBaseURL, offerID)
	body, _ := json.Marshal(payload)

	resp, err := c.doRequest("PATCH", url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
}

type SaleItem struct {
	OrderItemID          int64   `json:"order_item_id"`
	OrderID              int64   `json:"order_id"`
	OrderDate            string  `json:"order_date"`
	SaleStatus           string  `json:"sale_status"`
	OfferID              int64   `json:"offer_id"`
	TSIN                 int64   `json:"tsin"`
	SKU                  string  `json:"sku"`
	ProductTitle         string  `json:"product_title"`
	SellingPrice         float64 `json:"selling_price"`
	SuccessFee           float64 `json:"success_fee"`
	FulfillmentFee       float64 `json:"fulfillment_fee"`
	CourierCollectionFee float64 `json:"courier_collection_fee"`
	Customer             string  `json:"customer"`
	DC                   string  `json:"dc"`
}

type SalesResponse struct {
	PageSummary struct {
		Total      int `json:"total"`
		PageSize   int `json:"page_size"`
		PageNumber int `json:"page_number"`
	} `json:"page_summary"`
	Sales []SaleItem `json:"sales"`
}

func (c *Client) GetSales(page, pageSize int, startDate, endDate string) (*SalesResponse, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	url := fmt.Sprintf("%s/v2/sales?page_number=%d&page_size=%d", SellerBaseURL, page, pageSize)
	if startDate != "" {
		url += fmt.Sprintf("&start_date=%s", startDate)
	}
	if endDate != "" {
		url += fmt.Sprintf("&end_date=%s", endDate)
	}

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result SalesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

type SalesSummaryItem struct {
	DateRange string  `json:"date_range"`
	Total     float64 `json:"total"`
	Quantity  int     `json:"quantity"`
}

func (c *Client) GetSalesSummary() ([]SalesSummaryItem, error) {
	url := fmt.Sprintf("%s/v2/sales/summary", SellerBaseURL)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result []SalesSummaryItem
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

type OrderItemInfo struct {
	OrderItemID int64  `json:"order_item_id"`
	MerchantSKU string `json:"merchant_sku"`
	Title       string `json:"title"`
	TSINID      int64  `json:"tsin_id"`
	TakealotURL string `json:"takealot_url"`
}

type OrderRecord struct {
	OrderID    int64           `json:"order_id"`
	DateAuthed string          `json:"date_authed"`
	OrderItems []OrderItemInfo `json:"order_items"`
}

type SalesOrdersResponse struct {
	PageSummary struct {
		Total      int `json:"total"`
		PageSize   int `json:"page_size"`
		PageNumber int `json:"page_number"`
	} `json:"page_summary"`
	Orders []OrderRecord `json:"orders"`
}

func (c *Client) GetSalesOrders(startDate, endDate string, page, pageSize int) (*SalesOrdersResponse, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	url := fmt.Sprintf("%s/v1/sales/orders?start_date=%s&end_date=%s&page_number=%d&page_size=%d",
		SellerBaseURL, startDate, endDate, page, pageSize)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result SalesOrdersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

type InvoiceDocument struct {
	DocumentID     int64  `json:"document_id"`
	DocumentDate   string `json:"document_date"`
	DocumentReason string `json:"document_reason"`
	FileName       string `json:"file_name"`
	DocumentType   string `json:"document_type"`
	Downloadable   bool   `json:"downloadable"`
}

func (c *Client) GetCustomerInvoices(orderID int64) ([]InvoiceDocument, error) {
	url := fmt.Sprintf("%s/v1/sales/orders/%d/customer_invoices", SellerBaseURL, orderID)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Documents []InvoiceDocument `json:"documents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Documents, nil
}

type StockCounts struct {
	TotalStockCount      int `json:"total_stock_count"`
	UnbalancedStockCount int `json:"unbalanced_stock_count"`
}

func (c *Client) GetStockCounts() (*StockCounts, error) {
	url := fmt.Sprintf("%s/v2/offers/stock_counts", SellerBaseURL)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result StockCounts
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

type StockHealthStats struct {
	StorageFeeEnabledOfferCount           int `json:"storage_fee_enabled_offer_count"`
	RecommendedForReplenishmentOfferCount int `json:"recommended_for_replenishment_offer_count"`
}

func (c *Client) GetStockHealthStats() (*StockHealthStats, error) {
	url := fmt.Sprintf("%s/v2/offers/stock_health_stats", SellerBaseURL)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result StockHealthStats
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOfficialSingleOffer retrieves a single offer by identifier (Offer ID, BARCODE..., or SKU...)
func (c *Client) GetOfficialSingleOffer(identifier string) (map[string]any, error) {
	url := fmt.Sprintf("%s/v2/offers/offer/%s", SellerBaseURL, identifier)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateOfficialSingleOffer creates a new offer under a catalogue product barcode
func (c *Client) CreateOfficialSingleOffer(barcode string, payload map[string]any) (map[string]any, error) {
	url := fmt.Sprintf("%s/v2/offers/offer?identifier=%s", SellerBaseURL, barcode)
	body, _ := json.Marshal(payload)

	resp, err := c.doRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return map[string]any{"success": true}, nil
	}
	return result, nil
}

// UpdateOfficialOfferStatus sets status_action to "Disable" or "Re-enable"
func (c *Client) UpdateOfficialOfferStatus(identifier string, statusAction string) error {
	payload := map[string]any{
		"status_action": statusAction,
	}
	return c.UpdateSingleOffer(identifier, payload)
}

// CreateOfficialBatch submits a batch of up to 10,000 offers to Takealot's official batch processing queue
func (c *Client) CreateOfficialBatch(offers []any) (map[string]any, error) {
	url := fmt.Sprintf("%s/v2/offers/batch", SellerBaseURL)
	body, _ := json.Marshal(offers)

	resp, err := c.doRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetOfficialBatch queries the status and validation errors of a submitted batch
func (c *Client) GetOfficialBatch(batchID string) (map[string]any, error) {
	url := fmt.Sprintf("%s/v2/offers/batch/%s", SellerBaseURL, batchID)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

