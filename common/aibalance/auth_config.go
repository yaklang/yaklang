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

	// First, check X-Ops-Key header for OPS user API authentication
	opsKey := request.Header.Get("X-Ops-Key")
	if opsKey != "" {
		// Validate the OPS key and get the OPS user
		opsUser, err := GetOpsUserByOpsKey(opsKey)
		if err == nil && opsUser != nil && opsUser.Active {
			authInfo.Authenticated = true
			authInfo.UserID = opsUser.ID
			authInfo.Username = opsUser.Username
			authInfo.Role = RoleOps
			log.Debugf("Auth via X-Ops-Key header for user: %s", opsUser.Username)
			return authInfo
		}
		log.Warnf("Invalid or inactive X-Ops-Key: %s...", opsKey[:min(20, len(opsKey))])
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
// Uses wildcard patterns for simplified permission management:
// - /portal/** -> Admin only (except public paths)
// - /ops/** -> OPS users (except public paths)
func DefaultAuthConfig() *AuthConfig {
	config := NewAuthConfig()

	// ========== Public Paths (No Authentication Required) ==========
	// These are checked BEFORE route permissions
	config.PublicPaths = []string{
		"/portal/login",
		"/portal/static/*",
		"/ops/login",
		"/ops/static/*",
	}

	// ========== Route Permissions ==========
	// NOTE: Routes are matched in order, first match wins!
	// Specific routes should be defined before wildcard routes.

	routes := []RoutePermission{
		// ========== Special Routes (Multi-Role or Specific Handling) ==========
		// OPS change password (both OPS and Admin can access)
		{Path: "/ops/change-password", Method: "POST", AllowedRoles: []UserRole{RoleOps, RoleAdmin}, RequireAuth: true, Description: "Change password (OPS/Admin)"},

		// ========== Portal Wildcard (Admin Only) ==========
		// All /portal/** routes require Admin role
		{Path: "/portal/*", Method: "*", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Admin Portal (all routes)"},
		{Path: "/portal", Method: "*", AllowedRoles: []UserRole{RoleAdmin}, RequireAuth: true, Description: "Admin Portal home"},

		// ========== OPS Wildcard (OPS Users) ==========
		// All /ops/** routes require OPS role
		{Path: "/ops/*", Method: "*", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS Portal (all routes)"},
		{Path: "/ops", Method: "*", AllowedRoles: []UserRole{RoleOps}, RequireAuth: true, Description: "OPS Portal home"},
	}

	// Add all routes
	for _, route := range routes {
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
