package aibalance

import (
	"bytes"
	"encoding/base64"
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
	"github.com/yaklang/yaklang/common/utils"
)

// 关键词: memfit_version_gate_test, memfit 客户端版本控流测试

// resetMemfitVersionGateConfig 把限流配置恢复成版本控流关闭、min build time 为空的状态，
// 避免跨用例污染。
func resetMemfitVersionGateConfig(t *testing.T) {
	t.Helper()
	require.NoError(t, EnsureRateLimitConfigTable())
	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	cfg.MemfitVersionGateEnabled = false
	cfg.MemfitVersionMinBuildTime = ""
	require.NoError(t, SaveRateLimitConfig(cfg))
}

func setMemfitVersionGate(t *testing.T, enabled bool, minBuildTime string) {
	t.Helper()
	require.NoError(t, EnsureRateLimitConfigTable())
	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	cfg.MemfitVersionGateEnabled = enabled
	cfg.MemfitVersionMinBuildTime = minBuildTime
	require.NoError(t, SaveRateLimitConfig(cfg))
}

// ==================== 一、checkMemfitVersionGate 单元测试 ====================

// 关键词: TestCheckMemfitVersionGate_Disabled, 控流关闭一律放行
func TestCheckMemfitVersionGate_Disabled(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	resetMemfitVersionGateConfig(t)
	defer resetMemfitVersionGateConfig(t)

	srv := NewServerConfig()
	defer srv.Close()

	cases := []struct {
		name    string
		version string
		bt      string
	}{
		{"empty version", "", ""},
		{"unknown version", "unknown", ""},
		{"dev version", "dev", ""},
		{"release version", "v1.0.0", ""},
		{"old buildtime", "v1.0.0", "2000-01-01T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := srv.checkMemfitVersionGate(tc.version, tc.bt)
			assert.False(t, res.Blocked, "with gate disabled, should never block: %+v", res)
		})
	}
}

// 关键词: TestCheckMemfitVersionGate_DevBypass, dev 版本始终放行
func TestCheckMemfitVersionGate_DevBypass(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	setMemfitVersionGate(t, true, "2099-01-01T00:00:00Z")
	defer resetMemfitVersionGateConfig(t)

	srv := NewServerConfig()
	defer srv.Close()

	devVariants := []string{"dev", "v1.2.3-dev", "DEV", "dev-build", "main-dev"}
	for _, v := range devVariants {
		t.Run(v, func(t *testing.T) {
			res := srv.checkMemfitVersionGate(v, "")
			assert.False(t, res.Blocked, "dev variant %q should bypass gate: %+v", v, res)
		})
	}
}

// 关键词: TestCheckMemfitVersionGate_MissingVersion, 未知版本拦截
func TestCheckMemfitVersionGate_MissingVersion(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	setMemfitVersionGate(t, true, "")
	defer resetMemfitVersionGateConfig(t)

	srv := NewServerConfig()
	defer srv.Close()

	for _, v := range []string{"", "unknown", "UNKNOWN", "  "} {
		t.Run("missing="+v, func(t *testing.T) {
			res := srv.checkMemfitVersionGate(v, "")
			assert.True(t, res.Blocked)
			assert.Equal(t, "missing_version", res.Reason)
		})
	}
}

// 关键词: TestCheckMemfitVersionGate_OutdatedBuildTime, 老 BuildTime 拦截
func TestCheckMemfitVersionGate_OutdatedBuildTime(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	minBT := "2025-06-01T00:00:00Z"
	setMemfitVersionGate(t, true, minBT)
	defer resetMemfitVersionGateConfig(t)

	srv := NewServerConfig()
	defer srv.Close()

	t.Run("old buildtime", func(t *testing.T) {
		res := srv.checkMemfitVersionGate("v1.0.0", "2025-01-01T00:00:00Z")
		assert.True(t, res.Blocked)
		assert.Equal(t, "outdated_buildtime", res.Reason)
		assert.Equal(t, minBT, res.MinBuildTime)
	})

	t.Run("new buildtime", func(t *testing.T) {
		res := srv.checkMemfitVersionGate("v1.0.0", "2025-12-01T00:00:00Z")
		assert.False(t, res.Blocked)
	})

	t.Run("equal buildtime", func(t *testing.T) {
		res := srv.checkMemfitVersionGate("v1.0.0", "2025-06-01T00:00:00Z")
		assert.False(t, res.Blocked, "equal-to-min should be allowed")
	})

	t.Run("buildtime missing when min set", func(t *testing.T) {
		res := srv.checkMemfitVersionGate("v1.0.0", "")
		assert.True(t, res.Blocked)
		assert.Equal(t, "missing_version", res.Reason)
	})

	t.Run("unparsable buildtime treated as outdated", func(t *testing.T) {
		res := srv.checkMemfitVersionGate("v1.0.0", "not-a-time")
		assert.True(t, res.Blocked)
		assert.Equal(t, "outdated_buildtime", res.Reason)
	})
}

