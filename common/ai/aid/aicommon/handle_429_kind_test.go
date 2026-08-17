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

func TestHandle429_WithRetryAfter_RateLimitNotify(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// 有 Retry-After → rate-limit 通知，可重试
	rsp := make429ResponseWithBody(
		[]string{"Retry-After: 10"},
		`{"error":{"message":"too many requests","type":"rate_limit_exceeded"}}`,
	)

	is429, shouldRetry, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, shouldRetry, "rate-limit 429 should be retryable")
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	// Retry-After=10 + jitter(0-3) => 10-13s
	dur := payload["duration"].(float64)
	require.GreaterOrEqual(t, dur, float64(10))
	require.LessOrEqual(t, dur, float64(13))
}

func TestHandle429_NoRetryAfter_QuotaExceededNotify(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// 无 Retry-After → quota-exceeded 通知，不可重试
	rsp := make429KindResponse("token", nil, `{"error":{"message":"token exhausted","type":"token_limit_exceeded","limit_kind":"token"}}`)

	is429, shouldRetry, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.False(t, shouldRetry, "quota-exceeded 429 should not be retryable")
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "quota-exceeded", payload["type"])
	dur := payload["duration"].(float64)
	require.GreaterOrEqual(t, dur, float64(5))
	require.LessOrEqual(t, dur, float64(15))
}

func TestHandle429_LargeRetryAfter_3600_CappedTo30s(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// AIBalance daily_token/free_ip 发送 Retry-After:3600（额度到次日才恢复），
	// 不可重试，等待限制到 [5,30]s
	rsp := make429KindResponse("daily_token",
		[]string{"Retry-After: 3600"},
		`{"error":{"message":"daily token exceeded","limit_kind":"daily_token_quota","tokens_used":100000000,"tokens_limit":100000000}}`,
	)

	is429, shouldRetry, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.False(t, shouldRetry, "large Retry-After (>=3600) means quota exhaustion, not retryable")
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	// 大额 Retry-After → quota-exceeded 通知
	require.Equal(t, "quota-exceeded", payload["type"])
	dur := payload["duration"].(float64)
	require.GreaterOrEqual(t, dur, float64(5))
	require.LessOrEqual(t, dur, float64(30))
}

func TestHandle429_RetryAfterCappedAt120(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// 异常大的 Retry-After（但 <3600）应被限制到 120s
	rsp := make429ResponseWithBody(
		[]string{"Retry-After: 9999"},
		`{"error":{"message":"x"}}`,
	)

	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	dur := payload["duration"].(float64)
	require.LessOrEqual(t, dur, float64(123)) // 120 + max 3 jitter
}

// --- message 提取 ---

func TestHandle429_ExtractsMessageFromAIBalanceBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// AIBalance 服务端在 error.message 中注入完整中文文案，客户端应直接展示。
	msg := "该 API Key 的 Token 计费额度已用尽（已达单 Key 上限）。请联系管理员提升额度或重置用量。"
	body := `{"type":"error","error":{"message":"` + msg + `","type":"token_limit_exceeded","limit_kind":"token"}}`
	rsp := make429KindResponse("token", nil, body)

	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Contains(t, payload["content"], "Token 计费额度已用尽")
	require.Contains(t, payload["content"], "请联系管理员")
}

func TestHandle429_ExtractsMessageFromOpenAIBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// OpenAI 风格 429 body
	rsp := make429ResponseWithBody(
		[]string{"Retry-After: 5"},
		`{"error":{"message":"You exceeded your current quota, please check your plan and billing details.","type":"rate_limit_exceeded","code":"insufficient_quota"}}`,
	)

	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Contains(t, payload["content"], "exceeded your current quota")
}

func TestHandle429_ExtractsMessageFromAnthropicBody(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// Anthropic 风格 429 body
	rsp := make429ResponseWithBody(
		[]string{"Retry-After: 10"},
		`{"type":"error","error":{"type":"rate_limit_error","message":"Number of request resources exceeded."}}`,
	)

	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Contains(t, payload["content"], "Number of request resources exceeded")
}

