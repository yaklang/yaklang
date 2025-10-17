package aibalance

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	uuid "github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed templates/portal.html templates/login.html templates/index.html
var templatesFS embed.FS

// formatBytes 将字节大小转换为人类可读的格式（KB、MB、GB等）
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

// ProviderData contains data for template rendering
type ProviderData struct {
	ID                uint
	WrapperName       string
	ModelName         string
	TypeName          string
	DomainOrURL       string
	APIKey            string // 添加 APIKey 字段
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
}

// PortalData contains all data for the management panel page
type PortalData struct {
	CurrentTime      string
	TotalProviders   int
	HealthyProviders int
	TotalRequests    int64
	SuccessRate      float64
	Providers        []ProviderData
	AllowedModels    map[string]string
	APIKeys          []APIKeyData
}

// Session represents a user session (application level, not DB schema)
type Session struct {
	ID        string    // Session ID
	CreatedAt time.Time // Creation time
	ExpiresAt time.Time // Expiration time
}

// SessionManager manages user sessions stored in the database
type SessionManager struct {
	// sessions map and mutex are removed as we use the database now
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{} // No in-memory map to initialize
}

// CreateSession creates a new session and stores it in the database
func (sm *SessionManager) CreateSession() string {
	sessionID := uuid.New().String()
	expiresAt := time.Now().Add(30 * time.Minute) // 30 minutes expiration

	dbSession := schema.LoginSession{
		SessionID: sessionID,
		ExpiresAt: expiresAt,
	}

	// Save to database
	// Assume GetDB() returns a valid *gorm.DB instance
	if err := GetDB().Create(&dbSession).Error; err != nil {
		log.Errorf("Failed to create session in database: %v", err)
		// In a real-world scenario, you might want to handle this error more gracefully
		// For now, we'll log it and return an empty string or potentially panic
		return "" // Indicate failure
	}

	log.Infof("Created new sessio1n %s, expires at %s", sessionID, expiresAt.Format(time.RFC3339))
	return sessionID
}

// GetSession retrieves a session from the database and checks its validity
func (sm *SessionManager) GetSession(sessionID string) *Session {
	var dbSession schema.LoginSession
	// Retrieve from database
	err := GetDB().Where("session_id = ?", sessionID).First(&dbSession).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Errorf("Error retrieving session %s from database: %v", sessionID, err)
		}
		// If not found or other error, return nil
		return nil
	}

	// Check if session has expired
	if time.Now().After(dbSession.ExpiresAt) {
		log.Infof("Session %s has expired at %s, deleting.", sessionID, dbSession.ExpiresAt.Format(time.RFC3339))
		// Delete expired session asynchronously to not block the request
		go sm.DeleteSession(sessionID) // Run deletion in a separate goroutine
		return nil
	}

	log.Debugf("Retrieved valid session %s", sessionID)
	// Return the application-level Session struct
	return &Session{
		ID:        dbSession.SessionID,
		CreatedAt: dbSession.CreatedAt, // Use GORM's CreatedAt
		ExpiresAt: dbSession.ExpiresAt,
	}
}

