package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 关键词: daily_token_e2e_test, aibalance 免费用户日 Token 限额端到端集成测试
//
// 本文件验证「免费用户日 Token 限额」策略的端到端行为：
//   1. 通过 net.Listen + ServerConfig.Serve 启动真实 TCP 服务
//   2. 用 net.Dial 发起真实 HTTP 请求
//   3. 覆盖 429 daily_token / 模型独立桶 / 模型豁免 / portal 快照 /
//      portal 修改限额 / 越权防护 / 付费 key Token 限额接入等场景
//
// 所有用例使用未来日期作为 freeTokenNowDate mock，避免污染线上数据。

// startDailyTokenTestServer 启动一个完整的 aibalance 服务，
// 用未来日期 mock freeTokenNowDate 隔离 DB 数据。
// 返回 addr、cfg、清理函数（清理函数会重置 mock 日期 & 删除测试数据）。
func startDailyTokenTestServer(t *testing.T, dayOffset int) (string, *ServerConfig, string, func()) {
	t.Helper()

	consts.InitializeYakitDatabase("", "", "")
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())
	require.NoError(t, EnsureRateLimitConfigTable())
	require.NoError(t, GetDB().AutoMigrate(&schema.AiApiKeys{}).Error)

	mockDate := time.Now().AddDate(0, 0, dayOffset).Format("2006-01-02")
	origNow := freeTokenNowDate
	freeTokenNowDate = func() string { return mockDate }

	// 预清空本测试日期下的 token 桶 + 配置覆盖项，避免互相污染
	require.NoError(t, freeTokenDB().Where("date = ?", mockDate).Delete(&schema.FreeUserDailyTokenUsage{}).Error)

	cfg := NewServerConfig()
	cfg.AdminPassword = "test-admin-password-secure"
	cfg.AuthMiddleware = NewAuthMiddleware(cfg, DefaultAuthConfig())
	// 禁用免费用户预延迟，避免测试拖慢
	cfg.freeUserDelaySec = 0
	// 较高的 RPM，避免 RPM 限流提前触发，保证我们能稳定测到 Token 限额
	if cfg.chatRateLimiter != nil {
		cfg.chatRateLimiter.SetDefaultRPM(10000)
	}

	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	lis, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go cfg.Serve(conn)
		}
	}()

	time.Sleep(50 * time.Millisecond)

	cleanup := func() {
		lis.Close()
		freeTokenNowDate = origNow
		_ = freeTokenDB().Where("date = ?", mockDate).Delete(&schema.FreeUserDailyTokenUsage{}).Error
		// 还原默认 rate limit 配置（防止跨测试污染）
		if rlCfg, err := GetRateLimitConfig(); err == nil {
			rlCfg.FreeUserTokenLimitM = 1200
			rlCfg.FreeUserTokenModelOverrides = "{}"
			rlCfg.DefaultRPM = 600
			rlCfg.ModelRPMOverrides = "{}"
			_ = SaveRateLimitConfig(rlCfg)
		}
		cfg.Close()
	}

	return addr, cfg, mockDate, cleanup
}

