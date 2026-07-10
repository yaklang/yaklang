package dingtalk

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/notify"
	"github.com/yaklang/yaklang/common/notify/internal/httpclient"
	"rsc.io/qr"
)

// 钉钉应用扫码注册（device-code onboarding）官方流程。
//
// 对齐 AstrBot app_registration.py 揭示的钉钉开放平台 device-code 协议，
// 与飞书 onboarding.go 高度对称，但端点/请求体/状态字段不同：
//
//	端点（base = https://oapi.dingtalk.com，JSON body）：
//	  POST /app/registration/init  body {"source":"DING_DWS_CLAW"}  -> {"nonce":"..."}
//	  POST /app/registration/begin body {"nonce":"..."}            -> {device_code, verification_uri_complete, interval, expires_in, ...}
//	  POST /app/registration/poll  body {"device_code":"..."}      -> {status:"WAITING|SUCCESS|FAIL|EXPIRED", client_id, client_secret, fail_reason}
//
// 用户扫码 → 钉钉页面创建/绑定机器人 → poll 返回 SUCCESS + client_id/client_secret，
// 即可直接用于 Stream 接收 + 发送，免去手动建应用填凭证。
const (
	// registrationSource 钉钉要求的来源标识（AstrBot 同值）。
	registrationSource = "DING_DWS_CLAW"

	registrationInitPath  = "/app/registration/init"
	registrationBeginPath = "/app/registration/begin"
	registrationPollPath  = "/app/registration/poll"
)

// registrationBase 钉钉扫码注册默认域名（oapi，与新版 api.dingtalk.com 区分）。
// 用包变量而非常量，便于测试覆盖到本地 mock server。
var registrationBase = registrationBaseURL

// registrationBaseURL 钉钉扫码注册默认域名（oapi，与新版 api.dingtalk.com 区分）。
const registrationBaseURL = "https://oapi.dingtalk.com"

