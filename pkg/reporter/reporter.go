package reporter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Reporter 定义错误上报接口。
type Reporter interface {
	ReportCloudflareBlock(storeName, tsinID, plid, errorMsg string)
	ReportPriceFetchError(storeName, tsinID, plid, errorMsg string)
}

// Config 包含连接 GoCMS 的配置。
type Config struct {
	BaseURL                  string
	SiteID                   string
	APIKey                   string
	CloudflareCategoryID     int64
	PriceFetchCategoryID     int64
	RateLimitDuration        time.Duration
}

// DefaultConfig 返回基于线上创建分类的默认配置。
func DefaultConfig() Config {
	return Config{
		BaseURL:              "https://id.0122.vip",
		SiteID:               "2",
		CloudflareCategoryID: 1235,
		PriceFetchCategoryID: 1236,
		RateLimitDuration:    15 * time.Minute,
	}
}

// CMSReporter 实现将错误以文章形式上报到 GoCMS 对应分类中。
type CMSReporter struct {
	cfg        Config
	httpClient *http.Client
	mu         sync.Mutex
	lastReport map[string]time.Time
}

// LoadConfigFromDotKey 尝试从指定路径的 .api.key 读取配置。
func LoadConfigFromDotKey(filePath string, cfg *Config) {
	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "XIAN_URL":
			if val != "" {
				cfg.BaseURL = strings.TrimRight(val, "/")
			}
		case "XIAN_SITE_ID":
			if val != "" {
				cfg.SiteID = val
			}
		case "XIAN_API_KEY":
			if val != "" {
				cfg.APIKey = val
			}
		}
	}
}

