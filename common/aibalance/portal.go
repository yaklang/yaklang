package aibalance

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	uuid "github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed templates/portal.html templates/login.html templates/index.html templates/ops_portal.html templates/static/*
var templatesFS embed.FS

// ==================== Helper Functions ====================

// formatBytes converts bytes to human-readable format (KB, MB, GB, etc.)
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ==================== Data Types ====================

// ProviderData contains data for template rendering
type ProviderData struct {
	ID                uint
	WrapperName       string
	ModelName         string
	TypeName          string
	DomainOrURL       string
	APIKey            string
	TotalRequests     int64
	SuccessRate       float64
	LastLatency       int64
	IsHealthy         bool
	HealthStatusClass string // CSS class for health status (healthy, unhealthy, unknown)
}

// APIKeyData contains data for displaying an API key
type APIKeyData struct {
	ID                   uint
	Key                  string
	DisplayKey           string
	AllowedModels        string
	CreatedAt            string
	LastUsedAt           string
	UsageCount           int64
	SuccessCount         int64
	FailureCount         int64
	InputBytes           int64
	OutputBytes          int64
	InputBytesFormatted  string
	OutputBytesFormatted string
	Active               bool
	// Traffic limit fields
	TrafficLimit          int64
	TrafficUsed           int64
	TrafficLimitEnable    bool
	TrafficLimitFormatted string
	TrafficUsedFormatted  string
	TrafficPercent        float64
}

// PortalData contains all data for the management panel page
type PortalData struct {
	CurrentTime      string
	TotalProviders   int
	HealthyProviders int
	TotalRequests    int64
	SuccessRate      float64
	TotalTraffic     int64
	TotalTrafficStr  string
	Providers        []ProviderData
	AllowedModels    map[string]string
	APIKeys          []APIKeyData
	ModelMetas       []ModelInfo
	// TOTP related fields
	TOTPSecret  string
	TOTPWrapped string
	TOTPCode    string
}

// ModelInfo contains model metadata for display
type ModelInfo struct {
	Name              string
	Description       string
	Tags              string
	ProviderCount     int
	TrafficMultiplier float64
}

// ==================== Session Management ====================

// Session represents a user session (application level, not DB schema)
type Session struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionManager manages user sessions stored in the database
type SessionManager struct{}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

// CreateSession creates a new session and stores it in the database
// For backward compatibility, creates session for root admin
func (sm *SessionManager) CreateSession() string {
	return sm.CreateSessionWithRole(0, "root", "admin")
}

// CreateSessionWithRole creates a new session with user role information
func (sm *SessionManager) CreateSessionWithRole(userID uint, username, role string) string {
	sessionID := uuid.New().String()
	expiresAt := time.Now().Add(30 * time.Minute) // 30 minutes expiration

	dbSession := schema.LoginSession{
		SessionID: sessionID,
		ExpiresAt: expiresAt,
		UserID:    userID,
		Username:  username,
		UserRole:  role,
	}

	if err := GetDB().Create(&dbSession).Error; err != nil {
		log.Errorf("Failed to create session in database: %v", err)
		return ""
	}

	log.Infof("Created new session %s for user %s (role: %s), expires at %s",
		sessionID, username, role, expiresAt.Format(time.RFC3339))
	return sessionID
}

// GetSession retrieves a session from the database and checks its validity
func (sm *SessionManager) GetSession(sessionID string) *Session {
	var dbSession schema.LoginSession
	err := GetDB().Where("session_id = ?", sessionID).First(&dbSession).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Errorf("Error retrieving session %s from database: %v", sessionID, err)
		}
		return nil
	}

	// Check if session has expired
	if time.Now().After(dbSession.ExpiresAt) {
		log.Infof("Session %s has expired at %s, deleting.", sessionID, dbSession.ExpiresAt.Format(time.RFC3339))
		go sm.DeleteSession(sessionID)
		return nil
	}

	log.Debugf("Retrieved valid session %s", sessionID)
	return &Session{
		ID:        dbSession.SessionID,
		CreatedAt: dbSession.CreatedAt,
		ExpiresAt: dbSession.ExpiresAt,
	}
}

