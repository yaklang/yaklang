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

// AIBalance 429 限流类别。
//
// AIBalance 的 429 响应通过响应头 X-AIBalance-Limit-Kind 标识具体的限流原因，
// 共 17 种 kind，按恢复方式分为三大类（参见 AIBalance HTTP 429 错误信息参考）：
//
//   - quotaCategory   账户与额度（6 种）：用量已达上限，多数不会短期自动恢复。
//                     token/api_user_token/paid_daily_token/memfit_version 不携带
//                     Retry-After；daily_token/free_ip 携带 Retry-After:3600（建议
//                     等到次日 06:00 重置）。客户端等待较短时间后由上层有限重试
//                     循环继续，最终暴露错误。
//   - frequencyCategory 频率与 QoS（8 种）：请求过快 / 并发超限，携带 Retry-After，
//                     应读取 Retry-After 并指数退避重试。
//   - systemCategory  系统与上游（3 种）：服务器侧 / 上游侧暂时不可用，携带
//                     Retry-After，按 Retry-After 退避重试。
//
// 客户端可同时从响应体 JSON 的 error 对象中读取动态字段（reason / model /
// tokens_used / tokens_limit / queue_length / exceeded_kind /
// upstream_last_error 等）以精确判断触发原因。
const (
	// 账户与额度类：多数不会短期自动恢复。
	// token/api_user_token/paid_daily_token/memfit_version 不携带 Retry-After；
	// daily_token/free_ip 携带 Retry-After:3600。
	kindToken             = "token"             // 单 Key Token 硬上限
	kindAPIUserToken      = "api_user_token"    // UID 共享 Token 额度未初始化/已用尽
	kindDailyToken        = "daily_token"       // 全局/模型日额度耗尽（06:00 重置）
	kindFreeIP            = "free_ip"           // 单 IP 当日免费请求数/Token 上限
	kindPaidDailyToken    = "paid_daily_token"  // 付费聚合日额度平台级保护上限
	kindMemfitVersion     = "memfit_version"    // 旧版本客户端免费用量达上限（1 亿）

	// 频率与 QoS 类：携带 Retry-After，应指数退避。
	kindRPM               = "rpm"                 // 进程级 RPM 队列
	kindThrottledIPRPM    = "throttled_ip_rpm"    // 单 IP 频率限流
	kindWebSearchRPM      = "web_search_rpm"      // Web 搜索接口频率限制
	kindAPIUserRPS        = "api_user_rps"        // UID RPS 上限
	kindAPIUserRPM        = "api_user_rpm"        // UID QoS RPM 硬上限
	kindAPIUserTPM        = "api_user_tpm"        // UID QoS TPM 硬上限
	kindAPIUserConcurrent = "api_user_concurrent" // UID 并发超限

	// 系统与上游类：携带 Retry-After，按 Retry-After 退避。
	kindServerConcurrency   = "server_concurrency"   // 进程级上游并发队列满
	kindProvidersCircuitOpen = "providers_circuit_open" // 候选供应商全部熔断
	kindUpstreamRateLimit   = "upstream_rate_limit"   // 上游供应商返回 429

	// 旧版/特殊值：memfit 客户端版本控流使用特殊头值 memfit_client_version。
	limitKindMemfitClientVersion = "memfit_client_version"
)

// aibalance429Body 是 AIBalance 429 响应体 JSON 的最小子集，仅解析客户端分流
// 所需的动态字段。未声明的字段会被忽略。
//
// 响应体契约（统一）：
//
//	{
//	  "type": "error",
//	  "error": {
//	    "message": "...",
//	    "type": "rate_limit_exceeded | token_limit_exceeded | ...",
//	    "limit_kind": "rpm | token | ...",
//	    "limit_kind_zh": "...",
//	    "code": "...(可选)",
//	    "notice": "...(可选)",
//	    "reason": "quota_exhausted | quota_not_initialized", // api_user_token
//	    "quota_initialized": false,                          // api_user_token
//	    "bucket": "...",                                     // daily_token
//	    "model": "...",                                      // daily_token / providers_circuit_open / upstream_rate_limit
//	    "tokens_used": 12345,                                // daily_token
//	    "tokens_limit": 99999,                               // daily_token
//	    "exceeded_kind": "request | token",                  // free_ip
//	    "queue_length": 5,                                   // rpm
//	    "retry_after": 10,                                   // web_search_rpm
//	    "upstream_last_error": "...",                        // upstream_rate_limit
//	    "...": "动态字段(因 kind 而异)"
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

