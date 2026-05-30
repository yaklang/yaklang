package aibalance

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: db_free_ip_test, 单 IP 免费模型每日用量限额单元测试

func cleanupFreeUserIPForDate(t *testing.T, date string) {
	require.NoError(t, freeIPDB().Where("date = ?", date).Delete(&FreeUserIPDailyUsage{}).Error)
}

// setRateLimitConfigForIPTest 设置单 IP 限额相关配置并返回 restore 函数。
// 关键词: setRateLimitConfigForIPTest, 单 IP 限额配置注入
func setRateLimitConfigForIPTest(t *testing.T, enable bool, requestLimit, tokenLimitM int64) func() {
	require.NoError(t, EnsureRateLimitConfigTable())
	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	origEnable := cfg.FreeUserIPLimitEnable
	origReq := cfg.FreeUserIPDailyRequestLimit
	origTok := cfg.FreeUserIPDailyTokenLimitM
	cfg.FreeUserIPLimitEnable = enable
	cfg.FreeUserIPDailyRequestLimit = requestLimit
	cfg.FreeUserIPDailyTokenLimitM = tokenLimitM
	require.NoError(t, SaveRateLimitConfig(cfg))
	return func() {
		cfg2, err := GetRateLimitConfig()
		if err != nil {
			return
		}
		cfg2.FreeUserIPLimitEnable = origEnable
		cfg2.FreeUserIPDailyRequestLimit = origReq
		cfg2.FreeUserIPDailyTokenLimitM = origTok
		_ = SaveRateLimitConfig(cfg2)
	}
}

func TestEnsureFreeUserIPDailyUsageTable(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())
}

func TestAddFreeUserIPDailyUsage_Accumulate(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	date := time.Now().AddDate(0, 0, 400).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, date)
	defer setFreeTokenNowDate(date)()

	ip := "203.0.113.7"
	require.NoError(t, AddFreeUserIPDailyRequest(ip))
	require.NoError(t, AddFreeUserIPDailyRequest(ip))
	require.NoError(t, AddFreeUserIPDailyTokens(ip, 1500))
	require.NoError(t, AddFreeUserIPDailyTokens(ip, 500))

	req, tokens, err := GetFreeUserIPDailyUsage(date, ip)
	require.NoError(t, err)
	assert.Equal(t, int64(2), req)
	assert.Equal(t, int64(2000), tokens)
}

func TestAddFreeUserIPDailyUsage_IgnoredIP(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	date := time.Now().AddDate(0, 0, 401).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// 空 IP / unknown 占位均不计数
	require.NoError(t, AddFreeUserIPDailyRequest(""))
	require.NoError(t, AddFreeUserIPDailyRequest("unknown"))
	require.NoError(t, AddFreeUserIPDailyTokens("", 1000))
	require.NoError(t, AddFreeUserIPDailyTokens("unknown", 1000))

	var count int64
	require.NoError(t, freeIPDB().Model(&FreeUserIPDailyUsage{}).Where("date = ?", date).Count(&count).Error)
	assert.Equal(t, int64(0), count, "ignored IPs must not create rows")
}

func TestAddFreeUserIPDailyTokens_NonPositive(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	date := time.Now().AddDate(0, 0, 402).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, date)
	defer setFreeTokenNowDate(date)()

	require.NoError(t, AddFreeUserIPDailyTokens("198.51.100.1", 0))
	require.NoError(t, AddFreeUserIPDailyTokens("198.51.100.1", -10))

	req, tokens, err := GetFreeUserIPDailyUsage(date, "198.51.100.1")
	require.NoError(t, err)
	assert.Equal(t, int64(0), req)
	assert.Equal(t, int64(0), tokens)
}

func TestCheckFreeUserIPDailyLimit_Disabled(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	date := time.Now().AddDate(0, 0, 403).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, date)
	defer setFreeTokenNowDate(date)()
	defer setRateLimitConfigForIPTest(t, false, 1, 1)()

	// 已经用满，但限额被禁用 -> 放行
	require.NoError(t, AddFreeUserIPDailyRequest("192.0.2.50"))
	require.NoError(t, AddFreeUserIPDailyTokens("192.0.2.50", 10*FreeUserTokenMUnit))

	d, err := CheckFreeUserIPDailyLimit("192.0.2.50")
	require.NoError(t, err)
	assert.True(t, d.Allowed, "disabled limit should always allow")
}

func TestCheckFreeUserIPDailyLimit_IgnoredIPAllowed(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())
	defer setRateLimitConfigForIPTest(t, true, 1, 1)()

	for _, ip := range []string{"", "unknown", "  "} {
		d, err := CheckFreeUserIPDailyLimit(ip)
		require.NoError(t, err)
		assert.True(t, d.Allowed, "ignored IP %q should be allowed", ip)
	}
}

