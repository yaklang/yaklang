package aibalance

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// ==================== SessionManager.RefreshSession ====================

// TestSessionManager_RefreshSession_Success 验证 RefreshSession 能把未过期的
// session 顺延 SessionLifetime。要点：
//   - 创建 session 后人工把 ExpiresAt 改成接近现在的值（仍未过期），方便观察延长效果。
//   - 续期后 DB 中的 ExpiresAt 必须明显晚于原值，且与 now+SessionLifetime 接近。
//
// 关键词: RefreshSession 续期成功 单测
func TestSessionManager_RefreshSession_Success(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	sm := NewSessionManager()

	sessionID := sm.CreateSession()
	require.NotEmpty(t, sessionID, "create session should succeed")
	defer sm.DeleteSession(sessionID)

	// 把 ExpiresAt 调小到 5 分钟后，便于断言"被显著延长"。
	original := time.Now().Add(5 * time.Minute)
	require.NoError(t,
		GetDB().Model(&schema.LoginSession{}).
			Where("session_id = ?", sessionID).
			Update("expires_at", original).Error,
		"set up original expires_at",
	)

	got := sm.RefreshSession(sessionID)
	require.NotNil(t, got, "refresh should succeed on a non-expired session")

	expectedLowerBound := time.Now().Add(SessionLifetime - 30*time.Second)
	expectedUpperBound := time.Now().Add(SessionLifetime + 30*time.Second)
	assert.True(t, got.ExpiresAt.After(expectedLowerBound),
		"new expires_at %s should be >= now + (SessionLifetime - 30s)", got.ExpiresAt)
	assert.True(t, got.ExpiresAt.Before(expectedUpperBound),
		"new expires_at %s should be <= now + (SessionLifetime + 30s)", got.ExpiresAt)
	assert.True(t, got.ExpiresAt.After(original),
		"new expires_at %s should be strictly later than original %s",
		got.ExpiresAt, original)

	// 同步落盘验证。
	var reloaded schema.LoginSession
	require.NoError(t,
		GetDB().Where("session_id = ?", sessionID).First(&reloaded).Error)
	assert.True(t, reloaded.ExpiresAt.After(original),
		"DB row expires_at should be updated")
}

// TestSessionManager_RefreshSession_AlreadyExpired 验证已过期的 session 不能被
// 续回来，必须重新登录，避免被反复续到一个本应失效的会话上。
// 关键词: RefreshSession expired 拒绝续期
func TestSessionManager_RefreshSession_AlreadyExpired(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	sm := NewSessionManager()

	sessionID := sm.CreateSession()
	require.NotEmpty(t, sessionID)

	// 将 ExpiresAt 直接改成过去。
	require.NoError(t,
		GetDB().Model(&schema.LoginSession{}).
			Where("session_id = ?", sessionID).
			Update("expires_at", time.Now().Add(-1*time.Hour)).Error)

	got := sm.RefreshSession(sessionID)
	assert.Nil(t, got, "expired session must not be refreshable")
}

// TestSessionManager_RefreshSession_UnknownID 未知 session id 返回 nil 即可，
// 不应 panic 也不应误创建。
// 关键词: RefreshSession 未知 session id
func TestSessionManager_RefreshSession_UnknownID(t *testing.T) {
	consts.InitializeYakitDatabase("", "", "")
	sm := NewSessionManager()
	assert.Nil(t, sm.RefreshSession("non-existent-session-id"))
	assert.Nil(t, sm.RefreshSession(""))
}

// ==================== HTTP endpoint ====================

// TestPortalSessionRefresh_RequiresAuth /portal/api/session/refresh 必须像
// 其他 portal API 一样在缺少 cookie 时返回 401，避免任意人触发续期接口。
// 关键词: portal session refresh 401 auth required
func TestPortalSessionRefresh_RequiresAuth(t *testing.T) {
	addr, _ := startTestPortalServer(t)
	statusCode, _, body := sendRawHTTPRequest(t, addr, "POST",
		"/portal/api/session/refresh", nil, "{}")
	assert.Equal(t, http.StatusUnauthorized, statusCode,
		"unauthenticated refresh must return 401, body=%s", body)
}

// TestOpsSessionRefresh_RequiresAuth /ops/api/session/refresh 同理需要 401。
// 关键词: ops session refresh 401 auth required
func TestOpsSessionRefresh_RequiresAuth(t *testing.T) {
	addr, _ := startTestPortalServer(t)
	statusCode, _, body := sendRawHTTPRequest(t, addr, "POST",
		"/ops/api/session/refresh", nil, "{}")
	assert.Equal(t, http.StatusUnauthorized, statusCode,
		"unauthenticated ops refresh must return 401, body=%s", body)
}

