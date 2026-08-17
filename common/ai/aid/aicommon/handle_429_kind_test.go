package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// make429ResponseWithBody 构造一个带自定义响应头与响应体的 429 响应。
func make429ResponseWithBody(headers []string, body string) *AIResponse {
	header := "HTTP/1.1 429 Too Many Requests\r\n"
	for _, h := range headers {
		header += h + "\r\n"
	}
	header += "\r\n"
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte(header), []byte(body))
	return rsp
}

// make429KindResponse 构造一个带 X-AIBalance-Limit-Kind 头与可选响应体的 429 响应。
func make429KindResponse(kind string, extraHeaders []string, body string) *AIResponse {
	headers := append([]string{}, "X-AIBalance-Limit-Kind: "+kind)
	headers = append(headers, extraHeaders...)
	if body == "" {
		body = `{"error":{"message":"rate limited","type":"rate_limit_exceeded","limit_kind":"` + kind + `"}}`
	}
	return make429ResponseWithBody(headers, body)
}

// --- 账户与额度类 ---

func TestHandle429_Quota_TokenKind_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := newTestConfigForHandle429(ctx)
	rsp := make429KindResponse("token", nil, "")

	start := time.Now()
	is429, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.True(t, ctxDone)
	assert.Less(t, elapsed, 2*time.Second)
}

func TestHandle429_Quota_TokenKind_EmitsQuotaExceededNotify(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("token", nil, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "API Key 的 Token 额度已用尽")
	require.GreaterOrEqual(t, payload["duration"].(float64), float64(5))
	require.LessOrEqual(t, payload["duration"].(float64), float64(15))
}

func TestHandle429_Quota_DailyTokenKind_EmitsYiUnitMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"daily token exceeded","type":"daily_token_limit_exceeded","limit_kind":"daily_token","bucket":"global","tokens_used":100000000,"tokens_limit":200000000}}`
	rsp := make429KindResponse("daily_token", nil, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "额度 2.00 亿")
	require.Contains(t, payload["content"], "已消耗 1.00 亿")
	require.Contains(t, payload["content"], "06:00")
}

func TestHandle429_Quota_DailyTokenKind_NoBody_FallsBackToGenericMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// 响应体不含 tokens_used/tokens_limit，应使用回退文案。
	rsp := make429KindResponse("daily_token", nil, `{"error":{"message":"x","limit_kind":"daily_token"}}`)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "免费词元额度已经全部消耗完毕")
}

func TestHandle429_Quota_APIUserTokenKind_ReasonExhausted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"api_user_token","reason":"quota_exhausted","quota_initialized":true}}`
	rsp := make429KindResponse("api_user_token", nil, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "共享 Token 额度已用尽")
}

func TestHandle429_Quota_APIUserTokenKind_ReasonNotInitialized(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"api_user_token","reason":"quota_not_initialized","quota_initialized":false}}`
	rsp := make429KindResponse("api_user_token", nil, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "尚未初始化")
}

func TestHandle429_Quota_FreeIPKind_ExceededKindRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"free_ip","exceeded_kind":"request"}}`
	rsp := make429KindResponse("free_ip", nil, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "请求数")
}

func TestHandle429_Quota_FreeIPKind_ExceededKindToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"free_ip","exceeded_kind":"token"}}`
	rsp := make429KindResponse("free_ip", nil, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "Token 用量")
}

func TestHandle429_Quota_PaidDailyTokenKind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("paid_daily_token", nil, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "平台级保护上限")
}

func TestHandle429_Quota_MemfitVersionKind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("memfit_version", nil, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "升级到最新 Yak 引擎")
}

func TestHandle429_Quota_MemfitClientVersionHeader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// memfit 使用特殊头值 memfit_client_version
	rsp := make429KindResponse("memfit_client_version", nil, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "升级到最新 Yak 引擎")
}

// --- 频率与 QoS 类 ---

func TestHandle429_Frequency_RPM_WithQueueLength(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"rpm","queue_length":3}}`
	rsp := make429KindResponse("rpm", nil, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "此刻有 3 位用户正在与我深度对话中")
}

