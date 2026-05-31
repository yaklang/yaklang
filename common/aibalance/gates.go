package aibalance

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// gates.go 把 serveChatCompletions / serveEmbeddings 里内联的鉴权与限流门控抽成
// 独立方法，主流程顺序与判定口径与原内联实现完全一致：每个 gate 命中拦截时已在
// 方法内部写好对应的 4xx/429 响应并返回 true（blocked），调用方只需 `if gate {return}`。
//
// 注意：免费用户 in-flight token 预扣的 defer c.inFlightTokens.Remove(...) 必须仍绑定在
// serveChatCompletions 的调用栈上（见该函数内联实现），不能搬进 gate，否则会提前释放。
//
// 关键词: aibalance server 门控抽离, gate memfit/apikey/daily-token/RPM/delay, 行为等价重构

// gateLightweightDowngrade 根据请求头 X-Yak-AI-Model-Usage-Type(客户端上报的模型用途类型)
// 与配置的降级规则，可能把请求模型降级以保护用量（如 lightweight 调用 memfit-standard-free
// 自动降级到 memfit-light-free）。返回（可能被改写后的）modelName。
//
// 与限流类 gate 不同：本方法从不拦截请求，只做模型名改写；命中时记录英文日志。
// 须在 isFreeModel 判定 / memfit gate / provider 查找 / 计费之前调用，使全链路沿用降级后模型。
// 关键词: gateLightweightDowngrade, X-Yak-AI-Model-Usage-Type, 轻量降级保护用量
func (c *ServerConfig) gateLightweightDowngrade(rawPacket []byte, modelName string) string {
	usageType := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(rawPacket, ModelUsageTypeHeader))
	if usageType == "" {
		return modelName
	}
	newModel, downgraded := c.resolveModelDowngrade(usageType, modelName)
	if downgraded {
		c.logInfo("model usage-type downgrade applied: usage_type=%s from=%s to=%s",
			usageType, modelName, newModel)
		return newModel
	}
	return modelName
}

// writeKeyLimit429 统一写出 API key Token 维度的 429（注入自定义文案与 notice）。
// kind 取 "token"（单 Key Token 额度）或 "paid_daily_token"（付费用户全局日总额度）；
// typ 为对外 error.type（token_limit_exceeded / paid_daily_token_limit_exceeded）。
// defaultMessage 统一传 custom_429.go 中的 Default429Message* 常量，保证与编辑界面默认文案一致。
// 关键词: writeKeyLimit429, token/paid_daily_token 429, resolveLimit429 notice
func (c *ServerConfig) writeKeyLimit429(conn net.Conn, kind, typ, defaultMessage string) {
	message, notice := c.resolveLimit429(kind, defaultMessage)
	errMap := map[string]interface{}{
		"message":    message,
		"type":       typ,
		"limit_kind": kind,
	}
	if notice != "" {
		errMap["notice"] = notice
	}
	c.writeJSONResponse(conn, http.StatusTooManyRequests, map[string]interface{}{"error": errMap})
}

// gateChatMemfitAuthAndVersion 处理 chat 入口的 memfit 模型 TOTP 鉴权 + 客户端版本控流。
// 关键词: gateChatMemfitAuthAndVersion, memfit TOTP, X-Yak-Version 控流
func (c *ServerConfig) gateChatMemfitAuthAndVersion(conn net.Conn, rawPacket []byte, modelName string) bool {
	if !IsMemfitModel(modelName) {
		return false
	}
	c.logInfo("Memfit model detected, checking TOTP authentication...")
	totpHeader := lowhttp.GetHTTPPacketHeader(rawPacket, "X-Memfit-OTP-Auth")
	if totpHeader == "" {
		c.logError("Memfit model requires TOTP authentication, but X-Memfit-OTP-Auth header is missing")
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "Memfit TOTP authentication required. Please provide X-Memfit-OTP-Auth header with base64 encoded TOTP code.",
				"type":    "memfit_totp_auth_required",
			},
		})
		return true
	}

	verified, err := VerifyMemfitTOTP(totpHeader)
	if err != nil || !verified {
		c.logError("Memfit TOTP authentication failed: %v", err)
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "Memfit TOTP authentication failed. Please refresh your TOTP secret and try again.",
				"type":    "memfit_totp_auth_failed",
			},
		})
		return true
	}
	c.logInfo("Memfit TOTP authentication successful for model: %s", modelName)

	// memfit-* 客户端版本控流：记录客户端版本统计 + 按配置可选拦截
	// 关键词: serveChatCompletions memfit 版本控流入口, X-Yak-Version X-Yak-Build-Time, RecordClientVersion
	yakVer := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(rawPacket, "X-Yak-Version"))
	yakBT := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(rawPacket, "X-Yak-Build-Time"))
	if yakVer == "" {
		yakVer = "unknown"
	}
	go func(v, bt string) {
		defer func() {
			if r := recover(); r != nil {
				log.Debugf("RecordClientVersion panic recovered: %v", r)
			}
		}()
		_ = RecordClientVersion(v, bt)
	}(yakVer, yakBT)

	gateRes := c.checkMemfitVersionGate(yakVer, yakBT)
	if gateRes.Blocked {
		c.logWarn("Memfit version gate blocked request: model=%s version=%s buildTime=%s reason=%s minBuildTime=%s",
			modelName, gateRes.ClientVersion, gateRes.ClientBuildTime, gateRes.Reason, gateRes.MinBuildTime)
		c.writeMemfitVersionRateLimitResponse(conn, gateRes.Reason)
		return true
	}
	return false
}

