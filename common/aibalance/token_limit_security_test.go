package aibalance

// Token-limit (推荐 API Key 限额维度) 越权防护与功能集成测试。
//
// 测试覆盖：
//   1. Portal admin 端 /portal/api-key-token-limit/{id}, /portal/reset-api-key-token/{id}
//      - 携带 admin_session 时，能设置/重置 Token 限额；
//      - 无 cookie 时，必须返回 401（已在 portal_auth_security_test.go 中覆盖未授权用例）；
//   2. OPS 端 /ops/api/reset-token
//      - OPS 用户 A 能重置自己创建的 key 的 token；
//      - OPS 用户 A 不能重置 OPS 用户 B 创建的 key 的 token（403）；
//   3. OPS 端 /ops/api/update-api-key 的 token 字段
//      - OPS 用户能更新自己 key 的 token_limit；
//      - OPS 用户 A 不能更新 OPS 用户 B 的 key 的 token_limit（403）。
//
// 关键词: token_limit_security_test, API Key Token 限额越权防护, OPS 用户隔离

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// opsLoginGetSession is a test helper that performs OPS login and returns
// the freshly issued ops_session cookie value.
// 关键词: OPS 测试登录, ops_session cookie 提取
func opsLoginGetSession(t *testing.T, addr, username, plainPassword string) string {
	t.Helper()
	formBody := fmt.Sprintf("username=%s&password=%s", username, plainPassword)
	status, rawResp := rawHTTPRoundtrip(t, addr, "POST", "/ops/login",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		formBody)
	require.Equal(t, http.StatusSeeOther, status,
		"OPS login should succeed (303), got %d, raw=%s", status, rawResp)
	sess := extractSetCookie(rawResp, "ops_session")
	require.NotEmpty(t, sess, "ops_session cookie must be issued, raw=%s", rawResp)
	return sess
}

// createOpsUserWithPlain returns the user record together with the plaintext
// password so test code can authenticate.
func createOpsUserWithPlain(t *testing.T, prefix string) (*schema.OpsUser, string) {
	t.Helper()
	plain := fmt.Sprintf("ops-test-pwd-%d", time.Now().UnixNano())
	hashed, err := HashPassword(plain)
	require.NoError(t, err)
	user := &schema.OpsUser{
		Username:     fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()),
		Password:     hashed,
		OpsKey:       GenerateOpsKey(),
		Role:         "ops",
		Active:       true,
		DefaultLimit: 52428800,
	}
	require.NoError(t, SaveOpsUser(user))
	t.Cleanup(func() { DeleteOpsUser(user.ID) })
	return user, plain
}

func createApiKeyOwnedByOps(t *testing.T, ops *schema.OpsUser, apiKeySuffix string) *schema.AiApiKeys {
	t.Helper()
	db := GetDB()
	require.NotNil(t, db)
	apiKey := &schema.AiApiKeys{
		APIKey:           fmt.Sprintf("mf-token-sec-%s-%d", apiKeySuffix, time.Now().UnixNano()),
		AllowedModels:    "gpt-3.5-turbo",
		Active:           true,
		CreatedByOpsID:   ops.ID,
		CreatedByOpsName: ops.Username,
		// 故意预置一些 Token 使用，方便观察 reset。
		TokenUsed:        12345,
		TokenLimit:       0,
		TokenLimitEnable: false,
	}
	require.NoError(t, db.Create(apiKey).Error)
	t.Cleanup(func() { db.Delete(apiKey) })
	return apiKey
}

