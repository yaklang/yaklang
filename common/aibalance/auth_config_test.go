package aibalance

import (
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== UserRole Tests ====================

func TestUserRoleIsValid(t *testing.T) {
	tests := []struct {
		name     string
		role     UserRole
		expected bool
	}{
		{"admin role is valid", RoleAdmin, true},
		{"ops role is valid", RoleOps, true},
		{"empty role is invalid", RoleNone, false},
		{"unknown role is invalid", UserRole("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.role.IsValid())
		})
	}
}

func TestUserRoleString(t *testing.T) {
	assert.Equal(t, "admin", RoleAdmin.String())
	assert.Equal(t, "ops", RoleOps.String())
	assert.Equal(t, "", RoleNone.String())
}

// ==================== AuthInfo Tests ====================

func TestAuthInfoIsAdmin(t *testing.T) {
	tests := []struct {
		name     string
		authInfo AuthInfo
		expected bool
	}{
		{
			"authenticated admin is admin",
			AuthInfo{Authenticated: true, Role: RoleAdmin},
			true,
		},
		{
			"authenticated ops is not admin",
			AuthInfo{Authenticated: true, Role: RoleOps},
			false,
		},
		{
			"unauthenticated admin role is not admin",
			AuthInfo{Authenticated: false, Role: RoleAdmin},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.authInfo.IsAdmin())
		})
	}
}

func TestAuthInfoIsOps(t *testing.T) {
	tests := []struct {
		name     string
		authInfo AuthInfo
		expected bool
	}{
		{
			"authenticated ops is ops",
			AuthInfo{Authenticated: true, Role: RoleOps},
			true,
		},
		{
			"authenticated admin is not ops",
			AuthInfo{Authenticated: true, Role: RoleAdmin},
			false,
		},
		{
			"unauthenticated ops role is not ops",
			AuthInfo{Authenticated: false, Role: RoleOps},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.authInfo.IsOps())
		})
	}
}

func TestAuthInfoHasRole(t *testing.T) {
	tests := []struct {
		name     string
		authInfo AuthInfo
		roles    []UserRole
		expected bool
	}{
		{
			"admin has admin role",
			AuthInfo{Authenticated: true, Role: RoleAdmin},
			[]UserRole{RoleAdmin},
			true,
		},
		{
			"admin has role in admin or ops",
			AuthInfo{Authenticated: true, Role: RoleAdmin},
			[]UserRole{RoleAdmin, RoleOps},
			true,
		},
		{
			"ops does not have admin only role",
			AuthInfo{Authenticated: true, Role: RoleOps},
			[]UserRole{RoleAdmin},
			false,
		},
		{
			"unauthenticated user has no roles",
			AuthInfo{Authenticated: false, Role: RoleAdmin},
			[]UserRole{RoleAdmin},
			false,
		},
		{
			"empty role list returns false",
			AuthInfo{Authenticated: true, Role: RoleAdmin},
			[]UserRole{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.authInfo.HasRole(tt.roles...))
		})
	}
}

// ==================== RoutePermission Tests ====================

func TestRoutePermissionMatches(t *testing.T) {
	tests := []struct {
		name     string
		perm     RoutePermission
		path     string
		method   string
		expected bool
	}{
		// Exact match
		{
			"exact path match",
			RoutePermission{Path: "/portal/api/data", Method: "GET"},
			"/portal/api/data",
			"GET",
			true,
		},
		{
			"exact path wrong method",
			RoutePermission{Path: "/portal/api/data", Method: "GET"},
			"/portal/api/data",
			"POST",
			false,
		},
		{
			"exact path any method",
			RoutePermission{Path: "/portal/api/data", Method: "*"},
			"/portal/api/data",
			"POST",
			true,
		},
		// Prefix match with /*
		{
			"prefix match with /*",
			RoutePermission{Path: "/portal/delete-provider/*", Method: "DELETE"},
			"/portal/delete-provider/123",
			"DELETE",
			true,
		},
		{
			"prefix match with /* longer path",
			RoutePermission{Path: "/portal/api/*", Method: "GET"},
			"/portal/api/users/123/info",
			"GET",
			true,
		},
		{
			"prefix match exact prefix",
			RoutePermission{Path: "/portal/api/*", Method: "GET"},
			"/portal/api/",
			"GET",
			true,
		},
		// Prefix match with *
		{
			"prefix match with *",
			RoutePermission{Path: "/portal/static*", Method: "GET"},
			"/portal/static/css/main.css",
			"GET",
			true,
		},
		// No match
		{
			"no match different path",
			RoutePermission{Path: "/portal/api/data", Method: "GET"},
			"/portal/api/users",
			"GET",
			false,
		},
		{
			"no match prefix not matching",
			RoutePermission{Path: "/portal/api/*", Method: "GET"},
			"/ops/api/data",
			"GET",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.perm.Matches(tt.path, tt.method))
		})
	}
}

