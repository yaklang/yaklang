package aibalance

// OPS 用户 API Key「增删改查」补强能力的单元 / 集成测试。
//
// 覆盖本次新增 / 修复的能力：
//   1. OPS 通过 /ops/api/update-api-key 的 active 字段启用 / 禁用自己创建的 Key；
//      - 仅本人可改（越权 403），未携带凭证 401；
//      - 仅传 {api_key, active} 的部分更新不会误改 allowed_models / traffic / token / 绑定信息；
//   2. 禁用(Active=false)的 Key 不再进入内存生效集合（LoadAPIKeysFromDB 跳过），
//      启用后重新进入，保证"禁用"对实际请求即时生效；
//   3. /ops/api/my-keys 的 username / active / q 过滤，且任何过滤都强制 created_by_ops_id
//      越权隔离；过滤 LIKE 防 SQL 注入；
//   4. my-keys 返回补全的用量统计字段。
//
// 关键词: ops_apikey_crud_test, OPS API Key 增删改查, active 启用禁用, my-keys 过滤, 越权隔离, 防注入

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createApiKeyOwnedByOpsFull 创建一条由指定 OPS 用户拥有、可定制关键字段的 API Key。
// 关键词: 测试夹具 createApiKeyOwnedByOpsFull, 可定制 username/active/remark
func createApiKeyOwnedByOpsFull(t *testing.T, ops *OpsUser, suffix, username, remark string, active bool) *AiApiKeys {
	t.Helper()
	db := GetDB()
	require.NotNil(t, db)
	apiKey := &AiApiKeys{
		APIKey:           fmt.Sprintf("mf-crud-%s-%d", suffix, time.Now().UnixNano()),
		AllowedModels:    "gpt-3.5-turbo,gpt-4",
		Active:           active,
		CreatedByOpsID:   ops.ID,
		CreatedByOpsName: ops.Username,
		Username:         username,
		Remark:           remark,
	}
	require.NoError(t, db.Create(apiKey).Error)
	// 注意：jinzhu/gorm 对带 `default:true` 的 bool 字段在 Create 时会省略零值(false)，
	// 导致 active=false 被写成默认 true。这里显式用 map Update 强制落库正确的 active 值。
	// 关键词: 测试夹具 active=false 显式 Update, 规避 gorm default 零值省略
	require.NoError(t, db.Model(&AiApiKeys{}).Where("id = ?", apiKey.ID).Update("active", active).Error)
	apiKey.Active = active
	t.Cleanup(func() { db.Delete(apiKey) })
	return apiKey
}

// TestMUSTPASS_OpsUpdateApiKey_ActiveToggleOwnerOnly 验证 active 启用/禁用的鉴权与越权防护。
func TestMUSTPASS_OpsUpdateApiKey_ActiveToggleOwnerOnly(t *testing.T) {
	addr, _ := startTestPortalServer(t)
	setupTestDBForOps(t)

	userA, plainA := createOpsUserWithPlain(t, "ops-active-A")
	userB, _ := createOpsUserWithPlain(t, "ops-active-B")
	keyA := createApiKeyOwnedByOpsFull(t, userA, "A", "alice", "remark-a", true)
	keyB := createApiKeyOwnedByOpsFull(t, userB, "B", "bob", "remark-b", true)

	sessA := opsLoginGetSession(t, addr, userA.Username, plainA)
	authA := map[string]string{"Cookie": "ops_session=" + sessA}

	t.Run("ops A can disable own key", func(t *testing.T) {
		body := fmt.Sprintf(`{"api_key":"%s","active":false}`, keyA.APIKey)
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST", "/ops/api/update-api-key", authA, body)
		require.Equal(t, http.StatusOK, status, "ops A should disable own key, body=%s", respBody)

		var reloaded AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyA.ID).Error)
		assert.False(t, reloaded.Active, "keyA should be disabled")
	})

	t.Run("ops A can re-enable own key", func(t *testing.T) {
		body := fmt.Sprintf(`{"api_key":"%s","active":true}`, keyA.APIKey)
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST", "/ops/api/update-api-key", authA, body)
		require.Equal(t, http.StatusOK, status, "ops A should re-enable own key, body=%s", respBody)

		var reloaded AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyA.ID).Error)
		assert.True(t, reloaded.Active, "keyA should be active again")
	})

	t.Run("ops A cannot toggle user B's key (403)", func(t *testing.T) {
		body := fmt.Sprintf(`{"api_key":"%s","active":false}`, keyB.APIKey)
		status, _, respBody := sendRawHTTPRequest(t, addr, "POST", "/ops/api/update-api-key", authA, body)
		assert.Equal(t, http.StatusForbidden, status, "ops A must be forbidden from toggling ops B's key, body=%s", respBody)

		var reloaded AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyB.ID).Error)
		assert.True(t, reloaded.Active, "ops B's key must remain active (untouched)")
	})

	t.Run("unauthenticated toggle returns 401", func(t *testing.T) {
		body := fmt.Sprintf(`{"api_key":"%s","active":false}`, keyA.APIKey)
		status, _, _ := sendRawHTTPRequest(t, addr, "POST", "/ops/api/update-api-key", nil, body)
		assert.Equal(t, http.StatusUnauthorized, status, "no cookie must be rejected with 401")

		var reloaded AiApiKeys
		require.NoError(t, GetDB().First(&reloaded, keyA.ID).Error)
		assert.True(t, reloaded.Active, "keyA must remain active when request is unauthorized")
	})
}