func TestHandle429_EmptyMessage_FallsBackToDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429ResponseWithBody(
		[]string{"Retry-After: 5"},
		`{"error":{"message":"","type":"rate_limit_exceeded"}}`,
	)

	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Contains(t, payload["content"], "HTTP 429")
}

func TestHandle429_NoBody_FallsBackToDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// make429Response 的 body 是 {"error":"rate limited"} — error 是字符串不是对象，
	// JSON 解析会失败，body 为 nil，回退到默认文案。
	rsp := make429Response()

	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Contains(t, payload["content"], "HTTP 429")
}

// --- AIBalance limit_kind 日志记录 ---

func TestHandle429_LimitKindFromBody_WhenHeaderMissing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// 不带 X-AIBalance-Limit-Kind 头，响应体 error.limit_kind=daily_token_quota
	headers := []string{"Content-Type: application/json", "Retry-After: 3600"}
	body := `{"type":"error","error":{"message":"daily exceeded","limit_kind":"daily_token_quota","tokens_used":100000000,"tokens_limit":100000000}}`
	rsp := make429ResponseWithBody(headers, body)

	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Contains(t, payload["content"], "daily exceeded")
}

func TestHandle429_UnknownLimitKind_StillHandledByRetryAfter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	// 未知 kind 有 Retry-After → rate-limit 路径
	rsp := make429KindResponse("some_unknown_kind", []string{"Retry-After: 8"}, "")

	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Equal(t, "rate-limit", payload["type"])
	require.Contains(t, payload["content"], "rate limited")
}

// --- 旧版队列头兼容 ---

func TestHandle429_LegacyQueue_ParseableQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg, snapshot := newTestConfigForHandle429WithEvents(ctx)
	rsp := make429Response("X-AIBalance-Info: 2")

	is429, shouldRetry, ctxDone := cfg.handle429RateLimit(rsp)
	cfg.Emitter.WaitForStream()

	assert.True(t, is429)
	assert.True(t, shouldRetry, "legacy queue 429 is rate-limit, should be retryable")
	assert.True(t, ctxDone)
	payload := requireNotifyPayload(t, snapshot())
	require.Contains(t, payload["content"], "此刻有 2 位用户正在与我深度对话中")
}

func TestHandle429_LegacyQueue_ZeroQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := newTestConfigForHandle429(ctx)
	rsp := make429Response("X-AIBalance-Info: 0")

	start := time.Now()
	is429, _, ctxDone := cfg.handle429RateLimit(rsp)
	elapsed := time.Since(start)

	assert.True(t, is429)
	assert.True(t, ctxDone)
	assert.Less(t, elapsed, 2*time.Second)
}

// --- parse429Body ---

func TestParse429Body_NilResponse(t *testing.T) {
	assert.Nil(t, parse429Body(nil))
}

func TestParse429Body_EmptyBody(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), nil)
	assert.Nil(t, parse429Body(rsp))
}

func TestParse429Body_InvalidJSON(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), []byte("not json"))
	assert.Nil(t, parse429Body(rsp))
}

func TestParse429Body_ValidJSON(t *testing.T) {
	rsp := NewUnboundAIResponse()
	body := `{"error":{"message":"hi","limit_kind":"rpm","queue_length":5,"tokens_used":100,"tokens_limit":200}}`
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), []byte(body))
	parsed := parse429Body(rsp)
	require.NotNil(t, parsed)
	assert.Equal(t, "rpm", parsed.Error.LimitKind)
	assert.Equal(t, int64(5), parsed.Error.QueueLength)
	assert.Equal(t, int64(100), parsed.Error.TokensUsed)
	assert.Equal(t, int64(200), parsed.Error.TokensLimit)
}

// --- resolveLimitKind ---