// gateChatAPIKeyAndLimits 处理 chat 入口的 API key 解析、流量/Token 限额、允许模型校验。
// 返回解析出的 key（免费模型为 nil）、用于统计的 apiKeyForStat、是否被拦截。
// 关键词: gateChatAPIKeyAndLimits, CheckAiApiKeyTrafficLimit, CheckAiApiKeyTokenLimit, IsModelAllowed
func (c *ServerConfig) gateChatAPIKeyAndLimits(conn net.Conn, auth, modelName string, isFreeModel bool) (key *Key, apiKeyForStat string, blocked bool) {
	if isFreeModel {
		apiKeyForStat = "free-user"
		return nil, apiKeyForStat, false
	}

	value := strings.TrimPrefix(auth, "Bearer ")
	c.logInfo("Extracted key from authentication info: %s", value)
	if value == "" {
		c.logError("No valid authentication info provided")
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
		return nil, "", true
	}

	var ok bool
	key, ok = c.Keys.Get(value)
	if !ok {
		c.logError("No matching key configuration found: %s", value)
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
		return nil, "", true
	}
	apiKeyForStat = key.Key
	c.logInfo("Successfully verified key: %s", key.Key)

	// 字节流量限额已停用：统一改用 Token 维度限额（CheckAiApiKeyTokenLimit）。
	// 任何 DB 错误降级为放行（不阻塞业务）。
	// 关键词: 字节流量限额停用, CheckAiApiKeyTokenLimit hot path chat, token_limit_exceeded 429
	tokenAllowed, tErr := CheckAiApiKeyTokenLimit(key.Key)
	if tErr != nil {
		c.logError("Failed to check token limit for key %s: %v", utils.ShrinkString(key.Key, 8), tErr)
	} else if !tokenAllowed {
		c.logError("API key %s has exceeded token limit", utils.ShrinkString(key.Key, 8))
		c.writeKeyLimit429(conn, "token", "token_limit_exceeded", Default429MessageToken)
		return nil, "", true
	}

	// 付费用户全局日 Token 总额度（第二道硬门）：聚合所有付费 key 当天用量，超额一律 429。
	// 任何 DB 错误降级为放行（不阻塞业务）。
	// 关键词: gateChatPaidUserDailyToken, CheckPaidUserDailyTokenLimit, paid_daily_token 429
	if pd, pErr := CheckPaidUserDailyTokenLimit(); pErr != nil {
		c.logError("CheckPaidUserDailyTokenLimit failed (key=%s): %v", utils.ShrinkString(key.Key, 8), pErr)
	} else if pd != nil && !pd.Allowed {
		c.logWarn("Paid user global daily token limit exceeded (key=%s used=%d limit=%d date=%s)",
			utils.ShrinkString(key.Key, 8), pd.TokensUsed, pd.TokensLimit, pd.Date)
		c.writeKeyLimit429(conn, "paid_daily_token", "paid_daily_token_limit_exceeded", Default429MessagePaidDailyToken)
		return nil, "", true
	}

	// Authorization check with glob pattern support
	allowedModels, ok := c.KeyAllowedModels.Get(key.Key)
	if !ok {
		c.logError("Key[%v] has no allowed models configured", key.Key)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return nil, "", true
	}

	// Use IsModelAllowed which supports glob patterns
	if !c.KeyAllowedModels.IsModelAllowed(key.Key, modelName) {
		allowedModelKeys := make([]string, 0, len(allowedModels))
		for k := range allowedModels {
			allowedModelKeys = append(allowedModelKeys, k)
		}
		c.logError("Key[%v] requested model %s is not in allowed list (including glob patterns), allowed models/patterns: %v", key.Key, modelName, allowedModelKeys)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return nil, "", true
	}
	return key, apiKeyForStat, false
}