// TestMUSTPASS_OpsUpdateApiKey_PartialActiveDoesNotClobber 验证仅传 {api_key, active}
// 的部分更新不会误改其它字段（allowed_models / traffic / token / username / remark）。
func TestMUSTPASS_OpsUpdateApiKey_PartialActiveDoesNotClobber(t *testing.T) {
	addr, _ := startTestPortalServer(t)
	setupTestDBForOps(t)

	userA, plainA := createOpsUserWithPlain(t, "ops-partial-A")

	db := GetDB()
	apiKey := &AiApiKeys{
		APIKey:             fmt.Sprintf("mf-partial-%d", time.Now().UnixNano()),
		AllowedModels:      "claude-3,gpt-4",
		Active:             true,
		CreatedByOpsID:     userA.ID,
		CreatedByOpsName:   userA.Username,
		Username:           "charlie",
		Remark:             "vip-user",
		MetaInfo:           `{"plan":"pro"}`,
		TrafficLimitEnable: true,
		TrafficLimit:       123456,
		TokenLimitEnable:   true,
		TokenLimit:         654321,
	}
	require.NoError(t, db.Create(apiKey).Error)
	t.Cleanup(func() { db.Delete(apiKey) })

	sessA := opsLoginGetSession(t, addr, userA.Username, plainA)
	authA := map[string]string{"Cookie": "ops_session=" + sessA}

	body := fmt.Sprintf(`{"api_key":"%s","active":false}`, apiKey.APIKey)
	status, _, respBody := sendRawHTTPRequest(t, addr, "POST", "/ops/api/update-api-key", authA, body)
	require.Equal(t, http.StatusOK, status, "partial update should succeed, body=%s", respBody)

	var reloaded AiApiKeys
	require.NoError(t, GetDB().First(&reloaded, apiKey.ID).Error)
	assert.False(t, reloaded.Active, "active should be turned off")
	// 其它字段必须保持不变
	assert.Equal(t, "claude-3,gpt-4", reloaded.AllowedModels, "allowed_models must be untouched")
	assert.Equal(t, "charlie", reloaded.Username, "username must be untouched")
	assert.Equal(t, "vip-user", reloaded.Remark, "remark must be untouched")
	assert.Equal(t, `{"plan":"pro"}`, reloaded.MetaInfo, "metainfo must be untouched")
	assert.True(t, reloaded.TrafficLimitEnable, "traffic_limit_enable must be untouched")
	assert.Equal(t, int64(123456), reloaded.TrafficLimit, "traffic_limit must be untouched")
	assert.True(t, reloaded.TokenLimitEnable, "token_limit_enable must be untouched")
	assert.Equal(t, int64(654321), reloaded.TokenLimit, "token_limit must be untouched")
}

// TestMUSTPASS_LoadAPIKeysFromDB_SkipsInactive 验证禁用(Active=false)的 Key 不进入内存生效集合，
// 启用后重新进入。直接驱动 LoadAPIKeysFromDB，避免 HTTP 噪声。
func TestMUSTPASS_LoadAPIKeysFromDB_SkipsInactive(t *testing.T) {
	setupTestDBForOps(t)
	db := GetDB()

	config := NewServerConfig()

	activeKey := &AiApiKeys{
		APIKey:        fmt.Sprintf("mf-load-active-%d", time.Now().UnixNano()),
		AllowedModels: "gpt-4",
		Active:        true,
	}
	require.NoError(t, db.Create(activeKey).Error)
	t.Cleanup(func() { db.Delete(activeKey) })

	// 初始：active key 应被加载进内存（无论 modelMap 是否为空，key 本身一定登记到 Keys）。
	require.NoError(t, config.LoadAPIKeysFromDB())
	_, ok := config.Keys.Get(activeKey.APIKey)
	assert.True(t, ok, "active key must be loaded into in-memory Keys map")

	// 禁用后重载：必须从内存移除。
	require.NoError(t, db.Model(&AiApiKeys{}).Where("id = ?", activeKey.ID).Update("active", false).Error)
	require.NoError(t, config.LoadAPIKeysFromDB())
	_, ok = config.Keys.Get(activeKey.APIKey)
	assert.False(t, ok, "disabled key must NOT be present in in-memory Keys map")

	// 重新启用后重载：必须重新进入内存。
	require.NoError(t, db.Model(&AiApiKeys{}).Where("id = ?", activeKey.ID).Update("active", true).Error)
	require.NoError(t, config.LoadAPIKeysFromDB())
	_, ok = config.Keys.Get(activeKey.APIKey)
	assert.True(t, ok, "re-enabled key must be loaded back into memory")
}