// parseAIBalance429Body 解析 429 响应体为结构化错误信息。解析失败时返回 nil。
func parseAIBalance429Body(rsp *AIResponse) *aibalance429Body {
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

// isQuotaKind 判断 limit_kind 是否属于"账户与额度"类（不会短期自动恢复）。
func isQuotaKind(kind string) bool {
	switch kind {
	case kindToken, kindAPIUserToken, kindDailyToken, kindFreeIP,
		kindPaidDailyToken, kindMemfitVersion, limitKindMemfitClientVersion:
		return true
	default:
		return false
	}
}

// isFrequencyKind 判断 limit_kind 是否属于"频率与 QoS"类（携带 Retry-After）。
func isFrequencyKind(kind string) bool {
	switch kind {
	case kindRPM, kindThrottledIPRPM, kindWebSearchRPM,
		kindAPIUserRPS, kindAPIUserRPM, kindAPIUserTPM, kindAPIUserConcurrent:
		return true
	default:
		return false
	}
}

// isSystemKind 判断 limit_kind 是否属于"系统与上游"类（携带 Retry-After）。
func isSystemKind(kind string) bool {
	switch kind {
	case kindServerConcurrency, kindProvidersCircuitOpen, kindUpstreamRateLimit:
		return true
	default:
		return false
	}
}

// bodyLimitKindAliases 把响应体 error.limit_kind（服务端 catalog 的
// ResponseKind）中与响应头 X-AIBalance-Limit-Kind（HeaderKind/Kind）不一致的值
// 映射回客户端分类所用的稳定 header kind。
//
// 服务端 custom_429.go 中以下 kind 的 ResponseKind != Kind：
//   - daily_token       -> daily_token_quota
//   - free_ip           -> free_ip_quota
// 其余 kind 的 ResponseKind 与 Kind 相同，无需映射。
var bodyLimitKindAliases = map[string]string{
	"daily_token_quota": "daily_token",
	"free_ip_quota":     "free_ip",
}

// resolveLimitKind 优先从 X-AIBalance-Limit-Kind 响应头读取稳定的限流标识；
// 若响应头缺失，则回退到响应体 error.limit_kind 字段（经别名归一化）。
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
// 防止上游给出异常大的等待值导致客户端长时间卡死。
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
// 用于频率类限流的退避，避免大量客户端在同一时刻同时重试（惊群）。
func jitterSeconds(base, maxJitter int) int {
	if maxJitter <= 0 {
		return base
	}
	return base + rand.Intn(maxJitter)
}

// emit429Notify 统一发出 429 通知事件并记录日志。notifyType 用于区分前端展示样式。
// source 用于日志中标识 429 来源（如 "aibalance" / "generic"），避免把非 AIBalance
// 的 429 误标为 aibalance。
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
// 处理流程（对齐 AIBalance HTTP 429 错误信息参考文档）：
//  1. 等待响应头就绪，校验状态码为 429。
//  2. 通过 X-AIBalance-Limit-Kind 响应头（回退到响应体 error.limit_kind）确定
//     限流类别：账户与额度 / 频率与 QoS / 系统与上游 / 未知。
//  3. 按类别选择等待策略与用户提示文案：
//     - 账户与额度类：多数不会短期自动恢复；展示对应额度耗尽友好提示。
//       token/api_user_token/paid_daily_token/memfit_version 等待 5-15s；
//       daily_token/free_ip 携带 Retry-After:3600，客户端限制到 [5,30]s，
//       由上层有限重试循环继续（最终耗尽重试次数）。
//     - 频率与 QoS 类：读取 Retry-After，叠加随机抖动后退避重试。
//     - 系统与上游类：读取 Retry-After 退避重试，提示非客户端问题。
//  4. 兼容旧版 X-AIBalance-Info 队列头：当无 limit_kind 但存在队列头时，沿用
//     队列长度估算等待时间。
//  5. 通用 429 兜底（非 AIBalance 提供商或未知来源）：读标准 Retry-After 头
//     做退避，尝试提取响应体 error.message 展示，无 Retry-After 时回退到 5-15s。
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

	body := parseAIBalance429Body(rsp)
	limitKind := resolveLimitKind(rsp, body)

	switch {
	case isQuotaKind(limitKind):
		return c.handleQuota429(ctx, limitKind, rsp, body)
	case isFrequencyKind(limitKind):
		return c.handleFrequency429(ctx, limitKind, rsp, body)
	case isSystemKind(limitKind):
		return c.handleSystem429(ctx, limitKind, rsp, body)
	case limitKind == "":
		// 没有 AIBalance limit_kind：可能是非 AIBalance 提供商（直连 OpenAI/Anthropic 等）。
		// 先兼容旧版 X-AIBalance-Info 队列头，否则走通用 429（读标准 Retry-After）。
		if queueInfo := strings.TrimSpace(rsp.GetHTTPHeader("X-AIBalance-Info")); queueInfo != "" {
			return c.handleLegacyQueue429(ctx, queueInfo)
		}
		return c.handleGeneric429(ctx, rsp, body)
	default:
		// 存在 limit_kind 但不在已知 17 种之内：按通用 429 兜底，日志记录未知 kind。
		log.Infof("429 with unknown limit_kind=%q, falling back to generic 429", limitKind)
		return c.handleGeneric429(ctx, rsp, body)
	}
}