// DeleteSession removes a session from the database
func (sm *SessionManager) DeleteSession(sessionID string) {
	log.Infof("Deleting session %s from database", sessionID)
	// Delete from database
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
	// Delete expired sessions from database
	result := GetDB().Where("expires_at < ?", now).Delete(&schema.LoginSession{})
	if result.Error != nil {
		log.Errorf("Error cleaning up expired sessions: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Infof("Cleaned up %d expired sessions.", result.RowsAffected)
	} else {
		log.Debugf("No expired sessions found to clean up.")
	}
}

// checkAuth checks admin authentication using session ID from cookie
func (c *ServerConfig) checkAuth(request *http.Request) bool {
	// Get session ID from cookie
	cookie, err := request.Cookie("admin_session")
	if err == nil && cookie.Value != "" {
		// Validate session using the database-backed SessionManager
		session := c.SessionManager.GetSession(cookie.Value)
		if session != nil {
			// Session is valid
			log.Debugf("Authentication successful via session cookie: %s", cookie.Value)
			return true
		}
		log.Warnf("Invalid or expired session cookie found: %s", cookie.Value)
	} else if err != http.ErrNoCookie {
		// Log error only if it's not ErrNoCookie
		log.Warnf("Error reading admin_session cookie: %v", err)
	}

	// Fallback: Get password authentication from query parameters (for one-time access)
	// This part remains unchanged, allowing temporary access via password if needed.
	query := request.URL.Query()
	password := query.Get("password")
	if c.AdminPassword != "" && password == c.AdminPassword {
		// Password authentication successful, but no session is generated.
		log.Infof("Authentication successful via query parameter password (one-time access).")
		return true
	}

	log.Debugf("Authentication failed for request: %s", request.URL.Path)
	return false
}

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
		// Use embedded file system template
		tmpl, err = template.ParseFS(templatesFS, "templates/login.html")
		if err != nil {
			c.logError("Failed to parse embedded login template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to parse template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// Create a buffer to save rendered HTML
	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, nil)
	if err != nil {
		c.logError("Failed to execute login template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to render template: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Prepare HTTP response header
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
		"\r\n"

	// Write header and HTML content
	conn.Write([]byte(header))
	conn.Write(htmlBuffer.Bytes())
}

// processLogin handles login requests
func (c *ServerConfig) processLogin(conn net.Conn, request *http.Request) {
	// Parse form data
	err := request.ParseForm()
	if err != nil {
		c.logError("Failed to parse login form: %v", err)
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		return
	}

	// Get submitted password
	password := request.PostForm.Get("password")
	if password == "" {
		log.Warnf("Received empty password during login attempt.")
		// Empty password, redirect back to login page with error
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /portal?error=invalid_password\r\n" + // Use same error message for consistency
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	// Validate password
	if password != c.AdminPassword {
		log.Infof("Invalid password: %s, origin: %s", password, c.AdminPassword)
		// Password error, redirect back to login page
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /portal?error=invalid_password\r\n" +
			"\r\n"
		conn.Write([]byte(header))
		return
	}

	// Create new session
	session := c.SessionManager.CreateSession()

	// Set session cookie and redirect to management panel
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal\r\n" +
		"Set-Cookie: admin_session=" + session + "; Path=/; HttpOnly; SameSite=Strict\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// servePortal handles requests for the management panel page
func (c *ServerConfig) servePortal(conn net.Conn) {
	c.logInfo("Serving portal page")

	// Get all providers
	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to get providers: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Prepare template data
	data := PortalData{
		CurrentTime:   time.Now().Format("2006-01-02 15:04:05"),
		TotalRequests: 0,
	}

	// Process provider data
	var totalSuccess int64
	healthyCount := 0

	for _, p := range providers {
		// Calculate success rate
		successRate := 0.0
		if p.TotalRequests > 0 {
			successRate = float64(p.SuccessCount) / float64(p.TotalRequests) * 100
		}

		var healthClass string
		if !p.IsFirstCheckCompleted {
			healthClass = "unknown"
		} else if p.IsHealthy {
			healthClass = "healthy"
		} else {
			healthClass = "unhealthy"
		}

		// Add to provider list
		data.Providers = append(data.Providers, ProviderData{
			ID:                p.ID,
			WrapperName:       p.WrapperName,
			ModelName:         p.ModelName,
			TypeName:          p.TypeName,
			DomainOrURL:       p.DomainOrURL,
			APIKey:            p.APIKey, // 设置 APIKey
			TotalRequests:     p.TotalRequests,
			SuccessRate:       successRate,
			LastLatency:       p.LastLatency,
			IsHealthy:         p.IsHealthy,
			HealthStatusClass: healthClass,
		})

		// Accumulate statistics
		data.TotalRequests += p.TotalRequests
		totalSuccess += p.SuccessCount
		if p.IsHealthy && p.IsFirstCheckCompleted {
			healthyCount++
		}
	}

	// Set overall statistics
	data.TotalProviders = len(providers)
	data.HealthyProviders = healthyCount

	// Calculate overall success rate
	if data.TotalRequests > 0 {
		data.SuccessRate = float64(totalSuccess) / float64(data.TotalRequests) * 100
	}

	// Get API keys and allowed models
	data.AllowedModels = make(map[string]string)
	for _, key := range c.KeyAllowedModels.Keys() {
		models, _ := c.KeyAllowedModels.Get(key)
		modelNames := make([]string, 0, len(models))
		for model := range models {
			modelNames = append(modelNames, model)
		}
		data.AllowedModels[key] = strings.Join(modelNames, ", ")
	}

	// 获取API密钥数据
	dbApiKeys, err := GetAllAiApiKeys()
	if err == nil {
		for _, apiKey := range dbApiKeys {
			// 创建部分隐藏的API密钥显示
			displayKey := apiKey.APIKey
			if len(displayKey) > 8 {
				displayKey = displayKey[:4] + "..." + displayKey[len(displayKey)-4:]
			}

			// 格式化流量数据，使其更具可读性
			inputBytesFormatted := formatBytes(apiKey.InputBytes)
			outputBytesFormatted := formatBytes(apiKey.OutputBytes)

			// 创建APIKeyData结构
			keyData := APIKeyData{
				ID:                   apiKey.ID,
				Key:                  apiKey.APIKey,
				DisplayKey:           displayKey,
				AllowedModels:        apiKey.AllowedModels,
				CreatedAt:            apiKey.CreatedAt.Format("2006-01-02 15:04:05"),
				UsageCount:           apiKey.UsageCount,
				SuccessCount:         apiKey.SuccessCount,
				FailureCount:         apiKey.FailureCount,
				InputBytes:           apiKey.InputBytes,
				OutputBytes:          apiKey.OutputBytes,
				InputBytesFormatted:  inputBytesFormatted,
				OutputBytesFormatted: outputBytesFormatted,
				Active:               apiKey.Active,
			}

			// 设置最后使用时间
			if !apiKey.LastUsedTime.IsZero() {
				keyData.LastUsedAt = apiKey.LastUsedTime.Format("2006-01-02 15:04:05")
			}

			data.APIKeys = append(data.APIKeys, keyData)
		}
	}

	var tmpl *template.Template

	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/portal.html",
		"templates/portal.html",
		"../templates/portal.html",
	); result != "" {
		rawTemp, err := os.ReadFile(result)
		if err != nil {
			c.logError("Failed to read template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
		tmpl, err = template.New("portal").Parse(string(rawTemp))
		if err != nil {
			c.logError("Failed to parse template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	} else {
		// Render template
		tmpl, err = template.ParseFS(templatesFS, "templates/portal.html")
		if err != nil {
			c.logError("Failed to parse template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// Create a buffer to save rendered HTML
	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, data)
	if err != nil {
		c.logError("Failed to execute template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to render template: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Prepare HTTP response header
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
		"\r\n"

	// Write header and HTML content
	conn.Write([]byte(header))
	conn.Write(htmlBuffer.Bytes())
}

// servePortalWithAuth handles management panel requests using session ID instead of password
func (c *ServerConfig) servePortalWithAuth(conn net.Conn) {
	// Directly call the method to render the page, authentication is done in the upper layer
	c.servePortal(conn)
}

// serveAddProviderPage handles requests to add an AI provider
func (c *ServerConfig) serveAddProviderPage(conn net.Conn, request *http.Request) {
	// Check if it's a GET or POST request
	if request.Method == "POST" {
		// Process form submission for adding a provider
		// TODO: Parse form data and add new AI provider

		// Redirect back to home page
		header := "HTTP/1.1 303 See Other\r\n" +
			"Location: /portal\r\n" +
			"\r\n"
		conn.Write([]byte(header))
	} else {
		c.logInfo("Serving add provider page")

		var tmpl *template.Template
		var err error

		// Try to read template from filesystem
		if result := utils.GetFirstExistedFile(
			"common/aibalance/templates/add_provider.html",
			"templates/add_provider.html",
			"../templates/add_provider.html",
		); result != "" {
			rawTemp, err := os.ReadFile(result)
			if err != nil {
				c.logError("Failed to read add provider template: %v", err)
				errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
				conn.Write([]byte(errorResponse))
				return
			}
			tmpl, err = template.New("add_provider").Parse(string(rawTemp))
			if err != nil {
				c.logError("Failed to parse add provider template: %v", err)
				errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
				conn.Write([]byte(errorResponse))
				return
			}
		} else {
			// Use embedded file system template
			tmpl, err = template.ParseFS(templatesFS, "templates/add_provider.html")
			if err != nil {
				c.logError("Failed to parse embedded add provider template: %v", err)
				errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
				conn.Write([]byte(errorResponse))
				return
			}
		}

		// Create a buffer to save rendered HTML
		var htmlBuffer bytes.Buffer
		err = tmpl.Execute(&htmlBuffer, nil)
		if err != nil {
			c.logError("Failed to execute add provider template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to render template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}

		// Prepare HTTP response header
		header := "HTTP/1.1 200 OK\r\n" +
			"Content-Type: text/html; charset=utf-8\r\n" +
			"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
			"\r\n"

		// Write header and HTML content
		conn.Write([]byte(header))
		conn.Write(htmlBuffer.Bytes())
	}
}

// processAddProviders handles batch requests to add AI providers
func (c *ServerConfig) processAddProviders(conn net.Conn, request *http.Request) {
	c.logInfo("Processing add providers request")

	// Parse form data
	err := request.ParseForm()
	if err != nil {
		c.logError("Failed to parse form: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 400 Bad Request\r\n\r\nFailed to parse form: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Get form data
	wrapperName := request.PostForm.Get("wrapper_name")
	modelName := request.PostForm.Get("model_name")
	modelType := request.PostForm.Get("model_type")
	domainOrURL := request.PostForm.Get("domain_or_url")
	apiKeysStr := request.PostForm.Get("api_keys")
	noHTTPS := request.PostForm.Get("no_https") == "on" // 获取 NoHTTPS 参数

	// Validate required fields
	if wrapperName == "" || modelName == "" || modelType == "" || domainOrURL == "" || apiKeysStr == "" {
		c.logError("Missing required fields")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\nAll fields are required"
		conn.Write([]byte(errorResponse))
		return
	}

	// Split API keys by line
	apiKeys := make([]string, 0)
	for _, line := range strings.Split(apiKeysStr, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			apiKeys = append(apiKeys, line)
		}
	}

	if len(apiKeys) == 0 {
		c.logError("No valid API keys provided")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\nNo valid API keys provided"
		conn.Write([]byte(errorResponse))
		return
	}

	// Create ConfigProvider object
	configProvider := &ConfigProvider{
		ModelName:   modelName,
		TypeName:    modelType,
		DomainOrURL: domainOrURL,
		Keys:        apiKeys,
		NoHTTPS:     noHTTPS, // 设置 NoHTTPS 参数
	}

	// Convert to Provider object
	providers := configProvider.ToProviders()
	if len(providers) == 0 {
		c.logError("Failed to create providers")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\nFailed to create providers, please check your input"
		conn.Write([]byte(errorResponse))
		return
	}

	// Success count
	successCount := 0

	// Save to database
	for _, provider := range providers {
		// Create database object
		dbProvider := &schema.AiProvider{
			ModelName:             modelName,
			TypeName:              modelType,
			DomainOrURL:           domainOrURL,
			APIKey:                provider.APIKey,
			WrapperName:           wrapperName, // Use WrapperName from form
			NoHTTPS:               noHTTPS,     // 设置 NoHTTPS 参数
			IsHealthy:             false,       // 修改：新provider默认为不健康，需要通过健康检查
			IsFirstCheckCompleted: false,       // 修改：明确设置首次检查未完成
			LastRequestTime:       time.Time{}, // 修改：不设置时间，让健康检查来更新
			HealthCheckTime:       time.Time{}, // 修改：不设置时间，让健康检查来更新
		}

		// Save to database
		err = SaveAiProvider(dbProvider)
		if err != nil {
			c.logError("Failed to save provider: %v", err)
			continue
		}

		// Associate database object with Provider
		provider.DbProvider = dbProvider
		successCount++
	}

	// Build result message
	resultMessage := fmt.Sprintf("Successfully added %d providers(total %d)", successCount, len(providers))
	c.logInfo(resultMessage)

	// --- 开始修改: 重新加载 Provider 配置到内存 ---
	err = LoadProvidersFromDatabase(c) // 使用 balancer.go 中的函数刷新配置
	if err != nil {
		// 记录错误，但可能仍然继续，因为数据库写入已成功
		c.logError("Failed to reload providers into memory after adding new ones: %v", err)
		// 根据策略，这里可以选择是否向用户显示错误或阻止重定向
	} else {
		c.logInfo("Successfully reloaded providers into memory.")
	}

	// --- 新增：立即对新添加的provider执行健康检查 ---
	go func() {
		// 异步执行健康检查，避免阻塞用户请求
		for _, provider := range providers {
			if provider.DbProvider != nil {
				c.logInfo("Triggering immediate health check for newly added provider: %s (ID: %d)", provider.DbProvider.WrapperName, provider.DbProvider.ID)
				_, err := RunSingleProviderHealthCheck(provider.DbProvider.ID)
				if err != nil {
					c.logError("Failed to run immediate health check for provider %d: %v", provider.DbProvider.ID, err)
				} else {
					c.logInfo("Immediate health check completed for provider %d", provider.DbProvider.ID)
				}
			}
		}
	}()
	// --- 结束修改 ---

	// Redirect back to home page
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// handleLogout handles logout requests
func (c *ServerConfig) handleLogout(conn net.Conn, request *http.Request) {
	// Get session ID from cookie
	cookies := request.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "admin_session" {
			// Delete session
			c.SessionManager.DeleteSession(cookie.Value)
			break
		}
	}

	// Clear cookie and redirect to login page
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal\r\n" +
		"Set-Cookie: admin_session=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; HttpOnly; SameSite=Strict\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

func (c *ServerConfig) ForwarderDomain() []string {
	domains := make([]string, 0)
	for _, rule := range c.forwardRule.Values() {
		domains = append(domains, rule.SNI)
	}
	return domains
}

// serveAutoCompleteData provides autocomplete data
func (c *ServerConfig) serveAutoCompleteData(conn net.Conn, request *http.Request) {
	c.logInfo("Serving autocomplete data")

	// Get all provider data
	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers for autocomplete: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to get providers: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Extract unique data
	wrapperNames := make(map[string]bool)
	modelNames := make(map[string]bool)

	for _, p := range providers {
		if p.WrapperName != "" {
			wrapperNames[p.WrapperName] = true
		}
		if p.ModelName != "" {
			modelNames[p.ModelName] = true
		}
	}

	// Convert to array
	wrapperNamesList := make([]string, 0, len(wrapperNames))
	for name := range wrapperNames {
		wrapperNamesList = append(wrapperNamesList, name)
	}

	modelNamesList := make([]string, 0, len(modelNames))
	for name := range modelNames {
		modelNamesList = append(modelNamesList, name)
	}

	modelTypesList := aispec.GetRegisteredAIGateways()

	var domainOrUrl []string
	for _, domain := range c.ForwarderDomain() {
		domainOrUrl = append(domainOrUrl, domain)
		domainOrUrl = append(domainOrUrl, "https://"+domain+"/v1/chat/completions")
	}
	domainOrUrl = utils.RemoveRepeatStringSlice(domainOrUrl)

	// Build JSON response
	autoCompleteData := struct {
		WrapperNames []string `json:"wrapper_names"`
		ModelNames   []string `json:"model_names"`
		ModelTypes   []string `json:"model_types"`
		DomainOrURLs []string `json:"domain_or_urls"`
	}{
		WrapperNames: wrapperNamesList,
		ModelNames:   modelNamesList,
		ModelTypes:   modelTypesList,
		DomainOrURLs: domainOrUrl,
	}

	// Convert to JSON
	jsonData, err := json.Marshal(autoCompleteData)
	if err != nil {
		c.logError("Failed to encode autocomplete data: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to encode data: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Prepare HTTP response header
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", len(jsonData)) + "\r\n" +
		"\r\n"

	// Write header and JSON content
	conn.Write([]byte(header))
	conn.Write(jsonData)
}

// serveAPIKeysPage displays the API key information page
func (c *ServerConfig) serveAPIKeysPage(conn net.Conn) {
	c.logInfo("Serving API keys page")

	// Prepare template data
	data := struct {
		CurrentTime  string
		APIKeys      map[string]string
		AllModelList []string // All available model list for creating new API keys
	}{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
		APIKeys:     make(map[string]string),
	}

	// Get API keys from database
	dbApiKeys, err := GetAllAiApiKeys()
	if err == nil && len(dbApiKeys) > 0 {
		// Database has API keys, use database records
		for _, apiKey := range dbApiKeys {
			data.APIKeys[apiKey.APIKey] = apiKey.AllowedModels
		}
	} else {
		// Get API keys and allowed models from memory configuration (use as fallback option)
		for _, key := range c.KeyAllowedModels.Keys() {
			models, _ := c.KeyAllowedModels.Get(key)
			modelNames := make([]string, 0, len(models))
			for model := range models {
				modelNames = append(modelNames, model)
			}
			data.APIKeys[key] = strings.Join(modelNames, ", ")
		}
	}

	// Get all available model list
	providers, err := GetAllAiProviders()
	if err == nil {
		modelSet := make(map[string]bool)
		for _, p := range providers {
			if p.WrapperName != "" {
				modelSet[p.WrapperName] = true
			}
		}

		data.AllModelList = make([]string, 0, len(modelSet))
		for model := range modelSet {
			data.AllModelList = append(data.AllModelList, model)
		}
	}

	// Render template
	var htmlBuffer bytes.Buffer
	tmpl, err := template.ParseFS(templatesFS, "templates/api_keys.html")
	if err != nil {
		c.logError("Failed to parse api_keys template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	err = tmpl.Execute(&htmlBuffer, data)
	if err != nil {
		c.logError("Failed to execute api_keys template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to render template: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Prepare HTTP response header
	header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", htmlBuffer.Len()) + "\r\n" +
		"\r\n"

	// Write header and HTML content
	conn.Write([]byte(header))
	conn.Write(htmlBuffer.Bytes())
}

// processCreateAPIKey handles requests to create a new API key
func (c *ServerConfig) processCreateAPIKey(conn net.Conn, request *http.Request) {
	c.logInfo("Processing create API key request")

	// Parse form data
	err := request.ParseForm()
	if err != nil {
		c.logError("Failed to parse form: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 400 Bad Request\r\n\r\nFailed to parse form: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Get form data
	apiKey := request.PostForm.Get("api_key")
	allowedModels := request.PostForm["allowed_models"] // Multi-select values

	// Validate required fields
	if apiKey == "" || len(allowedModels) == 0 {
		c.logError("Missing required fields for API key creation")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\nAPI key and allowed models are required"
		conn.Write([]byte(errorResponse))
		return
	}

	// Add allowed models to configuration
	modelMap := make(map[string]bool)
	for _, model := range allowedModels {
		modelMap[model] = true
	}

	// Add to configuration (maintain compatibility with memory configuration)
	// c.KeyAllowedModels.allowedModels[apiKey] = modelMap // -- DEPRECATED: 直接修改内存可能不完整

	// Save API key to database
	allowedModelsStr := strings.Join(allowedModels, ",")
	err = SaveAiApiKey(apiKey, allowedModelsStr)
	if err != nil {
		c.logError("Failed to save API key to database: %v", err)
		// Continue execution, because it's already added to memory configuration
	}

	// --- 开始修改: 保存成功后，重新从数据库加载所有 Key 到内存 ---
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after creating key '%s': %v", apiKey, err)
		// 记录错误，但数据库可能已成功写入
	} else {
		c.logInfo("Successfully reloaded API keys into memory after creating key '%s'.", apiKey)
	}
	// --- 结束修改 ---

	// Build result message
	c.logInfo("Successfully created API key: %s with %d allowed models", apiKey, len(allowedModels))

	// Redirect back to API key page
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal/\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// HandlePortalRequest handles the main entry point for management portal requests
func (c *ServerConfig) HandlePortalRequest(conn net.Conn, request *http.Request, uriIns *url.URL) {
	c.logInfo("Processing portal request: %s", uriIns.Path)

	// Process login POST request
	if uriIns.Path == "/portal/login" && request.Method == "POST" {
		c.processLogin(conn, request)
		return
	}

	// Check authentication status (except for login page)
	if !c.checkAuth(request) {
		c.serveLoginPage(conn)
		return
	}

	// Process different requests based on path
	if uriIns.Path == "/portal" || uriIns.Path == "/portal/" {
		c.servePortalWithAuth(conn)
	} else if uriIns.Path == "/portal/add-ai-provider" {
		c.serveAddProviderPage(conn, request)
	} else if uriIns.Path == "/portal/add-providers" && request.Method == "POST" {
		c.processAddProviders(conn, request)
	} else if uriIns.Path == "/portal/validate-provider" && request.Method == "POST" {
		c.handleValidateProvider(conn, request)
	} else if uriIns.Path == "/portal/autocomplete" {
		c.serveAutoCompleteData(conn, request)
	} else if uriIns.Path == "/portal/api-keys" {
		c.serveAPIKeysPage(conn)
	} else if uriIns.Path == "/portal/create-api-key" && request.Method == "POST" {
		c.handleGenerateApiKey(conn, request)
	} else if uriIns.Path == "/portal/api/health-check" {
		c.serveHealthCheckAPI(conn, request)
	} else if uriIns.Path == "/portal/api/providers" {
		c.serveProvidersAPI(conn, request)
	} else if uriIns.Path == "/portal/check-all-health" && request.Method == "POST" {
		c.handleCheckAllHealth(conn, request)
	} else if strings.HasPrefix(uriIns.Path, "/portal/check-health/") && request.Method == "POST" {
		c.handleCheckSingleHealth(conn, request, uriIns.Path)
	} else if uriIns.Path == "/portal/generate-api-key" && request.Method == "POST" {
		c.handleGenerateApiKey(conn, request)
	} else if uriIns.Path == "/portal/logout" {
		c.handleLogout(conn, request)
	} else if strings.HasPrefix(uriIns.Path, "/portal/delete-provider/") && request.Method == "DELETE" {
		c.handleDeleteProvider(conn, request, uriIns.Path)
	} else if uriIns.Path == "/portal/delete-providers" && request.Method == "POST" {
		c.handleDeleteMultipleProviders(conn, request)
	} else if strings.HasPrefix(uriIns.Path, "/portal/activate-api-key/") && request.Method == "POST" {
		c.handleToggleAPIKeyStatus(conn, request, uriIns.Path, true)
	} else if strings.HasPrefix(uriIns.Path, "/portal/deactivate-api-key/") && request.Method == "POST" {
		c.handleToggleAPIKeyStatus(conn, request, uriIns.Path, false)
	} else if uriIns.Path == "/portal/batch-activate-api-keys" && request.Method == "POST" {
		c.handleBatchToggleAPIKeyStatus(conn, request, true)
	} else if uriIns.Path == "/portal/batch-deactivate-api-keys" && request.Method == "POST" {
		c.handleBatchToggleAPIKeyStatus(conn, request, false)
	} else if strings.HasPrefix(uriIns.Path, "/portal/update-api-key-allowed-models/") && request.Method == "POST" {
		c.handleUpdateAPIKeyAllowedModels(conn, request, uriIns.Path)
	} else {
		// Default return home page
		c.servePortalWithAuth(conn)
	}
}

// serveHealthCheckAPI handles health check API requests
func (c *ServerConfig) serveHealthCheckAPI(conn net.Conn, request *http.Request) {
	c.logInfo("Handling health check API request")

	// 解析请求体，检查是否指定了特定的提供者
	var requestData struct {
		ProviderID uint `json:"ProviderID"`
	}

	if request.Method == "POST" && request.Body != nil {
		if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
			if err != io.EOF {
				c.logError("Failed to parse request body: %v", err)
				c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
					"success":      false,
					"message":      "Invalid request format",
					"totalCount":   0,
					"healthyCount": 0,
					"healthRate":   0.0,
				})
				return
			}
		}
	}

	var results []*HealthCheckResult
	var err error

	// 根据是否指定提供者 ID 执行不同的健康检查
	if requestData.ProviderID > 0 {
		// 单个提供者健康检查
		result, err := RunSingleProviderHealthCheck(requestData.ProviderID)
		if err != nil {
			c.logError("Failed to run health check for provider %d: %v", requestData.ProviderID, err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"success":      false,
				"message":      fmt.Sprintf("Health check failed for provider %d: %v", requestData.ProviderID, err),
				"totalCount":   0,
				"healthyCount": 0,
				"healthRate":   0.0,
				"singleProvider": map[string]interface{}{
					"id":           requestData.ProviderID,
					"name":         "",
					"healthy":      false,
					"responseTime": 0,
					"error":        err.Error(),
				},
			})
			return
		}
		results = []*HealthCheckResult{result}
	} else {
		// 全量健康检查
		results, err = RunManualHealthCheck()
		if err != nil {
			c.logError("Failed to run health check: %v", err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"success":      false,
				"message":      fmt.Sprintf("Health check failed: %v", err),
				"totalCount":   0,
				"healthyCount": 0,
				"healthRate":   0.0,
			})
			return
		}
	}

	// 防止空结果导致除零错误
	if results == nil {
		results = []*HealthCheckResult{}
	}

	// 统计结果
	totalCount := len(results)
	healthyCount := 0
	for _, result := range results {
		if result != nil && result.IsHealthy {
			healthyCount++
		}
	}

	// 统一响应格式
	healthRate := 0.0
	if totalCount > 0 {
		healthRate = float64(healthyCount) * 100 / float64(totalCount)
	}

	response := map[string]interface{}{
		"success":      true,
		"totalCount":   totalCount,
		"healthyCount": healthyCount,
		"healthRate":   healthRate,
		"checkTime":    time.Now().Format("2006-01-02 15:04:05"),
	}

	// 如果是单个提供者检查，添加详细信息
	if requestData.ProviderID > 0 && len(results) > 0 && results[0] != nil {
		result := results[0]

		var errorMsg string
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}

		response["singleProvider"] = map[string]interface{}{
			"id":           result.Provider.ID,
			"name":         result.Provider.WrapperName,
			"healthy":      result.IsHealthy,
			"responseTime": result.ResponseTime,
			"error":        errorMsg,
		}
		response["message"] = fmt.Sprintf("Health check completed for provider %d", requestData.ProviderID)
	} else if requestData.ProviderID > 0 {
		// 单个提供者但结果为空
		response["singleProvider"] = map[string]interface{}{
			"id":           requestData.ProviderID,
			"name":         "",
			"healthy":      false,
			"responseTime": 0,
			"error":        "Provider not found or health check failed",
		}
		response["message"] = fmt.Sprintf("Health check failed for provider %d", requestData.ProviderID)
	} else {
		response["message"] = fmt.Sprintf("Health check completed: %d/%d providers healthy", healthyCount, totalCount)
	}

	c.writeJSONResponse(conn, http.StatusOK, response)
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

	// Prepare HTTP status line
	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = "Unknown"
	}

	// Prepare HTTP response header
	header := fmt.Sprintf("HTTP/1.1 %d %s\r\n"+
		"Content-Type: application/json; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n",
		statusCode, statusText, len(jsonData))

	// Write header and JSON content
	conn.Write([]byte(header))
	conn.Write(jsonData)
}

// serveProvidersAPI handles requests to get all provider information
func (c *ServerConfig) serveProvidersAPI(conn net.Conn, request *http.Request) {
	c.logInfo("Handling providers API request")

	// 获取所有AI提供者信息
	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to get providers: %v", err),
		})
		return
	}

	// 准备返回的数据
	providersData := make([]map[string]interface{}, 0, len(providers))
	for _, p := range providers {
		// 计算成功率
		successRate := 0.0
		if p.TotalRequests > 0 {
			successRate = float64(p.SuccessCount) / float64(p.TotalRequests) * 100
		}

		// 添加到结果列表，包含 APIKey
		providersData = append(providersData, map[string]interface{}{
			"id":             p.ID,
			"wrapper_name":   p.WrapperName,
			"model_name":     p.ModelName,
			"type_name":      p.TypeName,
			"domain_or_url":  p.DomainOrURL,
			"api_key":        p.APIKey, // 添加 APIKey
			"total_requests": p.TotalRequests,
			"success_rate":   successRate,
			"last_latency":   p.LastLatency,
			"is_healthy":     p.IsHealthy,
			"no_https":       p.NoHTTPS, // 添加 NoHTTPS 配置
		})
	}

	// 返回结果
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Providers retrieved successfully",
		"data":    providersData,
	})
}

// handleCheckSingleHealth 处理单个提供者的健康检查请求
func (c *ServerConfig) handleCheckSingleHealth(conn net.Conn, request *http.Request, path string) {
	c.logInfo("处理单个提供者健康检查请求: %s", path)

	// 从路径中提取提供者ID
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		c.logError("无效的路径格式: %s", path)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "无效的请求路径",
		})
		return
	}

	providerIDStr := parts[len(parts)-1]
	providerID, err := strconv.ParseUint(providerIDStr, 10, 32)
	if err != nil {
		c.logError("无效的提供者ID: %s, 错误: %v", providerIDStr, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("无效的提供者ID: %s", providerIDStr),
		})
		return
	}

	// 执行健康检查
	result, err := RunSingleProviderHealthCheck(uint(providerID))
	if err != nil {
		c.logError("提供者健康检查失败 ID=%d: %v", providerID, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("健康检查失败: %v", err),
		})
		return
	}

	// 构造响应
	var errorMsg string
	if result.Error != nil {
		errorMsg = result.Error.Error()
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("提供者 %d 健康检查完成", providerID),
		"data": map[string]interface{}{
			"id":           result.Provider.ID,
			"name":         result.Provider.WrapperName,
			"healthy":      result.IsHealthy,
			"responseTime": result.ResponseTime,
			"error":        errorMsg,
		},
	})
}

// handleCheckAllHealth 处理全部提供者的健康检查请求
func (c *ServerConfig) handleCheckAllHealth(conn net.Conn, request *http.Request) {
	c.logInfo("处理全部提供者健康检查请求")

	// 执行健康检查
	results, err := RunManualHealthCheck()
	if err != nil {
		c.logError("健康检查失败: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("健康检查失败: %v", err),
		})
		return
	}

	// 统计结果
	totalCount := len(results)
	healthyCount := 0
	for _, result := range results {
		if result != nil && result.IsHealthy {
			healthyCount++
		}
	}

	// 计算健康率
	healthRate := 0.0
	if totalCount > 0 {
		healthRate = float64(healthyCount) * 100 / float64(totalCount)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":      true,
		"message":      fmt.Sprintf("健康检查完成: %d/%d 个提供者健康", healthyCount, totalCount),
		"totalCount":   totalCount,
		"healthyCount": healthyCount,
		"healthRate":   healthRate,
		"checkTime":    time.Now().Format("2006-01-02 15:04:05"),
	})
}

// handleGenerateApiKey handles requests to generate a new API key
func (c *ServerConfig) handleGenerateApiKey(conn net.Conn, request *http.Request) {
	c.logInfo("Processing generate API key request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	// Parse request body
	var reqBody struct {
		AllowedModels []string `json:"allowed_models"`
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body for API key generation: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Failed to read request body"})
		return
	}
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to unmarshal request body for API key generation: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Invalid request body format"})
		return
	}

	// Validate whether models are selected
	if len(reqBody.AllowedModels) == 0 {
		c.logWarn("API key generation request missing allowed_models")
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Missing or empty 'allowed_models' field"})
		return
	}

	// Call new function to generate and store API key
	apiKey, err := c.generateAndStoreAPIKey(reqBody.AllowedModels)
	if err != nil {
		c.logError("Failed to generate and store API key: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "Failed to generate API key"})
		return
	}

	c.logInfo("Successfully generated new API key with allowed models: %v", reqBody.AllowedModels)
	c.writeJSONResponse(conn, http.StatusOK, map[string]string{"apiKey": apiKey})
}

// generateAndStoreAPIKey generates a new API key and stores it with associated models
func (c *ServerConfig) generateAndStoreAPIKey(allowedModels []string) (string, error) {
	apiKey := uuid.New().String() // Or use a more secure generation method
	allowedModelsStr := strings.Join(allowedModels, ",")

	// Linter Error Fix: Use existing schema.AiApiKeys struct
	newKeyData := &schema.AiApiKeys{ // Was schema.AIBalancerAPIKey
		APIKey:        apiKey, // Field name is APIKey, not Key
		AllowedModels: allowedModelsStr,
		// CreatedAt and Active are part of gorm.Model
		// Initialize stats
		UsageCount:   0,
		SuccessCount: 0,
		FailureCount: 0,
		InputBytes:   0,
		OutputBytes:  0,
		// LastUsedTime can be initialized or left zero
		LastUsedTime: time.Time{}, // Explicitly zero value
	}

	// Linter Error Fix: Use existing SaveAiApiKey function with correct arguments
	err := SaveAiApiKey(newKeyData.APIKey, newKeyData.AllowedModels) // Pass strings
	if err != nil {
		// Linter Error Fix: Use c.logError for logging within the method
		c.logError("Failed to store new API key in database: %v", err)
		// log.WithError(err).Error("Failed to store new API key in database") // Cannot use global log here
		return "", fmt.Errorf("failed to store new API key: %w", err)
	}

	// Linter Error Fix: Reload API keys from DB after adding a new one
	// Remove the assumption of reloadAPIKeysFromDB() and explicitly reload by calling LoadAPIKeysFromDB.
	err = c.LoadAPIKeysFromDB() // Reload keys into memory using the existing method
	if err != nil {
		c.logError("Failed to reload API keys into memory after adding a new one: %v", err)
		// Log the error but continue, the key was saved, but the in-memory list might be stale
	}
	/*
		newKeys, err := GetAllAiApiKeys() // Reload all keys from DB
		if err != nil {
			c.logError("Failed to reload API keys from DB after adding a new one: %v", err)
			// Log the error but continue, the key was saved, but the in-memory list might be stale
		} else {
			// Update the ServerConfig's in-memory map (assuming it's named APIKeys)
			newAPIKeysMap := make(map[string]*schema.AiApiKeys)
			for _, k := range newKeys {
				newAPIKeysMap[k.APIKey] = k
			}
			c.APIKeys = newAPIKeysMap // Update the map in ServerConfig // THIS WAS THE ERROR
			c.logInfo("Successfully reloaded %d API keys into memory", len(newKeys))
		}
	*/

	// c.reloadAPIKeysFromDB() // Was c.AddAPIKeyToMemory(newKey)
	// An alternative if direct manipulation is needed:
	// c.APIKeys[apiKey] = newKeyData // Assuming c.APIKeys is a map[string]*schema.AiApiKeys

	return apiKey, nil
}

// handleDeleteProvider handles requests to delete a provider
func (c *ServerConfig) handleDeleteProvider(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Processing delete provider request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Extract provider ID from URL path
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		c.logError("Invalid path format: %s", path)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request path",
		})
		return
	}

	providerIDStr := parts[len(parts)-1]
	providerID, err := strconv.ParseUint(providerIDStr, 10, 32)
	if err != nil {
		c.logError("Invalid provider ID: %s, error: %v", providerIDStr, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Invalid provider ID: %s", providerIDStr),
		})
		return
	}

	// Delete database record
	err = DeleteAiProviderByID(uint(providerID))
	if err != nil {
		c.logError("Delete provider failed ID=%d: %v", providerID, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Delete provider failed: %v", err),
		})
		return
	}

	c.logInfo("Successfully deleted provider with ID: %d", providerID)
	c.writeJSONResponse(conn, http.StatusOK, map[string]string{"message": "Provider deleted successfully"})
}

