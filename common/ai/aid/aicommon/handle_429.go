package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// 429 处理设计
//
// 本文件处理所有 AI 请求返回的 HTTP 429 响应，兼容 AIBalance 网关和直连
// 第三方提供商（OpenAI/Anthropic/Azure 等）两种场景。
//
// 设计原则：不硬编码任何提供商的 kind 列表。分类与退避策略完全由 429 响应
// 自身携带的标准信号驱动：
//
//   - Retry-After 响应头：HTTP 429 标准契约，所有主流 AI 提供商均支持。
//     有合理值（< 3600）→ 限流/过载类，可重试，按 Retry-After 退避。
//     无值 → 额度耗尽类，不会短期自动恢复，不应重试。
//     大额值（>= 3600）→ 额度到次日才恢复（如 AIBalance daily_token），
//     不应重试。
//   - error.message：响应体 JSON 的 error.message 字段，兼容 OpenAI /
//     Anthropic / AIBalance 三种格式。直接展示服务端返回的原始文案。
//   - X-AIBalance-Limit-Kind 响应头：AIBalance 的稳定限流标识，仅用于日志
//     记录和旧版队列头兼容，不参与分类逻辑。
//
// 返回值 (is429, shouldRetry, ctxDone)：
//   - is429:       是否检测到 429
//   - shouldRetry: 该 429 是否应该重试（频率限流/过载类=true，额度耗尽类=false）
//   - ctxDone:     context 在等待期间是否被取消

// aibalance429Body 是 429 响应体 JSON 的最小子集，仅解析客户端所需的通用字段。
// 该结构兼容 OpenAI / Anthropic / AIBalance 三种 error 格式，因为它们都用
// error.message 承载错误描述。AIBalance 专有的动态字段也一并列出，供日志和
// 未来扩展使用。
type aibalance429Body struct {
	Error struct {
		Message           string          `json:"message"`
		Type              string          `json:"type"`
		LimitKind         string          `json:"limit_kind"`
		LimitKindZh       string          `json:"limit_kind_zh"`
		Code              json.RawMessage `json:"code"`
		Notice            string          `json:"notice"`
		Reason            string          `json:"reason"`
		QuotaInitialized  *bool           `json:"quota_initialized"`
		Bucket            string          `json:"bucket"`
		Model             string          `json:"model"`
		TokensUsed        int64           `json:"tokens_used"`
		TokensLimit       int64           `json:"tokens_limit"`
		ExceededKind      string          `json:"exceeded_kind"`
		QueueLength       int64           `json:"queue_length"`
		RetryAfter        int64           `json:"retry_after"`
		UpstreamLastError string          `json:"upstream_last_error"`
	} `json:"error"`
}

// parse429Body 解析 429 响应体为结构化错误信息。解析失败时返回 nil。
func parse429Body(rsp *AIResponse) *aibalance429Body {
	if rsp == nil {
		return nil
	}
	body := rsp.GetHTTPResponseBody()
	if len(body) == 0 {
		return nil
	}
	var parsed aibalance429Body
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return &parsed
}

// 429 通知类型枚举（英文稳定标识，作为 notify 事件的 NodeId / type，
// 由 schema.NodeIdAndTypeToI18n 翻译为 NodeIdVerbose 供前端 i18n 展示）。
const (
	// notify429TypeRateLimited 表示频率限流/过载类 429，可重试。
	notify429TypeRateLimited = "rate_limited"
	// notify429TypeQuotaExceeded 表示额度耗尽类 429，不可重试。
	notify429TypeQuotaExceeded = "quota_exceeded"
)

// bodyLimitKindAliases 把 AIBalance 响应体 error.limit_kind 中与响应头
// X-AIBalance-Limit-Kind 不一致的值映射回稳定标识，仅用于日志记录。
var bodyLimitKindAliases = map[string]string{
	"daily_token_quota": "daily_token",
	"free_ip_quota":     "free_ip",
}

// resolveLimitKind 优先从 X-AIBalance-Limit-Kind 响应头读取 AIBalance 的限流
// 标识；若响应头缺失，则回退到响应体 error.limit_kind（经别名归一化）。
// 返回值仅用于日志记录和旧版队列头兼容判断，不参与分类逻辑。
func resolveLimitKind(rsp *AIResponse, body *aibalance429Body) string {
	if rsp != nil {
		if hk := strings.TrimSpace(rsp.GetHTTPHeader("X-AIBalance-Limit-Kind")); hk != "" {
			return hk
		}
	}
	if body != nil {
		bk := strings.TrimSpace(body.Error.LimitKind)
		if bk != "" {
			if alias, ok := bodyLimitKindAliases[bk]; ok {
				return alias
			}
			return bk
		}
	}
	return ""
}