// gateChatFreeUserDailyToken 处理 chat 入口免费用户日 Token 限额检查（含 in-flight 预扣份额）。
// 注意：本 gate 仅做「是否超额」判定；真正的 in-flight Add + defer Remove 仍保留在
// serveChatCompletions 调用栈内联实现，避免 defer 随本方法返回提前释放。
// 关键词: gateChatFreeUserDailyToken, checkFreeUserDailyTokenLimitWithInFlight, daily token 429
func (c *ServerConfig) gateChatFreeUserDailyToken(conn net.Conn, modelName string) bool {
	if decision, dErr := c.checkFreeUserDailyTokenLimitWithInFlight(modelName); dErr != nil {
		c.logError("CheckFreeUserDailyTokenLimit failed (model=%s): %v", modelName, dErr)
	} else if decision != nil && !decision.Allowed {
		c.logWarn("Daily token limit exceeded for free user (model=%s bucket=%s effective_used=%d limit=%d in_flight_included=true)",
			modelName, decision.Bucket, decision.TokensUsed, decision.TokensLimit)
		c.writeDailyTokenLimitResponse(conn, modelName, decision.Bucket, decision.TokensUsed, decision.TokensLimit)
		return true
	}
	return false
}

// gateChatFreeUserIPLimit 处理 chat 入口的「单 IP 免费模型每日用量限额」：
// 防止单个客户端 IP 高频盗刷公共免费接口，保证免费额度对所有用户公平。
//
// 仅统计「计费免费模型」：exempt（不计费）模型直接放行不计数；IP 不可识别（空/unknown）
// 也直接放行。检查通过即累加一次请求计数；超限则写 429 固定提示并拦截。
// Token 维度的累加在 onUsageForward / fallback 计费分流处完成（与请求计数分离）。
// 关键词: gateChatFreeUserIPLimit, 单 IP 每日限额, 防盗刷, 计费免费模型, 请求计数
func (c *ServerConfig) gateChatFreeUserIPLimit(conn net.Conn, clientIP, modelName string) bool {
	if freeIPUsageIgnoredIP(clientIP) {
		return false
	}
	// 模型豁免计费时不参与单 IP 限额（与 onUsageForward 计费分流口径一致）。
	// 但仍记录「按模型请求计数」用于面板展示——不计费模型也要能看到用量（计数量、不算钱）。
	// 关键词: 不计费模型 仅展示请求计数, 不参与限额
	if isFreeModelBillingExempt(modelName) {
		if addErr := AddFreeUserIPModelDailyRequest(clientIP, modelName); addErr != nil {
			c.logWarn("AddFreeUserIPModelDailyRequest (exempt) failed (ip=%s model=%s): %v", clientIP, modelName, addErr)
		}
		return false
	}

	decision, err := CheckFreeUserIPDailyLimit(clientIP)
	if err != nil {
		c.logError("CheckFreeUserIPDailyLimit failed (ip=%s model=%s): %v", clientIP, modelName, err)
	} else if decision != nil && !decision.Allowed {
		c.logWarn("Free IP daily limit exceeded (ip=%s model=%s kind=%s request_used=%d/%d tokens_used=%d/%d)",
			clientIP, modelName, decision.ExceededKind,
			decision.RequestUsed, decision.RequestLimit,
			decision.TokensUsed, decision.TokensLimit)
		c.writeFreeIPLimitResponse(conn, decision)
		return true
	}

	// 检查通过：累加一次请求计数（Token 在计费分流处累加）。失败仅 logWarn，不阻塞业务。
	if addErr := AddFreeUserIPDailyRequest(clientIP); addErr != nil {
		c.logWarn("AddFreeUserIPDailyRequest failed (ip=%s model=%s): %v", clientIP, modelName, addErr)
	}
	// 按模型拆分的请求计数（仅展示用，不参与限额）。失败仅 logWarn。
	// 关键词: AddFreeUserIPModelDailyRequest, per-IP 按模型计数
	if addErr := AddFreeUserIPModelDailyRequest(clientIP, modelName); addErr != nil {
		c.logWarn("AddFreeUserIPModelDailyRequest failed (ip=%s model=%s): %v", clientIP, modelName, addErr)
	}
	return false
}

// isFreeModelBillingExempt 判断某个免费模型是否被配置为「豁免计费」(exempt=true)。
// 与 onUsageForward / resolveInFlightBucketKey 的 exempt 判定保持一致。
// 关键词: isFreeModelBillingExempt, 免费模型豁免计费判定
func isFreeModelBillingExempt(modelName string) bool {
	overrides := parseFreeUserTokenModelOverridesFromConfig()
	if ov, ok := overrides[modelName]; ok && ov.Exempt {
		return true
	}
	return false
}