func TestRoutePermissionAllowsRole(t *testing.T) {
	tests := []struct {
		name     string
		perm     RoutePermission
		role     UserRole
		expected bool
	}{
		{
			"admin only allows admin",
			RoutePermission{AllowedRoles: []UserRole{RoleAdmin}},
			RoleAdmin,
			true,
		},
		{
			"admin only denies ops",
			RoutePermission{AllowedRoles: []UserRole{RoleAdmin}},
			RoleOps,
			false,
		},
		{
			"both roles allow admin",
			RoutePermission{AllowedRoles: []UserRole{RoleAdmin, RoleOps}},
			RoleAdmin,
			true,
		},
		{
			"both roles allow ops",
			RoutePermission{AllowedRoles: []UserRole{RoleAdmin, RoleOps}},
			RoleOps,
			true,
		},
		{
			"empty roles denies all",
			RoutePermission{AllowedRoles: []UserRole{}},
			RoleAdmin,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.perm.AllowsRole(tt.role))
		})
	}
}

// ==================== AuthConfig Tests ====================

func TestAuthConfigPublicPaths(t *testing.T) {
	config := NewAuthConfig()
	config.PublicPaths = []string{
		"/portal/login",
		"/portal/static/*",
		"/ops/login",
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"exact public path", "/portal/login", true},
		{"prefix public path", "/portal/static/css/main.css", true},
		{"prefix public path root", "/portal/static/", true},
		{"non-public path", "/portal/api/data", false},
		{"partial match is not public", "/portal/login/extra", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, config.IsPublicPath(tt.path))
		})
	}
}

func TestAuthConfigFindMatchingPermission(t *testing.T) {
	config := NewAuthConfig()
	config.Routes = []RoutePermission{
		{Path: "/portal/api/data", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}},
		{Path: "/portal/delete-provider/*", Method: "DELETE", AllowedRoles: []UserRole{RoleAdmin}},
		{Path: "/ops/dashboard", Method: "GET", AllowedRoles: []UserRole{RoleOps}},
	}

	tests := []struct {
		name        string
		path        string
		method      string
		expectFound bool
		expectPath  string
	}{
		{
			"find exact route",
			"/portal/api/data",
			"GET",
			true,
			"/portal/api/data",
		},
		{
			"find prefix route",
			"/portal/delete-provider/123",
			"DELETE",
			true,
			"/portal/delete-provider/*",
		},
		{
			"find ops route",
			"/ops/dashboard",
			"GET",
			true,
			"/ops/dashboard",
		},
		{
			"not found",
			"/unknown/path",
			"GET",
			false,
			"",
		},
		{
			"wrong method",
			"/portal/api/data",
			"POST",
			false,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perm := config.FindMatchingPermission(tt.path, tt.method)
			if tt.expectFound {
				require.NotNil(t, perm, "expected to find permission")
				assert.Equal(t, tt.expectPath, perm.Path)
			} else {
				assert.Nil(t, perm, "expected not to find permission")
			}
		})
	}
}

func TestAuthConfigAddRoute(t *testing.T) {
	config := NewAuthConfig()
	assert.Equal(t, 0, config.CountRoutes())

	config.AddRoute(RoutePermission{Path: "/test", Method: "GET"})
	assert.Equal(t, 1, config.CountRoutes())

	config.AddRoute(RoutePermission{Path: "/test2", Method: "POST"})
	assert.Equal(t, 2, config.CountRoutes())
}

func TestAuthConfigAddPublicPath(t *testing.T) {
	config := NewAuthConfig()
	assert.False(t, config.IsPublicPath("/test"))

	config.AddPublicPath("/test")
	assert.True(t, config.IsPublicPath("/test"))
}

// ==================== DefaultAuthConfig Tests ====================

func TestDefaultAuthConfigHasPublicPaths(t *testing.T) {
	config := DefaultAuthConfig()

	publicPaths := config.GetPublicPaths()
	assert.Contains(t, publicPaths, "/portal/login")
	assert.Contains(t, publicPaths, "/portal/static/*")
	assert.Contains(t, publicPaths, "/ops/login")
	assert.Contains(t, publicPaths, "/ops/static/*")
}