// TestMUSTPASS_OpsMyKeysFiltering 验证 my-keys 的 username / active / q 过滤与越权隔离。
func TestMUSTPASS_OpsMyKeysFiltering(t *testing.T) {
	addr, _ := startTestPortalServer(t)
	setupTestDBForOps(t)

	userA, plainA := createOpsUserWithPlain(t, "ops-filter-A")
	userB, _ := createOpsUserWithPlain(t, "ops-filter-B")

	// userA 的 key：alice(active), alice(inactive), bob(active)
	keyAliceActive := createApiKeyOwnedByOpsFull(t, userA, "alice1", "alice", "need-gpt4-access", true)
	keyAliceInactive := createApiKeyOwnedByOpsFull(t, userA, "alice2", "alice", "suspended-temp", false)
	keyBobActive := createApiKeyOwnedByOpsFull(t, userA, "bob1", "bob", "trial-user", true)

	// userB 的 key，绝不能出现在 userA 的结果里
	keyBSecret := createApiKeyOwnedByOpsFull(t, userB, "secret", "alice", "should-not-leak", true)

	sessA := opsLoginGetSession(t, addr, userA.Username, plainA)
	authA := map[string]string{"Cookie": "ops_session=" + sessA}

	// fetchKeys 发起 my-keys 查询并返回 api_key 集合。
	fetchKeys := func(t *testing.T, queryString string) map[string]map[string]interface{} {
		t.Helper()
		status, _, respBody := sendRawHTTPRequest(t, addr, "GET", "/ops/api/my-keys"+queryString, authA, "")
		require.Equal(t, http.StatusOK, status, "my-keys should return 200, body=%s", respBody)
		var parsed struct {
			Success bool                     `json:"success"`
			Keys    []map[string]interface{} `json:"keys"`
		}
		require.NoError(t, json.Unmarshal([]byte(respBody), &parsed))
		require.True(t, parsed.Success)
		out := make(map[string]map[string]interface{})
		for _, k := range parsed.Keys {
			if s, ok := k["api_key"].(string); ok {
				out[s] = k
			}
		}
		return out
	}

	assertNoLeak := func(t *testing.T, keys map[string]map[string]interface{}) {
		t.Helper()
		_, leaked := keys[keyBSecret.APIKey]
		assert.False(t, leaked, "userB's key must never appear in userA's results")
	}

	t.Run("filter by username=alice", func(t *testing.T) {
		keys := fetchKeys(t, "?username=alice&page_size=100")
		assert.Contains(t, keys, keyAliceActive.APIKey)
		assert.Contains(t, keys, keyAliceInactive.APIKey)
		assert.NotContains(t, keys, keyBobActive.APIKey, "bob's key must be excluded")
		assertNoLeak(t, keys)
	})

	t.Run("filter by active=true", func(t *testing.T) {
		keys := fetchKeys(t, "?active=true&page_size=100")
		assert.Contains(t, keys, keyAliceActive.APIKey)
		assert.Contains(t, keys, keyBobActive.APIKey)
		assert.NotContains(t, keys, keyAliceInactive.APIKey, "inactive key must be excluded")
		assertNoLeak(t, keys)
	})

	t.Run("filter by active=false", func(t *testing.T) {
		keys := fetchKeys(t, "?active=false&page_size=100")
		assert.Contains(t, keys, keyAliceInactive.APIKey)
		assert.NotContains(t, keys, keyAliceActive.APIKey)
		assert.NotContains(t, keys, keyBobActive.APIKey)
		assertNoLeak(t, keys)
	})

	t.Run("filter by q (remark keyword)", func(t *testing.T) {
		keys := fetchKeys(t, "?q=trial-user&page_size=100")
		assert.Contains(t, keys, keyBobActive.APIKey)
		assert.NotContains(t, keys, keyAliceActive.APIKey)
		assertNoLeak(t, keys)
	})

	t.Run("combined username=alice and active=false", func(t *testing.T) {
		keys := fetchKeys(t, "?username=alice&active=false&page_size=100")
		assert.Contains(t, keys, keyAliceInactive.APIKey)
		assert.NotContains(t, keys, keyAliceActive.APIKey)
		assert.NotContains(t, keys, keyBobActive.APIKey)
		assertNoLeak(t, keys)
	})

	t.Run("response includes enriched stat fields", func(t *testing.T) {
		keys := fetchKeys(t, "?username=alice&page_size=100")
		entry, ok := keys[keyAliceActive.APIKey]
		require.True(t, ok)
		for _, field := range []string{
			"usage_count", "success_count", "failure_count",
			"input_bytes", "output_bytes", "web_search_count",
			"created_by_ops_name", "active", "username", "remark", "metainfo",
		} {
			_, present := entry[field]
			assert.True(t, present, "my-keys entry should expose field %q", field)
		}
		assert.Equal(t, userA.Username, entry["created_by_ops_name"])
	})
}