// TestPortalTokenLimitEndpoints_AdminCanSetAndReset 验证 Portal 管理员端点的正向链路：
//   - POST /portal/api-key-token-limit/{id} 写入 token_limit + token_limit_enable；
//   - POST /portal/reset-api-key-token/{id} 把 TokenUsed 清零。
func TestPortalTokenLimitEndpoints_AdminCanSetAndReset(t *testing.T) {
	addr, config := startTestPortalServer(t)
	setupTestDBForOps(t)

	adminSession := loginAndGetSession(t, addr, config.AdminPassword)
	require.NotEmpty(t, adminSession, "admin login must yield session cookie")

	// 通过 ops 用户作为载体，复用 createApiKeyOwnedByOps 帮助方法快速建一条 Key。
	carrier, _ := createOpsUserWithPlain(t, "portal-carrier")
	apiKey := createApiKeyOwnedByOps(t, carrier, "portal-admin")

	authHeader := map[string]string{
		"Cookie": "admin_session=" + adminSession,
	}

	t.Run("admin set token_limit ok", func(t *testing.T) {
		// 关键词: portal admin set token_limit happy path
		body := `{"token_limit": 5000000, "enable": true}`
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST",
			fmt.Sprintf("/portal/api-key-token-limit/%d", apiKey.ID),
			authHeader, body)
		require.Equal(t, http.StatusOK, status, "admin should be allowed, body=%s", respBody)
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(respBody), &parsed))
		assert.Equal(t, true, parsed["success"], "success flag")

		// Verify DB state
		var reloaded schema.AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, apiKey.ID).Error)
		assert.Equal(t, int64(5000000), reloaded.TokenLimit)
		assert.True(t, reloaded.TokenLimitEnable)
	})

	t.Run("admin reset token used ok", func(t *testing.T) {
		// 先确保 TokenUsed > 0
		require.NoError(t, GetDB().Model(&schema.AiApiKeys{}).Where("id = ?", apiKey.ID).
			Update("token_used", 999999).Error)

		status, _, respBody := sendRawHTTPRequest(t, addr, "POST",
			fmt.Sprintf("/portal/reset-api-key-token/%d", apiKey.ID),
			authHeader, "{}")
		require.Equal(t, http.StatusOK, status, "admin should be allowed, body=%s", respBody)

		var reloaded schema.AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, apiKey.ID).Error)
		assert.Equal(t, int64(0), reloaded.TokenUsed, "TokenUsed should be reset to 0")
	})

	t.Run("negative token limit clamps to zero", func(t *testing.T) {
		// 关键词: token_limit 负数 clamp to 0, 防御性
		body := `{"token_limit": -1, "enable": false}`
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST",
			fmt.Sprintf("/portal/api-key-token-limit/%d", apiKey.ID),
			authHeader, body)
		require.Equal(t, http.StatusOK, status, "should still succeed, body=%s", respBody)

		var reloaded schema.AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, apiKey.ID).Error)
		assert.Equal(t, int64(0), reloaded.TokenLimit, "negative should clamp to 0")
		assert.False(t, reloaded.TokenLimitEnable)
	})

	t.Run("non existent api key id returns 404", func(t *testing.T) {
		body := `{"token_limit": 100, "enable": true}`
		// 取一个不可能存在但仍在 uint32 范围内的 ID（4_000_000_000 < 2^32），
		// 避免命中 ParseUint 上溢导致 400 而非 404 的边界情况。
		// 关键词: api-key-token-limit 不存在 ID 返回 404, uint32 范围
		status, _, _ := sendRawHTTPRequest(t, addr, "POST",
			"/portal/api-key-token-limit/4000000000", authHeader, body)
		assert.Equal(t, http.StatusNotFound, status)
	})
}

// TestOpsResetToken_OwnerOnly 验证 OPS reset-token 接口的越权防护：
//   - 用户A 重置自己的 key -> 200，TokenUsed=0；
//   - 用户A 重置用户B 的 key -> 403。
func TestOpsResetToken_OwnerOnly(t *testing.T) {
	addr, _ := startTestPortalServer(t)
	setupTestDBForOps(t)

	userA, plainA := createOpsUserWithPlain(t, "ops-A")
	userB, _ := createOpsUserWithPlain(t, "ops-B")
	keyA := createApiKeyOwnedByOps(t, userA, "owned-by-A")
	keyB := createApiKeyOwnedByOps(t, userB, "owned-by-B")

	sessA := opsLoginGetSession(t, addr, userA.Username, plainA)

	authA := map[string]string{"Cookie": "ops_session=" + sessA}

	t.Run("ops A can reset own key", func(t *testing.T) {
		// 关键词: OPS reset-token 本人 happy path
		body := fmt.Sprintf(`{"api_key": "%s"}`, keyA.APIKey)
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST",
			"/ops/api/reset-token", authA, body)
		require.Equal(t, http.StatusOK, status, "ops A should be allowed, body=%s", respBody)

		var reloaded schema.AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyA.ID).Error)
		assert.Equal(t, int64(0), reloaded.TokenUsed)
	})

	t.Run("ops A cannot reset user B's key (403)", func(t *testing.T) {
		// 关键词: OPS 越权防护 reset-token, 不能重置他人 key
		body := fmt.Sprintf(`{"api_key": "%s"}`, keyB.APIKey)
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST",
			"/ops/api/reset-token", authA, body)
		assert.Equal(t, http.StatusForbidden, status,
			"ops A must be forbidden to reset ops B's key, body=%s", respBody)

		// Ensure keyB's token_used is unchanged
		var reloaded schema.AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyB.ID).Error)
		assert.Equal(t, int64(12345), reloaded.TokenUsed,
			"ops B's TokenUsed must remain untouched")
	})

	t.Run("empty api_key returns 400", func(t *testing.T) {
		body := `{"api_key": ""}`
		status, _, _ := sendRawHTTPRequest(t, addr, "POST",
			"/ops/api/reset-token", authA, body)
		assert.Equal(t, http.StatusBadRequest, status)
	})

	t.Run("non-existent api_key returns 404", func(t *testing.T) {
		body := `{"api_key": "mf-does-not-exist-zzz"}`
		status, _, _ := sendRawHTTPRequest(t, addr, "POST",
			"/ops/api/reset-token", authA, body)
		assert.Equal(t, http.StatusNotFound, status)
	})
}