func TestHandle429_Frequency_RPM_RetryAfterHeader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"rpm","queue_length":0}}`
	rsp := make429KindResponse("rpm", []string{"Retry-After: 10"}, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	// baseSec=10 + jitter(0-5) => 10-15s
	dur := payload["duration"].(float64)
	require.GreaterOrEqual(t, dur, float64(10))
	require.LessOrEqual(t, dur, float64(15))
}

func TestHandle429_Frequency_RPM_WaitDurationHonored(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := newTestConfigForHandle429(ctx)
	body := `{"error":{"message":"x","limit_kind":"rpm","queue_length":1}}`
	// Retry-After=1, queue=1 => baseSec=1, capped min 5 => 5+jitter(0-5)
	rsp := make429KindResponse("rpm", []string{"Retry-After: 1"}, body)

	start := time.Now()
	is429, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.False(t, ctxDone)
	// baseSec=1 -> capped to 5, +jitter(0-5) => 5-10s, but min 5 enforced
	require.GreaterOrEqual(t, elapsed, 4*time.Second)
}

func TestHandle429_Frequency_APIUserRPS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("api_user_rps", []string{"Retry-After: 3"}, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "RPS 上限")
}

func TestHandle429_Frequency_APIUserTPM_SuggestsShrink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("api_user_tpm", []string{"Retry-After: 5"}, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "缩小请求体")
}

func TestHandle429_Frequency_APIUserConcurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("api_user_concurrent", []string{"Retry-After: 2"}, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "并发")
}

func TestHandle429_Frequency_WebSearchRPM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"web_search_rpm","retry_after":7}}`
	rsp := make429KindResponse("web_search_rpm", nil, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "Web 搜索")
	// baseSec=7 (from body retry_after) + jitter(0-5) => 7-12
	dur := payload["duration"].(float64)
	require.GreaterOrEqual(t, dur, float64(7))
	require.LessOrEqual(t, dur, float64(12))
}

func TestHandle429_Frequency_ThrottledIPRPM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("throttled_ip_rpm", []string{"Retry-After: 8"}, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "IP 的请求频率")
}

// --- 系统与上游类 ---

func TestHandle429_System_ServerConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("server_concurrency", []string{"Retry-After: 1"}, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "全局并发队列已满")
	require.Contains(t, payload["content"], "不是账户额度问题")
}