// DeleteSession removes a session from the database
func (sm *SessionManager) DeleteSession(sessionID string) {
	log.Infof("Deleting session %s from database", sessionID)
	result := GetDB().Where("session_id = ?", sessionID).Delete(&schema.LoginSession{})
	if result.Error != nil {
		log.Errorf("Failed to delete session %s from database: %v", sessionID, result.Error)
	} else if result.RowsAffected == 0 {
		log.Warnf("Attempted to delete session %s, but it was not found.", sessionID)
	} else {
		log.Infof("Successfully deleted session %s.", sessionID)
	}
}

// CleanupExpiredSessions removes expired sessions from the database
func (sm *SessionManager) CleanupExpiredSessions() {
	log.Infof("Running cleanup for expired sessions...")
	now := time.Now()
	result := GetDB().Where("expires_at < ?", now).Delete(&schema.LoginSession{})
	if result.Error != nil {
		log.Errorf("Error cleaning up expired sessions: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Infof("Cleaned up %d expired sessions.", result.RowsAffected)
	} else {
		log.Debugf("No expired sessions found to clean up.")
	}
}

// ==================== Authentication ====================

// checkAuth checks admin authentication using session ID from cookie
// For backward compatibility, returns bool only
func (c *ServerConfig) checkAuth(request *http.Request) bool {
	authInfo := c.getAuthInfo(request)
	return authInfo.Authenticated
}

// getAuthInfo returns detailed authentication information
// Uses AuthMiddleware if available, otherwise falls back to legacy behavior
func (c *ServerConfig) getAuthInfo(request *http.Request) *AuthInfo {
	// Use AuthMiddleware if available
	if c.AuthMiddleware != nil {
		return c.AuthMiddleware.GetAuthInfo(request)
	}

	// Legacy fallback
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
			log.Debugf("Auth via X-Ops-Key header for user: %s (legacy fallback)", opsUser.Username)
			return authInfo
		}
		if opsKey != "" {
			log.Warnf("Invalid or inactive X-Ops-Key: %s... (legacy fallback)", opsKey[:min(20, len(opsKey))])
		}
	}

	// Get session ID from cookie
	cookie, err := request.Cookie("admin_session")
	if err == nil && cookie.Value != "" {
		session := c.SessionManager.GetSession(cookie.Value)
		if session != nil {
			log.Debugf("Authentication successful via session cookie: %s", cookie.Value)
			authInfo.Authenticated = true
			authInfo.SessionID = cookie.Value
			authInfo.Username = "root"
			authInfo.Role = RoleAdmin
			return authInfo
		}
		log.Warnf("Invalid or expired session cookie found: %s", cookie.Value)
	} else if err != http.ErrNoCookie {
		log.Warnf("Error reading admin_session cookie: %v", err)
	}

	// Fallback: Get password authentication from query parameters
	query := request.URL.Query()
	password := query.Get("password")
	if c.AdminPassword != "" && password == c.AdminPassword {
		log.Infof("Authentication successful via query parameter password (one-time access).")
		authInfo.Authenticated = true
		authInfo.Username = "root"
		authInfo.Role = RoleAdmin
		return authInfo
	}

	log.Debugf("Authentication failed for request: %s", request.URL.Path)
	return authInfo
}

// checkPermission checks if the request has permission to access the path
// Returns: (allowed, authInfo, reason)
func (c *ServerConfig) checkPermission(request *http.Request, path string) (bool, *AuthInfo, string) {
	if c.AuthMiddleware != nil {
		return c.AuthMiddleware.CheckPermission(request, path)
	}
	// Legacy fallback - only admin auth
	authInfo := c.getAuthInfo(request)
	if authInfo.Authenticated {
		return true, authInfo, "legacy auth"
	}
	return false, authInfo, "not authenticated"
}

