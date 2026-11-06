package aicommon

import (
	"context"
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	notifyTypeNetworkError         = "network_error"
	notifyTypeAPIError             = "api_error"
	notifyTypeAuthenticationFailed = "authentication_failed"
)

// emitAITransactionFailureNotify 将最终 AI 请求失败转换为用户可见通知。
// 模型输出校验失败不属于请求/API 错误，不会触发这里。
func emitAITransactionFailureNotify(c AICallerConfigIf, rsp *AIResponse, err error) {
	if c == nil || err == nil ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return
	}
	emitter := c.GetEmitter()
	if utils.IsNil(emitter) {
		return
	}

	status := 0
	if rsp != nil && !utils.IsNil(rsp) {
		status = rsp.GetHTTPStatusCode()
	}
	// 429 已由 handle429RateLimitContext 发出 rate_limited/quota_exceeded 通知。
	if status == 429 {
		return
	}

	detail := strings.ToLower(strings.TrimSpace(err.Error()))
	if rsp != nil && !utils.IsNil(rsp) {
		detail += " " + strings.ToLower(string(decodeAIHTTPErrorBody(rsp)))
	}

	notifyType, message := classifyAIFailureNotify(status, detail)
	if notifyType == "" {
		return
	}
	if _, emitErr := emitter.EmitNotify(notifyType, message, 0); emitErr != nil {
		log.Warnf("emit ai failure notification failed: %v", emitErr)
		return
	}
	log.Warnf("AI request failure notified as %s: %s", notifyType, strings.ReplaceAll(message, "\n", " | "))
}

func classifyAIFailureNotify(status int, detail string) (notifyType string, message string) {
	quotaMarkers := []string{
		"insufficient_quota", "insufficient quota", "quota exceeded", "quota exhausted",
		"insufficient balance", "insufficient credit", "payment required", "billing",
		"余额不足", "欠费", "额度耗尽", "额度不足",
	}
	if status == 402 || containsAny(detail, quotaMarkers...) {
		return notify429TypeQuotaExceeded,
			"AI 服务额度不足或已欠费，恢复额度后将继续任务。\nAI service quota is exhausted or payment is required; the task will continue when capacity is restored."
	}
	if status == 401 || status == 403 ||
		containsAny(detail, "unauthorized", "authentication failed", "invalid api key", "forbidden") {
		return notifyTypeAuthenticationFailed,
			"AI 服务鉴权失败，请检查 API Key 或访问权限。\nAI service authentication failed; check the API key or access permissions."
	}
	if status == 0 && isAICallbackInfrastructureError(errors.New(detail)) ||
		containsAny(detail, "connection refused", "connection reset", "no such host", "i/o timeout", "unexpected eof", "broken pipe") {
		return notifyTypeNetworkError,
			"AI 服务网络请求失败，正在等待网络恢复后继续任务。\nAI service network request failed; the task will continue after connectivity recovers."
	}
	return notifyTypeAPIError,
		"AI 服务返回错误，任务将保留当前进度并继续尝试。\nAI service returned an error; current progress is preserved and execution will continue."
}

func containsAny(text string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