func TestHandle429_System_ProvidersCircuitOpen_WithModel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"providers_circuit_open","model":"gpt-4o"}}`
	rsp := make429KindResponse("providers_circuit_open", nil, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "gpt-4o")
	require.Contains(t, payload["content"], "熔断冷却期")
}

func TestHandle429_System_UpstreamRateLimit_WithModel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	body := `{"error":{"message":"x","limit_kind":"upstream_rate_limit","model":"claude-3","upstream_last_error":"429 too many requests"}}`
	rsp := make429KindResponse("upstream_rate_limit", []string{"Retry-After: 3"}, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "claude-3")
	require.Contains(t, payload["content"], "上游供应商")
}

// --- limit_kind 回退与兜底 ---

func TestHandle429_LimitKindFromBody_WhenHeaderMissing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// 不带 X-AIBalance-Limit-Kind 头，但响应体 error.limit_kind=daily_token
	headers := []string{"Content-Type: application/json"}
	body := `{"type":"error","error":{"message":"daily exceeded","limit_kind":"daily_token","tokens_used":100000000,"tokens_limit":100000000}}`
	rsp := make429ResponseWithBody(headers, body)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	require.Contains(t, payload["content"], "06:00")
}

func TestHandle429_UnknownLimitKind_FallsBackToGeneric(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("some_unknown_kind", nil, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "HTTP 429")
}

// --- Retry-After 解析与上限 ---

func TestHandle429_Frequency_RetryAfterCapped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// Retry-After=9999 应被限制到 120s
	rsp := make429KindResponse("api_user_rpm", []string{"Retry-After: 9999"}, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	dur := payload["duration"].(float64)
	require.LessOrEqual(t, dur, float64(125)) // 120 + max 5 jitter
}

func TestHandle429_System_RetryAfterCapped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429KindResponse("server_concurrency", []string{"Retry-After: 9999"}, "")

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	dur := payload["duration"].(float64)
	require.LessOrEqual(t, dur, float64(63)) // 60 + max 3 jitter
}

// --- body 解析 ---

func TestParseAIBalance429Body_NilResponse(t *testing.T) {
	assert.Nil(t, parseAIBalance429Body(nil))
}

func TestParseAIBalance429Body_EmptyBody(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), nil)
	assert.Nil(t, parseAIBalance429Body(rsp))
}

func TestParseAIBalance429Body_InvalidJSON(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), []byte("not json"))
	assert.Nil(t, parseAIBalance429Body(rsp))
}

func TestParseAIBalance429Body_ValidJSON(t *testing.T) {
	rsp := NewUnboundAIResponse()
	body := `{"error":{"message":"hi","limit_kind":"rpm","queue_length":5,"tokens_used":100,"tokens_limit":200}}`
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), []byte(body))
	parsed := parseAIBalance429Body(rsp)
	require.NotNil(t, parsed)
	assert.Equal(t, "rpm", parsed.Error.LimitKind)
	assert.Equal(t, int64(5), parsed.Error.QueueLength)
	assert.Equal(t, int64(100), parsed.Error.TokensUsed)
	assert.Equal(t, int64(200), parsed.Error.TokensLimit)
}

// --- kind 分类 ---

func TestIsQuotaKind(t *testing.T) {
	quotaKinds := []string{"token", "api_user_token", "daily_token", "free_ip", "paid_daily_token", "memfit_version", "memfit_client_version"}
	for _, k := range quotaKinds {
		assert.True(t, isQuotaKind(k), "%s should be quota kind", k)
	}
	assert.False(t, isQuotaKind("rpm"))
	assert.False(t, isQuotaKind("server_concurrency"))
	assert.False(t, isQuotaKind(""))
}

func TestIsFrequencyKind(t *testing.T) {
	freqKinds := []string{"rpm", "throttled_ip_rpm", "web_search_rpm", "api_user_rps", "api_user_rpm", "api_user_tpm", "api_user_concurrent"}
	for _, k := range freqKinds {
		assert.True(t, isFrequencyKind(k), "%s should be frequency kind", k)
	}
	assert.False(t, isFrequencyKind("token"))
	assert.False(t, isFrequencyKind("server_concurrency"))
}

func TestIsSystemKind(t *testing.T) {
	sysKinds := []string{"server_concurrency", "providers_circuit_open", "upstream_rate_limit"}
	for _, k := range sysKinds {
		assert.True(t, isSystemKind(k), "%s should be system kind", k)
	}
	assert.False(t, isSystemKind("token"))
	assert.False(t, isSystemKind("rpm"))
}

// --- resolveLimitKind ---

func TestResolveLimitKind_HeaderTakesPriority(t *testing.T) {
	rsp := NewUnboundAIResponse()
	hdr := "HTTP/1.1 429 Too Many Requests\r\nX-AIBalance-Limit-Kind: rpm\r\n\r\n"
	rsp.SetRawHTTPResponseData([]byte(hdr), []byte(`{"error":{"limit_kind":"token"}}`))
	kind := resolveLimitKind(rsp, parseAIBalance429Body(rsp))
	assert.Equal(t, "rpm", kind)
}

func TestResolveLimitKind_BodyFallback(t *testing.T) {
	rsp := NewUnboundAIResponse()
	hdr := "HTTP/1.1 429 Too Many Requests\r\n\r\n"
	rsp.SetRawHTTPResponseData([]byte(hdr), []byte(`{"error":{"limit_kind":"token"}}`))
	kind := resolveLimitKind(rsp, parseAIBalance429Body(rsp))
	assert.Equal(t, "token", kind)
}

func TestResolveLimitKind_Empty(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), []byte(`{"error":{}}`))
	kind := resolveLimitKind(rsp, parseAIBalance429Body(rsp))
	assert.Equal(t, "", kind)
}

// --- parseRetryAfterSeconds ---

func TestParseRetryAfterSeconds_Valid(t *testing.T) {
	rsp := NewUnboundAIResponse()
	hdr := "HTTP/1.1 429 Too Many Requests\r\nRetry-After: 15\r\n\r\n"
	rsp.SetRawHTTPResponseData([]byte(hdr), nil)
	assert.Equal(t, 15, parseRetryAfterSeconds(rsp, 5))
}

func TestParseRetryAfterSeconds_MissingFallback(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), nil)
	assert.Equal(t, 5, parseRetryAfterSeconds(rsp, 5))
}

func TestParseRetryAfterSeconds_InvalidFallback(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\nRetry-After: abc\r\n\r\n"), nil)
	assert.Equal(t, 7, parseRetryAfterSeconds(rsp, 7))
}

func TestParseRetryAfterSeconds_ZeroFallback(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\nRetry-After: 0\r\n\r\n"), nil)
	assert.Equal(t, 9, parseRetryAfterSeconds(rsp, 9))
}

// --- capRetryAfterSeconds ---

func TestCapRetryAfterSeconds(t *testing.T) {
	assert.Equal(t, 5, capRetryAfterSeconds(3, 5, 120))
	assert.Equal(t, 50, capRetryAfterSeconds(50, 5, 120))
	assert.Equal(t, 120, capRetryAfterSeconds(999, 5, 120))
	assert.Equal(t, 5, capRetryAfterSeconds(5, 5, 120))
	// maxSec <= 0 means no upper cap
	assert.Equal(t, 9999, capRetryAfterSeconds(9999, 5, 0))
}

// --- GetHTTPResponseBody ---

func TestGetHTTPResponseBody_Nil(t *testing.T) {
	var rsp *AIResponse
	assert.Nil(t, rsp.GetHTTPResponseBody())
}

func TestGetHTTPResponseBody_Empty(t *testing.T) {
	rsp := NewUnboundAIResponse()
	assert.Nil(t, rsp.GetHTTPResponseBody())
}

func TestGetHTTPResponseBody_Present(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"),
		[]byte(`{"error":"rate limited"}`),
	)
	body := rsp.GetHTTPResponseBody()
	require.NotNil(t, body)
	assert.Equal(t, `{"error":"rate limited"}`, string(body))
}

func TestGetHTTPResponseBody_ReturnsCopy(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData(
		[]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"),
		[]byte("original"),
	)
	body := rsp.GetHTTPResponseBody()
	body[0] = 'X'
	// mutating returned slice must not affect internal state
	assert.Equal(t, "original", string(rsp.GetHTTPResponseBody()))
}

// --- 完整响应体契约测试 ---

func TestHandle429_FullResponseBodyContract(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// 模拟真实 AIBalance 429 响应体（包含顶层 type:error 与 error 对象）
	fullBody := `{"type":"error","error":{"message":"日额度已用尽","type":"daily_token_limit_exceeded","limit_kind":"daily_token","limit_kind_zh":"日Token限额","bucket":"global","model":"gpt-4o-mini","tokens_used":150000000,"tokens_limit":200000000,"notice":"系统维护中"}}`
	headers := []string{
		"X-AIBalance-Limit-Kind: daily_token",
		"Content-Type: application/json; charset=utf-8",
	}
	rsp := make429ResponseWithBody(headers, fullBody)

	is429, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	content := payload["content"].(string)
	require.Contains(t, content, "额度 2.00 亿")
	require.Contains(t, content, "已消耗 1.50 亿")
	require.Contains(t, content, "06:00")
}