func TestCheckFreeUserIPDailyLimit_RequestExceeded(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	date := time.Now().AddDate(0, 0, 404).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, date)
	defer setFreeTokenNowDate(date)()
	// 请求上限=2，Token 不限
	defer setRateLimitConfigForIPTest(t, true, 2, 0)()

	ip := "203.0.113.99"
	d0, err := CheckFreeUserIPDailyLimit(ip)
	require.NoError(t, err)
	assert.True(t, d0.Allowed)
	assert.Equal(t, int64(2), d0.RequestLimit)

	require.NoError(t, AddFreeUserIPDailyRequest(ip))
	require.NoError(t, AddFreeUserIPDailyRequest(ip))

	d, err := CheckFreeUserIPDailyLimit(ip)
	require.NoError(t, err)
	assert.False(t, d.Allowed, "should reject when request_used >= request_limit")
	assert.Equal(t, "request", d.ExceededKind)
	assert.Equal(t, int64(2), d.RequestUsed)
}

func TestCheckFreeUserIPDailyLimit_TokenExceeded(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	date := time.Now().AddDate(0, 0, 405).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, date)
	defer setFreeTokenNowDate(date)()
	// 请求不限，Token 上限=2M
	defer setRateLimitConfigForIPTest(t, true, 0, 2)()

	ip := "203.0.113.100"
	require.NoError(t, AddFreeUserIPDailyTokens(ip, 2*FreeUserTokenMUnit))

	d, err := CheckFreeUserIPDailyLimit(ip)
	require.NoError(t, err)
	assert.False(t, d.Allowed, "should reject when tokens_used >= tokens_limit")
	assert.Equal(t, "token", d.ExceededKind)
	assert.Equal(t, int64(2*FreeUserTokenMUnit), d.TokensLimit)
	assert.Equal(t, int64(2*FreeUserTokenMUnit), d.TokensUsed)
}

func TestCheckFreeUserIPDailyLimit_BothZeroAllow(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	date := time.Now().AddDate(0, 0, 406).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, date)
	defer setFreeTokenNowDate(date)()
	// 启用但两个上限都为 0 -> 不限制
	defer setRateLimitConfigForIPTest(t, true, 0, 0)()

	ip := "203.0.113.101"
	require.NoError(t, AddFreeUserIPDailyRequest(ip))
	require.NoError(t, AddFreeUserIPDailyTokens(ip, 999*FreeUserTokenMUnit))

	d, err := CheckFreeUserIPDailyLimit(ip)
	require.NoError(t, err)
	assert.True(t, d.Allowed, "both limits zero means unlimited")
}

func TestFreeUserIPDailyUsage_DayRollover(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	day1 := time.Now().AddDate(0, 0, 407).Format("2006-01-02")
	day2 := time.Now().AddDate(0, 0, 408).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, day1)
	defer cleanupFreeUserIPForDate(t, day2)

	ip := "203.0.113.111"
	restore := setFreeTokenNowDate(day1)
	require.NoError(t, AddFreeUserIPDailyRequest(ip))
	require.NoError(t, AddFreeUserIPDailyTokens(ip, 777))
	restore()

	restore2 := setFreeTokenNowDate(day2)
	defer restore2()
	require.NoError(t, AddFreeUserIPDailyRequest(ip))

	req1, tok1, err := GetFreeUserIPDailyUsage(day1, ip)
	require.NoError(t, err)
	assert.Equal(t, int64(1), req1)
	assert.Equal(t, int64(777), tok1)

	req2, tok2, err := GetFreeUserIPDailyUsage(day2, ip)
	require.NoError(t, err)
	assert.Equal(t, int64(1), req2)
	assert.Equal(t, int64(0), tok2, "day 2 starts fresh, tokens not carried over")
}

func TestQueryFreeUserIPUsageSnapshot(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	date := time.Now().AddDate(0, 0, 409).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// 三个 IP，不同 Token 用量
	require.NoError(t, AddFreeUserIPDailyTokens("10.0.0.1", 3*FreeUserTokenMUnit))
	require.NoError(t, AddFreeUserIPDailyRequest("10.0.0.1"))
	require.NoError(t, AddFreeUserIPDailyTokens("10.0.0.2", 5*FreeUserTokenMUnit))
	require.NoError(t, AddFreeUserIPDailyTokens("10.0.0.3", 1*FreeUserTokenMUnit))

	distinct, top, gotDate, err := QueryFreeUserIPUsageSnapshot(20)
	require.NoError(t, err)
	assert.Equal(t, date, gotDate)
	assert.Equal(t, int64(3), distinct, "should count 3 distinct IPs")
	require.Len(t, top, 3)

	// 按 tokens_used 降序：10.0.0.2(5M) > 10.0.0.1(3M) > 10.0.0.3(1M)
	assert.Equal(t, "10.0.0.2", top[0].IP)
	assert.Equal(t, "10.0.0.1", top[1].IP)
	assert.Equal(t, "10.0.0.3", top[2].IP)
	assert.InDelta(t, 5.0, top[0].UsedM, 0.0001)
	assert.Equal(t, int64(1), top[1].RequestCount)
}

