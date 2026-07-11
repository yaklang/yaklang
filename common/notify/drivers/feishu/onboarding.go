package feishu

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"rsc.io/qr"
)

// 飞书/Lark 应用注册（device-code onboarding）官方流程。
//
// 端点：POST <base>/oauth/v1/app/registration  (form-encoded)
//
//	action=init  -> 检查是否支持 client_secret
//	action=begin -> 拿 device_code + verification_uri_complete(扫码URL) + interval + expire_in
//	action=poll  -> 轮询，成功返回 client_id/client_secret
//
// 默认域名：飞书 https://accounts.feishu.cn，Lark 海外版 https://accounts.larksuite.com
// 通过 BaseURL 切换（前端可让用户选飞书/Lark）。
const (
	accountsFeishuBase = "https://accounts.feishu.cn"
	accountsLarkBase   = "https://accounts.larksuite.com"
	onboardingPath     = "/oauth/v1/app/registration"
)

type registrationInitResp struct {
	SupportedAuthMethods []string `json:"supported_auth_methods"`
	Error                string   `json:"error"`
	ErrorDescription     string   `json:"error_description"`
}
type registrationBeginResp struct {
	DeviceCode              string `json:"device_code"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpireIn                int    `json:"expire_in"`
	Error                   string `json:"error"`
	ErrorDescription        string `json:"error_description"`
}
type registrationPollResp struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	UserInfo     struct {
		OpenID      string `json:"open_id"`
		TenantBrand string `json:"tenant_brand"`
	} `json:"user_info"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// accountsBase 根据 isLark 选择 onboarding 域名。
func accountsBase(isLark bool) string {
	if isLark {
		return accountsLarkBase
	}
	return accountsFeishuBase
}

// registrationCall 调一次 onboarding 端点（form-encoded）。
func registrationCall(base, action string, params map[string]string, out any) error {
	form := url.Values{}
	form.Set("action", action)
	for k, v := range params {
		form.Set(k, v)
	}
	urlStr := base + onboardingPath
	req, err := http.NewRequest(http.MethodPost, urlStr, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s response: %w (body=%s)", action, err, string(body))
	}
	return nil
}

// RunOnboarding 执行完整的飞书/Lark 扫码注册流程，通过 handler 流式推送状态。
//
// opts 里用 "is_lark"="true" 切换 Lark 海外版域名。
// gRPC 层只通过 notify driver 的 onboarding:start stream 入口触发。
// timeoutSeconds<=0 时默认 600s。
func RunOnboarding(timeoutSeconds int, opts map[string]string, handler notify.OnboardingHandler) error {
	if handler == nil {
		return fmt.Errorf("onboarding handler is nil")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 600
	}
	isLark := strings.EqualFold(opts["is_lark"], "true")
	platform := notify.PlatformFeishu
	if isLark {
		platform = notify.PlatformType("lark")
	}
	base := accountsBase(isLark)

	// 1) init：检查环境是否支持 client_secret
	var initRes registrationInitResp
	if err := registrationCall(base, "init", nil, &initRes); err != nil {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("init failed: %v", err)})
	}
	if initRes.Error != "" {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("%s: %s", initRes.Error, initRes.ErrorDescription)})
	}
	if len(initRes.SupportedAuthMethods) > 0 && !containsAuthMethod(initRes.SupportedAuthMethods, "client_secret") {
		return handler(&notify.OnboardingStep{State: "error", Message: "current environment does not support client_secret auth"})
	}

	// 2) begin：拿 device_code + 扫码 URL
	var beginRes registrationBeginResp
	beginParams := map[string]string{
		"archetype":         "PersonalAgent",
		"auth_method":       "client_secret",
		"request_user_info": "open_id",
	}
	if err := registrationCall(base, "begin", beginParams, &beginRes); err != nil {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("begin failed: %v", err)})
	}
	if beginRes.Error != "" {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("%s: %s", beginRes.Error, beginRes.ErrorDescription)})
	}
	if beginRes.DeviceCode == "" || beginRes.VerificationURIComplete == "" {
		return handler(&notify.OnboardingStep{State: "error", Message: "incomplete onboarding response"})
	}

	// 推送二维码给前端（URL + 渲染好的 PNG，前端零依赖直接 <img> 展示）。
	// IM 消息事件和卡片按钮回调都需要在扫码注册时申请；否则新建应用可能
	// 只能展示卡片，不能在客户端输入消息或把按钮点击推给后端。
	qrURL, err := buildFeishuOnboardingQRURL(beginRes.VerificationURIComplete, opts)
	if err != nil {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("build qr url failed: %v", err)})
	}
	qrPNG := renderQRPNG(qrURL)
	if err := handler(&notify.OnboardingStep{
		State: "qr",
		QrURL: qrURL,
		QrPNG: qrPNG,
	}); err != nil {
		return err
	}

	interval := beginRes.Interval
	if interval <= 0 {
		interval = 5
	}
	expireIn := beginRes.ExpireIn
	if expireIn <= 0 {
		expireIn = timeoutSeconds
	}
	deadline := time.Now().Add(time.Duration(expireIn) * time.Second)
	if limitByFlag := time.Now().Add(time.Duration(timeoutSeconds) * time.Second); limitByFlag.Before(deadline) {
		deadline = limitByFlag
	}

	// 3) poll：轮询直到成功/超时/拒绝
	for time.Now().Before(deadline) {
		var pollRes registrationPollResp
		if err := registrationCall(base, "poll", map[string]string{"device_code": beginRes.DeviceCode}, &pollRes); err != nil {
			return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("poll failed: %v", err)})
		}

		// 成功：拿到凭证
		if pollRes.ClientID != "" && pollRes.ClientSecret != "" {
			result := onboardingResultFromPoll(platform, pollRes)
			return handler(&notify.OnboardingStep{State: "success", Result: result, Message: "onboarding succeeded"})
		}

		// 状态分支
		switch pollRes.Error {
		case "", "authorization_pending":
			_ = handler(&notify.OnboardingStep{State: "pending", Message: "waiting for scan/confirm"})
		case "slow_down":
			interval += 5
			log.Debugf("feishu onboarding: slow_down, interval -> %d", interval)
		case "access_denied":
			return handler(&notify.OnboardingStep{State: "error", Message: "authorization denied by user"})
		case "expired_token":
			return handler(&notify.OnboardingStep{State: "expired", Message: "onboarding session expired"})
		default:
			return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("%s: %s", pollRes.Error, pollRes.ErrorDescription)})
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}
	return handler(&notify.OnboardingStep{State: "expired", Message: "timed out waiting for QR onboarding result"})
}

