package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	*clientState
	ctx context.Context
}

// Context-scoped clients share credentials, transport and rate limits, but
// stopping a worker must not cancel unrelated requests from the control panel.
type clientState struct {
	mu               sync.RWMutex
	authorization    string
	proxyURL         string
	httpClient       *http.Client
	logger           func(message, level string)
	requestTransport RequestTransport

	rateMu      sync.Mutex
	lastReqTime time.Time
	minInterval time.Duration
	cooldowns   map[string]time.Time
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
	return &Client{clientState: &clientState{
		authorization: FormatAuth(authorization),
		proxyURL:      proxy,
		httpClient:    makeHTTPClient(proxy),
		minInterval:   350 * time.Millisecond, // 默认全局请求间隔不少于 350ms，限制每秒发包量在安全区间
		cooldowns:     make(map[string]time.Time),
	}, ctx: context.Background()}
}

func (c *Client) WithContext(ctx context.Context) *Client {
	return &Client{clientState: c.clientState, ctx: ctx}
}

func (c *Client) SetLogger(logger func(message, level string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger = logger
}

func (c *Client) log(message, level string) {
	c.mu.RLock()
	logger := c.logger
	c.mu.RUnlock()
	log.Print(message)
	if logger != nil {
		logger(message, level)
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
	mu        sync.RWMutex
	clients   map[string]*Client
	logger    func(storeID, message, level string)
	transport RequestTransport
}

// RequestTransport preserves the existing retry and rate-limit layer while
// selecting a persistent, per-store browser session when the user enables it.
type RequestTransport func(storeID, proxy string, req *http.Request, direct func(*http.Request) (*http.Response, error)) (*http.Response, error)

type VerificationRequiredError struct{ Message string }

func (e *VerificationRequiredError) Error() string { return e.Message }
func (e *VerificationRequiredError) NoRetry() bool { return true }

func (p *ClientPool) SetRequestTransport(transport RequestTransport) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.transport = transport
	for id, c := range p.clients {
		p.setClientTransport(id, c)
	}
}

func (p *ClientPool) setClientTransport(id string, c *Client) {
	transport := p.transport
	c.mu.Lock()
	defer c.mu.Unlock()
	if transport == nil {
		c.requestTransport = nil
		return
	}
	c.requestTransport = func(_ string, proxy string, req *http.Request, direct func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		return transport(id, proxy, req, direct)
	}
}

// SetLogger also covers clients created before the engine was initialized.
func (p *ClientPool) SetLogger(logger func(storeID, message, level string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger = logger
	for id, client := range p.clients {
		p.setClientLogger(id, client)
	}
}

func (p *ClientPool) setClientLogger(storeID string, client *Client) {
	logger := p.logger
	if logger != nil {
		client.SetLogger(func(message, level string) { logger(storeID, message, level) })
	} else {
		client.SetLogger(nil)
	}
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
	p.setClientLogger(storeID, client)
	p.setClientTransport(storeID, client)
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
	p.setClientLogger(storeID, c)
	p.setClientTransport(storeID, c)
	p.clients[storeID] = c
	return c
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	auth := c.getAuth()
	if auth != "" && req.URL.Host == "seller-api.takealot.com" {
		req.Header.Set("authorization", auth)
	}
}

// Do not hold the rate mutex while sleeping: every waiting worker must be
// cancellable, including when the upstream server requests a long cooldown.
func (c *Client) waitRateLimit(key string) error {
	for {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		c.rateMu.Lock()
		interval := c.minInterval
		if interval <= 0 {
			interval = 350 * time.Millisecond
		}
		due := c.lastReqTime.Add(interval)
		if cooldown := c.cooldowns[key]; cooldown.After(due) {
			due = cooldown
		}
		if !time.Now().Before(due) {
			c.lastReqTime = time.Now()
			c.rateMu.Unlock()
			return nil
		}
		c.rateMu.Unlock()
		if err := c.wait(time.Until(due)); err != nil {
			return err
		}
	}
}

func (c *Client) wait(delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	case <-timer.C:
		return nil
	}
}

type RateLimitError struct {
	StatusCode int
	RetryAfter time.Duration
}

type AccessBlockedError struct {
	StatusCode int
	Endpoint   string
}

func (e *AccessBlockedError) Error() string {
	return fmt.Sprintf("HTTP %d: Cloudflare 访问验证拦截（%s），请检查网络或店铺代理配置；当前无法判断 Key 是否有效", e.StatusCode, e.Endpoint)
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("HTTP %d: 请求频率超限，自动重试后仍受限，请稍后再试", e.StatusCode)
}

func retryAfterDelay(header string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if until, err := http.ParseTime(header); err == nil && until.After(now) {
		return until.Sub(now)
	}
	return 0
}

// doRequest 封装了发包速率控制、请求头注入以及对 HTTP 420 (Rate Limit Exceeded) 的自动退避重试
func (c *Client) doRequest(method, urlStr string, body []byte) (*http.Response, error) {
	maxRetries := 3
	if method != http.MethodGet && method != http.MethodHead {
		maxRetries = 0
	}
	baseDelay := 1500 * time.Millisecond
	endpoint, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	key := endpoint.Host + endpoint.Path

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := c.waitRateLimit(key); err != nil {
			return nil, err
		}

		var bodyReader io.Reader
		if len(body) > 0 {
			bodyReader = bytes.NewReader(body)
		}

		req, err := http.NewRequestWithContext(c.ctx, method, urlStr, bodyReader)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)

		c.mu.RLock()
		httpClient := c.httpClient
		transport, proxy := c.requestTransport, c.proxyURL
		c.mu.RUnlock()
		var resp *http.Response
		if transport != nil {
			resp, err = transport("", proxy, req, httpClient.Do)
		} else {
			resp, err = httpClient.Do(req)
		}
		if err != nil {
			if c.ctx.Err() != nil {
				return nil, c.ctx.Err()
			}
			var terminal interface{ NoRetry() bool }
			if errors.As(err, &terminal) && terminal.NoRetry() {
				return nil, err
			}
			// A lost write response may already have changed the price. Do not
			// replay it blindly; the next cycle must read the current price.
			if method != http.MethodGet && method != http.MethodHead {
				return nil, fmt.Errorf("写入请求未确认，请回读平台状态后再操作: %w", err)
			}
			if attempt < maxRetries {
				delay := baseDelay * time.Duration(attempt+1)
				c.log(fmt.Sprintf("⚠️ [API 网络重试] %s 请求失败，等待 %v 后第 %d 次重试: %v", key, delay, attempt+1, err), "WARN")
				if err := c.wait(delay); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		if strings.EqualFold(resp.Header.Get("Cf-Mitigated"), "challenge") {
			resp.Body.Close()
			blocked := &AccessBlockedError{StatusCode: resp.StatusCode, Endpoint: key}
			c.log(blocked.Error(), "WARN")
			return nil, blocked
		}

		// 重点处理 Takealot 官方特有的 HTTP 420 (Rate Limit Exceeded) 或标准 429
		if resp.StatusCode == 420 || resp.StatusCode == 429 {
			delay := baseDelay * time.Duration(attempt+1)
			if retryAfter := retryAfterDelay(resp.Header.Get("Retry-After"), time.Now()); retryAfter > delay {
				delay = retryAfter
			}
			resp.Body.Close()
			c.rateMu.Lock()
			if due := time.Now().Add(delay); due.After(c.cooldowns[key]) {
				c.cooldowns[key] = due
			}
			c.rateMu.Unlock()

			if attempt < maxRetries {
				c.log(fmt.Sprintf("⚠️ [API 限流保护] Takealot HTTP %d，等待 %v 后第 %d 次重试: %s", resp.StatusCode, delay, attempt+1, key), "WARN")
				continue
			}
			return nil, &RateLimitError{StatusCode: resp.StatusCode, RetryAfter: delay}
		}

		// 临时服务端故障 502/503/504 自动重试
		if resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504 {
			resp.Body.Close()
			if attempt < maxRetries {
				delay := baseDelay * time.Duration(attempt+1)
				c.log(fmt.Sprintf("⚠️ [API 服务重试] HTTP %d，等待 %v 后第 %d 次重试: %s", resp.StatusCode, delay, attempt+1, key), "WARN")
				if err := c.wait(delay); err != nil {
					return nil, err
				}
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
	var blocked *AccessBlockedError
	if errors.As(err, &blocked) {
		return TestResult{Success: false, Message: blocked.Error()}
	}
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
		if errors.As(err, &blocked) {
			return TestResult{Success: false, Message: blocked.Error()}
		}
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
		if err != nil {
			return allOffers, fmt.Errorf("获取商品第 %d 页失败: %w", page, err)
		}
		if len(res.Offers) == 0 {
			break
		}
		allOffers = append(allOffers, res.Offers...)
		if len(res.Offers) < pageSize {
			break
		}
		page++
		if err := c.wait(300 * time.Millisecond); err != nil {
			return allOffers, err
		}
	}

	if len(allOffers) > maxItems {
		allOffers = allOffers[:maxItems]
	}
	return allOffers, nil
}

// GetOfferIDByTSIN resolves a catalogue product to this seller's actual offer.
// A TSIN is not a valid substitute for Offer ID in the single-offer PATCH API.
func (c *Client) GetOfferIDByTSIN(tsinID string) (string, error) {
	tsinID = strings.TrimSpace(tsinID)
	tsin, err := strconv.ParseInt(tsinID, 10, 64)
	if err != nil || tsin <= 0 {
		return "", fmt.Errorf("无效的商品 TSIN: %s", tsinID)
	}
	res, err := c.GetOffersPage(1, 100, tsinID)
	if err != nil {
		return "", fmt.Errorf("查询商品 Offer ID 失败: %w", err)
	}
	for _, offer := range res.Offers {
		if offer.TSINID == tsin && offer.OfferID > 0 {
			return strconv.FormatInt(offer.OfferID, 10), nil
		}
	}
	return "", fmt.Errorf("当前店铺未找到 TSIN %s 对应的 Offer，请同步商品后重试", tsinID)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MPV 查询失败: HTTP %d", resp.StatusCode)
	}
	var data struct {
		Results []MPVResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("MPV 响应解析失败: %w", err)
	}
	if len(data.Results) == 0 {
		return nil, fmt.Errorf("未找到 TSIN %s 的 MPV 数据", tsinID)
	}
	return &data.Results[0], nil
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

// PublicBuyboxItem 表示前台商品详情页买盒 (buybox) 中的单个卖家报价。
// 买盒展示的报价不保证覆盖所有卖家；summary 只是多变体商品的聚合起价。
type PublicBuyboxItem struct {
	Price       float64 `json:"price"`
	SKU         any     `json:"sku"`
	SellerID    string  `json:"sponsored_ads_seller_id"`
	IsSelected  bool    `json:"is_selected"`
	IsAddToCart bool    `json:"is_add_to_cart_available"`
	OfferDetail struct {
		Preference  string `json:"preference"`
		DisplayText string `json:"display_text"`
	} `json:"offer_detail"`
	StockAvailability struct {
		Status string `json:"status"`
	} `json:"stock_availability"`
}

type PublicBuybox struct {
	TSINID    any                     `json:"tsin"`
	ItemsType string                  `json:"buybox_items_type"`
	Items     []PublicBuyboxItem      `json:"items"`
	Selectors []PublicVariantSelector `json:"-"`
}

// GetPublicBuybox 拉取前台商品详情页展示的买盒报价。
// 该接口无需卖家授权，可补充卖家 API 未提供的竞品报价；
// 卖家 API 的 MPV 只返回最低价 (bestPrice)，当自己就是最低价时无法得知竞品价格。
func (c *Client) GetPublicBuybox(plid string) (*PublicBuybox, error) {
	return c.getPublicBuybox(plid, "", "")
}

// GetPublicBuyboxForVariant 针对多变体商品，按颜色/尺码定位到具体变体后再拉取买盒报价。
// 只有定位到具体变体，前台接口才会返回逐个卖家的报价（buybox_items_type = multi）。
func (c *Client) GetPublicBuyboxForVariant(plid, colour, size string) (*PublicBuybox, error) {
	return c.getPublicBuybox(plid, colour, size)
}

func (c *Client) getPublicBuybox(plid, colour, size string) (*PublicBuybox, error) {
	cleanPLID := strings.TrimSpace(strings.ReplaceAll(strings.ToUpper(plid), "PLID", ""))
	if cleanPLID == "" {
		return nil, fmt.Errorf("empty plid")
	}
	endpoint := fmt.Sprintf("%s/rest/v-1-16-0/product-details/PLID%s?platform=desktop", PublicBaseURL, cleanPLID)
	if colour != "" {
		endpoint += "&colour_variant=" + url.QueryEscape(colour)
	}
	if size != "" {
		endpoint += "&size=" + url.QueryEscape(size)
	}

	resp, err := c.doRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("HTTP 403: 前台报价访问被拒绝（可能为 Cloudflare 验证或访问限制）")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var data struct {
		Buybox   PublicBuybox `json:"buybox"`
		Variants struct {
			Selectors []PublicVariantSelector `json:"selectors"`
		} `json:"variants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("前台报价响应不是有效 JSON（可能返回了验证页面）: %w", err)
		}
		return nil, fmt.Errorf("前台报价字段格式无法解析: %w", err)
	}
	data.Buybox.Selectors = data.Variants.Selectors
	return &data.Buybox, nil
}

// MPVVariant 是卖家目录接口返回的变体信息，用于把 TSIN 映射到前台的颜色/尺码参数。
type MPVVariant struct {
	TSINID          any     `json:"tsinId"`
	GTIN            string  `json:"gtin"`
	Title           string  `json:"title"`
	Size            string  `json:"size"`
	BasicColors     string  `json:"basicColors"`
	ColorName       string  `json:"colorName"`
	HasOffer        any     `json:"hasOffer"`
	CompetingOffers any     `json:"competingOffers"`
	BestPrice       float64 `json:"bestPrice"`
}

type CatalogueMPVProduct struct {
	ProductlineID any          `json:"productlineId"`
	Title         string       `json:"title"`
	Variants      []MPVVariant `json:"variants"`
}

// GetCatalogueMPVByPLID 获取商品线下的全部变体（含颜色、尺码）。
// 用于把当前处理的 TSIN 定位到前台对应的颜色/尺码变体。
func (c *Client) GetCatalogueMPVByPLID(plid string) ([]CatalogueMPVProduct, error) {
	cleanPLID := strings.TrimSpace(strings.ReplaceAll(strings.ToUpper(plid), "PLID", ""))
	if cleanPLID == "" {
		return nil, fmt.Errorf("empty plid")
	}
	url := fmt.Sprintf("%s/1/catalogue/mpv/PLID%s", SellerBaseURL, cleanPLID)

	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var data []CatalogueMPVProduct
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// FindVariant 在商品线变体列表中按 TSIN 查找变体。
func FindVariant(products []CatalogueMPVProduct, tsinID string) (MPVVariant, bool) {
	if tsinID == "" {
		return MPVVariant{}, false
	}
	for _, p := range products {
		for _, v := range p.Variants {
			if AnyToString(v.TSINID) == tsinID {
				return v, true
			}
		}
	}
	return MPVVariant{}, false
}

// HasOwnOffer 判断报价列表中是否存在我方报价（按平台 SKU，其次按售价）。
func (b *PublicBuybox) HasOwnOffer(ownSKU string, ownPrice int) bool {
	if !b.HasOfferPrices() {
		return false
	}
	for _, item := range b.Items {
		if ownSKU != "" && AnyToString(item.SKU) == ownSKU {
			return true
		}
	}
	if ownPrice <= 0 {
		return false
	}
	for _, item := range b.Items {
		if int(item.Price) == ownPrice {
			return true
		}
	}
	return false
}

// CompetitorLowestPrice 返回除自身报价之外的最低竞品价格，0 表示无法确定竞品价格。
// 优先按平台 SKU 精确剔除自身报价；只有当列表里找不到自己的 SKU 时，
// 才退化为按当前售价剔除，避免把同价竞品误当成自己而漏掉真正的竞争对手。
func (b *PublicBuybox) CompetitorLowestPrice(ownSKU string, ownPrice int) int {
	if !b.HasOfferPrices() {
		return 0
	}

	ownBySKU := false
	if ownSKU != "" {
		for _, item := range b.Items {
			if AnyToString(item.SKU) == ownSKU {
				ownBySKU = true
				break
			}
		}
	}

	lowest := 0
	for _, item := range b.Items {
		price := int(item.Price)
		if price <= 0 {
			continue
		}
		if ownBySKU {
			if AnyToString(item.SKU) == ownSKU {
				continue // 自己的报价
			}
		} else if ownPrice > 0 && price == ownPrice {
			continue // 无法用 SKU 区分时，按价格剔除自身报价
		}
		if lowest == 0 || price < lowest {
			lowest = price
		}
	}
	return lowest
}

// IsSolo 判断前台买盒是否只有一个报价，且该报价确实属于自己。
// 只有在这种情况下才能确认"独家在售"，从而安全地回调提价。
func (b *PublicBuybox) IsSolo(ownSKU string, ownPrice int) bool {
	if !b.HasOfferPrices() || b.ItemsType != "single" || len(b.Items) != 1 {
		return false
	}
	item := b.Items[0]
	if ownSKU != "" && AnyToString(item.SKU) == ownSKU {
		return true
	}
	return ownPrice > 0 && int(item.Price) == ownPrice
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

// Takealot returns catalogue_quality_score as either a number or a string
// (including an unavailable marker). A display-only score must not prevent
// loading otherwise valid offers or their real seller Offer IDs.
func (o *OfficialOfferItem) UnmarshalJSON(data []byte) error {
	type offer OfficialOfferItem
	var value struct {
		*offer
		Score any `json:"catalogue_quality_score"`
	}
	value.offer = (*offer)(o)
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.CatalogueQualityScore = AnyToInt(value.Score)
	return nil
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