func TestResolveLimitKind_HeaderTakesPriority(t *testing.T) {
	rsp := NewUnboundAIResponse()
	hdr := "HTTP/1.1 429 Too Many Requests\r\nX-AIBalance-Limit-Kind: rpm\r\n\r\n"
	rsp.SetRawHTTPResponseData([]byte(hdr), []byte(`{"error":{"limit_kind":"token"}}`))
	kind := resolveLimitKind(rsp, parse429Body(rsp))
	assert.Equal(t, "rpm", kind)
}

func TestResolveLimitKind_BodyFallback(t *testing.T) {
	rsp := NewUnboundAIResponse()
	hdr := "HTTP/1.1 429 Too Many Requests\r\n\r\n"
	rsp.SetRawHTTPResponseData([]byte(hdr), []byte(`{"error":{"limit_kind":"token"}}`))
	kind := resolveLimitKind(rsp, parse429Body(rsp))
	assert.Equal(t, "token", kind)
}

func TestResolveLimitKind_BodyAliasFallback(t *testing.T) {
	rsp := NewUnboundAIResponse()
	hdr := "HTTP/1.1 429 Too Many Requests\r\n\r\n"
	rsp.SetRawHTTPResponseData([]byte(hdr), []byte(`{"error":{"limit_kind":"daily_token_quota"}}`))
	kind := resolveLimitKind(rsp, parse429Body(rsp))
	assert.Equal(t, "daily_token", kind)
}

func TestResolveLimitKind_Empty(t *testing.T) {
	rsp := NewUnboundAIResponse()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Too Many Requests\r\n\r\n"), []byte(`{"error":{}}`))
	kind := resolveLimitKind(rsp, parse429Body(rsp))
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
	assert.Equal(t, 9999, capRetryAfterSeconds(9999, 5, 0)) // maxSec <= 0: no upper cap
}

// --- extract429Message ---

func TestExtract429Message_NilBody(t *testing.T) {
	assert.Equal(t, default429Message, extract429Message(nil, ""))
}

func TestExtract429Message_EmptyMessage(t *testing.T) {
	body := &aibalance429Body{}
	body.Error.Message = ""
	assert.Equal(t, default429Message, extract429Message(body, ""))
}

func TestExtract429Message_WithMessage(t *testing.T) {
	body := &aibalance429Body{}
	body.Error.Message = "rate limit from provider"
	assert.Equal(t, "rate limit from provider", extract429Message(body, ""))
}

func TestExtract429Message_WhitespaceMessage(t *testing.T) {
	body := &aibalance429Body{}
	body.Error.Message = "   "
	assert.Equal(t, default429Message, extract429Message(body, ""))
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
	assert.Equal(t, "original", string(rsp.GetHTTPResponseBody()))
}


// --- is429Retryable ---

func TestIs429Retryable_WithRetryAfter(t *testing.T) {
	rsp := make429ResponseWithBody(
		[]string{"Retry-After: 10"},
		`{"error":{"message":"x"}}`,
	)
	assert.True(t, is429Retryable(context.Background(), rsp))
}

func TestIs429Retryable_NoRetryAfter(t *testing.T) {
	rsp := make429KindResponse("token", nil, `{"error":{"message":"x","limit_kind":"token"}}`)
	assert.False(t, is429Retryable(context.Background(), rsp))
}

func TestIs429Retryable_LargeRetryAfter(t *testing.T) {
	rsp := make429KindResponse("daily_token",
		[]string{"Retry-After: 3600"},
		`{"error":{"message":"x","limit_kind":"daily_token"}}`,
	)
	assert.False(t, is429Retryable(context.Background(), rsp))
}

func TestIs429Retryable_Non429(t *testing.T) {
	rsp := make200Response()
	assert.False(t, is429Retryable(context.Background(), rsp))
}

func TestIs429Retryable_Nil(t *testing.T) {
	assert.False(t, is429Retryable(context.Background(), nil))
}