func onboardingResultFromPoll(platform notify.PlatformType, pollRes registrationPollResp) *notify.OnboardingResult {
	return &notify.OnboardingResult{
		AppID:     pollRes.ClientID,
		AppSecret: pollRes.ClientSecret,
		Platform:  platform,
		OwnerID:   strings.TrimSpace(pollRes.UserInfo.OpenID),
	}
}

func containsAuthMethod(values []string, expected string) bool {
	for _, v := range values {
		if strings.EqualFold(strings.TrimSpace(v), expected) {
			return true
		}
	}
	return false
}

func buildFeishuOnboardingQRURL(rawURL string, opts map[string]string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("from", "sdk")
	query.Set("tp", "sdk")
	query.Set("source", "go-sdk/yaklang-im")
	encodedAddons, err := encodeFeishuOnboardingAddons()
	if err != nil {
		return "", err
	}
	query.Set("addons", encodedAddons)
	if strings.EqualFold(opts["create_only"], "true") {
		query.Set("createOnly", "true")
	}
	if appID := strings.TrimSpace(opts["app_id"]); appID != "" {
		query.Set("clientID", appID)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func encodeFeishuOnboardingAddons() (string, error) {
	payload := map[string]any{
		"events": map[string]any{
			"items": map[string]any{
				"tenant": []string{"im.message.receive_v1"},
			},
		},
		"callbacks": map[string]any{
			"items": []string{"card.action.trigger"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

// renderQRPNG 把内容渲染成二维码 PNG 字节（rsc.io/qr）。
// 失败时返回 nil，调用方应回退到用 QrURL 自行渲染或展示链接。
func renderQRPNG(content string) []byte {
	if content == "" {
		return nil
	}
	code, err := qr.Encode(content, qr.Q)
	if err != nil {
		log.Warnf("feishu onboarding: render qr failed: %v", err)
		return nil
	}
	return code.PNG()
}
