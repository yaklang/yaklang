package feishu

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// feishuOpen 默认飞书开放平台域名（Lark 海外版可经 BaseURL 覆盖）。
const feishuOpen = "https://open.feishu.cn"

// tokenManager 管理飞书 tenant_access_token 的获取与缓存（默认有效期 7200s，预留 5min）。
type tokenManager struct {
	mu         sync.Mutex
	appID      string
	appSecret  string
	baseURL    string
	proxy      string
	timeout    time.Duration
	token      string
	expireTime time.Time
}

func newTokenManager(cfg *notify.SendConfig) *tokenManager {
	base := feishuOpen
	if cfg.BaseURL != "" {
		base = cfg.BaseURL
	}
	return &tokenManager{
		appID:     cfg.AppID,
		appSecret: cfg.AppSecret,
		baseURL:   base,
		proxy:     cfg.Proxy,
		timeout:   cfg.Timeout,
	}
}

type tenantTokenResp struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

func (t *tokenManager) getToken() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && time.Now().Before(t.expireTime) {
		return t.token, nil
	}
	if t.appID == "" || t.appSecret == "" {
		return "", fmt.Errorf("feishu: app_id/app_secret (AppID/AppSecret) is required")
	}
	url := t.baseURL + "/open-apis/auth/v3/tenant_access_token/internal"
	opts := buildHTTPOpts(t.proxy, t.timeout)
	result, err := httpclient.Do("POST", url, jsonHeaders(), nil,
		map[string]string{"app_id": t.appID, "app_secret": t.appSecret}, opts...)
	if err != nil {
		return "", fmt.Errorf("feishu: get tenant_access_token: %w", err)
	}
	if result.StatusCode != 200 {
		return "", fmt.Errorf("feishu: get token status %d: %s", result.StatusCode, string(result.Body))
	}
	var resp tenantTokenResp
	if err := json.Unmarshal(result.Body, &resp); err != nil {
		return "", fmt.Errorf("feishu: decode token resp: %w", err)
	}
	if resp.Code != 0 || resp.TenantAccessToken == "" {
		return "", fmt.Errorf("feishu: get token failed code=%d msg=%s", resp.Code, resp.Msg)
	}
	expire := time.Duration(resp.Expire) * time.Second
	if expire <= 0 {
		expire = 2 * time.Hour
	}
	t.token = resp.TenantAccessToken
	t.expireTime = time.Now().Add(expire - 5*time.Minute)
	return t.token, nil
}

func jsonHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json; charset=utf-8"}
}

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