// TestPortalSessionRefresh_ExtendsExpiry 端到端验证：
//  1. 用 admin 密码登录拿到 session cookie。
//  2. 把 DB 中该 session 的 ExpiresAt 设为接近现在的时间（仍未过期）。
//  3. POST /portal/api/session/refresh 带 cookie。
//  4. 返回 200 + success=true + expires_in_seconds 大致等于 SessionLifetime。
//  5. DB 中的 ExpiresAt 明显被延长。
//
// 关键词: portal session refresh 端到端 extend expires_at
func TestPortalSessionRefresh_ExtendsExpiry(t *testing.T) {
	addr, cfg := startTestPortalServer(t)

	session := loginAndGetSession(t, addr, cfg.AdminPassword)
	require.NotEmpty(t, session, "should get a valid session after login")

	original := time.Now().Add(2 * time.Minute)
	require.NoError(t,
		GetDB().Model(&schema.LoginSession{}).
			Where("session_id = ?", session).
			Update("expires_at", original).Error)

	statusCode, headers, body := sendRawHTTPRequest(t, addr, "POST",
		"/portal/api/session/refresh",
		map[string]string{"Cookie": "admin_session=" + session},
		"{}")
	require.Equal(t, http.StatusOK, statusCode,
		"refresh should return 200, headers=%v body=%s", headers, body)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &parsed),
		"response body should be valid JSON, body=%s", body)
	assert.Equal(t, true, parsed["success"], "response should report success=true")
	assert.Equal(t, true, parsed["refreshed"],
		"cookie-based session should be refreshed=true")

	expiresIn, ok := parsed["expires_in_seconds"].(float64)
	require.True(t, ok, "expires_in_seconds should be a number, got %v", parsed["expires_in_seconds"])
	lifetimeSec := float64(SessionLifetime / time.Second)
	assert.InDelta(t, lifetimeSec, expiresIn, 60.0,
		"expires_in_seconds should be close to SessionLifetime (%.0fs), got %.0f",
		lifetimeSec, expiresIn)

	var reloaded schema.LoginSession
	require.NoError(t,
		GetDB().Where("session_id = ?", session).First(&reloaded).Error)
	assert.True(t, reloaded.ExpiresAt.After(original.Add(5*time.Minute)),
		"DB row expires_at should be extended well beyond original, got %s vs original %s",
		reloaded.ExpiresAt, original)
}

// TestPortalSessionRefresh_RejectsExpired 已经过期的 session（即便客户端
// 还揣着 cookie）必须返回 401，让前端 authFetch 走统一登出流程。
// 关键词: portal session refresh expired 401
func TestPortalSessionRefresh_RejectsExpired(t *testing.T) {
	addr, cfg := startTestPortalServer(t)

	session := loginAndGetSession(t, addr, cfg.AdminPassword)
	require.NotEmpty(t, session)

	require.NoError(t,
		GetDB().Model(&schema.LoginSession{}).
			Where("session_id = ?", session).
			Update("expires_at", time.Now().Add(-1*time.Hour)).Error)

	statusCode, _, body := sendRawHTTPRequest(t, addr, "POST",
		"/portal/api/session/refresh",
		map[string]string{"Cookie": "admin_session=" + session},
		"{}")
	assert.Equal(t, http.StatusUnauthorized, statusCode,
		"expired session must not be refreshable, body=%s", body)
}

// TestSessionLifetimeConstant 是一个回归测试：万一有人把 SessionLifetime
// 改成奇怪的值（比如 30 秒），这里能立刻报警。前后端约定 30 分钟。
// 关键词: SessionLifetime constant 30 分钟 回归
func TestSessionLifetimeConstant(t *testing.T) {
	assert.Equal(t, 30*time.Minute, SessionLifetime,
		"前端按 30 分钟有效期 / 每 10 分钟续期一次实现，"+
			"若需要修改这里的常量，请同步检查 portal.js 与 ops_portal.js "+
			"中的 SESSION_REFRESH_INTERVAL_MS（应小于 SessionLifetime）。")

	// 防止后端续期周期被设得比前端轮询还短，导致前端来不及续期。
	const frontendRefreshInterval = 10 * time.Minute
	require.True(t, SessionLifetime > frontendRefreshInterval,
		"SessionLifetime (%v) must be strictly greater than frontend refresh interval (%v)",
		SessionLifetime, frontendRefreshInterval)
}

// 防御性：如果将来有人把 SessionLifetime 改成 0 或负数，这里能拦住。
// 关键词: SessionLifetime defensive guard
func TestSessionLifetime_NonZeroAndPositive(t *testing.T) {
	require.Greater(t, int64(SessionLifetime), int64(0),
		"SessionLifetime must be a positive duration, got %v", SessionLifetime)
	// 防止误改成 30 秒之类的 30 倍数小单位。
	require.GreaterOrEqual(t, SessionLifetime, time.Minute,
		"SessionLifetime should be at least 1 minute, got %v", SessionLifetime)
}

// portalSessionRefreshSmoke 是 sanity check，确保整条链 (handler 路由 → checkAuth
// → RefreshSession → JSON) 至少不会 panic 或返回 5xx。
// 关键词: session refresh smoke 不报 5xx
func TestPortalSessionRefresh_NoServerError(t *testing.T) {
	addr, cfg := startTestPortalServer(t)
	session := loginAndGetSession(t, addr, cfg.AdminPassword)
	require.NotEmpty(t, session)

	statusCode, _, body := sendRawHTTPRequest(t, addr, "POST",
		"/portal/api/session/refresh",
		map[string]string{"Cookie": "admin_session=" + session},
		"{}")
	require.NotEqual(t, http.StatusInternalServerError, statusCode,
		"refresh should never return 500, body=%s", body)
	require.NotEqual(t, http.StatusBadGateway, statusCode,
		"refresh should never return 502, body=%s", body)

	// 顺便确认 body 是合法 JSON，便于前端解析。
	var anyJSON map[string]interface{}
	assert.NoError(t, json.Unmarshal([]byte(body), &anyJSON),
		"refresh response should be valid JSON, body=%s", body)
}
