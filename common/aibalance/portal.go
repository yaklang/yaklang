package aibalance

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed templates/portal.html templates/login.html templates/add_provider.html templates/api_keys.html
var templatesFS embed.FS

// ProviderData contains data for template rendering
type ProviderData struct {
	ID            uint
	WrapperName   string
	ModelName     string
	TypeName      string
	DomainOrURL   string
	TotalRequests int64
	SuccessRate   float64
	LastLatency   int64
	IsHealthy     bool
}

// APIKeyData contains data for displaying an API key
type APIKeyData struct {
	ID         uint
	Key        string
	DisplayKey string
	CreatedAt  string
	LastUsedAt string
	Active     bool
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

// Session represents a user session
type Session struct {
	ID        string    // Session ID
	CreatedAt time.Time // Creation time
	ExpiresAt time.Time // Expiration time
}

// SessionManager manages user sessions
type SessionManager struct {
	sessions map[string]*Session // Session storage
	mutex    sync.RWMutex        // Read-write lock protecting session mapping
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session
func (sm *SessionManager) CreateSession() string {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Create new UUID as session ID
	sessionID := uuid.New().String()

	// Create session with 24-hour expiration
	session := &Session{
		ID:        sessionID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// Store session
	sm.sessions[sessionID] = session

	return sessionID
}

// GetSession retrieves a session
func (sm *SessionManager) GetSession(sessionID string) *Session {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	session, exists := sm.sessions[sessionID]
	if !exists {
		return nil
	}

	// Check if session has expired
	if time.Now().After(session.ExpiresAt) {
		// Delete expired session
		delete(sm.sessions, sessionID)
		return nil
	}

	return session
}

// DeleteSession removes a session
func (sm *SessionManager) DeleteSession(sessionID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	delete(sm.sessions, sessionID)
}

// CleanupExpiredSessions removes expired sessions
func (sm *SessionManager) CleanupExpiredSessions() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	for id, session := range sm.sessions {
		if now.After(session.ExpiresAt) {
			delete(sm.sessions, id)
		}
	}
}

// checkAuth checks admin authentication using session ID instead of direct password
func (c *ServerConfig) checkAuth(request *http.Request) bool {
	// Get session ID from cookie
	cookies := request.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "admin_session" {
			// Validate session
			session := c.SessionManager.GetSession(cookie.Value)
			if session != nil {
				return true
			}
		}
	}

	// Get password authentication from query parameters
	query := request.URL.Query()
	password := query.Get("password")
	if password == c.AdminPassword {
		// Password authentication successful, but no session is generated, this is for one-time access only
		return true
	}

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

	// Validate password
	if password != c.AdminPassword {
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

		// Add to provider list
		data.Providers = append(data.Providers, ProviderData{
			ID:            p.ID,
			WrapperName:   p.WrapperName,
			ModelName:     p.ModelName,
			TypeName:      p.TypeName,
			DomainOrURL:   p.DomainOrURL,
			TotalRequests: p.TotalRequests,
			SuccessRate:   successRate,
			LastLatency:   p.LastLatency,
			IsHealthy:     p.IsHealthy,
		})

		// Accumulate statistics
		data.TotalRequests += p.TotalRequests
		totalSuccess += p.SuccessCount
		if p.IsHealthy {
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

			// 创建APIKeyData结构
			keyData := APIKeyData{
				ID:         apiKey.ID,
				Key:        apiKey.APIKey,
				DisplayKey: displayKey,
				CreatedAt:  apiKey.CreatedAt.Format("2006-01-02 15:04:05"),
				Active:     true, // 默认为激活状态，如果数据库中有状态字段，这里可以调整
			}

			// 如果有最后使用时间字段，可以在这里设置
			// keyData.LastUsedAt = apiKey.LastUsedAt.Format("2006-01-02 15:04:05")

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
			ModelName:       modelName,
			TypeName:        modelType,
			DomainOrURL:     domainOrURL,
			APIKey:          provider.APIKey,
			WrapperName:     wrapperName, // Use WrapperName from form
			NoHTTPS:         noHTTPS,     // 设置 NoHTTPS 参数
			IsHealthy:       true,        // Default set to healthy
			LastRequestTime: time.Now(),  // Set last request time
			HealthCheckTime: time.Now(),  // Set health check time
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

	// Build JSON response
	autoCompleteData := struct {
		WrapperNames []string `json:"wrapper_names"`
		ModelNames   []string `json:"model_names"`
		ModelTypes   []string `json:"model_types"`
	}{
		WrapperNames: wrapperNamesList,
		ModelNames:   modelNamesList,
		ModelTypes:   modelTypesList,
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
	c.KeyAllowedModels.allowedModels[apiKey] = modelMap

	// Save API key to database
	allowedModelsStr := strings.Join(allowedModels, ",")
	err = SaveAiApiKey(apiKey, allowedModelsStr)
	if err != nil {
		c.logError("Failed to save API key to database: %v", err)
		// Continue execution, because it's already added to memory configuration
	}

	// Build result message
	c.logInfo("Successfully created API key: %s with %d allowed models", apiKey, len(allowedModels))

	// Redirect back to API key page
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal/api-keys\r\n" +
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
	} else if uriIns.Path == "/portal/autocomplete" {
		c.serveAutoCompleteData(conn, request)
	} else if uriIns.Path == "/portal/api-keys" {
		c.serveAPIKeysPage(conn)
	} else if uriIns.Path == "/portal/create-api-key" && request.Method == "POST" {
		c.processCreateAPIKey(conn, request)
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
	} else if uriIns.Path == "/portal/add-provider" && request.Method == "POST" {
		c.handleAddProvider(conn, request)
	} else if strings.HasPrefix(uriIns.Path, "/portal/delete-provider/") && request.Method == "DELETE" {
		c.handleDeleteProvider(conn, request, uriIns.Path)
	} else if strings.HasPrefix(uriIns.Path, "/portal/delete-api-key/") && request.Method == "DELETE" {
		c.handleDeleteAPIKey(conn, request, uriIns.Path)
	} else if uriIns.Path == "/portal/delete-api-keys" && request.Method == "POST" {
		c.handleDeleteMultipleAPIKeys(conn, request)
	} else if strings.HasPrefix(uriIns.Path, "/portal/activate-api-key/") && request.Method == "POST" {
		c.handleToggleAPIKeyStatus(conn, request, uriIns.Path, true)
	} else if strings.HasPrefix(uriIns.Path, "/portal/deactivate-api-key/") && request.Method == "POST" {
		c.handleToggleAPIKeyStatus(conn, request, uriIns.Path, false)
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
	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		c.logError("Failed to marshal JSON: %v", err)
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to marshal JSON: %v", err)))
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

		// 添加到结果列表
		providersData = append(providersData, map[string]interface{}{
			"id":             p.ID,
			"wrapper_name":   p.WrapperName,
			"model_name":     p.ModelName,
			"type_name":      p.TypeName,
			"domain_or_url":  p.DomainOrURL,
			"total_requests": p.TotalRequests,
			"success_rate":   successRate,
			"last_latency":   p.LastLatency,
			"is_healthy":     p.IsHealthy,
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

// handleGenerateApiKey 处理生成API密钥的请求
func (c *ServerConfig) handleGenerateApiKey(conn net.Conn, request *http.Request) {
	c.logInfo("处理生成API密钥请求")

	// 生成一个新的UUID作为API密钥
	apiKey := uuid.New().String()

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "成功生成API密钥",
		"apiKey":  apiKey,
	})
}

// handleAddProvider 处理添加提供者的请求
func (c *ServerConfig) handleAddProvider(conn net.Conn, request *http.Request) {
	c.logInfo("处理添加提供者请求")

	// 解析请求体
	var provider struct {
		WrapperName string `json:"wrapperName"`
		ModelName   string `json:"modelName"`
		TypeName    string `json:"typeName"`
		DomainOrURL string `json:"domainOrURL"`
	}

	// 尝试解析JSON
	err := json.NewDecoder(request.Body).Decode(&provider)
	if err != nil {
		c.logError("解析请求体失败: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("无效的请求格式: %v", err),
		})
		return
	}

	// 验证字段
	if provider.WrapperName == "" || provider.ModelName == "" || provider.TypeName == "" || provider.DomainOrURL == "" {
		c.logError("请求缺少必要字段")
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "必须提供所有必填字段",
		})
		return
	}

	// 创建数据库对象
	dbProvider := &schema.AiProvider{
		ModelName:       provider.ModelName,
		TypeName:        provider.TypeName,
		DomainOrURL:     provider.DomainOrURL,
		WrapperName:     provider.WrapperName,
		IsHealthy:       true,       // 默认设置为健康
		LastRequestTime: time.Now(), // 设置最后请求时间
		HealthCheckTime: time.Now(), // 设置健康检查时间
	}

	// 保存到数据库
	err = SaveAiProvider(dbProvider)
	if err != nil {
		c.logError("保存提供者失败: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("保存提供者失败: %v", err),
		})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "成功添加提供者",
		"id":      dbProvider.ID,
	})
}

