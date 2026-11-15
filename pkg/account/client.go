package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// memberClient owns the GoCMS wire protocol. Credentials are passed per request;
// this client never reads cookies, persists tokens, or changes account state.
type memberClient struct {
	base string
	http *http.Client
}

func newMemberClient(base string) (*memberClient, error) {
	c := &memberClient{
		base: strings.TrimRight(base, "/"),
		http: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("会员服务不允许重定向")
			},
		},
	}
	if base == "" {
		return c, nil
	}
	u, err := url.Parse(c.base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()))) {
		return nil, errors.New("会员服务地址必须为 HTTPS；仅本机开发允许 HTTP")
	}
	return c, nil
}

// validBearerToken accepts the Bearer token alphabet, including legacy session
// values saved by the Cookie client. Never put whitespace/control bytes in headers.
func validBearerToken(token string) bool {
	if token == "" || len(token) > 512 {
		return false
	}
	padding := false
	for _, ch := range token {
		if ch == '=' {
			padding = true
			continue
		}
		if padding || !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("-._~+/", ch)) {
			return false
		}
	}
	return token[0] != '='
}

func (c *memberClient) do(ctx context.Context, method, path, token, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		if !validBearerToken(token) {
			return nil, &APIError{Code: "authentication_required", Message: "登录凭据无效，请重新登录", Status: http.StatusUnauthorized}
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, &APIError{Code: "service_unavailable", Message: "无法连接会员服务，请检查网络后重试", Status: http.StatusServiceUnavailable}
	}
	return res, nil
}

func invalidResponse() error {
	return &APIError{Code: "service_unavailable", Message: "会员服务响应异常", Status: http.StatusServiceUnavailable}
}

func (c *memberClient) call(ctx context.Context, method, path, token string, input, output any) error {
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	res, err := c.do(ctx, method, "/api/v1"+path, token, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	const maxResponseSize = 1 << 20
	raw, readErr := io.ReadAll(io.LimitReader(res.Body, maxResponseSize+1))
	var envelope struct {
		Data  json.RawMessage `json:"data"`
		Error APIError        `json:"error"`
	}
	valid := readErr == nil && len(raw) <= maxResponseSize && json.Unmarshal(raw, &envelope) == nil
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Preserve HTTP 401 even when a proxy returns HTML instead of JSON, so a
		// rejected session cannot remain cached as an authenticated account.
		if !valid || envelope.Error.Code == "" {
			envelope.Error = APIError{Code: "service_error", Message: "会员请求失败"}
		}
		envelope.Error.Status = res.StatusCode
		if envelope.Error.Message == "" {
			envelope.Error.Message = "会员请求失败"
		}
		return &envelope.Error
	}
	if !valid || len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) || envelope.Error.Code != "" {
		return invalidResponse()
	}
	if output != nil && json.Unmarshal(envelope.Data, output) != nil {
		return invalidResponse()
	}
	return nil
}

type tokenGrant struct {
	User        User   `json:"user"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (c *memberClient) login(ctx context.Context, currentToken, identifier, password string) (tokenGrant, error) {
	var grant tokenGrant
	err := c.call(ctx, http.MethodPost, "/auth/token", currentToken,
		map[string]string{"identifier": identifier, "password": password}, &grant)
	if err != nil {
		return tokenGrant{}, err
	}
	if !validBearerToken(grant.AccessToken) || !strings.EqualFold(grant.TokenType, "Bearer") ||
		grant.ExpiresIn <= 0 || grant.User.ID <= 0 || grant.User.Username == "" {
		return tokenGrant{}, invalidResponse()
	}
	return grant, nil
}