func sendChatCompletionHTTP(t *testing.T, addr, apiKey, model string) (int, map[string]string, string) {
	t.Helper()

	body := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"hello"}],"stream":false}`, model)

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	var reqBuf bytes.Buffer
	reqBuf.WriteString("POST /v1/chat/completions HTTP/1.1\r\n")
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", addr))
	if apiKey != "" {
		reqBuf.WriteString(fmt.Sprintf("Authorization: Bearer %s\r\n", apiKey))
	}
	reqBuf.WriteString("Content-Type: application/json\r\n")
	reqBuf.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(body)))
	reqBuf.WriteString("Connection: close\r\n\r\n")
	reqBuf.WriteString(body)

	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Write(reqBuf.Bytes())
	require.NoError(t, err)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var respBuf bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, readErr := conn.Read(buf)
		if n > 0 {
			respBuf.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}

	raw := respBuf.String()
	return parseHTTPResponse(raw)
}

func parseHTTPResponse(raw string) (int, map[string]string, string) {
	parts := strings.SplitN(raw, "\r\n\r\n", 2)
	headerSection := parts[0]
	bodySection := ""
	if len(parts) > 1 {
		bodySection = parts[1]
	}

	headerLines := strings.Split(headerSection, "\r\n")
	statusCode := 0
	if len(headerLines) > 0 {
		fmt.Sscanf(headerLines[0], "HTTP/1.1 %d", &statusCode)
	}

	respHeaders := make(map[string]string)
	for _, line := range headerLines[1:] {
		kv := strings.SplitN(line, ": ", 2)
		if len(kv) == 2 {
			respHeaders[strings.ToLower(kv[0])] = kv[1]
		}
	}
	return statusCode, respHeaders, bodySection
}

// loginAdminGetCookie 复用 portal_auth_security_test 的登录流程。
func loginAdminGetCookie(t *testing.T, addr, password string) string {
	return loginAndGetSession(t, addr, password)
}

// ==================== A. 免费用户全局桶超额 -> 429 daily_token ====================

// 关键词: TestE2E_FreeUser_GlobalBucket_429, 免费用户全局桶超额端到端
func TestE2E_FreeUser_GlobalBucket_429(t *testing.T) {
	addr, _, mockDate, cleanup := startDailyTokenTestServer(t, 401)
	defer cleanup()

	setRateLimitConfigForFreeTokenTest(t, 2, "{}")

	// 预先把全局桶累加到 == 2M（=2*FreeUserTokenMUnit），让下一次请求被拒
	// 注意：使用非 memfit- 前缀的模型名，避免触发 TOTP 拦截
	require.NoError(t, AddFreeUserDailyTokenUsage("anything-free", 2*FreeUserTokenMUnit, false))

	// 发起一个 free model 请求，期望被 daily_token 429 阻挡
	status, headers, body := sendChatCompletionHTTP(t, addr, "", "dailytest-light-free")
	assert.Equal(t, http.StatusTooManyRequests, status, "should be 429 when global bucket exhausted")
	assert.Equal(t, "daily_token", headers["x-aibalance-limit-kind"], "should mark as daily_token kind")
	assert.Equal(t, fmt.Sprintf("%d", 2*FreeUserTokenMUnit), headers["x-aibalance-token-used"])
	assert.Equal(t, fmt.Sprintf("%d", 2*FreeUserTokenMUnit), headers["x-aibalance-token-limit"])
	assert.Equal(t, "3600", headers["retry-after"])

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))
	errObj := parsed["error"].(map[string]interface{})
	assert.Equal(t, "daily_token_limit_exceeded", errObj["type"])
	assert.Equal(t, "daily_token_quota", errObj["limit_kind"])
	assert.Equal(t, "日限额已满", errObj["limit_kind_zh"])
	assert.Equal(t, "global", errObj["bucket"])
	assert.Equal(t, "dailytest-light-free", errObj["model"])
	assert.Equal(t, float64(2), errObj["tokens_limit_m"])

	// 校验数据落在期望的 mock 日期
	got, err := GetFreeUserDailyTokenUsage(mockDate, "")
	require.NoError(t, err)
	assert.Equal(t, int64(2*FreeUserTokenMUnit), got)
}

// ==================== B. 模型独立桶不污染全局桶 ====================

// 关键词: TestE2E_FreeUser_ModelBucket_Isolated, 模型独立桶端到端隔离
func TestE2E_FreeUser_ModelBucket_Isolated(t *testing.T) {
	addr, _, mockDate, cleanup := startDailyTokenTestServer(t, 402)
	defer cleanup()

	// 全局 1M；dailytest-light-free 模型独立桶 5M
	setRateLimitConfigForFreeTokenTest(t, 1,
		`{"dailytest-light-free":{"limit_m":5,"exempt":false}}`)

	// 预先用满全局桶
	require.NoError(t, AddFreeUserDailyTokenUsage("foo-free", 1*FreeUserTokenMUnit, false))

	// dailytest-light-free 走模型独立桶 5M（还有空间），不应该被全局桶拖累
	// 它会通过 daily token 检查（不返 429）；后续 provider 缺失会返 5xx
	status, headers, _ := sendChatCompletionHTTP(t, addr, "", "dailytest-light-free")
	assert.NotEqual(t, http.StatusTooManyRequests, status,
		"model with own bucket should not be blocked by global bucket exhaust (got %d)", status)
	if status == http.StatusTooManyRequests {
		// 如果意外 429，也不应是 daily_token 那种
		assert.NotEqual(t, "daily_token", headers["x-aibalance-limit-kind"])
	}

	// 没覆盖的 model 仍然走全局桶 -> 应该被 429 阻挡
	status2, headers2, _ := sendChatCompletionHTTP(t, addr, "", "foo-free")
	assert.Equal(t, http.StatusTooManyRequests, status2)
	assert.Equal(t, "daily_token", headers2["x-aibalance-limit-kind"])

	// 模型桶不应该写入全局桶 (因为我们前面没真正落 token；这里仅断言独立桶未被错误污染)
	usedModel, err := GetFreeUserDailyTokenUsage(mockDate, "dailytest-light-free")
	require.NoError(t, err)
	assert.Equal(t, int64(0), usedModel, "model bucket should still be 0 (no provider invoked, no usage settled)")
}

// ==================== C. 模型豁免直接放行 ====================

// 关键词: TestE2E_FreeUser_ExemptModel_NotBlocked, 模型豁免端到端
func TestE2E_FreeUser_ExemptModel_NotBlocked(t *testing.T) {
	addr, _, _, cleanup := startDailyTokenTestServer(t, 403)
	defer cleanup()

	// 全局 1M（已用满）、dailytest-exempt-free 模型豁免
	setRateLimitConfigForFreeTokenTest(t, 1,
		`{"dailytest-exempt-free":{"limit_m":0,"exempt":true}}`)
	require.NoError(t, AddFreeUserDailyTokenUsage("other-free", 1*FreeUserTokenMUnit, false))

	// 豁免模型即便全局桶满了也要放行；返回不会是 429 daily_token
	status, headers, body := sendChatCompletionHTTP(t, addr, "", "dailytest-exempt-free")
	if status == http.StatusTooManyRequests {
		assert.NotEqual(t, "daily_token", headers["x-aibalance-limit-kind"],
			"exempt model should NEVER be blocked by daily_token. body=%s", body)
	}

	// 非豁免模型受全局桶限制 -> 429 daily_token
	status2, headers2, _ := sendChatCompletionHTTP(t, addr, "", "other-free")
	assert.Equal(t, http.StatusTooManyRequests, status2)
	assert.Equal(t, "daily_token", headers2["x-aibalance-limit-kind"])
}

// ==================== D. RPM vs daily_token 两类 429 的 X-AIBalance-Limit-Kind 区分 ====================

// 关键词: TestE2E_RPM_vs_DailyToken_Kind, X-AIBalance-Limit-Kind 区分
func TestE2E_RPM_vs_DailyToken_Kind(t *testing.T) {
	addr, cfg, _, cleanup := startDailyTokenTestServer(t, 404)
	defer cleanup()

	// 准备：daily token 限额未触发；RPM=1 强制下一个请求 429 rpm
	// 注意：必须同时把 DB 里的 DefaultRPM 写成 1，否则 chat hot path 在
	// 找不到 provider 时会触发 LoadProvidersFromDatabase -> applyRateLimitConfig
	// 把内存 RPM 还原回 DB 的值（默认 600），使 SetDefaultRPM(1) 不再生效。
	// 关键词: RPM 测试 DB 持久化, applyRateLimitConfig reload 覆盖修复
	rlCfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	rlCfg.DefaultRPM = 1
	rlCfg.FreeUserTokenLimitM = 1200
	rlCfg.FreeUserTokenModelOverrides = "{}"
	require.NoError(t, SaveRateLimitConfig(rlCfg))
	cfg.chatRateLimiter.SetDefaultRPM(1)

	// 1) 第一个请求通过 RPM，但会被下游 provider 缺失打挂；状态不为 429
	status1, headers1, _ := sendChatCompletionHTTP(t, addr, "", "rpm-test-free")
	assert.NotEqual(t, http.StatusTooManyRequests, status1)
	_ = headers1

	// 2) 第二次请求 -> RPM 限流，应该带 X-AIBalance-Limit-Kind: rpm
	status2, headers2, _ := sendChatCompletionHTTP(t, addr, "", "rpm-test-free")
	assert.Equal(t, http.StatusTooManyRequests, status2)
	assert.Equal(t, "rpm", headers2["x-aibalance-limit-kind"])
	assert.NotEqual(t, "daily_token", headers2["x-aibalance-limit-kind"])
}

// ==================== E. Portal rate-limit-config 写入 + 读取 ====================

// 关键词: TestE2E_Portal_RateLimitConfig_RoundTrip, portal 写入并读回配置
func TestE2E_Portal_RateLimitConfig_RoundTrip(t *testing.T) {
	addr, _, _, cleanup := startDailyTokenTestServer(t, 405)
	defer cleanup()

	cookie := loginAdminGetCookie(t, addr, "test-admin-password-secure")
	require.NotEmpty(t, cookie, "should login as admin")

	cookieHeader := map[string]string{"Cookie": "admin_session=" + cookie}

	payload := map[string]interface{}{
		"default_rpm":             600,
		"free_user_delay_sec":     0,
		"free_user_token_limit_m": 777,
		"free_user_token_model_overrides": map[string]map[string]interface{}{
			"some-model-free":   {"limit_m": 50, "exempt": false},
			"another-free":      {"limit_m": 0, "exempt": true},
			"":                  {"limit_m": 999, "exempt": false}, // 应被过滤
			"   ":               {"limit_m": 999, "exempt": false}, // 应被过滤
		},
	}
	pb, _ := json.Marshal(payload)
	status, _, body := sendRawHTTPRequest(t, addr, "POST", "/portal/api/rate-limit-config",
		cookieHeader, string(pb))
	require.Equal(t, 200, status, "set rate-limit-config should 200: %s", body)

	// 读回
	status2, _, body2 := sendRawHTTPRequest(t, addr, "GET", "/portal/api/rate-limit-config",
		cookieHeader, "")
	require.Equal(t, 200, status2, "get rate-limit-config should 200: %s", body2)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body2), &resp))
	conf := resp["config"].(map[string]interface{})
	assert.Equal(t, float64(777), conf["free_user_token_limit_m"])

	ov := conf["free_user_token_model_overrides"].(map[string]interface{})
	assert.Contains(t, ov, "some-model-free")
	assert.Contains(t, ov, "another-free")
	assert.NotContains(t, ov, "", "empty key must be filtered out")
	assert.NotContains(t, ov, "   ", "blank key must be filtered out")
	smf := ov["some-model-free"].(map[string]interface{})
	assert.Equal(t, float64(50), smf["limit_m"])
	assert.Equal(t, false, smf["exempt"])
	af := ov["another-free"].(map[string]interface{})
	assert.Equal(t, true, af["exempt"])
}

// ==================== F. Portal rate-limit-status 暴露快照 ====================

// 关键词: TestE2E_Portal_RateLimitStatus_Snapshot, portal status 快照
func TestE2E_Portal_RateLimitStatus_Snapshot(t *testing.T) {
	addr, _, _, cleanup := startDailyTokenTestServer(t, 406)
	defer cleanup()

	setRateLimitConfigForFreeTokenTest(t, 100,
		`{"snap-light-free":{"limit_m":50,"exempt":false},"snap-exempt-free":{"limit_m":0,"exempt":true}}`)
	require.NoError(t, AddFreeUserDailyTokenUsage("snap-light-free", 3*FreeUserTokenMUnit, true))
	require.NoError(t, AddFreeUserDailyTokenUsage("snap-other-free", 2*FreeUserTokenMUnit, false))

	cookie := loginAdminGetCookie(t, addr, "test-admin-password-secure")
	require.NotEmpty(t, cookie)
	cookieHeader := map[string]string{"Cookie": "admin_session=" + cookie}

	status, _, body := sendRawHTTPRequest(t, addr, "GET", "/portal/api/rate-limit-status",
		cookieHeader, "")
	require.Equal(t, 200, status)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	usage := resp["free_user_token_usage"].(map[string]interface{})

	global := usage["global"].(map[string]interface{})
	assert.Equal(t, float64(100), global["limit_m"])
	assert.Equal(t, float64(2*FreeUserTokenMUnit), global["tokens_used"])
	assert.Equal(t, float64(2), global["used_m"])

	perModel := usage["per_model"].([]interface{})
	modelMap := map[string]map[string]interface{}{}
	for _, item := range perModel {
		m := item.(map[string]interface{})
		modelMap[m["model"].(string)] = m
	}
	assert.Equal(t, float64(50), modelMap["snap-light-free"]["limit_m"])
	assert.Equal(t, float64(3*FreeUserTokenMUnit), modelMap["snap-light-free"]["tokens_used"])
	assert.Equal(t, true, modelMap["snap-exempt-free"]["exempt"])
}

// ==================== G. 越权防护：未鉴权直接访问 rate-limit-config ====================

// 关键词: TestE2E_RateLimitConfig_Unauthorized, 未鉴权访问限额配置应 401
func TestE2E_RateLimitConfig_Unauthorized(t *testing.T) {
	addr, _, _, cleanup := startDailyTokenTestServer(t, 407)
	defer cleanup()

	// 未携带 admin_session，GET/POST 都必须 401
	status1, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/rate-limit-config", nil, "")
	assert.Equal(t, http.StatusUnauthorized, status1)

	status2, _, _ := sendRawHTTPRequest(t, addr, "POST", "/portal/api/rate-limit-config", nil,
		`{"free_user_token_limit_m":1}`)
	assert.Equal(t, http.StatusUnauthorized, status2)

	status3, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/rate-limit-status", nil, "")
	assert.Equal(t, http.StatusUnauthorized, status3)
}

// ==================== H. 付费 API key Token 限额接入 ====================

// 关键词: TestE2E_PaidKey_TokenLimit_Enforced, 付费 key TokenLimit 接入 hot path
// 这是为了验证 server.go 中是否调用了 CheckAiApiKeyTokenLimit。
// 一旦该缺口被修复，付费 key 在 TokenUsed>=TokenLimit 时应该返回 429 traffic_limit_exceeded
// （沿用 traffic 维度的响应风格）。
func TestE2E_PaidKey_TokenLimit_Enforced(t *testing.T) {
	addr, cfg, _, cleanup := startDailyTokenTestServer(t, 408)
	defer cleanup()

	apiKey := "paid-token-test-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("api_key = ?", apiKey).Delete(&schema.AiApiKeys{})

	// 直接创建一条 token used 已超额的 key
	require.NoError(t, GetDB().Create(&schema.AiApiKeys{
		APIKey:           apiKey,
		Active:           true,
		TokenLimit:       1000,
		TokenUsed:        2000,
		TokenLimitEnable: true,
	}).Error)

	// 注入内存 Keys 表，模拟正常的 LoadKeysFromDatabase 后状态
	cfg.Keys.keys[apiKey] = &Key{
		Key:           apiKey,
		AllowedModels: map[string]bool{"paid-test": true},
	}
	cfg.KeyAllowedModels.allowedModels[apiKey] = map[string]bool{"paid-test": true}

	// 期望：付费 key 已超过 TokenLimit -> 429 token_limit_exceeded
	// 注意 paid-test 不是 -free 模型，走的是付费 key 鉴权 + Token 检查路径
	status, _, body := sendChatCompletionHTTP(t, addr, apiKey, "paid-test")
	assert.Equal(t, http.StatusTooManyRequests, status,
		"paid key with TokenUsed>=TokenLimit should be rejected with 429, got status=%d body=%s",
		status, body)
	assert.Contains(t, body, "token_limit_exceeded",
		"response should hint token_limit_exceeded; body=%s", body)
}

// ==================== I. 字节维度 traffic limit 与 Token 维度互不影响 ====================

// 关键词: TestE2E_PaidKey_TrafficLimit_StillWorks, 字节限额仍生效
func TestE2E_PaidKey_TrafficLimit_StillWorks(t *testing.T) {
	addr, cfg, _, cleanup := startDailyTokenTestServer(t, 409)
	defer cleanup()

	apiKey := "paid-traffic-test-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("api_key = ?", apiKey).Delete(&schema.AiApiKeys{})

	require.NoError(t, GetDB().Create(&schema.AiApiKeys{
		APIKey:             apiKey,
		Active:             true,
		TrafficLimit:       1024,
		TrafficUsed:        2048,
		TrafficLimitEnable: true,
		TokenLimit:         0,
		TokenLimitEnable:   false,
	}).Error)
	cfg.Keys.keys[apiKey] = &Key{
		Key:           apiKey,
		AllowedModels: map[string]bool{"paid-test": true},
	}
	cfg.KeyAllowedModels.allowedModels[apiKey] = map[string]bool{"paid-test": true}

	status, _, body := sendChatCompletionHTTP(t, addr, apiKey, "paid-test")
	assert.Equal(t, http.StatusTooManyRequests, status,
		"paid key with TrafficUsed>=TrafficLimit should be rejected; got %d body=%s", status, body)
	assert.Contains(t, body, "traffic_limit_exceeded")
}

// ==================== J. 默认 limit_m=1200 兜底 ====================

// 关键词: TestE2E_FreeUserTokenLimit_DefaultFallback, 0/缺省 limit 走 1200M 默认
func TestE2E_FreeUserTokenLimit_DefaultFallback(t *testing.T) {
	addr, _, _, cleanup := startDailyTokenTestServer(t, 410)
	defer cleanup()

	// 故意把 limit 设为 0（视作未配置），GetRateLimitConfig 应该兜底为 1200
	setRateLimitConfigForFreeTokenTest(t, 0, "{}")

	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	assert.Equal(t, int64(1200), cfg.FreeUserTokenLimitM, "0 should fallback to 1200")

	// 端到端：远小于 1200M 应该不会被 429（除非 provider 失败）
	status, headers, _ := sendChatCompletionHTTP(t, addr, "", "fallback-free")
	if status == http.StatusTooManyRequests {
		assert.NotEqual(t, "daily_token", headers["x-aibalance-limit-kind"],
			"should not 429 daily_token under default 1200M for tiny usage")
	}
}

// ==================== K. 模型独立桶超额返回 429 (bucket=model) ====================

// 关键词: TestE2E_FreeUser_ModelBucket_Exhausted_429, 模型独立桶超额 429
func TestE2E_FreeUser_ModelBucket_Exhausted_429(t *testing.T) {
	addr, _, _, cleanup := startDailyTokenTestServer(t, 411)
	defer cleanup()

	setRateLimitConfigForFreeTokenTest(t, 1200,
		`{"iso-bucket-free":{"limit_m":1,"exempt":false}}`)

	// 模型独立桶预累加到 1M（=limit）
	require.NoError(t, AddFreeUserDailyTokenUsage("iso-bucket-free", 1*FreeUserTokenMUnit, true))

	status, headers, body := sendChatCompletionHTTP(t, addr, "", "iso-bucket-free")
	assert.Equal(t, http.StatusTooManyRequests, status)
	assert.Equal(t, "daily_token", headers["x-aibalance-limit-kind"])

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))
	errObj := parsed["error"].(map[string]interface{})
	assert.Equal(t, "model", errObj["bucket"], "should mark bucket as model-level")
	assert.Equal(t, "iso-bucket-free", errObj["model"])
	assert.Equal(t, float64(1), errObj["tokens_limit_m"])
}

// ==================== L. AiApiKeys table query: 确保 schema 字段创建成功 ====================

// 关键词: TestE2E_AiApiKeys_TokenFields_PersistedCorrectly, schema 持久化检查
func TestE2E_AiApiKeys_TokenFields_PersistedCorrectly(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	require.NoError(t, GetDB().AutoMigrate(&schema.AiApiKeys{}).Error)

	apiKey := "schema-token-test-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("api_key = ?", apiKey).Delete(&schema.AiApiKeys{})

	require.NoError(t, GetDB().Create(&schema.AiApiKeys{
		APIKey:           apiKey,
		Active:           true,
		TokenLimit:       12345,
		TokenUsed:        678,
		TokenLimitEnable: true,
	}).Error)

	var k schema.AiApiKeys
	require.NoError(t, GetDB().Where("api_key = ?", apiKey).First(&k).Error)
	assert.Equal(t, int64(12345), k.TokenLimit)
	assert.Equal(t, int64(678), k.TokenUsed)
	assert.True(t, k.TokenLimitEnable)

	// Update -> Reset roundtrip
	require.NoError(t, UpdateAiApiKeyTokenLimit(k.ID, 55555, true))
	require.NoError(t, ResetAiApiKeyTokenUsed(k.ID))

	var k2 schema.AiApiKeys
	require.NoError(t, GetDB().Where("id = ?", k.ID).First(&k2).Error)
	assert.Equal(t, int64(55555), k2.TokenLimit)
	assert.Equal(t, int64(0), k2.TokenUsed)
	assert.True(t, k2.TokenLimitEnable)
}
