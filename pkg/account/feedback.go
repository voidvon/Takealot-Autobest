package account

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// SubmitFeedback 通过会员服务所在站点的公开反馈表单提交异常。
// /api/feedback 接收 URL 编码表单，返回 {"ok":true}，与会员 API 的 JSON 协议不同。
func (m *Manager) SubmitFeedback(ctx context.Context, title, content string) error {
	m.mu.Lock()
	if m.base == "" {
		m.mu.Unlock()
		return fmt.Errorf("尚未配置会员服务")
	}
	if m.signedOut || m.token == "" || !m.status.Authenticated || m.status.User == nil {
		m.mu.Unlock()
		return fmt.Errorf("未登录，无法提交自动反馈")
	}
	base, token, user := m.base, m.token, *m.status.User
	m.mu.Unlock()

	name := user.DisplayName
	if name == "" {
		name = user.Username
	}
	form := url.Values{
		"title": {title},
		"name":  {name},
		// 默认反馈表单要求 phone；会员资料没有电话，不编造联系方式。
		"phone":     {"未提供（客户端自动反馈）"},
		"content":   {content},
		"source":    {"Takealot-Autobest"},
		"username":  {user.Username},
		"member_id": {strconv.FormatInt(user.ID, 10)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/feedback", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "gocms_user", Value: token})
	res, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("无法连接信息反馈服务: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("信息反馈服务返回 HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&result); err != nil || !result.OK {
		return fmt.Errorf("信息反馈服务未确认提交成功")
	}
	return nil
}