// ==================== Login/Logout Handlers ====================

// serveLoginPage displays the login page
func (c *ServerConfig) serveLoginPage(conn net.Conn) {
	c.logInfo("Serving login page")

	var tmpl *template.Template
	var err error

	// Try to read template from filesystem
	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/login.html",
		"templates/login.html",
		"../templates/login.html",
	); result != "" {
		rawTemp, err := os.ReadFile(result)
		if err != nil {
			c.logError("Failed to read login template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
		tmpl, err = template.New("login").Parse(string(rawTemp))
		if err != nil {
			c.logError("Failed to parse login template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to parse template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	} else {
		tmpl, err = template.ParseFS(templatesFS, "templates/login.html")
		if err != nil {
			c.logError("Failed to parse embedded login template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to parse template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, nil)
	if err != nil {
		c.logError("Failed to execute login template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to render template: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
		"\r\n"

	conn.Write([]byte(header))
	conn.Write(htmlBuffer.Bytes())
}

// processLogin handles login requests
func (c *ServerConfig) processLogin(conn net.Conn, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		c.logError("Failed to parse login form: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	password := request.PostForm.Get("password")
	if password == "" {
		log.Warnf("Received empty password during login attempt.")
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /portal?error=invalid_password\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	if password != c.AdminPassword {
		log.Infof("Invalid password: %s, origin: %s", password, c.AdminPassword)
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /portal?error=invalid_password\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	session := c.SessionManager.CreateSession()

	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal\r\n" +
		"Set-Cookie: admin_session=" + session + "; Path=/; HttpOnly; SameSite=Strict\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// handleLogout handles logout requests
func (c *ServerConfig) handleLogout(conn net.Conn, request *http.Request) {
	cookies := request.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "admin_session" {
			c.SessionManager.DeleteSession(cookie.Value)
			break
		}
	}

	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal\r\n" +
		"Set-Cookie: admin_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; HttpOnly; SameSite=Strict\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// ==================== Portal Page Rendering ====================

// servePortal handles requests for the management panel page
// Now serves static HTML - data is loaded via JavaScript from /portal/api/data
func (c *ServerConfig) servePortal(conn net.Conn) {
	c.logInfo("Serving portal page (static HTML)")

	// Try to read from local file first (for development)
	var htmlContent []byte
	var err error

	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/portal.html",
		"templates/portal.html",
		"../templates/portal.html",
	); result != "" {
		htmlContent, err = os.ReadFile(result)
		if err != nil {
			c.logError("Failed to read portal.html from file: %v", err)
			// Fall back to embedded
			htmlContent, err = templatesFS.ReadFile("templates/portal.html")
		}
	} else {
		htmlContent, err = templatesFS.ReadFile("templates/portal.html")
	}

	if err != nil {
		c.logError("Failed to read portal.html: %v", err)
		errorResponse := "HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to load portal page"
		conn.Write([]byte(errorResponse))
		return
	}

	header := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"Content-Type: text/html; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n", len(htmlContent))

	conn.Write([]byte(header))
	conn.Write(htmlContent)
}

// servePortalWithAuth handles management panel requests using session ID instead of password
func (c *ServerConfig) servePortalWithAuth(conn net.Conn) {
	c.servePortal(conn)
}

// ==================== Utility Functions ====================

// ForwarderDomain returns the list of domains configured for forwarding
func (c *ServerConfig) ForwarderDomain() []string {
	domains := make([]string, 0)
	for _, rule := range c.forwardRule.Values() {
		domains = append(domains, rule.SNI)
	}
	return domains
}

// writeJSONResponse sends a JSON-formatted response
func (c *ServerConfig) writeJSONResponse(conn net.Conn, statusCode int, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		c.logError("Failed to marshal JSON response: %v", err)
		errorHeader := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n", len(`{"error":"Internal server error"}`))
		conn.Write([]byte(errorHeader + `{"error":"Internal server error"}`))
		return
	}

	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = "Unknown"
	}

	header := fmt.Sprintf("HTTP/1.1 %d %s\r\n"+
		"Content-Type: application/json; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n",
		statusCode, statusText, len(jsonData))

	conn.Write([]byte(header))
	conn.Write(jsonData)
}

// redirectTo sends an HTTP 303 redirect response to the specified location
func (c *ServerConfig) redirectTo(conn net.Conn, location string) {
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: " + location + "\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// ==================== Main Request Router ====================

// HandlePortalRequest handles the main entry point for management portal requests
// Uses AuthMiddleware for permission checking
func (c *ServerConfig) HandlePortalRequest(conn net.Conn, request *http.Request, uriIns *url.URL) {
	c.logInfo("Processing portal request: %s %s", request.Method, uriIns.Path)

	// ========== Public Routes (No Auth Required) ==========

	// Process login POST request
	if uriIns.Path == "/portal/login" && request.Method == "POST" {
		c.processLogin(conn, request)
		return
	}

	// Serve static files (CSS, JS) without authentication
	if strings.HasPrefix(uriIns.Path, "/portal/static/") {
		c.serveStaticFile(conn, uriIns.Path)
		return
	}

	// ========== Permission Check via AuthMiddleware ==========

	allowed, authInfo, reason := c.checkPermission(request, uriIns.Path)

	// Log authentication info
	if authInfo.Authenticated {
		c.logDebug("Auth: user=%s, role=%s, path=%s", authInfo.Username, authInfo.Role, uriIns.Path)
	}

	// If not authenticated, redirect to login page
	if !authInfo.Authenticated {
		c.logInfo("Unauthenticated request to %s, redirecting to login", uriIns.Path)
		c.serveLoginPage(conn)
		return
	}

	// If authenticated but not allowed, handle role mismatch
	if !allowed {
		c.logWarn("Permission denied for user %s (role: %s) on %s %s: %s",
			authInfo.Username, authInfo.Role, request.Method, uriIns.Path, reason)

		// For page requests (not API), redirect to the appropriate portal based on role
		// This prevents infinite redirect loops when users access the wrong portal
		isPortalPage := uriIns.Path == "/portal" || uriIns.Path == "/portal/"
		isOpsPage := uriIns.Path == "/ops" || uriIns.Path == "/ops/"

		if isPortalPage && authInfo.Role == RoleOps {
			// OPS user trying to access admin portal, redirect to OPS portal
			c.logInfo("Redirecting OPS user %s from /portal to /ops", authInfo.Username)
			c.redirectTo(conn, "/ops")
			return
		}

		if isOpsPage && authInfo.Role == RoleAdmin {
			// Admin user trying to access OPS portal, redirect to admin portal
			c.logInfo("Redirecting Admin user %s from /ops to /portal", authInfo.Username)
			c.redirectTo(conn, "/portal")
			return
		}

		// For API requests or other cases, return 403 JSON response
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error":  "Permission denied",
			"reason": reason,
		})
		return
	}

	// ========== Route to Handler ==========
	// authInfo is available for handlers that need user context

	switch {
	// ========== Portal Home ==========
	case uriIns.Path == "/portal" || uriIns.Path == "/portal/":
		c.servePortalWithAuth(conn)

	// ========== Provider Routes ==========
	case uriIns.Path == "/portal/add-ai-provider":
		c.serveAddProviderPage(conn, request)
	case uriIns.Path == "/portal/add-providers" && request.Method == "POST":
		c.processAddProviders(conn, request)
	case uriIns.Path == "/portal/validate-provider" && request.Method == "POST":
		c.handleValidateProvider(conn, request)
	case uriIns.Path == "/portal/autocomplete":
		c.serveAutoCompleteData(conn, request)
	case uriIns.Path == "/portal/api/providers":
		c.serveProvidersAPI(conn, request)
	case strings.HasPrefix(uriIns.Path, "/portal/delete-provider/") && request.Method == "DELETE":
		c.handleDeleteProvider(conn, request, uriIns.Path)
	case uriIns.Path == "/portal/delete-providers" && request.Method == "POST":
		c.handleDeleteMultipleProviders(conn, request)

	// ========== Health Check Routes ==========
	case uriIns.Path == "/portal/api/health-check":
		c.serveHealthCheckAPI(conn, request)
	case uriIns.Path == "/portal/check-all-health" && request.Method == "POST":
		c.handleCheckAllHealth(conn, request)
	case strings.HasPrefix(uriIns.Path, "/portal/check-health/") && request.Method == "POST":
		c.handleCheckSingleHealth(conn, request, uriIns.Path)

	// ========== API Key Routes ==========
	case uriIns.Path == "/portal/api-keys":
		c.serveAPIKeysPage(conn)
	case uriIns.Path == "/portal/create-api-key" && request.Method == "POST":
		c.handleGenerateApiKey(conn, request)
	case uriIns.Path == "/portal/generate-api-key" && request.Method == "POST":
		c.handleGenerateApiKey(conn, request)
	case uriIns.Path == "/portal/api/api-keys":
		c.handleGetAPIKeysPaginated(conn, request)
	case strings.HasPrefix(uriIns.Path, "/portal/activate-api-key/") && request.Method == "POST":
		c.handleToggleAPIKeyStatus(conn, request, uriIns.Path, true)
	case strings.HasPrefix(uriIns.Path, "/portal/deactivate-api-key/") && request.Method == "POST":
		c.handleToggleAPIKeyStatus(conn, request, uriIns.Path, false)
	case uriIns.Path == "/portal/batch-activate-api-keys" && request.Method == "POST":
		c.handleBatchToggleAPIKeyStatus(conn, request, true)
	case uriIns.Path == "/portal/batch-deactivate-api-keys" && request.Method == "POST":
		c.handleBatchToggleAPIKeyStatus(conn, request, false)
	case strings.HasPrefix(uriIns.Path, "/portal/update-api-key-allowed-models/") && request.Method == "POST":
		c.handleUpdateAPIKeyAllowedModels(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/delete-api-key/") && request.Method == "DELETE":
		c.handleDeleteAPIKey(conn, request, uriIns.Path)
	case uriIns.Path == "/portal/delete-api-keys" && request.Method == "POST":
		c.handleBatchDeleteAPIKeys(conn, request)
	case strings.HasPrefix(uriIns.Path, "/portal/api-key-traffic-limit/") && request.Method == "POST":
		c.handleUpdateAPIKeyTrafficLimit(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/reset-api-key-traffic/") && request.Method == "POST":
		c.handleResetAPIKeyTraffic(conn, request, uriIns.Path)

	// ========== Web Search API Key Routes ==========
	case uriIns.Path == "/portal/api/web-search-keys" && request.Method == "GET":
		c.handleGetWebSearchApiKeys(conn, request)
	case uriIns.Path == "/portal/api/web-search-keys" && request.Method == "POST":
		c.handleCreateWebSearchApiKey(conn, request)
	case strings.HasPrefix(uriIns.Path, "/portal/api/web-search-keys/") && request.Method == "DELETE":
		c.handleDeleteWebSearchApiKey(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/api/web-search-keys/") && request.Method == "PUT":
		c.handleUpdateWebSearchApiKey(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/activate-web-search-key/") && request.Method == "POST":
		c.handleToggleWebSearchApiKeyStatus(conn, request, uriIns.Path, true)
	case strings.HasPrefix(uriIns.Path, "/portal/deactivate-web-search-key/") && request.Method == "POST":
		c.handleToggleWebSearchApiKeyStatus(conn, request, uriIns.Path, false)
	case strings.HasPrefix(uriIns.Path, "/portal/reset-web-search-key-health/") && request.Method == "POST":
		c.handleResetWebSearchApiKeyHealth(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/test-web-search-key/") && request.Method == "POST":
		c.handleTestWebSearchApiKey(conn, request, uriIns.Path)
	case uriIns.Path == "/portal/api/web-search-config" && request.Method == "GET":
		c.handleGetWebSearchConfig(conn, request)
	case uriIns.Path == "/portal/api/web-search-config" && request.Method == "POST":
		c.handleSetWebSearchConfig(conn, request)

	// ========== Amap API Key Routes ==========
	case uriIns.Path == "/portal/api/amap-keys" && request.Method == "GET":
		c.handleGetAmapApiKeys(conn, request)
	case uriIns.Path == "/portal/api/amap-keys" && request.Method == "POST":
		c.handleCreateAmapApiKey(conn, request)
	case strings.HasPrefix(uriIns.Path, "/portal/api/amap-keys/") && request.Method == "DELETE":
		c.handleDeleteAmapApiKey(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/toggle-amap-key/") && request.Method == "POST":
		c.handleToggleAmapApiKeyStatus(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/reset-amap-key-health/") && request.Method == "POST":
		c.handleResetAmapApiKeyHealth(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/test-amap-key/") && request.Method == "POST":
		c.handleTestAmapApiKey(conn, request, uriIns.Path)
	case uriIns.Path == "/portal/api/amap-keys/check-all" && request.Method == "POST":
		c.handleCheckAllAmapApiKeys(conn, request)
	case uriIns.Path == "/portal/api/amap-config" && request.Method == "GET":
		c.handleGetAmapConfig(conn, request)
	case uriIns.Path == "/portal/api/amap-config" && request.Method == "POST":
		c.handleSetAmapConfig(conn, request)

	// ========== Model Metadata Routes ==========
	case uriIns.Path == "/portal/update-model-meta" && request.Method == "POST":
		c.handleUpdateModelMeta(conn, request)

	// ========== TOTP Routes ==========
	case uriIns.Path == "/portal/totp-settings":
		c.serveTOTPSettingsPage(conn)
	case uriIns.Path == "/portal/refresh-totp" && request.Method == "POST":
		c.handleRefreshTOTP(conn, request)
	case uriIns.Path == "/portal/get-totp-code":
		c.handleGetTOTPCode(conn, request)

	// ========== Portal Data API Routes ==========
	case uriIns.Path == "/portal/api/data":
		c.servePortalDataAPI(conn, request)
	case uriIns.Path == "/portal/api/models":
		c.serveAvailableModelsAPI(conn, request)

	// ========== System Monitoring Routes ==========
	case uriIns.Path == "/portal/api/memory-stats":
		c.handleMemoryStatsAPI(conn, request)
	case uriIns.Path == "/portal/api/force-gc" && request.Method == "POST":
		c.handleForceGCAPI(conn, request)
	case uriIns.Path == "/portal/api/goroutine-dump":
		c.handleGoroutineDumpAPI(conn, request)

	// ========== Session Routes ==========
	case uriIns.Path == "/portal/logout":
		c.handleLogout(conn, request)

	// ========== OPS User Management Routes (Admin Only) ==========
	case uriIns.Path == "/portal/api/ops-users" && request.Method == "GET":
		c.handleListOpsUsers(conn, request)
	case uriIns.Path == "/portal/api/ops-users" && request.Method == "POST":
		c.handleCreateOpsUser(conn, request)
	case strings.HasPrefix(uriIns.Path, "/portal/api/ops-users/") && request.Method == "DELETE":
		c.handleDeleteOpsUser(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/api/ops-users/") && request.Method == "PUT":
		c.handleUpdateOpsUser(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/api/ops-users/") && strings.HasSuffix(uriIns.Path, "/reset-password") && request.Method == "POST":
		c.handleResetOpsUserPassword(conn, request, uriIns.Path)
	case strings.HasPrefix(uriIns.Path, "/portal/api/ops-users/") && strings.HasSuffix(uriIns.Path, "/reset-key") && request.Method == "POST":
		c.handleResetOpsUserKey(conn, request, uriIns.Path)

	// ========== OPS Logs and Stats Routes (Admin Only) ==========
	case uriIns.Path == "/portal/api/ops-logs" && request.Method == "GET":
		c.handleGetOpsLogs(conn, request)
	case uriIns.Path == "/portal/api/ops-stats" && request.Method == "GET":
		c.handleGetOpsStats(conn, request)

	// ========== Default ==========
	default:
		c.servePortalWithAuth(conn)
	}
}

// ==================== OPS Portal Handlers ====================

// HandleOpsPortalRequest handles the main entry point for OPS portal requests
func (c *ServerConfig) HandleOpsPortalRequest(conn net.Conn, request *http.Request, uriIns *url.URL) {
	c.logInfo("Processing OPS portal request: %s %s", request.Method, uriIns.Path)

	// ========== Public Routes (No Auth Required) ==========

	// Process OPS login POST request
	if uriIns.Path == "/ops/login" && request.Method == "POST" {
		c.processOpsLogin(conn, request)
		return
	}

	// Serve OPS login page
	if uriIns.Path == "/ops/login" && request.Method == "GET" {
		c.serveOpsLoginPage(conn)
		return
	}

	// Serve static files (CSS, JS) without authentication
	if strings.HasPrefix(uriIns.Path, "/ops/static/") {
		c.serveStaticFile(conn, uriIns.Path)
		return
	}

	// ========== Permission Check via AuthMiddleware ==========

	allowed, authInfo, reason := c.checkPermission(request, uriIns.Path)

	// Log authentication info
	if authInfo.Authenticated {
		c.logDebug("OPS Auth: user=%s, role=%s, path=%s", authInfo.Username, authInfo.Role, uriIns.Path)
	}

	// If not authenticated, redirect to OPS login page
	if !authInfo.Authenticated {
		c.logInfo("Unauthenticated OPS request to %s, redirecting to login", uriIns.Path)
		c.serveOpsLoginPage(conn)
		return
	}

	// If authenticated but not allowed (wrong role), return 403 Forbidden
	if !allowed {
		c.logWarn("Permission denied for OPS user %s (role: %s) on %s %s: %s",
			authInfo.Username, authInfo.Role, request.Method, uriIns.Path, reason)
		c.writeJSONResponse(conn, http.StatusForbidden, map[string]string{
			"error":  "Permission denied",
			"reason": reason,
		})
		return
	}

	// ========== Route to Handler ==========

	switch {
	// ========== OPS Dashboard ==========
	case uriIns.Path == "/ops" || uriIns.Path == "/ops/" || uriIns.Path == "/ops/dashboard":
		c.serveOpsPortal(conn)

	// ========== OPS API Key Management ==========
	case (uriIns.Path == "/ops/create-api-key" || uriIns.Path == "/ops/api/create-api-key") && request.Method == "POST":
		c.handleOpsCreateApiKey(conn, request, authInfo)
	case uriIns.Path == "/ops/api/my-keys" && request.Method == "GET":
		c.handleOpsGetMyKeys(conn, request, authInfo)
	case uriIns.Path == "/ops/api/delete-api-key" && request.Method == "POST":
		c.handleOpsDeleteApiKey(conn, request, authInfo)
	case uriIns.Path == "/ops/api/update-api-key" && request.Method == "POST":
		c.handleOpsUpdateApiKey(conn, request, authInfo)
	case uriIns.Path == "/ops/api/reset-traffic" && request.Method == "POST":
		c.handleOpsResetApiKeyTraffic(conn, request, authInfo)

	// ========== OPS Self-Service ==========
	case uriIns.Path == "/ops/my-info" && request.Method == "GET":
		c.handleOpsGetMyInfo(conn, request)
	case uriIns.Path == "/ops/change-password" && request.Method == "POST":
		c.handleOpsChangePassword(conn, request)
	case uriIns.Path == "/ops/reset-key" && request.Method == "POST":
		c.handleOpsResetOwnKey(conn, request)

	// ========== OPS Logout ==========
	case uriIns.Path == "/ops/logout":
		c.handleOpsLogout(conn, request)

	// ========== Default ==========
	default:
		c.serveOpsPortal(conn)
	}
}

// serveOpsLoginPage displays the OPS login page
// Uses the unified login.html template which supports both Admin and OPS login
func (c *ServerConfig) serveOpsLoginPage(conn net.Conn) {
	c.logInfo("Serving OPS login page (using unified login template)")

	// Use the same login page as admin portal
	// The page will auto-switch to OPS tab based on URL path
	c.serveLoginPage(conn)
}

// processOpsLogin handles OPS user login requests
func (c *ServerConfig) processOpsLogin(conn net.Conn, request *http.Request) {
	err := request.ParseForm()
	if err != nil {
		c.logError("Failed to parse OPS login form: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	username := request.PostForm.Get("username")
	password := request.PostForm.Get("password")

	if username == "" || password == "" {
		log.Warnf("Received empty username or password during OPS login attempt")
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /ops/login?error=invalid_credentials\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	// Get OPS user from database
	user, err := GetOpsUserByUsername(username)
	if err != nil {
		log.Warnf("OPS user not found: %s", username)
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /ops/login?error=invalid_credentials\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	// Check if user is active
	if !user.Active {
		log.Warnf("OPS user is inactive: %s", username)
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /ops/login?error=account_disabled\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	// Verify password
	if !CheckPassword(user.Password, password) {
		log.Warnf("Invalid password for OPS user: %s", username)
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /ops/login?error=invalid_credentials\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	// Create session with role
	session := c.SessionManager.CreateSessionWithRole(user.ID, user.Username, user.Role)
	if session == "" {
		c.logError("Failed to create session for OPS user: %s", username)
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /ops/login?error=server_error\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	log.Infof("OPS user logged in successfully: %s (ID: %d)", username, user.ID)

	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /ops/dashboard\r\n" +
		"Set-Cookie: ops_session=" + session + "; Path=/; HttpOnly; SameSite=Strict\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// serveOpsPortal serves the OPS portal dashboard page
func (c *ServerConfig) serveOpsPortal(conn net.Conn) {
	c.logInfo("Serving OPS portal page")

	var htmlContent []byte
	var err error

	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/ops_portal.html",
		"templates/ops_portal.html",
		"../templates/ops_portal.html",
	); result != "" {
		htmlContent, err = os.ReadFile(result)
		if err != nil {
			c.logError("Failed to read ops_portal.html from file: %v", err)
			htmlContent, err = templatesFS.ReadFile("templates/ops_portal.html")
		}
	} else {
		htmlContent, err = templatesFS.ReadFile("templates/ops_portal.html")
	}

	if err != nil {
		c.logError("Failed to read ops_portal.html: %v", err)
		errorResponse := "HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to load OPS portal page"
		conn.Write([]byte(errorResponse))
		return
	}

	header := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"Content-Type: text/html; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n", len(htmlContent))

	conn.Write([]byte(header))
	conn.Write(htmlContent)
}

// handleOpsLogout handles OPS user logout
func (c *ServerConfig) handleOpsLogout(conn net.Conn, request *http.Request) {
	cookies := request.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "ops_session" {
			c.SessionManager.DeleteSession(cookie.Value)
			break
		}
	}

	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /ops/login\r\n" +
		"Set-Cookie: ops_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; HttpOnly; SameSite=Strict\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}
