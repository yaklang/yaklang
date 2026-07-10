package dingtalk

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// dingtalkAPI 是钉钉新版 API 默认域名。
const dingtalkAPI = "https://api.dingtalk.com"

// tokenManager 管理钉钉 access_token 的获取与缓存。
//
// 钉钉 access_token 默认有效期 7200s，这里预留 5min 提前刷新，
// 避免高并发下临界点失效（借鉴 cc-connect platform/dingtalk getAccessToken）。
type tokenManager struct {
	mu         sync.Mutex
	appKey     string
	appSecret  string
	baseURL    string
	proxy      string
	timeout    time.Duration
	token      string
	expireTime time.Time
}

func newTokenManager(cfg *notify.SendConfig) *tokenManager {
	base := dingtalkAPI
	if cfg.BaseURL != "" {
		base = cfg.BaseURL
	}
	return &tokenManager{
		appKey:    cfg.AppID,
		appSecret: cfg.AppSecret,
		baseURL:   base,
		proxy:     cfg.Proxy,
		timeout:   cfg.Timeout,
	}
}

type accessTokenResp struct {
	AccessToken string `json:"accessToken"`
	ExpireIn    int    `json:"expireIn"`
}

func (t *tokenManager) getToken() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// 缓存命中
	if t.token != "" && time.Now().Before(t.expireTime) {
		return t.token, nil
	}
	if t.appKey == "" || t.appSecret == "" {
		return "", fmt.Errorf("dingtalk: appKey/appSecret (AppID/AppSecret) is required")
	}

	opts := buildHTTPOpts(t.proxy, t.timeout)
	url := t.baseURL + "/v1.0/oauth2/accessToken"
	result, err := httpclient.Do("POST", url, map[string]string{"Content-Type": "application/json"}, nil,
		map[string]string{"appKey": t.appKey, "appSecret": t.appSecret}, opts...)
	if err != nil {
		return "", fmt.Errorf("dingtalk: get access token: %w", err)
	}
	if result.StatusCode != 200 {
		return "", fmt.Errorf("dingtalk: get access token status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp accessTokenResp
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		return "", fmt.Errorf("dingtalk: decode access token: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("dingtalk: empty access token in response: %s", string(result.Body))
	}
	expire := time.Duration(resp.ExpireIn) * time.Second
	if expire <= 0 {
		expire = 2 * time.Hour // 钉钉默认 7200s
	}
	t.token = resp.AccessToken
	t.expireTime = time.Now().Add(expire - 5*time.Minute)
	return t.token, nil
}

// buildHTTPOpts 构造 lowhttp 选项（代理 / 超时）。
func buildHTTPOpts(proxy string, timeout time.Duration) []lowhttp.LowhttpOpt {
	var opts []lowhttp.LowhttpOpt
	if proxy != "" {
		opts = append(opts, lowhttp.WithProxy(proxy))
	}
	if timeout > 0 {
		opts = append(opts, lowhttp.WithTimeoutFloat(timeout.Seconds()))
	}
	return opts
}