// handleToggleAPIKeyStatus handles requests to activate or deactivate an API key
func (c *ServerConfig) handleToggleAPIKeyStatus(conn net.Conn, request *http.Request, path string, activate bool) {
	action := "deactivate"
	prefixPath := "/portal/deactivate-api-key/"
	if activate {
		action = "activate"
		prefixPath = "/portal/activate-api-key/"
	}
	c.logInfo("Processing %s API key request: %s", action, path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Ensure it's a POST request for state changes
	if request.Method != http.MethodPost {
		c.logError("Method not allowed for toggling API key status, expected POST")
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]interface{}{
			"success": false,
			"message": "Method Not Allowed, use POST",
		})
		return
	}

	// Extract API key ID from URL path
	idStr := strings.TrimPrefix(path, prefixPath)
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.logError("Invalid API key ID '%s': %v", idStr, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid API key ID",
		})
		return
	}

	// Update the API key status in the database
	err = UpdateAiApiKeyStatus(uint(id), activate)
	if err != nil {
		// Check if it's a record not found error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.logError("API key not found for ID %d: %v", id, err)
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "API key not found",
			})
		} else {
			c.logError("Failed to %s API key (ID: %d): %v", action, id, err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("Failed to %s API key", action),
			})
		}
		return
	}

	c.logInfo("Successfully %sd API key (ID: %d)", action, id)

	// --- 开始修改: 重新加载 API Key 配置到内存 ---
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after %s key ID %d: %v", action, id, err)
		// Log the error, but the primary action (DB update) succeeded.
	} else {
		c.logInfo("Successfully reloaded API keys into memory after %s key ID %d.", action, id)
	}
	// --- 结束修改 ---

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("API key %sd successfully", action),
	})
}