// handleQuota429 处理"账户与额度"类 429。这类限流表示用量已达上限，多数不会
// 短期自动恢复。按各 kind 生成针对性友好提示并选择等待时长：
//   - token / api_user_token / paid_daily_token / memfit_version：服务端不携带
//     Retry-After，等待 5-15s 后由上层重试循环继续（重试次数有限，最终停止）。
//   - daily_token / free_ip：服务端携带 Retry-After: 3600（建议等到次日 06:00
//     重置）。客户端不应真的等 1 小时，而是等待较短时间后让上层重试循环耗尽
//     次数并暴露错误，因此将 Retry-After 限制到 [5, 30] 秒。
func (c *Config) handleQuota429(ctx context.Context, kind string, rsp *AIResponse, body *aibalance429Body) (bool, bool) {
	const yiUnit = 100_000_000 // 1 亿 = 1e8 token

	var waitSec int
	switch kind {
	case kindDailyToken, kindFreeIP:
		// 服务端发送 Retry-After: 3600，但客户端不应长时间阻塞。
		// 限制到 [5, 30] 秒，让上层有限重试循环逐步暴露错误。
		baseSec := parseRetryAfterSeconds(rsp, 10)
		waitSec = capRetryAfterSeconds(baseSec, 5, 30)
	default:
		// token / api_user_token / paid_daily_token / memfit_version：
		// 服务端不携带 Retry-After，等待 5-15s。
		waitSec = 5 + rand.Intn(11)
	}
	waitDuration := time.Duration(waitSec) * time.Second
	notifyType := "quota-exceeded"
	msg := buildQuotaMessage(kind, rsp, body, yiUnit)

	c.EmitDefaultSystemStreamEvent(notifyType, strings.NewReader(msg), "")
	c.emit429Notify(notifyType, msg, waitDuration, "aibalance", kind)
	return c.wait429(ctx, waitDuration)
}