// gateChatThrottledIPRPM 处理「一键限流 IP」的按 IP 维度 RPM 限流。
// 与 gateRPM（按 apiKey|model）正交：只要该 IP 被管理员一键限流且配置了 RPM，
// 无论免费/付费请求都按该 IP 的低 RPM 滑动窗口限流，命中即写 429（复用 rpm 文案）。
// 未被限流 / IP 不可识别 / RPM<=0 一律放行。
// 关键词: gateChatThrottledIPRPM, 一键限流 IP, per-IP RPM 429
func (c *ServerConfig) gateChatThrottledIPRPM(conn net.Conn, clientIP, modelName string) bool {
	if c.chatRateLimiter == nil || freeIPUsageIgnoredIP(clientIP) {
		return false
	}
	rpm, _, ok := lookupThrottledIP(clientIP)
	if !ok || rpm <= 0 {
		return false
	}
	allowed, queueLen := c.chatRateLimiter.CheckIPRateLimit(clientIP, rpm)
	if !allowed {
		c.logWarn("throttled IP RPM limit exceeded: ip=%s model=%s rpm=%d queue_length=%d",
			clientIP, modelName, rpm, queueLen)
		c.writeRPMRateLimitResponse(conn, queueLen)
		return true
	}
	return false
}

// gateRPM 处理按 API key（含模型级覆盖）的 RPM 限流检查。
// 关键词: gateRPM, chatRateLimiter.CheckRateLimit, RPM 429
func (c *ServerConfig) gateRPM(conn net.Conn, apiKeyForStat, modelName string) bool {
	if c.chatRateLimiter == nil {
		return false
	}
	allowed, queueLen := c.chatRateLimiter.CheckRateLimit(apiKeyForStat, modelName)
	if !allowed {
		c.logWarn("RPM rate limit exceeded for key=%s model=%s, queue_length=%d",
			utils.ShrinkString(apiKeyForStat, 8), modelName, queueLen)
		c.writeRPMRateLimitResponse(conn, queueLen)
		return true
	}
	return false
}

// applyFreeUserPreCallDelay 在转发上游之前对免费用户施加调用前延迟（N~M 随机，兼容老 N~2N）。
// 关键词: applyFreeUserPreCallDelay, 免费用户调用前延迟, computeJitterDelaySec
func (c *ServerConfig) applyFreeUserPreCallDelay(isFreeModel bool, modelName string) {
	if !isFreeModel || c.chatRateLimiter == nil {
		return
	}
	minSec, maxSec := c.chatRateLimiter.GetEffectiveDelay(modelName, c.freeUserDelayMinSec, c.freeUserDelayMaxSec)
	actual := computeJitterDelaySec(minSec, maxSec)
	if actual > 0 {
		jitter := time.Duration(actual) * time.Second
		c.logInfo("free user pre-call delay: sleeping %v before forwarding model %s (range=%ds~%ds)",
			jitter, modelName, minSec, maxSec)
		time.Sleep(jitter)
	}
}

// gateEmbeddingMemfitTOTP 处理 embedding 入口 memfit 模型的 TOTP 鉴权（embedding 无版本控流）。
// 关键词: gateEmbeddingMemfitTOTP, embedding memfit TOTP
func (c *ServerConfig) gateEmbeddingMemfitTOTP(conn net.Conn, rawPacket []byte, modelName string) bool {
	if !IsMemfitModel(modelName) {
		return false
	}
	c.logInfo("Memfit embedding model detected, checking TOTP authentication...")
	totpHeader := lowhttp.GetHTTPPacketHeader(rawPacket, "X-Memfit-OTP-Auth")
	if totpHeader == "" {
		c.logError("Memfit model requires TOTP authentication, but X-Memfit-OTP-Auth header is missing")
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "Memfit TOTP authentication required. Please provide X-Memfit-OTP-Auth header with base64 encoded TOTP code.",
				"type":    "memfit_totp_auth_required",
			},
		})
		return true
	}

	verified, err := VerifyMemfitTOTP(totpHeader)
	if err != nil || !verified {
		c.logError("Memfit TOTP authentication failed: %v", err)
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]interface{}{
			"error": map[string]string{
				"message": "Memfit TOTP authentication failed. Please refresh your TOTP secret and try again.",
				"type":    "memfit_totp_auth_failed",
			},
		})
		return true
	}
	c.logInfo("Memfit TOTP authentication successful for embedding model: %s", modelName)
	return false
}