// handleBatchToggleAPIKeyStatus handles requests to batch activate or deactivate API keys
func (c *ServerConfig) handleBatchToggleAPIKeyStatus(conn net.Conn, request *http.Request, activate bool) {
	action := "deactivate"
	if activate {
		action = "activate"
	}
	c.logInfo("Processing batch %s API keys request", action)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Ensure it's a POST request
	if request.Method != http.MethodPost {
		c.logError("Method not allowed for batch toggling API key status, expected POST")
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]interface{}{
			"success": false,
			"message": "Method Not Allowed, use POST",
		})
		return
	}

	// Parse request body
	var reqBody struct {
		IDs []string `json:"ids"` // Changed to []string to accept string IDs from JSON
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body for batch %s API keys: %v", action, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()                              // Close the original body
	request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore body for potential re-reads if needed

	// Decode JSON
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	if err := decoder.Decode(&reqBody); err != nil {
		c.logError("Failed to parse request body for batch %s API keys: %v. Body: %s", action, err, string(bodyBytes))
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	if len(reqBody.IDs) == 0 {
		c.logWarn("No API key IDs specified for batch %s", action)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "No API key IDs specified",
		})
		return
	}

	// Convert string IDs to uint IDs
	uintIDs := make([]uint, 0, len(reqBody.IDs))
	for _, idStr := range reqBody.IDs {
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.logError("Invalid API key ID '%s' in batch request: %v", idStr, err)
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("Invalid API key ID format: %s", idStr),
			})
			return
		}
		uintIDs = append(uintIDs, uint(id))
	}

	// Call batch update function in the database layer with uint IDs
	affectedCount, err := BatchUpdateAiApiKeyStatus(uintIDs, activate)
	if err != nil {
		c.logError("Failed to batch %s %d API keys: %v", action, len(uintIDs), err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to %s API keys", action),
		})
		return
	}

	c.logInfo("Successfully %sd %d API keys (%d requested)", action, affectedCount, len(uintIDs))

	// --- 开始修改: 重新加载 API Key 配置到内存 ---
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after batch %s for %d keys: %v", action, len(uintIDs), err)
		// Log the error, but the primary action (DB update) succeeded.
	} else {
		c.logInfo("Successfully reloaded API keys into memory after batch %s.", action)
	}
	// --- 结束修改 ---

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Successfully %sd %d API keys", action, affectedCount),
		"count":   affectedCount,
	})
}

