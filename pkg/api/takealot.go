package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func NewClient(authorization string) *Client {
	return &Client{
		authorization: strings.TrimSpace(authorization),
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
	c.authorization = strings.TrimSpace(auth)
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
		return TestResult{Success: false, Message: "请先配置 Authorization 凭证 (Token)"}
	}

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
			Total  int `json:"total"`
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
		return TestResult{Success: false, Message: fmt.Sprintf("授权认证失败 (HTTP %d)，Token 可能已失效或权限不足", resp.StatusCode)}
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

type TSINInfo struct {
	Title         string `json:"title"`
	ProductlineID any    `json:"productline_id"`
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