// 关键词: TestCheckMemfitVersionGate_BadMinConfigPassThrough,
// 后台配置错误的 MinBuildTime 不应阻塞业务（降级放行 + 日志告警）
func TestCheckMemfitVersionGate_BadMinConfigPassThrough(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	setMemfitVersionGate(t, true, "not-a-valid-time-format")
	defer resetMemfitVersionGateConfig(t)

	srv := NewServerConfig()
	defer srv.Close()

	res := srv.checkMemfitVersionGate("v1.0.0", "2025-12-01T00:00:00Z")
	assert.False(t, res.Blocked, "bad MinBuildTime should pass-through, not block: %+v", res)
}

// ==================== 二、writeMemfitVersionRateLimitResponse 单元测试 ====================

// 关键词: TestWriteMemfitVersionRateLimitResponse_Format, memfit 版本控流 429 响应格式
func TestWriteMemfitVersionRateLimitResponse_Format(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.writeMemfitVersionRateLimitResponse(server, "outdated_buildtime")
		server.Close()
	}()

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
	resp := string(result)

	assert.Contains(t, resp, "HTTP/1.1 429 Too Many Requests")
	assert.Contains(t, resp, "X-AIBalance-Limit-Kind: memfit_client_version")
	assert.Contains(t, resp, "X-AIBalance-Memfit-Version-Reason: outdated_buildtime")

	bodyIdx := strings.Index(resp, "\r\n\r\n")
	require.Greater(t, bodyIdx, 0)
	body := resp[bodyIdx+4:]

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))
	errObj, ok := parsed["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "memfit_client_version_limited", errObj["type"])
	assert.Equal(t, "memfit_client_version", errObj["limit_kind"])
	assert.Equal(t, "outdated_buildtime", errObj["reason"])

	msg, _ := errObj["message"].(string)
	// 文案需要突出旧版本/请更新提示，按 PRD 写死
	assert.Contains(t, msg, "Memfit/Yak")
	assert.Contains(t, msg, "最大上限")
	assert.Contains(t, msg, "1 亿")
	assert.Contains(t, msg, "更新")
}

// 关键词: TestWriteMemfitVersionRateLimitResponse_DefaultReason, reason 为空时兜底 unknown
func TestWriteMemfitVersionRateLimitResponse_DefaultReason(t *testing.T) {
	cfg := NewServerConfig()
	defer cfg.Close()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		cfg.writeMemfitVersionRateLimitResponse(server, "")
		server.Close()
	}()

	var result []byte
	buf := make([]byte, 4096)
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
	resp := string(result)
	assert.Contains(t, resp, "X-AIBalance-Memfit-Version-Reason: unknown")
}

// ==================== 三、RecordClientVersion / QueryTopClientVersions ====================

// 关键词: TestRecordClientVersion_Upsert, 版本统计 upsert: count++ first_seen 不变 last_seen 更新
func TestRecordClientVersion_Upsert(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	require.NoError(t, EnsureClientVersionStatTable())

	// 用一个独一无二的 version 避免污染
	ver := "mvg-test-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("version = ?", ver).Delete(&AiBalanceClientVersionStat{})

	require.NoError(t, RecordClientVersion(ver, "2025-06-01T00:00:00Z"))

	var row1 AiBalanceClientVersionStat
	require.NoError(t, GetDB().Where("version = ?", ver).First(&row1).Error)
	require.Equal(t, int64(1), row1.RequestCount)
	require.NotZero(t, row1.FirstSeenUnix)
	require.Equal(t, row1.FirstSeenUnix, row1.LastSeenUnix)
	require.Equal(t, "2025-06-01T00:00:00Z", row1.BuildTime)

	// 间隔 >=1 秒，确保 last_seen 单位（秒）真的能往后走
	time.Sleep(1100 * time.Millisecond)

	require.NoError(t, RecordClientVersion(ver, "2025-07-01T00:00:00Z"))

	var row2 AiBalanceClientVersionStat
	require.NoError(t, GetDB().Where("version = ?", ver).First(&row2).Error)
	assert.Equal(t, int64(2), row2.RequestCount, "request_count should increment")
	assert.Equal(t, row1.FirstSeenUnix, row2.FirstSeenUnix, "first_seen should not change")
	assert.GreaterOrEqual(t, row2.LastSeenUnix, row1.LastSeenUnix+1, "last_seen should advance")
	assert.Equal(t, "2025-07-01T00:00:00Z", row2.BuildTime, "build_time should be updated to latest report")
}

