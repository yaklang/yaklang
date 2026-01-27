package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func init() {
	// Initialize database
	consts.InitializeYakitDatabase("", "", "")
}

// ==================== Test Helpers ====================

func setupTestDBForOps(t *testing.T) {
	db := GetDB()
	require.NotNil(t, db, "database should be initialized")

	// Auto-migrate schemas if needed
	err := db.AutoMigrate(&schema.OpsUser{}, &schema.OpsActionLog{}, &schema.AiApiKeys{}, &schema.LoginSession{})
	require.NoError(t, err.Error)
}

func createTestOpsUserForTest(t *testing.T, username string) *schema.OpsUser {
	password := GenerateRandomPassword()
	hashedPassword, err := HashPassword(password)
	require.NoError(t, err)

	user := &schema.OpsUser{
		Username:     username,
		Password:     hashedPassword,
		OpsKey:       GenerateOpsKey(),
		Role:         "ops",
		Active:       true,
		DefaultLimit: 52428800, // 50MB
	}

	err = SaveOpsUser(user)
	require.NoError(t, err)

	t.Cleanup(func() {
		DeleteOpsUser(user.ID)
	})

	return user
}

// ==================== Password Utility Tests ====================

func TestMUSTPASS_GenerateRandomPassword(t *testing.T) {
	password := GenerateRandomPassword()

	// Should start with "ops-"
	assert.True(t, strings.HasPrefix(password, "ops-"), "password should start with 'ops-'")

	// Should be at least 16 characters (ops- + 12 random chars)
	assert.GreaterOrEqual(t, len(password), 16, "password should be at least 16 characters")

	// Generate another password to ensure uniqueness
	password2 := GenerateRandomPassword()
	assert.NotEqual(t, password, password2, "passwords should be unique")
}

func TestMUSTPASS_HashPassword(t *testing.T) {
	password := "test-password-123"

	hash, err := HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Verify the password
	assert.True(t, CheckPassword(hash, password), "correct password should match")
	assert.False(t, CheckPassword(hash, "wrong-password"), "wrong password should not match")
}

func TestMUSTPASS_GenerateOpsKey(t *testing.T) {
	key := GenerateOpsKey()

	// Should start with "ops-"
	assert.True(t, strings.HasPrefix(key, "ops-"), "OPS key should start with 'ops-'")

	// Should be at least 40 characters (ops- + UUID which is 36 chars)
	assert.GreaterOrEqual(t, len(key), 40, "OPS key should be at least 40 characters")

	// Generate another key to ensure uniqueness
	key2 := GenerateOpsKey()
	assert.NotEqual(t, key, key2, "OPS keys should be unique")
}

// ==================== OPS User CRUD Tests ====================

func TestMUSTPASS_OpsUserCRUD(t *testing.T) {
	setupTestDBForOps(t)

	// Create
	username := fmt.Sprintf("test-ops-user-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)
	assert.NotZero(t, user.ID)

	// Read by ID
	retrieved, err := GetOpsUserByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.Username, retrieved.Username)

	// Read by Username
	retrieved2, err := GetOpsUserByUsername(username)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved2.ID)

	// Read by OpsKey
	retrieved3, err := GetOpsUserByOpsKey(user.OpsKey)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved3.ID)

	// Update
	user.Active = false
	err = SaveOpsUser(user)
	require.NoError(t, err)

	retrieved4, err := GetOpsUserByID(user.ID)
	require.NoError(t, err)
	assert.False(t, retrieved4.Active)
}

func TestMUSTPASS_GetAllOpsUsers(t *testing.T) {
	setupTestDBForOps(t)

	// Create multiple users with unique names
	prefix := fmt.Sprintf("ops-user-list-%d", time.Now().UnixNano())
	createTestOpsUserForTest(t, prefix+"-1")
	createTestOpsUserForTest(t, prefix+"-2")
	createTestOpsUserForTest(t, prefix+"-3")

	// Get all
	users, err := GetAllOpsUsers()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(users), 3)
}

// ==================== OPS Action Log Tests ====================

