package aibalance

import (
	"net/http"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// ==================== User Roles ====================

// UserRole represents user role type
type UserRole string

const (
	RoleAdmin UserRole = "admin" // Super administrator (root)
	RoleOps   UserRole = "ops"   // Operations user
	RoleNone  UserRole = ""      // Not authenticated
)

// IsValid checks if the role is valid
func (r UserRole) IsValid() bool {
	return r == RoleAdmin || r == RoleOps
}

// String returns the string representation of the role
func (r UserRole) String() string {
	return string(r)
}

// ==================== Auth Info ====================

// AuthInfo contains authentication information returned by checkAuth
type AuthInfo struct {
	Authenticated bool     // Whether the user is authenticated
	UserID        uint     // User ID (0 for root admin)
	Username      string   // Username
	Role          UserRole // User role
	SessionID     string   // Session ID
}

// IsAdmin checks if the user is an admin
func (a *AuthInfo) IsAdmin() bool {
	return a.Authenticated && a.Role == RoleAdmin
}

// IsOps checks if the user is an ops user
func (a *AuthInfo) IsOps() bool {
	return a.Authenticated && a.Role == RoleOps
}

// HasRole checks if the user has one of the specified roles
func (a *AuthInfo) HasRole(roles ...UserRole) bool {
	if !a.Authenticated {
		return false
	}
	for _, r := range roles {
		if a.Role == r {
			return true
		}
	}
	return false
}

// ==================== Route Permission ====================

// RoutePermission defines permission configuration for a single route
type RoutePermission struct {
	Path         string     // Route path (supports prefix matching with "*")
	Method       string     // HTTP method ("GET", "POST", "*" for all)
	AllowedRoles []UserRole // Allowed roles list
	RequireAuth  bool       // Whether authentication is required
	Description  string     // Route description (for logging and documentation)
}

// Matches checks if this permission matches the given path and method
func (rp *RoutePermission) Matches(path, method string) bool {
	// Check method
	if rp.Method != "*" && rp.Method != method {
		return false
	}

	// Check path
	if strings.HasSuffix(rp.Path, "/*") {
		// Prefix matching
		prefix := strings.TrimSuffix(rp.Path, "/*")
		return strings.HasPrefix(path, prefix)
	} else if strings.HasSuffix(rp.Path, "*") {
		// Prefix matching (alternative format)
		prefix := strings.TrimSuffix(rp.Path, "*")
		return strings.HasPrefix(path, prefix)
	}

	// Exact matching
	return rp.Path == path
}

// AllowsRole checks if this permission allows the given role
func (rp *RoutePermission) AllowsRole(role UserRole) bool {
	for _, r := range rp.AllowedRoles {
		if r == role {
			return true
		}
	}
	return false
}

// ==================== Auth Config ====================

// AuthConfig contains the complete authentication configuration
type AuthConfig struct {
	Routes      []RoutePermission // Route permission configurations
	DefaultDeny bool              // Deny access to unconfigured routes by default
	PublicPaths []string          // Public paths that don't require authentication
	mu          sync.RWMutex      // Mutex for thread-safe access
}

// NewAuthConfig creates a new AuthConfig with default settings
func NewAuthConfig() *AuthConfig {
	return &AuthConfig{
		Routes:      []RoutePermission{},
		DefaultDeny: true,
		PublicPaths: []string{},
	}
}

// AddRoute adds a route permission configuration
func (ac *AuthConfig) AddRoute(perm RoutePermission) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.Routes = append(ac.Routes, perm)
}

// AddPublicPath adds a public path
func (ac *AuthConfig) AddPublicPath(path string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.PublicPaths = append(ac.PublicPaths, path)
}