// 钉钉注册端点响应结构。
type registrationInitResp struct {
	Nonce   string `json:"nonce"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}
type registrationBeginResp struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	ErrCode                 int    `json:"errcode"`
	ErrMsg                  string `json:"errmsg"`
}
type registrationPollResp struct {
	Status       string `json:"status"` // WAITING / SUCCESS / FAIL / EXPIRED
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	FailReason   string `json:"fail_reason"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

// registrationPost 调一次钉钉 onboarding 端点（JSON body）。
func registrationPost(base, path string, body any, out any) error {
	urlStr := base + path
	headers := map[string]string{"Content-Type": "application/json"}
	result, err := httpclient.Do("POST", urlStr, headers, nil, body)
	if err != nil {
		return err
	}
	if result.StatusCode != 200 {
		return fmt.Errorf("dingtalk onboarding %s: status %d: %s", path, result.StatusCode, string(result.Body))
	}
	if err := json.Unmarshal(result.Body, out); err != nil {
		return fmt.Errorf("dingtalk onboarding %s: decode: %w (body=%s)", path, err, string(result.Body))
	}
	return nil
}

// RunOnboarding 执行完整的钉钉扫码注册流程，通过 handler 流式推送状态。
//
// opts 预留，钉钉暂未使用。
// gRPC 层只通过 notify driver 的 onboarding:start stream 入口触发。
// timeoutSeconds<=0 时默认 600s。
func RunOnboarding(timeoutSeconds int, opts map[string]string, handler notify.OnboardingHandler) error {
	if handler == nil {
		return fmt.Errorf("dingtalk onboarding handler is nil")
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 600
	}
	base := registrationBase

	// 1) init：拿 nonce
	var initRes registrationInitResp
	if err := registrationPost(base, registrationInitPath, map[string]string{
		"source": registrationSource,
	}, &initRes); err != nil {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("init failed: %v", err)})
	}
	if initRes.ErrCode != 0 {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("init errcode=%d: %s", initRes.ErrCode, initRes.ErrMsg)})
	}
	if initRes.Nonce == "" {
		return handler(&notify.OnboardingStep{State: "error", Message: "init: missing nonce"})
	}

	// 2) begin：拿 device_code + 扫码 URL
	var beginRes registrationBeginResp
	if err := registrationPost(base, registrationBeginPath, map[string]string{
		"nonce": initRes.Nonce,
	}, &beginRes); err != nil {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("begin failed: %v", err)})
	}
	if beginRes.ErrCode != 0 {
		return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("begin errcode=%d: %s", beginRes.ErrCode, beginRes.ErrMsg)})
	}
	if beginRes.DeviceCode == "" || beginRes.VerificationURIComplete == "" {
		return handler(&notify.OnboardingStep{State: "error", Message: "begin: incomplete response"})
	}

	// 推送二维码给前端（URL + 渲染好的 PNG，前端零依赖直接 <img> 展示）
	qrPNG := renderQRPNG(beginRes.VerificationURIComplete)
	if err := handler(&notify.OnboardingStep{
		State: "qr",
		QrURL: beginRes.VerificationURIComplete,
		QrPNG: qrPNG,
	}); err != nil {
		return err
	}

	interval := beginRes.Interval
	if interval <= 0 {
		interval = 3
	}
	expireIn := beginRes.ExpiresIn
	if expireIn <= 0 {
		expireIn = timeoutSeconds
	}
	deadline := time.Now().Add(time.Duration(expireIn) * time.Second)
	if limitByFlag := time.Now().Add(time.Duration(timeoutSeconds) * time.Second); limitByFlag.Before(deadline) {
		deadline = limitByFlag
	}

	// 3) poll：轮询直到 SUCCESS/FAIL/EXPIRED/超时
	for time.Now().Before(deadline) {
		var pollRes registrationPollResp
		if err := registrationPost(base, registrationPollPath, map[string]string{
			"device_code": beginRes.DeviceCode,
		}, &pollRes); err != nil {
			return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("poll failed: %v", err)})
		}
		if pollRes.ErrCode != 0 {
			return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("poll errcode=%d: %s", pollRes.ErrCode, pollRes.ErrMsg)})
		}

		switch pollRes.Status {
		case "", "WAITING":
			_ = handler(&notify.OnboardingStep{State: "pending", Message: "waiting for scan/confirm"})
		case "SUCCESS":
			if pollRes.ClientID == "" || pollRes.ClientSecret == "" {
				return handler(&notify.OnboardingStep{State: "error", Message: "扫码成功但未获取到钉钉应用凭证"})
			}
			result := &notify.OnboardingResult{
				AppID:     pollRes.ClientID,
				AppSecret: pollRes.ClientSecret,
				Platform:  notify.PlatformDingTalk,
			}
			return handler(&notify.OnboardingStep{State: "success", Result: result, Message: "onboarding succeeded"})
		case "FAIL":
			msg := pollRes.FailReason
			if msg == "" {
				msg = "钉钉扫码创建失败"
			}
			return handler(&notify.OnboardingStep{State: "error", Message: msg})
		case "EXPIRED":
			return handler(&notify.OnboardingStep{State: "expired", Message: "钉钉扫码已过期，请重新创建"})
		default:
			return handler(&notify.OnboardingStep{State: "error", Message: fmt.Sprintf("未知状态: %s", pollRes.Status)})
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}
	return handler(&notify.OnboardingStep{State: "expired", Message: "timed out waiting for QR onboarding result"})
}

// renderQRPNG 把内容渲染成二维码 PNG 字节（rsc.io/qr）。
// 失败时返回 nil，调用方应回退到用 QrURL 自行渲染或展示链接。
func renderQRPNG(content string) []byte {
	if content == "" {
		return nil
	}
	code, err := qr.Encode(content, qr.Q)
	if err != nil {
		log.Warnf("dingtalk onboarding: render qr failed: %v", err)
		return nil
	}
	return code.PNG()
}