// handleDeleteMultipleProviders handles requests to delete multiple providers
func (c *ServerConfig) handleDeleteMultipleProviders(conn net.Conn, request *http.Request) {
	c.logInfo("Processing delete multiple providers request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Read the body first for logging
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body for multiple provider deletion (before decode): %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to read request body",
		})
		return
	}
	request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore body
	defer request.Body.Close()

	c.logInfo("Raw request body for delete multiple providers: %s", string(bodyBytes))

	// Parse request body struct - CHANGED IDs to []string
	var reqBody struct {
		IDs []string `json:"ids"` // Expect strings from JSON
	}

	// Decode JSON body (Now using the restored body)
	// Need to use a new decoder instance as the body was replaced
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	if err := decoder.Decode(&reqBody); err != nil {
		c.logError("Failed to parse request body for multiple provider deletion: %v. Body was: %s", err, string(bodyBytes))
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	if len(reqBody.IDs) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "No provider IDs specified for deletion",
		})
		return
	}

	// Convert string IDs to uint IDs
	providerIDsUint := make([]uint, 0, len(reqBody.IDs))
	var conversionErrors []string
	for _, idStr := range reqBody.IDs {
		idUint, err := strconv.ParseUint(idStr, 10, 32) // Parse as uint (base 10, 32-bit size is appropriate for uint)
		if err != nil {
			c.logWarn("Failed to convert provider ID string '%s' to uint: %v", idStr, err)
			conversionErrors = append(conversionErrors, fmt.Sprintf("ID '%s': %v", idStr, err))
			continue // Skip invalid IDs or return error? Returning error might be safer.
		}
		providerIDsUint = append(providerIDsUint, uint(idUint)) // Convert uint64 result to uint
	}

	// If there were conversion errors, return a Bad Request
	if len(conversionErrors) > 0 {
		c.logError("Errors converting provider IDs: %v", conversionErrors)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Invalid provider IDs provided: %s", strings.Join(conversionErrors, "; ")),
		})
		return
	}

	// Check if after conversion, we still have IDs to delete
	if len(providerIDsUint) == 0 {
		c.logWarn("No valid provider IDs remained after conversion.")
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "No valid provider IDs provided after conversion.",
		})
		return
	}

	// Use GORM's batch delete for efficiency with the converted uint slice
	result := GetDB().Where("id IN (?)", providerIDsUint).Delete(&schema.AiProvider{})
	if result.Error != nil {
		c.logError("Failed to delete multiple providers: %v", result.Error)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to delete providers from database",
		})
		return
	}

	// Check if any rows were actually affected
	if result.RowsAffected == 0 { // Removed the check for len(reqBody.IDs) > 0 as we use providerIDsUint now
		c.logWarn("Attempted to delete providers, but no matching IDs found or no rows affected. Valid IDs provided: %v", providerIDsUint)
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{ // Changed from Warn to OK as it's not an error
			"success":      true,
			"message":      "No matching providers found to delete, or they were already deleted.",
			"deletedCount": 0,
		})
		return
	}

	c.logInfo("Successfully deleted %d providers", result.RowsAffected)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":      true,
		"message":      fmt.Sprintf("Successfully deleted %d providers", result.RowsAffected),
		"deletedCount": result.RowsAffected,
	})
}