func TestDefaultAuthConfigAdminRoutes(t *testing.T) {
	config := DefaultAuthConfig()

	// Test that admin-only routes are configured correctly
	adminOnlyPaths := []struct {
		path   string
		method string
	}{
		{"/portal/add-providers", "POST"},
		{"/portal/delete-provider/123", "DELETE"},
		{"/portal/api/ops-users", "GET"},
		{"/portal/api/ops-logs", "GET"},
		{"/portal", "GET"},
	}

	for _, route := range adminOnlyPaths {
		t.Run("admin route "+route.path, func(t *testing.T) {
			perm := config.FindMatchingPermission(route.path, route.method)
			require.NotNil(t, perm, "expected permission for %s %s", route.method, route.path)
			assert.True(t, perm.AllowsRole(RoleAdmin), "admin should be allowed for %s", route.path)
			assert.False(t, perm.AllowsRole(RoleOps), "ops should not be allowed for %s", route.path)
		})
	}
}

func TestDefaultAuthConfigOpsRoutes(t *testing.T) {
	config := DefaultAuthConfig()

	// Test that ops routes are configured correctly
	opsOnlyPaths := []struct {
		path   string
		method string
	}{
		{"/ops/dashboard", "GET"},
		{"/ops/create-api-key", "POST"},
		{"/ops/reset-key", "POST"},
		{"/ops/my-info", "GET"},
	}

	for _, route := range opsOnlyPaths {
		t.Run("ops route "+route.path, func(t *testing.T) {
			perm := config.FindMatchingPermission(route.path, route.method)
			require.NotNil(t, perm, "expected permission for %s %s", route.method, route.path)
			assert.True(t, perm.AllowsRole(RoleOps), "ops should be allowed for %s", route.path)
		})
	}
}

func TestDefaultAuthConfigSharedRoutes(t *testing.T) {
	config := DefaultAuthConfig()

	// Test that change-password allows both admin and ops
	perm := config.FindMatchingPermission("/ops/change-password", "POST")
	require.NotNil(t, perm)
	assert.True(t, perm.AllowsRole(RoleAdmin))
	assert.True(t, perm.AllowsRole(RoleOps))
}

// ==================== AuthMiddleware Tests ====================

func TestAuthMiddlewareIsPublicPath(t *testing.T) {
	config := DefaultAuthConfig()
	middleware := NewAuthMiddleware(nil, config)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"login is public", "/portal/login", true},
		{"static files are public", "/portal/static/js/app.js", true},
		{"ops login is public", "/ops/login", true},
		{"portal home is not public", "/portal", false},
		{"api is not public", "/portal/api/data", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, middleware.IsPublicPath(tt.path))
		})
	}
}

func TestAuthMiddlewareCheckPermissionPublicPath(t *testing.T) {
	config := DefaultAuthConfig()
	middleware := NewAuthMiddleware(nil, config)

	req := httptest.NewRequest("GET", "/portal/login", nil)
	allowed, authInfo, reason := middleware.CheckPermission(req, "/portal/login")

	assert.True(t, allowed)
	assert.False(t, authInfo.Authenticated)
	assert.Equal(t, "public path", reason)
}

func TestAuthMiddlewareCheckPermissionNoAuth(t *testing.T) {
	config := DefaultAuthConfig()
	serverConfig := NewServerConfig()
	middleware := NewAuthMiddleware(serverConfig, config)

	req := httptest.NewRequest("GET", "/portal/api/data", nil)
	allowed, authInfo, reason := middleware.CheckPermission(req, "/portal/api/data")

	assert.False(t, allowed)
	assert.False(t, authInfo.Authenticated)
	assert.Equal(t, "authentication required", reason)
}

// ==================== Concurrent Access Tests ====================

func TestAuthConfigConcurrentAccess(t *testing.T) {
	config := NewAuthConfig()
	config.PublicPaths = []string{"/public"}
	config.Routes = []RoutePermission{
		{Path: "/api", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}},
	}

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			config.IsPublicPath("/public")
			config.FindMatchingPermission("/api", "GET")
			config.CountRoutes()
		}()
	}

	// Concurrent writes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			config.AddPublicPath("/public" + string(rune(idx)))
			config.AddRoute(RoutePermission{Path: "/api" + string(rune(idx)), Method: "GET"})
		}(i)
	}

	wg.Wait()

	// Verify no data race occurred (test passes if no panic)
	assert.True(t, config.CountRoutes() > 0)
}

// ==================== Integration Tests ====================