// parseRetryAfterSeconds 解析 Retry-After 响应头（整数秒或 HTTP-date）。
// 失败时回退到 fallback 秒数。
func parseRetryAfterSeconds(rsp *AIResponse, fallback int) int {
	if rsp != nil {
		if raw := strings.TrimSpace(rsp.GetHTTPHeader("Retry-After")); raw != "" {
			if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
				return sec
			}
			if until, err := http.ParseTime(raw); err == nil {
				if seconds := int(math.Ceil(time.Until(until).Seconds())); seconds > 0 {
					return seconds
				}
			}
		}
	}
	return fallback
}

// is429Retryable 判断一个 429 响应是否值得重试。
// 分类逻辑与 handle429RateLimitContext 一致：
//   - 有合理 Retry-After（< 3600）：限流/过载类，可重试。
//   - 无 Retry-After 或大额 Retry-After（>= 3600）：额度耗尽类，不可重试。
func is429Retryable(ctx context.Context, rsp *AIResponse) bool {
	if !is429Response(ctx, rsp) {
		return false
	}
	retryAfter := parseRetryAfterSeconds(rsp, 0)
	return retryAfter > 0 && retryAfter < 3600
}

// capRetryAfterSeconds 将秒数限制在 [minSec, maxSec] 区间。
// maxSec <= 0 表示不限上限。
func capRetryAfterSeconds(sec, minSec, maxSec int) int {
	if sec < minSec {
		return minSec
	}
	if maxSec > 0 && sec > maxSec {
		return maxSec
	}
	return sec
}

// jitterSeconds 在 base 秒的基础上叠加最多 maxJitter 秒的随机抖动，
// 避免大量客户端在同一时刻同时重试（惊群）。
func jitterSeconds(base, maxJitter int) int {
	if maxJitter <= 0 {
		return base
	}
	return base + rand.Intn(maxJitter)
}

// emit429Notify 统一发出 429 通知事件并记录日志。
func emit429Notify(c AICallerConfigIf, notifyType, msg string, waitDuration time.Duration, source, kind string) {
	c.GetEmitter().EmitNotify(notifyType, msg, waitDuration)
	log.Infof("%s 429 [kind=%s] %s | waiting %s", source, kind, strings.ReplaceAll(msg, "\n", " "), waitDuration)
}

// handle429RateLimit checks the AI response for a 429 status code, emits the
// appropriate user-facing message, and waits for the correct duration using a
// context-aware select so the wait can be interrupted by context cancellation.
//
// Returns:
//   - is429:       true if a 429 was detected
//   - shouldRetry: true if this 429 is retryable (rate-limit/overload), false if
//     it's a quota-exhaustion that won't recover short-term
//   - ctxDone:     true if the context was cancelled during the wait
func (c *Config) handle429RateLimit(rsp *AIResponse) (is429 bool, shouldRetry bool, ctxDone bool) {
	return handle429RateLimitContext(c.Ctx, c, rsp, nil)
}