// NewFromEnv 从环境变量和当前目录下的 .api.key 初始化 Reporter。
func NewFromEnv() *CMSReporter {
	cfg := DefaultConfig()

	// 1. 尝试从本地 .api.key 加载
	LoadConfigFromDotKey(".api.key", &cfg)

	// 2. 环境变量具有更高优先级
	if v := os.Getenv("XIAN_URL"); v != "" {
		cfg.BaseURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("XIAN_SITE_ID"); v != "" {
		cfg.SiteID = v
	}
	if v := os.Getenv("XIAN_API_KEY"); v != "" {
		cfg.APIKey = v
	}

	return New(cfg)
}

// New 创建 Reporter 实例。
func New(cfg Config) *CMSReporter {
	if cfg.RateLimitDuration <= 0 {
		cfg.RateLimitDuration = 15 * time.Minute
	}
	if cfg.CloudflareCategoryID <= 0 {
		cfg.CloudflareCategoryID = 1235
	}
	if cfg.PriceFetchCategoryID <= 0 {
		cfg.PriceFetchCategoryID = 1236
	}
	return &CMSReporter{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		lastReport: make(map[string]time.Time),
	}
}

// Enabled 检查上报器是否具备必要参数。
func (r *CMSReporter) Enabled() bool {
	return r != nil && r.cfg.BaseURL != "" && r.cfg.SiteID != "" && r.cfg.APIKey != ""
}

func (r *CMSReporter) shouldRateLimit(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	// 定期清理超过24小时的旧缓存条目
	if len(r.lastReport) > 2000 {
		for k, t := range r.lastReport {
			if now.Sub(t) > 24*time.Hour {
				delete(r.lastReport, k)
			}
		}
	}

	if last, ok := r.lastReport[key]; ok {
		if now.Sub(last) < r.cfg.RateLimitDuration {
			return true
		}
	}
	r.lastReport[key] = now
	return false
}

type contentPayload struct {
	CategoryID int64  `json:"category_id"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Content    string `json:"content"`
	Visible    int64  `json:"visible"`
}

func (r *CMSReporter) postContent(categoryID int64, title, summary, htmlContent string) error {
	if !r.Enabled() {
		return nil
	}

	endpoint := fmt.Sprintf("%s/api/admin/content?site_id=%s", r.cfg.BaseURL, r.cfg.SiteID)
	payload := contentPayload{
		CategoryID: categoryID,
		Title:      title,
		Summary:    summary,
		Content:    htmlContent,
		Visible:    1,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", r.cfg.APIKey)
	req.Header.Set("X-Site-Id", r.cfg.SiteID)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cms report failed with status: %d", resp.StatusCode)
	}

	return nil
}

// ReportCloudflareBlock 异步上报 Cloudflare 拦截事件。
func (r *CMSReporter) ReportCloudflareBlock(storeName, tsinID, plid, errorMsg string) {
	if !r.Enabled() {
		return
	}

	rateKey := fmt.Sprintf("cf:%s:%s", plid, tsinID)
	if r.shouldRateLimit(rateKey) {
		return
	}

	go func() {
		cleanPLID := strings.TrimSpace(strings.ReplaceAll(strings.ToUpper(plid), "PLID", ""))
		urlStr := fmt.Sprintf("https://api.takealot.com/rest/v-1-16-0/product-details/PLID%s", cleanPLID)
		nowStr := time.Now().Format("2006-01-02 15:04:05")

		title := fmt.Sprintf("🚨 [Cloudflare拦截] 店铺: %s | PLID: %s", storeName, plid)
		if tsinID != "" {
			title += fmt.Sprintf(" (TSIN: %s)", tsinID)
		}
		summary := fmt.Sprintf("前台价格接口被拦截 (%s): %s", nowStr, errorMsg)

		htmlContent := fmt.Sprintf(`<h3>🚨 Takealot 前台价格抓取 Cloudflare 拦截告警</h3>
<ul>
  <li><b>发生时间:</b> %s</li>
  <li><b>关联店铺:</b> %s</li>
  <li><b>TSIN ID:</b> %s</li>
  <li><b>PLID:</b> %s</li>
  <li><b>目标接口 URL:</b> <a href="%s" target="_blank">%s</a></li>
  <li><b>错误详情:</b> <pre>%s</pre></li>
</ul>
<p><b>处理建议:</b></p>
<ol>
  <li>可能当前爬取 IP 已触发 Takealot 平台的 WAF 访问频率限制或验证码 Challenge；</li>
  <li>建议在客户端「系统设置」中检查或切换高质量代理（Proxy），避免使用已被污染的数据中心 IP；</li>
  <li>系统已在该轮巡检中对该商品自动跳过调价，保障自身报价安全。</li>
</ol>`, nowStr, storeName, tsinID, plid, urlStr, urlStr, errorMsg)

		_ = r.postContent(r.cfg.CloudflareCategoryID, title, summary, htmlContent)
	}()
}

// ReportPriceFetchError 异步上报非 Cloudflare 的其他价格抓取异常。
func (r *CMSReporter) ReportPriceFetchError(storeName, tsinID, plid, errorMsg string) {
	if !r.Enabled() {
		return
	}

	rateKey := fmt.Sprintf("err:%s:%s", plid, tsinID)
	if r.shouldRateLimit(rateKey) {
		return
	}

	go func() {
		cleanPLID := strings.TrimSpace(strings.ReplaceAll(strings.ToUpper(plid), "PLID", ""))
		urlStr := fmt.Sprintf("https://api.takealot.com/rest/v-1-16-0/product-details/PLID%s", cleanPLID)
		nowStr := time.Now().Format("2006-01-02 15:04:05")

		title := fmt.Sprintf("⚠️ [抓取失败] 店铺: %s | PLID: %s", storeName, plid)
		if tsinID != "" {
			title += fmt.Sprintf(" (TSIN: %s)", tsinID)
		}
		summary := fmt.Sprintf("前台价格接口异常 (%s): %s", nowStr, errorMsg)

		htmlContent := fmt.Sprintf(`<h3>⚠️ Takealot 前台竞品价格抓取失败</h3>
<ul>
  <li><b>发生时间:</b> %s</li>
  <li><b>关联店铺:</b> %s</li>
  <li><b>TSIN ID:</b> %s</li>
  <li><b>PLID:</b> %s</li>
  <li><b>目标接口 URL:</b> <a href="%s" target="_blank">%s</a></li>
  <li><b>错误详情:</b> <pre>%s</pre></li>
</ul>`, nowStr, storeName, tsinID, plid, urlStr, urlStr, errorMsg)

		_ = r.postContent(r.cfg.PriceFetchCategoryID, title, summary, htmlContent)
	}()
}