func TestCleanupOldFreeUserIPUsage(t *testing.T) {
	require.NoError(t, EnsureFreeUserIPDailyUsageTable())

	// 一个很旧的日期（远早于保留窗）
	oldDate := time.Now().AddDate(0, 0, -100).Format("2006-01-02")
	defer cleanupFreeUserIPForDate(t, oldDate)

	restore := setFreeTokenNowDate(oldDate)
	require.NoError(t, AddFreeUserIPDailyRequest("8.8.8.8"))
	restore()

	var before int64
	require.NoError(t, freeIPDB().Model(&FreeUserIPDailyUsage{}).Where("date = ?", oldDate).Count(&before).Error)
	require.Equal(t, int64(1), before)

	removed, err := CleanupOldFreeUserIPUsage(2)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, removed, int64(1))

	var after int64
	require.NoError(t, freeIPDB().Model(&FreeUserIPDailyUsage{}).Where("date = ?", oldDate).Count(&after).Error)
	assert.Equal(t, int64(0), after, "old row should be removed")
}

// TestWriteFreeIPLimitResponse_Format 验证单 IP 限额 429 响应字段与 header 契约。
// 关键词: TestWriteFreeIPLimitResponse_Format, free_ip 429 文案
func TestWriteFreeIPLimitResponse_Format(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	decision := &FreeUserIPLimitDecision{
		IP:           "203.0.113.7",
		RequestUsed:  501,
		RequestLimit: 500,
		TokensUsed:   12345,
		TokensLimit:  31457280,
		ExceededKind: "request",
	}
	go func() {
		cfg.writeFreeIPLimitResponse(server, decision)
		server.Close()
	}()

	resp := readAllFromPipe(t, client)

	assert.Contains(t, resp, "HTTP/1.1 429 Too Many Requests")
	assert.Contains(t, resp, "X-AIBalance-Limit-Kind: free_ip")
	assert.Contains(t, resp, "Retry-After: 3600")

	bodyIdx := strings.Index(resp, "\r\n\r\n")
	require.Greater(t, bodyIdx, 0)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp[bodyIdx+4:]), &parsed))
	errObj, ok := parsed["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "free_ip_limit_exceeded", errObj["type"])
	assert.Equal(t, "free_ip_quota", errObj["limit_kind"])
	assert.Equal(t, "免费用量已用尽", errObj["limit_kind_zh"])
	assert.Equal(t, "request", errObj["exceeded_kind"])
	assert.Equal(t, float64(501), errObj["request_used"])
	assert.Equal(t, float64(500), errObj["request_limit"])

	msg, _ := errObj["message"].(string)
	assert.Contains(t, msg, "当前环境免费用量已用尽", "should contain the fixed Chinese notice")
	assert.Contains(t, msg, "configure your own AI backend", "should keep English clue")
}

// TestWriteFreeIPLimitResponse_Custom429Override 验证自定义 429 文案 + notice 注入。
// 关键词: TestWriteFreeIPLimitResponse_Custom429Override, resolveLimit429 free_ip
func TestWriteFreeIPLimitResponse_Custom429Override(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	cfg.limitPolicyMu.Lock()
	cfg.custom429Enabled = true
	cfg.custom429Notice = "please buy a plan"
	cfg.custom429KindOverrides = map[string]string{"free_ip": "custom free ip message"}
	cfg.limitPolicyMu.Unlock()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.writeFreeIPLimitResponse(server, &FreeUserIPLimitDecision{ExceededKind: "token"})
		server.Close()
	}()

	resp := readAllFromPipe(t, client)
	bodyIdx := strings.Index(resp, "\r\n\r\n")
	require.Greater(t, bodyIdx, 0)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(resp[bodyIdx+4:]), &parsed))
	errObj := parsed["error"].(map[string]interface{})
	assert.Equal(t, "custom free ip message", errObj["message"])
	assert.Equal(t, "please buy a plan", errObj["notice"])
}

// readAllFromPipe 读尽 net.Pipe 客户端可读字节，供 429 响应断言。
// 关键词: readAllFromPipe, net.Pipe 读取辅助
func readAllFromPipe(t *testing.T, client net.Conn) string {
	t.Helper()
	var result []byte
	buf := make([]byte, 8192)
	for {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := client.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(result)
}