// buildQuotaMessage 根据具体的额度类 kind 生成用户可读的提示文案。
// daily_token 的 tokens_used/tokens_limit 同时存在于响应头
// (X-AIBalance-Token-Used/Limit) 和响应体中，优先读响应体，回退到响应头。
func buildQuotaMessage(kind string, rsp *AIResponse, body *aibalance429Body, yiUnit int64) string {
	switch kind {
	case kindToken:
		return "当前 API Key 的 Token 额度已用尽。\n" +
			"该额度不会自动恢复，请联系管理员提升额度或重置用量后再试。"
	case kindAPIUserToken:
		extra := ""
		if body != nil {
			if body.Error.Reason == "quota_not_initialized" {
				extra = "\n原因：该 API Key 所属用户的共享 Token 额度尚未初始化。"
			} else if body.Error.Reason == "quota_exhausted" {
				extra = "\n原因：该 API Key 所属用户的共享 Token 额度已用尽。"
			}
		}
		return "该 API Key 所属 API 用户的共享 Token 额度未初始化或已用尽。\n" +
			"同一用户下所有 Key 共用此额度，请联系管理员初始化、提升或重置用户额度。" + extra
	case kindDailyToken:
		// tokens_used/tokens_limit 同时存在于响应体和响应头
		// (X-AIBalance-Token-Used/Limit)，优先读响应体，回退到响应头。
		var tokensUsed, tokensLimit int64
		if body != nil {
			tokensUsed = body.Error.TokensUsed
			tokensLimit = body.Error.TokensLimit
		}
		if tokensLimit <= 0 && rsp != nil {
			tokensUsed, _ = strconv.ParseInt(strings.TrimSpace(rsp.GetHTTPHeader("X-AIBalance-Token-Used")), 10, 64)
			tokensLimit, _ = strconv.ParseInt(strings.TrimSpace(rsp.GetHTTPHeader("X-AIBalance-Token-Limit")), 10, 64)
		}
		usedYi := float64(tokensUsed) / float64(yiUnit)
		limitYi := float64(tokensLimit) / float64(yiUnit)
		if limitYi > 0 {
			return fmt.Sprintf(
				"今日免费词元额度 %.2f 亿 已消耗 %.2f 亿，额度已用尽。\n"+
					"每日北京时间 06:00 准时刷新，感谢您的耐心与支持。\n"+
					"稍后将自动重试，您也可以稍候再来。",
				limitYi, usedYi)
		}
		return "今日免费词元额度已经全部消耗完毕。\n" +
			"每日北京时间 06:00 准时刷新，感谢您的耐心与支持。\n" +
			"稍后将自动重试，您也可以稍候再来。"
	case kindFreeIP:
		exceeded := "请求数"
		if body != nil && body.Error.ExceededKind == "token" {
			exceeded = "Token 用量"
		}
		return fmt.Sprintf(
			"当前 IP 的当日免费%s已达上限。\n"+
				"每日北京时间 06:00 自动重置，或配置有效 API Key 切换到付费额度后重试。",
			exceeded)
	case kindPaidDailyToken:
		return "聚合付费 API Key 的当日 Token 用量已达平台级保护上限。\n" +
			"每日北京时间 06:00 自动重置，或联系管理员调整平台日总额度。"
	case kindMemfitVersion, limitKindMemfitClientVersion:
		return "当前客户端版本的免费用量已达上限（1 亿 Token）。\n" +
			"请升级到最新 Yak 引擎或 Memfit/Yak Project，或联系管理员关闭版本控流后重试。"
	default:
		return "当前账户额度已用尽，暂时无法继续请求。\n" +
			"请联系管理员提升额度或稍后再试。"
	}
}

// handleFrequency429 处理"频率与 QoS"类 429。这类限流携带 Retry-After，
// 应读取 Retry-After 并叠加随机抖动后退避重试，避免惊群。
func (c *Config) handleFrequency429(ctx context.Context, kind string, rsp *AIResponse, body *aibalance429Body) (bool, bool) {
	// 基础退避：优先 Retry-After 头，回退到响应体 retry_after（web_search_rpm），
	// 再回退到队列长度（rpm）估算，最终回退到 5-15s 随机。
	baseSec := parseRetryAfterSeconds(rsp, 0)
	if baseSec <= 0 && body != nil {
		if body.Error.RetryAfter > 0 {
			baseSec = int(body.Error.RetryAfter)
		} else if body.Error.QueueLength > 0 && kind == kindRPM {
			baseSec = int(body.Error.QueueLength) * 3
		}
	}
	if baseSec <= 0 {
		baseSec = 5 + rand.Intn(11)
	}
	// 限制在 [5, 120] 秒，叠加最多 5s 抖动。
	waitSec := capRetryAfterSeconds(jitterSeconds(baseSec, 5), 5, 120)
	waitDuration := time.Duration(waitSec) * time.Second

	msg := buildFrequencyMessage(kind, body, waitSec)
	notifyType := "rate-limit"
	c.emit429Notify(notifyType, msg, waitDuration, "aibalance", kind)
	if body != nil && body.Error.Message != "" {
		log.Infof("aibalance 429 [kind=%s] server message: %s", kind, body.Error.Message)
	}
	return c.wait429(ctx, waitDuration)
}