// handleValidateProvider handles requests to validate a provider configuration before adding
func (c *ServerConfig) handleValidateProvider(conn net.Conn, request *http.Request) {
	c.logInfo("Processing validate provider request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	// Parse form data
	if err := request.ParseForm(); err != nil {
		c.logError("Failed to parse form for provider validation: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Failed to parse form data",
		})
		return
	}

	wrapperName := request.PostForm.Get("wrapper_name")
	modelName := request.PostForm.Get("model_name")
	modelType := request.PostForm.Get("model_type")
	domainOrURL := request.PostForm.Get("domain_or_url")
	apiKeyToValidate := request.PostForm.Get("api_key_to_validate")
	noHTTPS := request.PostForm.Get("no_https") == "on"

	if wrapperName == "" || modelName == "" || modelType == "" || apiKeyToValidate == "" {
		c.logWarn("Validation request missing required fields (wrapper_name, model_name, model_type, api_key_to_validate)")
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Missing required fields: Provider Name, Model Name, Type, and API Key are required for validation.",
		})
		return
	}

	// Create a temporary provider instance for validation
	// Note: This provider is NOT saved to the database.
	tempProvider := &Provider{
		ModelName:   modelName,
		TypeName:    modelType,
		DomainOrURL: domainOrURL,
		APIKey:      apiKeyToValidate,
		WrapperName: wrapperName,
		NoHTTPS:     noHTTPS,
		// Initialize other fields as necessary for health check logic
		// For example, if your health check needs a DbProvider, you might need to mock it
		// or adjust the health check to work without it for this temporary validation.
		// For simplicity, we assume PerformHealthCheck can work with these core details.
	}

	c.logInfo("Attempting to validate temporary provider: Wrapper=%s, Model=%s, Type=%s, Domain=%s, Key=%s...", wrapperName, modelName, modelType, domainOrURL, apiKeyToValidate[:min(len(apiKeyToValidate), 4)])

	// Perform the health check on the temporary provider using the new ExecuteHealthCheckLogic function
	// The Provider's GetAIClient method is responsible for handling HTTP client needs.
	healthy, latency, checkErr := ExecuteHealthCheckLogic(tempProvider, wrapperName) // Use aibalance.ExecuteHealthCheckLogic if not in same package, assuming it is for now.

	if checkErr != nil {
		c.logWarn("Validation failed for temporary provider Wrapper=%s, Model=%s: %v", wrapperName, modelName, checkErr)
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Validation failed: %s", checkErr.Error()),
		})
		return
	}

	if !healthy {
		c.logWarn("Validation reported unhealthy for temporary provider Wrapper=%s, Model=%s. Latency: %dms", wrapperName, modelName, latency)
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Validation failed: Provider reported as unhealthy. Latency: %dms", latency),
		})
		return
	}

	c.logInfo("Validation successful for temporary provider Wrapper=%s, Model=%s. Latency: %dms", wrapperName, modelName, latency)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Configuration validated successfully. Latency: %dms", latency),
	})
}