// 关键词: TestRecordClientVersion_EmptyVersion, 空 version 兜底为 unknown
func TestRecordClientVersion_EmptyVersion(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	require.NoError(t, EnsureClientVersionStatTable())

	// 清理 unknown 行（仅本测试时段）
	defer GetDB().Unscoped().Where("version = ?", "unknown").Delete(&AiBalanceClientVersionStat{})
	_ = GetDB().Unscoped().Where("version = ?", "unknown").Delete(&AiBalanceClientVersionStat{}).Error

	require.NoError(t, RecordClientVersion("", ""))
	require.NoError(t, RecordClientVersion("   ", ""))

	var row AiBalanceClientVersionStat
	require.NoError(t, GetDB().Where("version = ?", "unknown").First(&row).Error)
	assert.Equal(t, int64(2), row.RequestCount)
}

// 关键词: TestQueryTopClientVersions_Order, 排序按 last_seen DESC, request_count DESC
func TestQueryTopClientVersions_Order(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	require.NoError(t, EnsureClientVersionStatTable())

	now := time.Now().Unix()
	tag := fmt.Sprintf("topq-%d-", now)
	defer GetDB().Unscoped().Where("version LIKE ?", tag+"%").Delete(&AiBalanceClientVersionStat{})

	versions := []AiBalanceClientVersionStat{
		{Version: tag + "older", BuildTime: "", FirstSeenUnix: now - 1000, LastSeenUnix: now - 1000, RequestCount: 100},
		{Version: tag + "newer1", BuildTime: "", FirstSeenUnix: now - 500, LastSeenUnix: now - 1, RequestCount: 2},
		{Version: tag + "newer2", BuildTime: "", FirstSeenUnix: now - 500, LastSeenUnix: now - 1, RequestCount: 5},
		{Version: tag + "latest", BuildTime: "", FirstSeenUnix: now, LastSeenUnix: now, RequestCount: 1},
	}
	for i := range versions {
		require.NoError(t, GetDB().Create(&versions[i]).Error)
	}

	items, err := QueryTopClientVersions(10)
	require.NoError(t, err)

	// 仅检查我们注入的这批顺序; 整体可能掺杂其它测试数据
	myOrder := []string{}
	want := []string{tag + "latest", tag + "newer2", tag + "newer1", tag + "older"}
	wantSet := map[string]bool{}
	for _, w := range want {
		wantSet[w] = true
	}
	for _, it := range items {
		if wantSet[it.Version] {
			myOrder = append(myOrder, it.Version)
		}
	}
	assert.Equal(t, want, myOrder, "expected sort: last_seen DESC, request_count DESC; got %v", myOrder)
}

// 关键词: TestQueryTopClientVersions_Limit, limit 钳制
func TestQueryTopClientVersions_Limit(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	require.NoError(t, EnsureClientVersionStatTable())

	// limit <= 0 -> 20
	items, err := QueryTopClientVersions(0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(items), 20)

	// limit > 200 -> 200
	items2, err := QueryTopClientVersions(99999)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(items2), 200)
}

// ==================== 四、e2e：memfit-* + 缺失版本头 -> 429 ====================

// startMemfitGateTestServer 启动一个 ServerConfig 实例供 e2e 用，开启 memfit 版本控流并把
// TOTP 初始化好，让我们可以用合法 TOTP 头通过 TOTP 校验、专门测到版本控流逻辑。
func startMemfitGateTestServer(t *testing.T) (string, *ServerConfig, func()) {
	t.Helper()

	consts.InitializeYakitDatabase("", "", "")
	require.NoError(t, EnsureRateLimitConfigTable())
	require.NoError(t, EnsureClientVersionStatTable())
	require.NoError(t, InitMemfitTOTP())

	cfg := NewServerConfig()
	cfg.AdminPassword = "memfit-gate-admin-secret"
	cfg.AuthMiddleware = NewAuthMiddleware(cfg, DefaultAuthConfig())
	cfg.freeUserDelayMinSec = 0
	cfg.freeUserDelayMaxSec = 0
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
		resetMemfitVersionGateConfig(t)
		cfg.Close()
	}
	return addr, cfg, cleanup
}