// TestMUSTPASS_OpsMyKeysFilterSQLInjectionSafe 验证 my-keys 过滤参数对 SQL 注入安全：
// 注入串既不报错、也绝不泄露他人 Key 或返回全量。
func TestMUSTPASS_OpsMyKeysFilterSQLInjectionSafe(t *testing.T) {
	addr, _ := startTestPortalServer(t)
	setupTestDBForOps(t)

	userA, plainA := createOpsUserWithPlain(t, "ops-sqli-A")
	userB, _ := createOpsUserWithPlain(t, "ops-sqli-B")

	keyA := createApiKeyOwnedByOpsFull(t, userA, "owned", "alice", "remark-a", true)
	keyBSecret := createApiKeyOwnedByOpsFull(t, userB, "secret", "alice", "should-not-leak", true)

	sessA := opsLoginGetSession(t, addr, userA.Username, plainA)
	authA := map[string]string{"Cookie": "ops_session=" + sessA}

	injections := []string{
		"%27%20OR%20%271%27%3D%271",           // ' OR '1'='1
		"%27%3B%20DROP%20TABLE%20ai_api_keys", // '; DROP TABLE ai_api_keys
		"%25",                                 // %
		"_%25",                                // _%
		"%5C",                                 // backslash
	}

	for _, inj := range injections {
		t.Run("username inj "+inj, func(t *testing.T) {
			status, _, respBody := sendRawHTTPRequest(t, addr, "GET",
				"/ops/api/my-keys?username="+inj+"&page_size=100", authA, "")
			require.Equal(t, http.StatusOK, status, "injection must not error, body=%s", respBody)
			var parsed struct {
				Success bool                     `json:"success"`
				Keys    []map[string]interface{} `json:"keys"`
			}
			require.NoError(t, json.Unmarshal([]byte(respBody), &parsed))
			for _, k := range parsed.Keys {
				// 必须只返回 userA 自己的 key，且绝不返回 userB 的 secret key
				assert.NotEqual(t, keyBSecret.APIKey, k["api_key"], "must never leak other user's key via injection")
			}
		})
		t.Run("q inj "+inj, func(t *testing.T) {
			status, _, respBody := sendRawHTTPRequest(t, addr, "GET",
				"/ops/api/my-keys?q="+inj+"&page_size=100", authA, "")
			require.Equal(t, http.StatusOK, status, "injection must not error, body=%s", respBody)
			var parsed struct {
				Success bool                     `json:"success"`
				Keys    []map[string]interface{} `json:"keys"`
			}
			require.NoError(t, json.Unmarshal([]byte(respBody), &parsed))
			for _, k := range parsed.Keys {
				assert.NotEqual(t, keyBSecret.APIKey, k["api_key"], "must never leak other user's key via injection")
			}
		})
	}

	// 正常关键字仍可命中本人 key（证明过滤不是"全拒绝"）。
	status, _, respBody := sendRawHTTPRequest(t, addr, "GET",
		"/ops/api/my-keys?q=remark-a&page_size=100", authA, "")
	require.Equal(t, http.StatusOK, status)
	var parsed struct {
		Success bool                     `json:"success"`
		Keys    []map[string]interface{} `json:"keys"`
	}
	require.NoError(t, json.Unmarshal([]byte(respBody), &parsed))
	found := false
	for _, k := range parsed.Keys {
		if k["api_key"] == keyA.APIKey {
			found = true
		}
		assert.NotEqual(t, keyBSecret.APIKey, k["api_key"])
	}
	assert.True(t, found, "normal keyword should still match own key")
}
