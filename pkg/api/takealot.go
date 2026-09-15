package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	httpClient    *http.Client
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

func NewClient(authorization string) *Client {
	return &Client{
		authorization: FormatAuth(authorization),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *Client) SetAuthorization(auth string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authorization = FormatAuth(auth)
}

func (c *Client) getAuth() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authorization
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
	reqCount, err := http.NewRequest("GET", countUrl, nil)
	if err == nil {
		c.setHeaders(reqCount)
		resp, err := c.httpClient.Do(reqCount)
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
			} else if resp.StatusCode == 401 || resp.StatusCode == 403 {
				return TestResult{Success: false, Message: fmt.Sprintf("授权认证失败 (HTTP %d)，API Key 可能失效或前缀格式不正确", resp.StatusCode)}
			}
		}
	}

	// 2. Fallback to /v2/offers/detailed
	url := fmt.Sprintf("%s/v2/offers/detailed?page_number=1&page_size=1&status_ids=1&status_ids=2", SellerBaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return TestResult{Success: false, Message: err.Error()}
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TestResult{Success: false, Message: fmt.Sprintf("网络连接失败: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var data struct {
			Total  int   `json:"total"`
			Offers []any `json:"offers"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&data)
		total := data.Total
		if total == 0 {
			total = len(data.Offers)
		}
		return TestResult{
			Success:     true,
			Message:     fmt.Sprintf("连接成功！当前店铺有效商品数: %d", total),
			TotalOffers: total,
		}
	} else if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return TestResult{Success: false, Message: fmt.Sprintf("授权认证失败 (HTTP %d)，Token 或 API Key 可能已失效", resp.StatusCode)}
	}
	return TestResult{Success: false, Message: fmt.Sprintf("接口返回异常状态码: HTTP %d", resp.StatusCode)}
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

	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				var result OffersResponse
				if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
					return &result, nil
				} else {
					fmt.Printf("❌ JSON decode error: %v\n", err)
				}
			} else {
				respBytes, _ := io.ReadAll(resp.Body)
				fmt.Printf("⚠️ Offers page HTTP %d: %s\n", resp.StatusCode, string(respBytes))
			}
		}
		time.Sleep(1 * time.Second)
	}
	return &OffersResponse{}, nil
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

	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var data struct {
				Results []MPVResult `json:"results"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && len(data.Results) > 0 {
				return &data.Results[0], nil
			}
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
	return nil, fmt.Errorf("mpv not found")
}

func (c *Client) GetMPVByPLID(plid string) ([]MPVResult, error) {
	cleanPLID := strings.TrimSpace(strings.ReplaceAll(strings.ToUpper(plid), "PLID", ""))
	url := fmt.Sprintf("%s/1/catalogue/mpv/search?search_by=plid&search_query=%s", SellerBaseURL, cleanPLID)

	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var data struct {
				Results []MPVResult `json:"results"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
				return data.Results, nil
			}
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
	return nil, fmt.Errorf("mpv query failed")
}

func (c *Client) GetPublicPrice(plid string) int {
	cleanPLID := strings.TrimSpace(strings.ReplaceAll(strings.ToUpper(plid), "PLID", ""))
	url := fmt.Sprintf("%s/rest/v-1-16-0/product-details/PLID%s", PublicBaseURL, cleanPLID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 204 {
				return nil
			}
			respBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("request timed out after 3 retries")
}

func (c *Client) CreateOffer(gtin string, sellingPrice, rrp, leadtimeDays int) error {
	url := fmt.Sprintf("%s/v2/offers/offer/%s", SellerBaseURL, gtin)
	payload := map[string]int{
		"selling_price": sellingPrice,
		"rrp":           rrp,
		"leadtime_days": leadtimeDays,
	}
	body, _ := json.Marshal(payload)

	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		c.setHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 201 {
				return nil
			}
			respBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBytes))
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("request timed out after 3 retries")
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

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