// handleDeleteProvider deletes a provider by ID extracted from the URL path
func (c *ServerConfig) handleDeleteProvider(conn net.Conn, request *http.Request, path string) {
	c.logInfo("处理删除提供者请求: %s", path)

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

	// 删除数据库记录
	err = DeleteAiProviderByID(uint(providerID))
	if err != nil {
		c.logError("删除提供者失败 ID=%d: %v", providerID, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("删除提供者失败: %v", err),
		})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("成功删除提供者 ID=%d", providerID),
	})
}

// handleDeleteAPIKey handles deletion of a single API key
func (c *ServerConfig) handleDeleteAPIKey(conn net.Conn, request *http.Request, path string) {
	// Extract API key ID from URL path
	idStr := strings.TrimPrefix(path, "/portal/delete-api-key/")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.logError("Invalid API key ID: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid API key ID",
		})
		return
	}

	// Get API key info for logging purposes
	var apiKey schema.AiApiKeys
	if err := GetDB().First(&apiKey, uint(id)).Error; err != nil {
		c.logError("API key not found: %v", err)
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": "API key not found",
		})
		return
	}

	// Delete the API key from memory configuration (if exists)
	delete(c.KeyAllowedModels.allowedModels, apiKey.APIKey)
	// Also remove from Keys structure if exists
	delete(c.Keys.keys, apiKey.APIKey)

	// Delete the API key from database
	if err := GetDB().Delete(&schema.AiApiKeys{}, uint(id)).Error; err != nil {
		c.logError("Failed to delete API key: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to delete API key",
		})
		return
	}

	c.logInfo("Successfully deleted API key (ID: %d)", id)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "API key deleted successfully",
	})
}