// handle429RateLimitContext 是三档模型 callback 共用的 429 处理入口。
// Transaction 中未经过 Config.wrapper 的自定义 callback 也复用这里的策略。
// budget 通过请求字段和函数参数显式传递，context 只负责取消与超时。
//
// 处理流程（提供商无关，由响应信号驱动）：
//  1. 等待响应头就绪，校验状态码为 429。
//  2. 解析响应体 JSON，提取 error.message 和 AIBalance limit_kind（用于日志）。
//  3. 按 Retry-After 响应头有无分流：
//     - 有合理 Retry-After（< 3600）：限流/过载类，可重试，按 Retry-After 退避。
//     - 无 Retry-After：额度耗尽类，不可重试，等待短时间后由上层处理。
//     - 大额 Retry-After（>= 3600）：额度到次日才恢复（如 daily_token），不可重试。
//  4. 兼容旧版 X-AIBalance-Info 队列头：无 limit_kind 但存在队列头时，
//     按队列长度估算等待时间（可重试）。
//  5. 用户文案统一从响应体 error.message 提取，提取失败时回退到默认文案。
func handle429RateLimitContext(ctx context.Context, c AICallerConfigIf, rsp *AIResponse, budget *rateLimitBudget) (is429 bool, shouldRetry bool, ctxDone bool) {
	if rsp == nil {
		return false, false, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if !rsp.WaitForHTTPHeaders(ctx) {
		return false, false, true
	}

	if rsp.GetHTTPStatusCode() != 429 {
		return false, false, false
	}

	// Wait for the response body before parsing the 429 error JSON.
	// The header callback (SetRawHTTPResponseHeader) fires before the body
	// callback (SetRawHTTPResponseData) — if we parse the body immediately
	// after headers arrive, the body may not be set yet.
	if !rsp.WaitForHTTPBody(ctx) {
		return true, false, true
	}

	body := parse429Body(rsp)
	limitKind := resolveLimitKind(rsp, body)
	source := "generic"
	if limitKind != "" {
		source = "aibalance"
	}

	// 旧版兼容：无 limit_kind 但有 X-AIBalance-Info 队列头。
	if limitKind == "" {
		if queueInfo := strings.TrimSpace(rsp.GetHTTPHeader("X-AIBalance-Info")); queueInfo != "" {
			return handleLegacyQueue429(ctx, c, queueInfo, budget)
		}
	}

	// 退避策略：由 Retry-After 驱动。
	retryAfter := parseRetryAfterSeconds(rsp, 0)

	var waitSec int
	var notifyType string

	switch {
	case retryAfter >= 3600:
		// 大额 Retry-After（如 AIBalance daily_token/free_ip 的 3600）：
		// 额度到次日才恢复，不可重试。限制到 [5,30]s 避免长时间阻塞，
		// 由上层立即暴露错误。
		waitSec = capRetryAfterSeconds(jitterSeconds(retryAfter, 0), 5, 30)
		notifyType = notify429TypeQuotaExceeded
		shouldRetry = false
	case retryAfter > 0:
		// 有合理 Retry-After：限流/过载类，可重试，按 Retry-After 退避。
		waitSec = capRetryAfterSeconds(jitterSeconds(retryAfter, 3), 1, 120)
		notifyType = notify429TypeRateLimited
		shouldRetry = true
	default:
		// 无 Retry-After：额度耗尽类（如 token/api_user_token），不可重试。
		// 等待短时间后由上层处理（消耗重试次数或暴露错误）。
		waitSec = 5 + rand.Intn(11)
		notifyType = notify429TypeQuotaExceeded
		shouldRetry = false
	}

	waitDuration := time.Duration(waitSec) * time.Second
	if budget != nil {
		// Never probe before Retry-After. If it exceeds our remaining budget,
		// return the upstream error instead of shortening the cooldown.
		if shouldRetry {
			waitDuration = time.Duration(retryAfter) * time.Second
		}
		if !shouldRetry {
			budget.reserve(0)
			waitDuration = 0
		} else if !budget.reserve(waitDuration) {
			shouldRetry = false
			waitDuration = 0
		}
	}

	// 用户文案：优先从响应体 error.message 提取，回退到默认文案。
	msg := extract429Message(body, limitKind)
	kindLabel := limitKind
	if kindLabel == "" {
		kindLabel = "generic"
	}

	emit429Notify(c, notifyType, msg, waitDuration, source, kindLabel)
	if budget != nil && !shouldRetry {
		return true, false, ctx.Err() != nil
	}
	done := waitBeforeAIRetry(ctx, c, waitDuration) != nil
	return true, shouldRetry, done
}

// extract429Message 从 429 响应体提取用户可见的错误信息。
// 优先展示服务端返回的 error.message（兼容 OpenAI/Anthropic/AIBalance），
// 提取失败时回退到默认文案。
func extract429Message(body *aibalance429Body, limitKind string) string {
	if body != nil {
		if msg := strings.TrimSpace(body.Error.Message); msg != "" {
			return msg
		}
	}
	return default429Message
}

// default429Message 是无法从响应体提取错误信息时的回退文案。
const default429Message = "当前遇到 429 服务器访问人数过多，稍后自动重试\n" +
	"Current request was rate-limited (HTTP 429), retrying shortly..."

// handleLegacyQueue429 兼容旧版 X-AIBalance-Info 队列头：当响应未携带
// X-AIBalance-Limit-Kind 但存在 X-AIBalance-Info 时，按队列长度估算等待时间。
// 队列限流属于频率类，可重试。
func handleLegacyQueue429(ctx context.Context, c AICallerConfigIf, queueInfo string, budget *rateLimitBudget) (is429 bool, shouldRetry bool, ctxDone bool) {
	queueCount, parseErr := strconv.Atoi(queueInfo)
	var waitDuration time.Duration
	var msg string
	if parseErr == nil && queueCount > 0 {
		waitSec := queueCount * 3
		if waitSec < 5 {
			waitSec = 5
		}
		msg = fmt.Sprintf(
			"此刻有 %d 位用户正在与我深度对话中\n"+
				"您的任务同样重要，我不想敷衍任何一位\n"+
				"预计等待约 %d 秒，感谢您的耐心",
			queueCount, waitSec)
		waitDuration = time.Duration(waitSec) * time.Second
	} else {
		msg = "当前有大量用户正在与我深度对话中\n" +
			"您的任务同样重要，我不想敷衍任何一位\n" +
			"预计等待一段时间后自动请求，感谢您的耐心"
		waitDuration = 15 * time.Second
	}
	shouldRetry = budget == nil || budget.reserve(waitDuration)
	if !shouldRetry {
		waitDuration = 0
	}
	emit429Notify(c, notify429TypeRateLimited, msg, waitDuration, "aibalance", "legacy-queue")
	if !shouldRetry {
		return true, false, ctx.Err() != nil
	}
	done := waitBeforeAIRetry(ctx, c, waitDuration) != nil
	return true, true, done
}