// TestOpsUpdateApiKey_TokenFieldOwnerOnly 验证 OPS update-api-key 的 token 字段同样受 ownership 限制。
func TestOpsUpdateApiKey_TokenFieldOwnerOnly(t *testing.T) {
	addr, _ := startTestPortalServer(t)
	setupTestDBForOps(t)

	userA, plainA := createOpsUserWithPlain(t, "ops-U-A")
	userB, _ := createOpsUserWithPlain(t, "ops-U-B")
	keyA := createApiKeyOwnedByOps(t, userA, "u-owned-by-A")
	keyB := createApiKeyOwnedByOps(t, userB, "u-owned-by-B")

	sessA := opsLoginGetSession(t, addr, userA.Username, plainA)
	authA := map[string]string{"Cookie": "ops_session=" + sessA}

	t.Run("ops A can update own key token_limit", func(t *testing.T) {
		// 关键词: OPS update-api-key 本人设置 token_limit happy path
		body := fmt.Sprintf(`{"api_key":"%s","allowed_models":["gpt-3.5-turbo"],"token_limit":7777777,"token_unlimited":false}`, keyA.APIKey)
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST",
			"/ops/api/update-api-key", authA, body)
		require.Equal(t, http.StatusOK, status, "ops A should be allowed, body=%s", respBody)

		var reloaded schema.AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyA.ID).Error)
		assert.Equal(t, int64(7777777), reloaded.TokenLimit)
		assert.True(t, reloaded.TokenLimitEnable)
	})

	t.Run("ops A cannot update user B's token_limit (403)", func(t *testing.T) {
		// 关键词: OPS 越权防护 update-api-key, 不能改他人 token_limit
		body := fmt.Sprintf(`{"api_key":"%s","allowed_models":["gpt-3.5-turbo"],"token_limit":1,"token_unlimited":false}`, keyB.APIKey)
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST",
			"/ops/api/update-api-key", authA, body)
		assert.Equal(t, http.StatusForbidden, status,
			"ops A must be forbidden from updating ops B's key, body=%s", respBody)

		// Verify ops B's key is unchanged
		var reloaded schema.AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyB.ID).Error)
		assert.Equal(t, int64(0), reloaded.TokenLimit,
			"ops B's TokenLimit must remain untouched")
		assert.False(t, reloaded.TokenLimitEnable)
	})

	t.Run("token_unlimited disables limit", func(t *testing.T) {
		// 关键词: OPS update-api-key token_unlimited=true 显式关闭
		body := fmt.Sprintf(`{"api_key":"%s","token_unlimited":true}`, keyA.APIKey)
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST",
			"/ops/api/update-api-key", authA, body)
		require.Equal(t, http.StatusOK, status, "ops A should be allowed, body=%s", respBody)

		var reloaded schema.AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyA.ID).Error)
		assert.False(t, reloaded.TokenLimitEnable, "token_unlimited should turn off")
		assert.Equal(t, int64(0), reloaded.TokenLimit)
	})

	// 防御性：跨角色访问 admin 端点。OPS 用户携带合法 ops_session 调用 /portal/*
	// admin endpoint 时，已认证但权限不足 -> 期望 403 Forbidden（而非 401，
	// 与 checkPermission 的统一行为一致：未认证 401，已认证但角色不允许 403）。
	// 关键词: admin token endpoint OPS cookie 跨角色防护, 403 Forbidden
	t.Run("admin endpoints inaccessible from OPS cookie", func(t *testing.T) {
		body := `{"token_limit": 1, "enable": true}`
		status, _, _ := sendRawHTTPRequest(t, addr, "POST",
			fmt.Sprintf("/portal/api-key-token-limit/%d", keyA.ID), authA, body)
		assert.Equal(t, http.StatusForbidden, status,
			"OPS cookie must not authorize admin-only token endpoint, got %d", status)

		status2, _, _ := sendRawHTTPRequest(t, addr, "POST",
			fmt.Sprintf("/portal/reset-api-key-token/%d", keyA.ID), authA, "{}")
		assert.Equal(t, http.StatusForbidden, status2,
			"OPS cookie must not authorize admin-only reset-token, got %d", status2)
	})

	// 防御：使用 keyB 的 ID 直接调用 portal admin endpoint 时如果带的不是 admin cookie，也是 401
	_ = userB
}