// 发起一次 memfit-* chat 请求，可控加上 X-Yak-Version / X-Yak-Build-Time
func sendMemfitChatRequest(t *testing.T, addr, model, version, buildTime string) (int, map[string]string, string) {
	t.Helper()

	totpCode := GetCurrentTOTPCode()
	require.NotEmpty(t, totpCode, "TOTP code must be available for memfit model test")
	totpHeader := base64.StdEncoding.EncodeToString([]byte(totpCode))

	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"stream":false}`, model)

	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	var reqBuf bytes.Buffer
	reqBuf.WriteString("POST /v1/chat/completions HTTP/1.1\r\n")
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", addr))
	reqBuf.WriteString("Authorization: Bearer dummy-key\r\n")
	reqBuf.WriteString(fmt.Sprintf("X-Memfit-OTP-Auth: %s\r\n", totpHeader))
	if version != "" {
		reqBuf.WriteString(fmt.Sprintf("X-Yak-Version: %s\r\n", version))
	}
	if buildTime != "" {
		reqBuf.WriteString(fmt.Sprintf("X-Yak-Build-Time: %s\r\n", buildTime))
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
	return parseHTTPResponse(respBuf.String())
}

// 关键词: TestE2E_MemfitVersionGate_MissingHeader_429, 缺头被拦截
func TestE2E_MemfitVersionGate_MissingHeader_429(t *testing.T) {
	addr, _, cleanup := startMemfitGateTestServer(t)
	defer cleanup()

	setMemfitVersionGate(t, true, "")

	status, headers, body := sendMemfitChatRequest(t, addr, "memfit-standard-test-noheader", "", "")
	assert.Equal(t, http.StatusTooManyRequests, status, body)
	assert.Equal(t, "memfit_client_version", headers["x-aibalance-limit-kind"])
	assert.Equal(t, "missing_version", headers["x-aibalance-memfit-version-reason"])

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))
	errObj := parsed["error"].(map[string]interface{})
	assert.Equal(t, "memfit_client_version_limited", errObj["type"])
}

// 关键词: TestE2E_MemfitVersionGate_DevPasses, dev 版本即便开关开启也放行
//
// 这里 dev 版本通过 gate 后会继续进入 chat 主路径，因没配置任何 provider 会返回 5xx；
// 我们只断言「不会被版本控流的 429 拦截」即可。
func TestE2E_MemfitVersionGate_DevPasses(t *testing.T) {
	addr, _, cleanup := startMemfitGateTestServer(t)
	defer cleanup()

	setMemfitVersionGate(t, true, "2099-01-01T00:00:00Z")

	status, headers, body := sendMemfitChatRequest(t, addr, "memfit-standard-test-dev", "dev", "")
	if status == http.StatusTooManyRequests {
		assert.NotEqual(t, "memfit_client_version", headers["x-aibalance-limit-kind"],
			"dev should bypass memfit version gate, body=%s", body)
	}
}

// 关键词: TestE2E_MemfitVersionGate_OldBuildTime_429, 老 BuildTime 被拦截
func TestE2E_MemfitVersionGate_OldBuildTime_429(t *testing.T) {
	addr, _, cleanup := startMemfitGateTestServer(t)
	defer cleanup()

	setMemfitVersionGate(t, true, "2025-06-01T00:00:00Z")

	status, headers, body := sendMemfitChatRequest(t, addr,
		"memfit-standard-test-oldbt", "v1.0.0", "2025-01-01T00:00:00Z")
	assert.Equal(t, http.StatusTooManyRequests, status, body)
	assert.Equal(t, "memfit_client_version", headers["x-aibalance-limit-kind"])
	assert.Equal(t, "outdated_buildtime", headers["x-aibalance-memfit-version-reason"])
}

// 关键词: TestE2E_MemfitVersionGate_NewBuildTime_NotBlocked, 新 BuildTime 不被本控流拦截
func TestE2E_MemfitVersionGate_NewBuildTime_NotBlocked(t *testing.T) {
	addr, _, cleanup := startMemfitGateTestServer(t)
	defer cleanup()

	setMemfitVersionGate(t, true, "2025-06-01T00:00:00Z")

	status, headers, body := sendMemfitChatRequest(t, addr,
		"memfit-standard-test-newbt", "v9.9.9", "2099-01-01T00:00:00Z")
	if status == http.StatusTooManyRequests {
		assert.NotEqual(t, "memfit_client_version", headers["x-aibalance-limit-kind"],
			"new BuildTime should NOT be blocked by memfit version gate, body=%s", body)
	}
}

// ==================== 五、portal GET /portal/api/client-version-stats ====================

// 关键词: TestE2E_Portal_ClientVersionStats_Roundtrip, portal 接口端到端
func TestE2E_Portal_ClientVersionStats_Roundtrip(t *testing.T) {
	addr, _, cleanup := startMemfitGateTestServer(t)
	defer cleanup()

	// 预先在 DB 里写一条统计行，确保接口能返回非空
	versionTag := "portal-rt-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("version = ?", versionTag).Delete(&AiBalanceClientVersionStat{})
	require.NoError(t, RecordClientVersion(versionTag, "2025-08-01T00:00:00Z"))

	// 未登录 -> 401
	status401, _, _ := sendRawHTTPRequest(t, addr, "GET", "/portal/api/client-version-stats?limit=20", nil, "")
	assert.Equal(t, http.StatusUnauthorized, status401)

	// 登录拿 cookie
	cookieValue := loginAndGetSession(t, addr, "memfit-gate-admin-secret")
	require.NotEmpty(t, cookieValue, "should obtain admin_session cookie")
	cookieHeader := map[string]string{"Cookie": "admin_session=" + cookieValue}

	status200, headers, body := sendRawHTTPRequest(t, addr, "GET",
		"/portal/api/client-version-stats?limit=20", cookieHeader, "")
	require.Equal(t, http.StatusOK, status200, body)
	assert.Contains(t, headers["content-type"], "application/json")

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))
	assert.Equal(t, true, parsed["success"])
	items, ok := parsed["items"].([]interface{})
	require.True(t, ok)
	require.GreaterOrEqual(t, len(items), 1)

	found := false
	for _, it := range items {
		m := it.(map[string]interface{})
		if m["version"] == versionTag {
			found = true
			assert.Equal(t, "2025-08-01T00:00:00Z", m["build_time"])
			assert.Equal(t, float64(1), m["request_count"])
			assert.NotEmpty(t, m["first_seen_text"])
			assert.NotEmpty(t, m["last_seen_text"])
		}
	}
	assert.True(t, found, "expect to find newly recorded version in items")
}

// 关键词: TestE2E_Portal_RateLimitConfig_MemfitGateRoundtrip, portal 配置 memfit gate 字段读写
func TestE2E_Portal_RateLimitConfig_MemfitGateRoundtrip(t *testing.T) {
	addr, _, cleanup := startMemfitGateTestServer(t)
	defer cleanup()

	cookieValue := loginAndGetSession(t, addr, "memfit-gate-admin-secret")
	require.NotEmpty(t, cookieValue)
	cookieHeader := map[string]string{"Cookie": "admin_session=" + cookieValue}

	// POST 修改
	payload := `{"memfit_version_gate_enabled":true,"memfit_version_min_build_time":"2025-06-01T00:00:00Z"}`
	statusPost, _, body := sendRawHTTPRequest(t, addr, "POST",
		"/portal/api/rate-limit-config", cookieHeader, payload)
	require.Equal(t, http.StatusOK, statusPost, body)

	// GET 读回
	statusGet, _, bodyGet := sendRawHTTPRequest(t, addr, "GET",
		"/portal/api/rate-limit-config", cookieHeader, "")
	require.Equal(t, http.StatusOK, statusGet, bodyGet)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(bodyGet), &parsed))
	cfgMap := parsed["config"].(map[string]interface{})
	assert.Equal(t, true, cfgMap["memfit_version_gate_enabled"])
	assert.Equal(t, "2025-06-01T00:00:00Z", cfgMap["memfit_version_min_build_time"])

	// 关掉
	statusPost2, _, _ := sendRawHTTPRequest(t, addr, "POST",
		"/portal/api/rate-limit-config", cookieHeader,
		`{"memfit_version_gate_enabled":false,"memfit_version_min_build_time":""}`)
	require.Equal(t, http.StatusOK, statusPost2)

	statusGet2, _, bodyGet2 := sendRawHTTPRequest(t, addr, "GET",
		"/portal/api/rate-limit-config", cookieHeader, "")
	require.Equal(t, http.StatusOK, statusGet2)

	var parsed2 map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(bodyGet2), &parsed2))
	cfgMap2 := parsed2["config"].(map[string]interface{})
	assert.Equal(t, false, cfgMap2["memfit_version_gate_enabled"])
	assert.Equal(t, "", cfgMap2["memfit_version_min_build_time"])
}
