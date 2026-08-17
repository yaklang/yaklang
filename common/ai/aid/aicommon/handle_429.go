package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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
//     有值 → 限流/过载类，应按 Retry-After 退避重试。
//     无值 → 额度耗尽类，不会短期自动恢复，等待短时间后由上层有限重试循环
//     继续并最终暴露错误。
//   - error.message：响应体 JSON 的 error.message 字段，兼容 OpenAI /
//     Anthropic / AIBalance 三种格式。直接展示服务端返回的原始文案，客户端
//     不再重复维护一份 per-kind 文案表。
//   - X-AIBalance-Limit-Kind 响应头：AIBalance 的稳定限流标识，仅用于日志
//     记录和旧版队列头兼容，不参与分类逻辑。
//
// 当 AIBalance 新增/重命名 kind 时，只要响应体 error.message 和 Retry-After
// 契约不变，客户端无需任何改动。

// aibalance429Body 是 429 响应体 JSON 的最小子集，仅解析客户端所需的通用字段。
// 该结构兼容 OpenAI / Anthropic / AIBalance 三种 error 格式，因为它们都用
// error.message 承载错误描述。AIBalance 专有的动态字段（limit_kind / reason /
// tokens_used / queue_length 等）也一并列出，供日志记录和未来扩展使用。
//
// 响应体格式（统一）：
//
//	{
//	  "type": "error",                          // Anthropic 必需，OpenAI 忽略
//	  "error": {
//	    "message": "...",                       // 通用：错误描述
//	    "type": "rate_limit_exceeded | ...",    // 通用：错误类型
//	    "limit_kind": "rpm | token | ...",      // AIBalance 专有
//	    "limit_kind_zh": "...",                 // AIBalance 专有
//	    "code": "...",                          // 可选
//	    "notice": "...",                        // AIBalance 可选
//	    "reason": "quota_exhausted | ...",      // AIBalance: api_user_token
//	    "model": "...",                         // AIBalance: daily_token 等
//	    "tokens_used": 12345,                   // AIBalance: daily_token
//	    "tokens_limit": 99999,                  // AIBalance: daily_token
//	    "exceeded_kind": "request | token",     // AIBalance: free_ip
//	    "queue_length": 5,                      // AIBalance: rpm
//	    "retry_after": 10,                      // AIBalance: web_search_rpm
//	    "upstream_last_error": "...",           // AIBalance: upstream_rate_limit
//	    "...": "动态字段(因提供商/kind而异)"
//	  }
//	}
type aibalance429Body struct {
	Error struct {
		Message           string `json:"message"`
		Type              string `json:"type"`
		LimitKind         string `json:"limit_kind"`
		LimitKindZh       string `json:"limit_kind_zh"`
		Code              string `json:"code"`
		Notice            string `json:"notice"`
		Reason            string `json:"reason"`
		QuotaInitialized  *bool  `json:"quota_initialized"`
		Bucket            string `json:"bucket"`
		Model             string `json:"model"`
		TokensUsed        int64  `json:"tokens_used"`
		TokensLimit       int64  `json:"tokens_limit"`
		ExceededKind      string `json:"exceeded_kind"`
		QueueLength       int64  `json:"queue_length"`
		RetryAfter        int64  `json:"retry_after"`
		UpstreamLastError string `json:"upstream_last_error"`
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

// bodyLimitKindAliases 把 AIBalance 响应体 error.limit_kind（服务端 catalog 的
// ResponseKind）中与响应头 X-AIBalance-Limit-Kind（HeaderKind）不一致的值
// 映射回稳定标识，仅用于日志记录。
//
// 服务端 custom_429.go 中以下 kind 的 ResponseKind != Kind：
//   - daily_token       -> daily_token_quota
//   - free_ip           -> free_ip_quota
// 其余 kind 的 ResponseKind 与 Kind 相同，无需映射。
var bodyLimitKindAliases = map[string]string{
	"daily_token_quota": "daily_token",
	"free_ip_quota":     "free_ip",
}

// resolveLimitKind 优先从 X-AIBalance-Limit-Kind 响应头读取 AIBalance 的限流
// 标识；若响应头缺失，则回退到响应体 error.limit_kind 字段（经别名归一化）。
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

// parseRetryAfterSeconds 解析 Retry-After 响应头（仅支持整数秒形式）。
// 失败时回退到 fallback 秒数。
func parseRetryAfterSeconds(rsp *AIResponse, fallback int) int {
	if rsp != nil {
		if raw := strings.TrimSpace(rsp.GetHTTPHeader("Retry-After")); raw != "" {
			if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
				return sec
			}
		}
	}
	return fallback
}

// capRetryAfterSeconds 将 Retry-After 秒数限制在 [minSec, maxSec] 区间，
// 防止上游给出异常大的等待值导致客户端长时间卡死。maxSec <= 0 表示不限上限。
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
// 用于退避重试，避免大量客户端在同一时刻同时重试（惊群）。
func jitterSeconds(base, maxJitter int) int {
	if maxJitter <= 0 {
		return base
	}
	return base + rand.Intn(maxJitter)
}

// emit429Notify 统一发出 429 通知事件并记录日志。
// source 用于日志标识 429 来源（"aibalance" / "generic"），kind 用于记录
// 限流标识（AIBalance 的 limit_kind 或 "generic" / "legacy-queue"）。
func (c *Config) emit429Notify(notifyType, msg string, waitDuration time.Duration, source, kind string) {
	c.EmitNotify(notifyType, msg, waitDuration)
	log.Infof("%s 429 [kind=%s] %s | waiting %s", source, kind, strings.ReplaceAll(msg, "\n", " "), waitDuration)
}

// handle429RateLimit checks the AI response for a 429 status code, emits the
// appropriate user-facing message, and waits for the correct duration using a
// context-aware select so the wait can be interrupted by context cancellation.
//
// Returns:
//   - is429:   true if a 429 was detected
//   - ctxDone: true if the context was cancelled during the wait
func (c *Config) handle429RateLimit(rsp *AIResponse) (is429 bool, ctxDone bool) {
	return c.handle429RateLimitContext(c.Ctx, rsp)
}

// handle429RateLimitContext 是 429 处理的核心实现。
//
// 处理流程（提供商无关，由响应信号驱动）：
//  1. 等待响应头就绪，校验状态码为 429。
//  2. 解析响应体 JSON，提取 error.message 和 AIBalance limit_kind（用于日志）。
//  3. 按 Retry-After 响应头有无分流：
//     - 有 Retry-After（含 AIBalance 频率/系统类、直连 OpenAI/Anthropic）：
//       读取 Retry-After 做退避，叠加抖动，限制到 [1,120]s。
//     - 无 Retry-After（AIBalance 额度耗尽类 token/api_user_token 等、
//       无 Retry-After 的通用 429）：等待 5-15s，由上层有限重试循环继续。
//  4. 特殊处理：AIBalance 的 daily_token/free_ip 携带 Retry-After:3600
//     （建议等到次日 06:00），客户端不应长时间阻塞，限制到 [5,30]s。
//  5. 兼容旧版 X-AIBalance-Info 队列头：无 limit_kind 但存在队列头时，
//     按队列长度估算等待时间。
//  6. 用户文案统一从响应体 error.message 提取，提取失败时回退到默认文案。
func (c *Config) handle429RateLimitContext(ctx context.Context, rsp *AIResponse) (is429 bool, ctxDone bool) {
	if rsp == nil {
		return false, false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if !rsp.WaitForHTTPHeaders(ctx) {
		return false, true
	}

	if rsp.GetHTTPStatusCode() != 429 {
		return false, false
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
			return c.handleLegacyQueue429(ctx, queueInfo)
		}
	}

	// 退避策略：由 Retry-After 驱动。
	retryAfter := parseRetryAfterSeconds(rsp, 0)

	var waitSec int
	var notifyType string

	switch {
	case retryAfter > 0:
		// 有 Retry-After：限流/过载类，按 Retry-After 退避。
		// AIBalance 的 daily_token/free_ip 发送 3600（建议等到 06:00），
		// 客户端不应长时间阻塞，限制到 [5,30]s；其余限制到 [1,120]s。
		if retryAfter >= 3600 {
			// 大额 Retry-After（如 AIBalance daily_token/free_ip 的 3600）：
			// 不会短期自动恢复，限制到 [5,30]s，由上层重试循环逐步耗尽。
			waitSec = capRetryAfterSeconds(jitterSeconds(retryAfter, 0), 5, 30)
			notifyType = "quota-exceeded"
		} else {
			waitSec = capRetryAfterSeconds(jitterSeconds(retryAfter, 3), 1, 120)
			notifyType = "rate-limit"
		}
	default:
		// 无 Retry-After：额度耗尽类或无标准信号的通用 429，等待 5-15s。
		waitSec = 5 + rand.Intn(11)
		notifyType = "quota-exceeded"
	}

	waitDuration := time.Duration(waitSec) * time.Second

	// 用户文案：优先从响应体 error.message 提取，回退到默认文案。
	msg := extract429Message(body, limitKind)
	kindLabel := limitKind
	if kindLabel == "" {
		kindLabel = "generic"
	}

	c.emit429Notify(notifyType, msg, waitDuration, source, kindLabel)
	return c.wait429(ctx, waitDuration)
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
func (c *Config) handleLegacyQueue429(ctx context.Context, queueInfo string) (bool, bool) {
	queueCount, parseErr := strconv.Atoi(queueInfo)
	var waitDuration time.Duration
	if parseErr == nil && queueCount > 0 {
		waitSec := queueCount * 3
		if waitSec < 5 {
			waitSec = 5
		}
		msg := fmt.Sprintf(
			"此刻有 %d 位用户正在与我深度对话中\n"+
				"您的任务同样重要，我不想敷衍任何一位\n"+
				"预计等待约 %d 秒，感谢您的耐心",
			queueCount, waitSec)
		waitDuration = time.Duration(waitSec) * time.Second
		c.emit429Notify("rate-limit", msg, waitDuration, "aibalance", "legacy-queue")
	} else {
		msg := "当前有大量用户正在与我深度对话中\n" +
			"您的任务同样重要，我不想敷衍任何一位\n" +
			"预计等待一段时间后自动请求，感谢您的耐心"
		waitDuration = 15 * time.Second
		c.emit429Notify("rate-limit", msg, waitDuration, "aibalance", "legacy-queue")
	}
	return c.wait429(ctx, waitDuration)
}

// wait429 在给定的等待时长内阻塞，期间响应 context 取消。
// 返回 (is429=true, ctxDone)。
func (c *Config) wait429(ctx context.Context, waitDuration time.Duration) (bool, bool) {
	select {
	case <-ctx.Done():
		return true, true
	case <-time.After(waitDuration):
		return true, false
	}
}