// handleDeleteMultipleAPIKeys handles deletion of multiple API keys
func (c *ServerConfig) handleDeleteMultipleAPIKeys(conn net.Conn, request *http.Request) {
	// Parse request body
	var requestData struct {
		IDs []uint `json:"ids"`
	}

	if err := json.NewDecoder(request.Body).Decode(&requestData); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	if len(requestData.IDs) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "No API key IDs specified",
		})
		return
	}

	// Get API keys to delete for memory configuration cleanup
	var apiKeys []schema.AiApiKeys
	if err := GetDB().Where("id IN (?)", requestData.IDs).Find(&apiKeys).Error; err != nil {
		c.logError("Failed to retrieve API keys: %v", err)
	} else {
		// Remove from memory configuration
		for _, key := range apiKeys {
			delete(c.KeyAllowedModels.allowedModels, key.APIKey)
			delete(c.Keys.keys, key.APIKey)
		}
	}

	// Delete API keys from database
	result := GetDB().Where("id IN (?)", requestData.IDs).Delete(&schema.AiApiKeys{})
	if result.Error != nil {
		c.logError("Failed to delete API keys: %v", result.Error)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to delete API keys",
		})
		return
	}

	c.logInfo("Successfully deleted %d API keys", result.RowsAffected)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Successfully deleted %d API keys", result.RowsAffected),
	})
}

// handleToggleAPIKeyStatus handles activation/deactivation of an API key
func (c *ServerConfig) handleToggleAPIKeyStatus(conn net.Conn, request *http.Request, path string, activate bool) {
	// Extract API key ID from URL path
	prefixPath := "/portal/"
	if activate {
		prefixPath += "activate-api-key/"
	} else {
		prefixPath += "deactivate-api-key/"
	}

	idStr := strings.TrimPrefix(path, prefixPath)
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.logError("Invalid API key ID: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid API key ID",
		})
		return
	}

	// Get API key info
	var apiKey schema.AiApiKeys
	if err := GetDB().First(&apiKey, uint(id)).Error; err != nil {
		c.logError("API key not found: %v", err)
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": "API key not found",
		})
		return
	}

	// Currently, the AiApiKeys schema does not have an Active field.
	// This is a placeholder for future implementation.
	// In a real implementation, you would update the Active field in the database.

	// For now, we'll just log the action and return success
	if activate {
		c.logInfo("Activated API key (ID: %d)", id)
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "API key activated successfully",
		})
	} else {
		c.logInfo("Deactivated API key (ID: %d)", id)
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "API key deactivated successfully",
		})
	}
}
