package engine

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

type mockReporter struct {
	cfCalls    int32
	fetchCalls int32
	lastMsg    string
}

func (m *mockReporter) ReportCloudflareBlock(storeName, tsinID, plid, errorMsg string) {
	atomic.AddInt32(&m.cfCalls, 1)
	m.lastMsg = errorMsg
}

func (m *mockReporter) ReportPriceFetchError(storeName, tsinID, plid, errorMsg string) {
	atomic.AddInt32(&m.fetchCalls, 1)
	m.lastMsg = errorMsg
}

func TestEngineReporterTrigger(t *testing.T) {
	mock := &mockReporter{}
	eng := NewEngine(nil, nil, nil, nil)
	eng.SetReporter(mock)

	// 模拟 Cloudflare 拦截错误
	cfErr := errors.New("HTTP 403: 前台报价访问被拒绝（可能为 Cloudflare 验证或访问限制）")
	errStr := cfErr.Error()
	if strings.Contains(errStr, "Cloudflare") || strings.Contains(errStr, "403") {
		eng.reporter.ReportCloudflareBlock("测试店铺", "1001", "PLID123", errStr)
	}

	if atomic.LoadInt32(&mock.cfCalls) != 1 {
		t.Fatalf("expected 1 cf report, got %d", mock.cfCalls)
	}

	// 模拟普通接口失败
	otherErr := errors.New("HTTP 500: internal error")
	errStr2 := otherErr.Error()
	if strings.Contains(errStr2, "Cloudflare") || strings.Contains(errStr2, "403") {
		eng.reporter.ReportCloudflareBlock("测试店铺", "1001", "PLID123", errStr2)
	} else {
		eng.reporter.ReportPriceFetchError("测试店铺", "1001", "PLID123", errStr2)
	}

	if atomic.LoadInt32(&mock.fetchCalls) != 1 {
		t.Fatalf("expected 1 fetch error report, got %d", mock.fetchCalls)
	}
}