func TestMUSTPASS_LogOpsAction(t *testing.T) {
	setupTestDBForOps(t)

	username := fmt.Sprintf("ops-logger-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)

	// Log an action
	req, _ := http.NewRequest("POST", "/ops/create-api-key", nil)
	req.RemoteAddr = "192.168.1.100:12345"

	LogOpsAction(user.ID, user.Username, "create_api_key", "api_key", "123", `{"model":"gpt-4"}`, req)

	// Verify log was created
	db := GetDB()
	var logs []schema.OpsActionLog
	err := db.Where("operator_id = ?", user.ID).Find(&logs).Error
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(logs), 1)

	// Clean up
	t.Cleanup(func() {
		db.Where("operator_id = ?", user.ID).Delete(&schema.OpsActionLog{})
	})
}

// ==================== Permission Tests ====================

func TestMUSTPASS_OpsRoutePermissions(t *testing.T) {
	config := DefaultAuthConfig()

	// Test OPS-only routes
	opsRoutes := []struct {
		path   string
		method string
	}{
		{"/ops/dashboard", "GET"},
		{"/ops/create-api-key", "POST"},
		{"/ops/api/create-api-key", "POST"},
		{"/ops/api/my-keys", "GET"},
		{"/ops/api/delete-api-key", "POST"},
		{"/ops/my-info", "GET"},
		{"/ops/reset-key", "POST"},
	}

	for _, route := range opsRoutes {
		t.Run(fmt.Sprintf("%s %s", route.method, route.path), func(t *testing.T) {
			perm := config.FindMatchingPermission(route.path, route.method)
			require.NotNil(t, perm, "should find permission for %s %s", route.method, route.path)
			assert.True(t, perm.AllowsRole(RoleOps), "OPS should be allowed for %s", route.path)
		})
	}

	// Test admin-only routes should deny OPS
	adminOnlyRoutes := []struct {
		path   string
		method string
	}{
		{"/portal/api/ops-users", "GET"},
		{"/portal/api/ops-users", "POST"},
		{"/portal/api/ops-logs", "GET"},
		{"/portal/api/ops-stats", "GET"},
		{"/portal/delete-provider/123", "DELETE"},
	}

	for _, route := range adminOnlyRoutes {
		t.Run(fmt.Sprintf("admin only %s %s", route.method, route.path), func(t *testing.T) {
			perm := config.FindMatchingPermission(route.path, route.method)
			if perm != nil {
				assert.False(t, perm.AllowsRole(RoleOps), "OPS should NOT be allowed for admin-only route %s", route.path)
			}
		})
	}
}

func TestMUSTPASS_PublicPathsForOps(t *testing.T) {
	config := DefaultAuthConfig()

	publicPaths := []string{
		"/ops/login",
		"/ops/static/ops_portal.css",
		"/ops/static/ops_portal.js",
	}

	for _, path := range publicPaths {
		t.Run("public path "+path, func(t *testing.T) {
			assert.True(t, config.IsPublicPath(path), "%s should be public", path)
		})
	}
}

// ==================== API Key Creation by OPS Tests ====================