// min is a helper function to avoid panics with string slicing
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleUpdateAPIKeyAllowedModels handles requests to update allowed models for an API key
func (c *ServerConfig) handleUpdateAPIKeyAllowedModels(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Processing update API key allowed models request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	// Extract API key ID from URL path
	// Example path: /portal/update-api-key-allowed-models/123
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		c.logError("Invalid path format for update API key allowed models: %s", path)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request path",
		})
		return
	}
	idStr := parts[len(parts)-1]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.logError("Invalid API key ID '%s' for update: %v", idStr, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid API key ID format",
		})
		return
	}

	// Parse request body
	var reqBody struct {
		AllowedModels string `json:"allowed_models"`
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body for updating API key allowed models: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to unmarshal request body for updating API key allowed models: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request body format",
		})
		return
	}

	// Update the API key in the database
	err = UpdateAiApiKeyAllowedModels(uint(id), reqBody.AllowedModels) // This function needs to be created in your DB interaction code.
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.logError("API key not found for ID %d during update of allowed models: %v", id, err)
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "API key not found",
			})
		} else {
			c.logError("Failed to update allowed models for API key (ID: %d): %v", id, err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": "Failed to update API key allowed models",
			})
		}
		return
	}

	c.logInfo("Successfully updated allowed models for API key (ID: %d)", id)

	// Reload API keys into memory
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after updating allowed models for key ID %d: %v", id, err)
	} else {
		c.logInfo("Successfully reloaded API keys into memory after updating allowed models for key ID %d.", id)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "API key allowed models updated successfully",
	})
}