// IsPublicPath checks if the path is public (no auth required)
func (ac *AuthConfig) IsPublicPath(path string) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	for _, publicPath := range ac.PublicPaths {
		if strings.HasSuffix(publicPath, "/*") {
			prefix := strings.TrimSuffix(publicPath, "/*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		} else if strings.HasSuffix(publicPath, "*") {
			prefix := strings.TrimSuffix(publicPath, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		} else if publicPath == path {
			return true
		}
	}
	return false
}

// FindMatchingPermission finds the first matching permission for the path and method
func (ac *AuthConfig) FindMatchingPermission(path, method string) *RoutePermission {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	for i := range ac.Routes {
		if ac.Routes[i].Matches(path, method) {
			return &ac.Routes[i]
		}
	}
	return nil
}

// ==================== Auth Middleware ====================

// AuthMiddleware handles authentication and authorization
type AuthMiddleware struct {
	config *AuthConfig
	server *ServerConfig
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(server *ServerConfig, config *AuthConfig) *AuthMiddleware {
	if config == nil {
		config = DefaultAuthConfig()
	}
	return &AuthMiddleware{
		config: config,
		server: server,
	}
}

// GetConfig returns the auth configuration
func (m *AuthMiddleware) GetConfig() *AuthConfig {
	return m.config
}

// GetAuthInfo extracts authentication information from the request
// This does NOT check permissions, only extracts user info
func (m *AuthMiddleware) GetAuthInfo(request *http.Request) *AuthInfo {
	authInfo := &AuthInfo{
		Authenticated: false,
		UserID:        0,
		Username:      "",
		Role:          RoleNone,
		SessionID:     "",
	}

	// Try to get session from cookie
	cookie, err := request.Cookie("admin_session")
	if err != nil || cookie.Value == "" {
		// Try ops_session cookie
		cookie, err = request.Cookie("ops_session")
		if err != nil || cookie.Value == "" {
			// Check fallback password authentication (for backward compatibility)
			query := request.URL.Query()
			password := query.Get("password")
			if m.server.AdminPassword != "" && password == m.server.AdminPassword {
				authInfo.Authenticated = true
				authInfo.Username = "root"
				authInfo.Role = RoleAdmin
				authInfo.UserID = 0
				log.Debugf("Auth via query password for root admin")
				return authInfo
			}
			return authInfo
		}
	}

	sessionID := cookie.Value
	authInfo.SessionID = sessionID

	// Get session from database
	var dbSession schema.LoginSession
	err = GetDB().Where("session_id = ?", sessionID).First(&dbSession).Error
	if err != nil {
		log.Debugf("Session not found: %s", sessionID)
		return authInfo
	}

	// Check if session has expired
	// Note: We rely on SessionManager to clean up expired sessions

	// Extract user info from session
	authInfo.Authenticated = true
	authInfo.UserID = dbSession.UserID
	authInfo.Username = dbSession.Username
	authInfo.Role = UserRole(dbSession.UserRole)

	// Default to admin for legacy sessions without UserRole
	if authInfo.Role == "" {
		authInfo.Role = RoleAdmin
		authInfo.Username = "root"
	}

	log.Debugf("Auth info extracted: user=%s, role=%s, session=%s",
		authInfo.Username, authInfo.Role, sessionID)

	return authInfo
}

// CheckPermission checks if the request has permission to access the path
// Returns: (allowed, authInfo, reason)
func (m *AuthMiddleware) CheckPermission(request *http.Request, path string) (bool, *AuthInfo, string) {
	method := request.Method

	// Check if it's a public path
	if m.config.IsPublicPath(path) {
		return true, &AuthInfo{Authenticated: false}, "public path"
	}

	// Get auth info
	authInfo := m.GetAuthInfo(request)

	// Find matching permission
	perm := m.config.FindMatchingPermission(path, method)

	// If no matching permission found
	if perm == nil {
		if m.config.DefaultDeny {
			// Default deny - require admin for unconfigured routes
			if authInfo.IsAdmin() {
				log.Debugf("Unconfigured route %s %s allowed for admin", method, path)
				return true, authInfo, "admin access to unconfigured route"
			}
			log.Warnf("Unconfigured route %s %s denied (default deny)", method, path)
			return false, authInfo, "route not configured and default deny is enabled"
		}
		// Default allow - just need authentication
		if authInfo.Authenticated {
			return true, authInfo, "authenticated (default allow)"
		}
		return false, authInfo, "not authenticated"
	}

	// Check if authentication is required
	if !perm.RequireAuth {
		return true, authInfo, "no auth required"
	}

	// Check if user is authenticated
	if !authInfo.Authenticated {
		return false, authInfo, "authentication required"
	}

	// Check role
	if !perm.AllowsRole(authInfo.Role) {
		log.Warnf("Permission denied for %s %s: user=%s, role=%s, allowed_roles=%v",
			method, path, authInfo.Username, authInfo.Role, perm.AllowedRoles)
		return false, authInfo, "insufficient permissions"
	}

	log.Debugf("Permission granted for %s %s: user=%s, role=%s",
		method, path, authInfo.Username, authInfo.Role)
	return true, authInfo, "permission granted"
}

// IsPublicPath checks if the path is public
func (m *AuthMiddleware) IsPublicPath(path string) bool {
	return m.config.IsPublicPath(path)
}

// ==================== Default Configuration ====================

// DefaultAuthConfig returns the default authentication configuration
func DefaultAuthConfig() *AuthConfig {
	config := NewAuthConfig()

	// ========== Public Paths (No Authentication Required) ==========
	config.PublicPaths = []string{
		"/portal/login",
		"/portal/static/*",
		"/ops/login",
		"/ops/static/*",
	}

	// ========== Admin Only Routes ==========
	adminOnlyRoutes := []RoutePermission{
		// Provider management
		{Path: "/portal/add-providers", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Add providers"},
		{Path: "/portal/add-ai-provider", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Add provider page"},
		{Path: "/portal/validate-provider", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Validate provider"},
		{Path: "/portal/delete-provider/*", Method: "DELETE", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Delete provider"},
		{Path: "/portal/delete-providers", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Delete multiple providers"},
		{Path: "/portal/api/providers", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Get providers API"},
		{Path: "/portal/autocomplete", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Autocomplete data"},

		// Health check
		{Path: "/portal/api/health-check", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Health check API"},
		{Path: "/portal/check-all-health", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Check all health"},
		{Path: "/portal/check-health/*", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Check single health"},

		// API Key management (admin has full control)
		{Path: "/portal/api-keys", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "API keys page"},
		{Path: "/portal/create-api-key", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Create API key"},
		{Path: "/portal/generate-api-key", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Generate API key"},
		{Path: "/portal/api/api-keys", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "API keys list"},
		{Path: "/portal/activate-api-key/*", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Activate API key"},
		{Path: "/portal/deactivate-api-key/*", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Deactivate API key"},
		{Path: "/portal/batch-activate-api-keys", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Batch activate API keys"},
		{Path: "/portal/batch-deactivate-api-keys", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Batch deactivate API keys"},
		{Path: "/portal/update-api-key-allowed-models/*", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Update API key allowed models"},
		{Path: "/portal/delete-api-key/*", Method: "DELETE", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Delete API key"},
		{Path: "/portal/delete-api-keys", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Batch delete API keys"},
		{Path: "/portal/api-key-traffic-limit/*", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Set API key traffic limit"},
		{Path: "/portal/reset-api-key-traffic/*", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Reset API key traffic"},

		// Model metadata
		{Path: "/portal/update-model-meta", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Update model metadata"},

		// TOTP settings
		{Path: "/portal/totp-settings", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "TOTP settings page"},
		{Path: "/portal/refresh-totp", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Refresh TOTP"},
		{Path: "/portal/get-totp-code", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Get TOTP code"},

		// Portal data
		{Path: "/portal/api/data", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Portal data API"},
		{Path: "/portal/api/models", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Available models API"},

		// System monitoring
		{Path: "/portal/api/memory-stats", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Memory stats API"},
		{Path: "/portal/api/force-gc", Method: "POST", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Force GC API"},
		{Path: "/portal/api/goroutine-dump", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Goroutine dump API"},

		// Portal home
		{Path: "/portal", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Portal home"},
		{Path: "/portal/", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Portal home"},

		// OPS user management (admin only)
		{Path: "/portal/ops-users", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "OPS users page"},
		{Path: "/portal/api/ops-users", Method: "*", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "OPS users CRUD API"},
		{Path: "/portal/api/ops-logs", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "OPS logs API"},
		{Path: "/portal/api/ops-stats", Method: "GET", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "OPS stats API"},

		// Logout (admin)
		{Path: "/portal/logout", Method: "*", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Admin logout"},
	}

	// ========== OPS Routes ==========
	opsRoutes := []RoutePermission{
		// OPS dashboard and info
		{Path: "/ops/dashboard", Method: "GET", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS dashboard"},
		{Path: "/ops/", Method: "GET", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS home"},
		{Path: "/ops/my-info", Method: "GET", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS user info"},

		// OPS API key management (with traffic limit enforced)
		{Path: "/ops/create-api-key", Method: "POST", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS create API key"},
		{Path: "/ops/api/create-api-key", Method: "POST", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS create API key API"},
		{Path: "/ops/api/my-keys", Method: "GET", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS get my API keys"},
		{Path: "/ops/api/delete-api-key", Method: "POST", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS delete API key"},

		// OPS self-service
		{Path: "/ops/change-password", Method: "POST", AllowedRoles: []UserRole{RoleOps, RoleAdmin}, RequireAuth: true, Description: "Change password"},
		{Path: "/ops/reset-key", Method: "POST", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "Reset OPS key"},

		// OPS logout
		{Path: "/ops/logout", Method: "*", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS logout"},
	}

	// Add all routes
	for _, route := range adminOnlyRoutes {
		config.AddRoute(route)
	}
	for _, route := range opsRoutes {
		config.AddRoute(route)
	}

	return config
}

// ==================== Helper Functions ====================

// GetRoutePermissions returns all configured route permissions (for debugging/testing)
func (ac *AuthConfig) GetRoutePermissions() []RoutePermission {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	result := make([]RoutePermission, len(ac.Routes))
	copy(result, ac.Routes)
	return result
}

// GetPublicPaths returns all configured public paths (for debugging/testing)
func (ac *AuthConfig) GetPublicPaths() []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	result := make([]string, len(ac.PublicPaths))
	copy(result, ac.PublicPaths)
	return result
}

// CountRoutes returns the number of configured routes
func (ac *AuthConfig) CountRoutes() int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return len(ac.Routes)
}