func TestAuthConfigIntegration(t *testing.T) {
	config := DefaultAuthConfig()

	// Simulate different scenarios
	scenarios := []struct {
		name        string
		path        string
		method      string
		role        UserRole
		expectAllow bool
	}{
		// Public paths
		{"public login", "/portal/login", "GET", RoleNone, true},
		{"public static", "/portal/static/app.js", "GET", RoleNone, true},

		// Admin routes with admin
		{"admin route with admin", "/portal/api/data", "GET", RoleAdmin, true},
		{"admin delete with admin", "/portal/delete-provider/1", "DELETE", RoleAdmin, true},

		// Admin routes with ops - should deny
		{"admin route with ops", "/portal/api/data", "GET", RoleOps, false},

		// Ops routes with ops
		{"ops dashboard with ops", "/ops/dashboard", "GET", RoleOps, true},
		{"ops create key with ops", "/ops/create-api-key", "POST", RoleOps, true},

		// Ops routes with admin - should deny (ops-only routes)
		{"ops reset-key with admin", "/ops/reset-key", "POST", RoleAdmin, false},

		// Shared routes
		{"change password with ops", "/ops/change-password", "POST", RoleOps, true},
		{"change password with admin", "/ops/change-password", "POST", RoleAdmin, true},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// Check if public
			if config.IsPublicPath(sc.path) {
				if sc.expectAllow {
					return // Public path, allowed
				}
				t.Errorf("expected public path to be denied but was allowed")
				return
			}

			// Find permission
			perm := config.FindMatchingPermission(sc.path, sc.method)
			if perm == nil {
				if sc.expectAllow && sc.role == RoleAdmin {
					// Default deny allows admin
					return
				}
				if !sc.expectAllow {
					return // Not found, denied as expected
				}
				t.Errorf("expected to find permission for %s %s", sc.method, sc.path)
				return
			}

			// Check role
			allowed := perm.AllowsRole(sc.role)
			if allowed != sc.expectAllow {
				t.Errorf("expected allow=%v but got allow=%v for role %s on %s %s",
					sc.expectAllow, allowed, sc.role, sc.method, sc.path)
			}
		})
	}
}

// ==================== Edge Case Tests ====================

func TestAuthConfigEmptyConfig(t *testing.T) {
	config := NewAuthConfig()
	config.DefaultDeny = true

	// No routes configured
	perm := config.FindMatchingPermission("/any/path", "GET")
	assert.Nil(t, perm)

	// No public paths
	assert.False(t, config.IsPublicPath("/any/path"))
}

func TestRoutePermissionEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		perm     RoutePermission
		path     string
		method   string
		expected bool
	}{
		{
			"empty path permission",
			RoutePermission{Path: "", Method: "GET"},
			"",
			"GET",
			true,
		},
		{
			"root path",
			RoutePermission{Path: "/", Method: "GET"},
			"/",
			"GET",
			true,
		},
		{
			"wildcard only",
			RoutePermission{Path: "*", Method: "*"},
			"/any/path",
			"POST",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.perm.Matches(tt.path, tt.method))
		})
	}
}

// ==================== Benchmark Tests ====================

func BenchmarkRoutePermissionMatches(b *testing.B) {
	perm := RoutePermission{Path: "/portal/api/*", Method: "GET"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		perm.Matches("/portal/api/users/123/info", "GET")
	}
}

func BenchmarkAuthConfigFindPermission(b *testing.B) {
	config := DefaultAuthConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.FindMatchingPermission("/portal/api/data", "GET")
	}
}

func BenchmarkAuthConfigIsPublicPath(b *testing.B) {
	config := DefaultAuthConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.IsPublicPath("/portal/static/js/app.js")
	}
}

// ==================== Helper Function Tests ====================

func TestGetRoutePermissions(t *testing.T) {
	config := NewAuthConfig()
	config.AddRoute(RoutePermission{Path: "/test1", Method: "GET"})
	config.AddRoute(RoutePermission{Path: "/test2", Method: "POST"})

	perms := config.GetRoutePermissions()
	assert.Equal(t, 2, len(perms))

	// Verify it's a copy
	perms[0].Path = "/modified"
	originalPerms := config.GetRoutePermissions()
	assert.Equal(t, "/test1", originalPerms[0].Path)
}

func TestGetPublicPaths(t *testing.T) {
	config := NewAuthConfig()
	config.AddPublicPath("/public1")
	config.AddPublicPath("/public2")

	paths := config.GetPublicPaths()
	assert.Equal(t, 2, len(paths))

	// Verify it's a copy
	paths[0] = "/modified"
	originalPaths := config.GetPublicPaths()
	assert.Equal(t, "/public1", originalPaths[0])
}
