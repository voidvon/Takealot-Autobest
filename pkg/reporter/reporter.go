package reporter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Reporter 定义错误上报接口。
type Reporter interface {
	ReportCloudflareBlock(storeName, tsinID, plid, errorMsg string)
	ReportPriceFetchError(storeName, tsinID, plid, errorMsg string)
}

// FeedbackSubmitter 使用普通用户的信息反馈渠道提交，不需要后台权限。
type FeedbackSubmitter interface {
	SubmitFeedback(ctx context.Context, title, content string) error
}

// CMSReporter 将抓价异常提交至 GoCMS 信息反馈。
type CMSReporter struct {
	submitter         FeedbackSubmitter
	onError           func(error)
	mu                sync.Mutex
	lastReport        map[string]time.Time
	rateLimitDuration time.Duration
}

func New(submitter FeedbackSubmitter, onError func(error)) *CMSReporter {
	return &CMSReporter{
		submitter:         submitter,
		onError:           onError,
		lastReport:        make(map[string]time.Time),
		rateLimitDuration: 15 * time.Minute,
	}
}

func (r *CMSReporter) shouldRateLimit(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if len(r.lastReport) > 2000 {
		for k, t := range r.lastReport {
			if now.Sub(t) > 24*time.Hour {
				delete(r.lastReport, k)
			}
		}
	}
	if last, ok := r.lastReport[key]; ok && now.Sub(last) < r.rateLimitDuration {
		return true
	}
	// 失败也保留限流，避免服务异常时每轮巡检反复请求。
	r.lastReport[key] = now
	return false
}

func (r *CMSReporter) report(kind, storeName, tsinID, plid, errorMsg string) {
	if r == nil || r.submitter == nil {
		return
	}
	key := fmt.Sprintf("%q:%q:%q:%q", kind, storeName, tsinID, plid)
	if r.shouldRateLimit(key) {
		return
	}

	go func() {
		cleanPLID := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(plid)), "PLID")
		title := fmt.Sprintf("[Takealot][%s] 店铺: %s | PLID: %s", kind, storeName, plid)
		if tsinID != "" {
			title += fmt.Sprintf(" (TSIN: %s)", tsinID)
		}
		content := fmt.Sprintf(`Takealot 前台价格抓取异常自动反馈
异常类型: %s
发生时间: %s
关联店铺: %s
TSIN ID: %s
PLID: %s
目标接口 URL: https://api.takealot.com/rest/v-1-16-0/product-details/PLID%s
错误详情: %s
本轮已暂停该商品调价。`, kind, time.Now().Format(time.RFC3339), storeName, tsinID, plid, cleanPLID, errorMsg)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := r.submitter.SubmitFeedback(ctx, title, content); err != nil && r.onError != nil {
			r.onError(fmt.Errorf("信息反馈提交失败 (店铺: %s, TSIN: %s, PLID: %s): %w", storeName, tsinID, plid, err))
		}
	}()
}

func (r *CMSReporter) ReportCloudflareBlock(storeName, tsinID, plid, errorMsg string) {
	r.report("Cloudflare拦截", storeName, tsinID, plid, errorMsg)
}

func (r *CMSReporter) ReportPriceFetchError(storeName, tsinID, plid, errorMsg string) {
	r.report("抓取失败", storeName, tsinID, plid, errorMsg)
}