// buildFrequencyMessage 根据具体的频率类 kind 生成用户可读的提示文案。
func buildFrequencyMessage(kind string, body *aibalance429Body, waitSec int) string {
	switch kind {
	case kindRPM:
		queue := int64(0)
		if body != nil {
			queue = body.Error.QueueLength
		}
		if queue > 0 {
			return fmt.Sprintf(
				"此刻有 %d 位用户正在与我深度对话中\n"+
					"您的任务同样重要，我不想敷衍任何一位\n"+
					"预计等待约 %d 秒，感谢您的耐心",
				queue, waitSec)
		}
		return fmt.Sprintf(
			"当前请求频率较高，已触发限流。\n"+
				"预计等待约 %d 秒后自动重试，感谢您的耐心。", waitSec)
	case kindThrottledIPRPM:
		return fmt.Sprintf(
			"当前 IP 的请求频率已触发限流。\n"+
				"预计等待约 %d 秒后自动重试。", waitSec)
	case kindWebSearchRPM:
		return fmt.Sprintf(
			"Web 搜索接口的请求频率已触发限流。\n"+
				"请等待约 %d 秒后重试。", waitSec)
	case kindAPIUserRPS:
		return fmt.Sprintf(
			"该 API 用户在最近 1 秒内的请求数已达 RPS 上限（按 UID 聚合所有 Key）。\n"+
				"预计等待约 %d 秒后自动重试。", waitSec)
	case kindAPIUserRPM:
		return fmt.Sprintf(
			"该 API 用户最近 1 分钟的请求数已达 QoS RPM 硬上限（按 UID 聚合所有 Key）。\n"+
				"预计等待约 %d 秒后自动重试。", waitSec)
	case kindAPIUserTPM:
		return fmt.Sprintf(
			"该 API 用户最近 1 分钟的 Token 用量已达 QoS TPM 硬上限（按 UID 聚合所有 Key）。\n"+
				"建议缩小请求体，预计等待约 %d 秒后自动重试。", waitSec)
	case kindAPIUserConcurrent:
		return fmt.Sprintf(
			"该 API 用户的并发请求数已达 QoS 上限（按 UID 聚合所有 Key），等待并发名额超时。\n"+
				"预计等待约 %d 秒后自动重试。", waitSec)
	default:
		return fmt.Sprintf(
			"当前请求已触发频率限流。\n"+
				"预计等待约 %d 秒后自动重试。", waitSec)
	}
}

// handleSystem429 处理"系统与上游"类 429。这类限流表示服务器侧 / 上游侧暂时
// 不可用，不是客户端额度问题，携带 Retry-After，按 Retry-After 退避重试。
func (c *Config) handleSystem429(ctx context.Context, kind string, rsp *AIResponse, body *aibalance429Body) (bool, bool) {
	// 基础退避：优先 Retry-After 头，回退到 1-5s 随机。
	baseSec := parseRetryAfterSeconds(rsp, 0)
	if baseSec <= 0 {
		baseSec = 1 + rand.Intn(5)
	}
	// 限制在 [1, 60] 秒，叠加最多 3s 抖动。
	waitSec := capRetryAfterSeconds(jitterSeconds(baseSec, 3), 1, 60)
	waitDuration := time.Duration(waitSec) * time.Second

	msg := buildSystemMessage(kind, body, waitSec)
	notifyType := "rate-limit"
	c.emit429Notify(notifyType, msg, waitDuration, "aibalance", kind)
	if body != nil && body.Error.Message != "" {
		log.Infof("aibalance 429 [kind=%s] server message: %s", kind, body.Error.Message)
	}
	return c.wait429(ctx, waitDuration)
}