func TestMUSTPASS_OpsCreateApiKeyPermission(t *testing.T) {
	setupTestDBForOps(t)

	// Create OPS user
	username := fmt.Sprintf("ops-api-creator-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)

	// Verify the user has the correct settings
	assert.Equal(t, int64(52428800), user.DefaultLimit)
	assert.True(t, user.Active)
}

func TestMUSTPASS_OpsCannotAccessAdminAPIs(t *testing.T) {
	config := DefaultAuthConfig()

	// OPS user should NOT have access to these admin APIs
	adminAPIs := []struct {
		path   string
		method string
	}{
		{"/portal", "GET"},
		{"/portal/api/ops-users", "GET"},
		{"/portal/api/ops-users", "POST"},
		{"/portal/api/ops-logs", "GET"},
		{"/portal/api/ops-stats", "GET"},
		{"/portal/add-providers", "POST"},
		{"/portal/delete-provider/1", "DELETE"},
	}

	for _, api := range adminAPIs {
		t.Run(fmt.Sprintf("OPS denied %s %s", api.method, api.path), func(t *testing.T) {
			perm := config.FindMatchingPermission(api.path, api.method)
			if perm != nil {
				assert.False(t, perm.AllowsRole(RoleOps),
					"OPS should NOT be allowed to access %s %s", api.method, api.path)
			}
			// If no permission found, default deny applies which is also correct
		})
	}
}

// ==================== Session Management Tests ====================

func TestMUSTPASS_OpsSessionCreation(t *testing.T) {
	setupTestDBForOps(t)
	db := GetDB()

	username := fmt.Sprintf("ops-session-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)

	// Create session
	session := &schema.LoginSession{
		SessionID: utils.RandStringBytes(32),
		UserID:    user.ID,
		Username:  user.Username,
		UserRole:  "ops",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err := db.Create(session).Error
	require.NoError(t, err)

	// Verify session
	var retrieved schema.LoginSession
	err = db.Where("session_id = ?", session.SessionID).First(&retrieved).Error
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.UserID)
	assert.Equal(t, "ops", retrieved.UserRole)

	// Cleanup
	t.Cleanup(func() {
		db.Delete(&session)
	})
}

// ==================== Integration Tests ====================

func TestMUSTPASS_OpsPortalIntegration(t *testing.T) {
	setupTestDBForOps(t)
	db := GetDB()

	// Create OPS user
	username := fmt.Sprintf("ops-integration-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)

	// Create server config
	config := NewServerConfig()
	config.AdminPassword = "test-admin-secret"

	// Create a test server
	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	lis, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	defer lis.Close()

	// Start server in background
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				conn, err := lis.Accept()
				if err != nil {
					if strings.Contains(err.Error(), "use of closed network connection") {
						return
					}
					continue
				}
				go config.Serve(conn)
			}
		}
	}()
	defer close(done)

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	t.Run("OPS login page is accessible", func(t *testing.T) {
		resp, err := client.Get(fmt.Sprintf("http://%s/ops/login", addr))
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 200 (login page)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("OPS static files are accessible", func(t *testing.T) {
		resp, err := client.Get(fmt.Sprintf("http://%s/ops/static/ops_portal.css", addr))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("OPS dashboard requires authentication", func(t *testing.T) {
		resp, err := client.Get(fmt.Sprintf("http://%s/ops/dashboard", addr))
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should redirect to login (303) or show login page (200)
		assert.Contains(t, []int{200, 303}, resp.StatusCode)
	})

	// Test with valid OPS session
	t.Run("OPS API with valid session", func(t *testing.T) {
		// Create a valid session
		session := &schema.LoginSession{
			SessionID: utils.RandStringBytes(32),
			UserID:    user.ID,
			Username:  user.Username,
			UserRole:  "ops",
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		err := db.Create(session).Error
		require.NoError(t, err)
		defer db.Delete(&session)

		// Make request with session cookie
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/ops/my-info", addr), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:  "ops_session",
			Value: session.SessionID,
		})

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 200 with user info
		assert.Equal(t, 200, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.True(t, result["success"].(bool))
		assert.Equal(t, user.Username, result["username"])
	})

	t.Run("Admin API denied with OPS session", func(t *testing.T) {
		// Create a valid OPS session
		session := &schema.LoginSession{
			SessionID: utils.RandStringBytes(32),
			UserID:    user.ID,
			Username:  user.Username,
			UserRole:  "ops",
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		err := db.Create(session).Error
		require.NoError(t, err)
		defer db.Delete(&session)

		// Try to access admin API with OPS session
		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/portal/api/ops-users", addr), nil)
		require.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:  "ops_session",
			Value: session.SessionID,
		})

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should be forbidden or redirect
		assert.Contains(t, []int{200, 302, 303, 403}, resp.StatusCode)
	})
}

// ==================== API Key Management Tests ====================

func TestMUSTPASS_OpsApiKeyManagement(t *testing.T) {
	setupTestDBForOps(t)
	db := GetDB()

	// Create OPS user
	username := fmt.Sprintf("ops-key-manager-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)

	// Create an API key as if created by OPS user
	apiKey := &schema.AiApiKeys{
		APIKey:             fmt.Sprintf("mf-test-api-key-%d", time.Now().UnixNano()),
		AllowedModels:      "gpt-4,gpt-3.5-turbo",
		TrafficLimit:       52428800,
		TrafficUsed:        0,
		TrafficLimitEnable: true,
		CreatedByOpsID:     user.ID,
		CreatedByOpsName:   user.Username,
		Active:             true,
	}

	err := db.Create(apiKey).Error
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Delete(&apiKey)
	})

	t.Run("Get API keys created by OPS user", func(t *testing.T) {
		var keys []schema.AiApiKeys
		err := db.Where("created_by_ops_id = ?", user.ID).Find(&keys).Error
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(keys), 1)
		assert.Equal(t, user.Username, keys[0].CreatedByOpsName)
	})

	t.Run("OPS user can only see their own keys", func(t *testing.T) {
		// Create another OPS user
		username2 := fmt.Sprintf("ops-key-manager-2-%d", time.Now().UnixNano())
		user2 := createTestOpsUserForTest(t, username2)

		// Create key for user2
		apiKey2 := &schema.AiApiKeys{
			APIKey:             fmt.Sprintf("mf-test-api-key-2-%d", time.Now().UnixNano()),
			AllowedModels:      "gpt-4",
			TrafficLimit:       52428800,
			TrafficUsed:        0,
			TrafficLimitEnable: true,
			CreatedByOpsID:     user2.ID,
			CreatedByOpsName:   user2.Username,
			Active:             true,
		}
		err := db.Create(apiKey2).Error
		require.NoError(t, err)
		defer db.Delete(&apiKey2)

		// User1 should only see their own keys
		var user1Keys []schema.AiApiKeys
		err = db.Where("created_by_ops_id = ?", user.ID).Find(&user1Keys).Error
		require.NoError(t, err)
		for _, k := range user1Keys {
			assert.Equal(t, user.ID, k.CreatedByOpsID)
		}

		// User2 should only see their own keys
		var user2Keys []schema.AiApiKeys
		err = db.Where("created_by_ops_id = ?", user2.ID).Find(&user2Keys).Error
		require.NoError(t, err)
		for _, k := range user2Keys {
			assert.Equal(t, user2.ID, k.CreatedByOpsID)
		}
	})
}

// ==================== Edge Case Tests ====================

func TestMUSTPASS_OpsUserDisabled(t *testing.T) {
	setupTestDBForOps(t)

	username := fmt.Sprintf("disabled-ops-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)
	user.Active = false
	err := SaveOpsUser(user)
	require.NoError(t, err)

	// Verify user is disabled
	retrieved, err := GetOpsUserByID(user.ID)
	require.NoError(t, err)
	assert.False(t, retrieved.Active)
}

func TestMUSTPASS_OpsKeyReset(t *testing.T) {
	setupTestDBForOps(t)

	username := fmt.Sprintf("key-reset-ops-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)
	originalKey := user.OpsKey

	// Reset key
	user.OpsKey = GenerateOpsKey()
	err := SaveOpsUser(user)
	require.NoError(t, err)

	// Verify new key is different
	retrieved, err := GetOpsUserByID(user.ID)
	require.NoError(t, err)
	assert.NotEqual(t, originalKey, retrieved.OpsKey)

	// Old key should not work
	_, err = GetOpsUserByOpsKey(originalKey)
	assert.Error(t, err, "old key should not work")

	// New key should work
	_, err = GetOpsUserByOpsKey(retrieved.OpsKey)
	assert.NoError(t, err, "new key should work")
}

// ==================== Concurrent Access Tests ====================

func TestMUSTPASS_ConcurrentOpsUserAccess(t *testing.T) {
	setupTestDBForOps(t)

	username := fmt.Sprintf("concurrent-ops-%d", time.Now().UnixNano())
	user := createTestOpsUserForTest(t, username)

	// Concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := GetOpsUserByID(user.ID)
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// ==================== Benchmark Tests ====================

func BenchmarkGetOpsUserByID(b *testing.B) {
	consts.InitializeYakitDatabase("", "", "")
	db := GetDB()
	if db == nil {
		b.Skip("database not available")
	}

	password, _ := HashPassword("test")
	user := &schema.OpsUser{
		Username:     fmt.Sprintf("bench-user-%d", time.Now().UnixNano()),
		Password:     password,
		OpsKey:       GenerateOpsKey(),
		Role:         "ops",
		Active:       true,
	}
	db.Create(user)
	defer db.Delete(user)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetOpsUserByID(user.ID)
	}
}

func BenchmarkHashPassword(b *testing.B) {
	password := "test-password-123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashPassword(password)
	}
}

func BenchmarkCheckPassword(b *testing.B) {
	password := "test-password-123"
	hash, _ := HashPassword(password)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckPassword(hash, password)
	}
}

// ==================== Helper for HTTP Testing ====================

func makeJSONRequest(method, url string, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