// gateEmbeddingAPIKeyAndLimits 处理 embedding 入口的 API key 解析、流量/Token 限额、允许模型校验。
// 返回解析出的 key（免费模型为 nil，后续上游成功后用于统计）、是否被拦截。
// 关键词: gateEmbeddingAPIKeyAndLimits, embedding traffic/token 限额, IsModelAllowed
func (c *ServerConfig) gateEmbeddingAPIKeyAndLimits(conn net.Conn, auth, modelName string, isFreeModel bool) (key *Key, blocked bool) {
	if isFreeModel {
		return nil, false
	}

	value := strings.TrimPrefix(auth, "Bearer ")
	c.logInfo("Extracted key from authentication info: %s", value)
	if value == "" {
		c.logError("No valid authentication info provided")
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
		return nil, true
	}

	var ok bool
	key, ok = c.Keys.Get(value)
	if !ok {
		c.logError("No matching key configuration found: %s", value)
		conn.Write([]byte("HTTP/1.1 401 Unauthorized\r\n\r\n"))
		return nil, true
	}
	c.logInfo("Successfully verified key: %s", key.Key)

	// 字节流量限额已停用：embedding 同样统一改用 Token 维度限额。
	// 关键词: 字节流量限额停用 embedding, CheckAiApiKeyTokenLimit hot path embedding, token_limit_exceeded 429
	tokenAllowed, tErr := CheckAiApiKeyTokenLimit(key.Key)
	if tErr != nil {
		c.logError("Failed to check token limit (embedding) for key %s: %v", utils.ShrinkString(key.Key, 8), tErr)
	} else if !tokenAllowed {
		c.logError("API key %s has exceeded token limit (embedding)", utils.ShrinkString(key.Key, 8))
		c.writeKeyLimit429(conn, "token", "token_limit_exceeded", Default429MessageToken)
		return nil, true
	}

	// 付费用户全局日 Token 总额度（第二道硬门）：embedding 同样校验。
	// 关键词: gateEmbeddingPaidUserDailyToken, CheckPaidUserDailyTokenLimit, paid_daily_token 429
	if pd, pErr := CheckPaidUserDailyTokenLimit(); pErr != nil {
		c.logError("CheckPaidUserDailyTokenLimit (embedding) failed (key=%s): %v", utils.ShrinkString(key.Key, 8), pErr)
	} else if pd != nil && !pd.Allowed {
		c.logWarn("Paid user global daily token limit exceeded (embedding key=%s used=%d limit=%d date=%s)",
			utils.ShrinkString(key.Key, 8), pd.TokensUsed, pd.TokensLimit, pd.Date)
		c.writeKeyLimit429(conn, "paid_daily_token", "paid_daily_token_limit_exceeded", Default429MessagePaidDailyToken)
		return nil, true
	}

	// Authorization check with glob pattern support
	allowedModels, ok := c.KeyAllowedModels.Get(key.Key)
	if !ok {
		c.logError("Key[%v] has no allowed models configured", key.Key)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return nil, true
	}

	// Use IsModelAllowed which supports glob patterns
	if !c.KeyAllowedModels.IsModelAllowed(key.Key, modelName) {
		allowedModelKeys := make([]string, 0, len(allowedModels))
		for k := range allowedModels {
			allowedModelKeys = append(allowedModelKeys, k)
		}
		c.logError("Key[%v] requested model %s is not in allowed list (including glob patterns), allowed models/patterns: %v", key.Key, modelName, allowedModelKeys)
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		return nil, true
	}
	return key, false
}

// gateEmbeddingFreeUserDailyToken 处理 embedding 入口免费用户日 Token 限额前置检查（无 in-flight 预扣）。
// 关键词: gateEmbeddingFreeUserDailyToken, CheckFreeUserDailyTokenLimit, embedding daily token 429
func (c *ServerConfig) gateEmbeddingFreeUserDailyToken(conn net.Conn, modelName string) bool {
	if decision, dErr := CheckFreeUserDailyTokenLimit(modelName); dErr != nil {
		c.logError("CheckFreeUserDailyTokenLimit (embedding) failed (model=%s): %v", modelName, dErr)
	} else if decision != nil && !decision.Allowed {
		c.logWarn("Daily token limit exceeded for free embedding (model=%s bucket=%s used=%d limit=%d)",
			modelName, decision.Bucket, decision.TokensUsed, decision.TokensLimit)
		c.writeDailyTokenLimitResponse(conn, modelName, decision.Bucket, decision.TokensUsed, decision.TokensLimit)
		return true
	}
	return false
}