// buildSystemMessage 根据具体的系统类 kind 生成用户可读的提示文案。
func buildSystemMessage(kind string, body *aibalance429Body, waitSec int) string {
	switch kind {
	case kindServerConcurrency:
		return fmt.Sprintf(
			"服务器当前负载较高，全局并发队列已满，请求等待上游名额超时。\n"+
				"这不是账户额度问题，预计等待约 %d 秒后自动重试。", waitSec)
	case kindProvidersCircuitOpen:
		model := ""
		if body != nil {
			model = strings.TrimSpace(body.Error.Model)
		}
		if model != "" {
			return fmt.Sprintf(
				"当前模型 %s 的所有候选供应商都处于熔断冷却期，暂时没有可转发的健康上游。\n"+
					"这不是客户端 RPM 或 Token 额度问题，请稍后重试。", model)
		}
		return "当前模型的所有候选供应商都处于熔断冷却期，暂时没有可转发的健康上游。\n" +
			"这不是客户端 RPM 或 Token 额度问题，请稍后重试。"
	case kindUpstreamRateLimit:
		model := ""
		if body != nil {
			model = strings.TrimSpace(body.Error.Model)
		}
		suffix := ""
		if model != "" {
			suffix = fmt.Sprintf("（触发模型：%s）", model)
		}
		return fmt.Sprintf(
			"上游供应商返回了限流错误，且没有其他供应商成功。\n"+
				"这不是本地额度判定，请稍后重试%s。", suffix)
	default:
		return fmt.Sprintf(
			"服务器或上游暂时不可用。\n"+
				"这不是客户端问题，预计等待约 %d 秒后自动重试。", waitSec)
	}
}

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

// handleGeneric429 处理非 AIBalance 来源或未知来源的通用 429（如直连
// OpenAI/Anthropic/Azure）。这类 429 遵循 HTTP 标准 429 契约：
//   - 通过标准 Retry-After 响应头告知等待时长（所有主流 AI 提供商均支持）；
//   - 响应体遵循各自的 error 格式，但 message 字段通用可提取：
//       OpenAI:    {"error":{"message":"...","type":"rate_limit_exceeded",...}}
//       Anthropic: {"type":"error","error":{"type":"rate_limit_error","message":"..."}}
//       通用:      {"error":{"message":"..."}}
//
// 处理策略：
//  1. 优先读标准 Retry-After 头做退避（叠加抖动，限制到 [5,120]s）；
//  2. 尝试从响应体提取 error.message 展示给用户，提取失败时回退到默认文案；
//  3. 日志标记为 "generic" 而非 "aibalance"，避免误导来源判断。
func (c *Config) handleGeneric429(ctx context.Context, rsp *AIResponse, body *aibalance429Body) (bool, bool) {
	// 1. 退避时长：优先标准 Retry-After 头，无则回退到 5-15s 随机。
	baseSec := parseRetryAfterSeconds(rsp, 0)
	if baseSec <= 0 {
		baseSec = 5 + rand.Intn(11)
	}
	waitSec := capRetryAfterSeconds(jitterSeconds(baseSec, 3), 5, 120)
	waitDuration := time.Duration(waitSec) * time.Second

	// 2. 提取用户可见的错误信息。
	msg := extractGeneric429Message(body)

	c.emit429Notify("rate-limit", msg, waitDuration, "generic", "generic")
	return c.wait429(ctx, waitDuration)
}

// defaultGeneric429Message 是无法从响应体提取错误信息时的回退文案。
const defaultGeneric429Message = "当前遇到 429 服务器访问人数过多，稍后自动重试\n" +
	"Current request was rate-limited (HTTP 429), retrying shortly..."

// extractGeneric429Message 尝试从 429 响应体中提取用户可见的错误信息。
// 兼容 OpenAI / Anthropic / 通用三种 error 格式，它们都用 error.message 承载
// 错误描述（aibalance429Body 的 Error.Message 字段已覆盖这三种格式）。
// 提取失败时回退到 defaultGeneric429Message。
func extractGeneric429Message(body *aibalance429Body) string {
	if body != nil {
		if msg := strings.TrimSpace(body.Error.Message); msg != "" {
			// 若服务端返回了非空的 message，直接展示原始信息，让用户看到真实原因。
			return msg
		}
	}
	return defaultGeneric429Message
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
