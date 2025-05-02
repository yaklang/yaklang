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
	"strconv"
	"strings"
	"sync"
	"time"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed templates/portal.html templates/login.html templates/add_provider.html templates/healthy_check.html templates/api_keys.html
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

// PortalData contains all data for the management panel page
type PortalData struct {
	CurrentTime      string
	TotalProviders   int
	HealthyProviders int
	TotalRequests    int64
	SuccessRate      float64
	Providers        []ProviderData
	AllowedModels    map[string]string
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

// serveHealthyCheckPage handles health check requests
func (c *ServerConfig) serveHealthyCheckPage(conn net.Conn, request *http.Request) {
	c.logInfo("Serving healthy check page")

	// Check if a new health check needs to be executed
	query := request.URL.Query()
	runCheck := query.Get("run") == "true"

	var results []*HealthCheckResult
	if runCheck {
		// If specified run=true parameter, execute real-time health check
		c.logInfo("Running manual health check as requested")
		var err error
		results, err = RunManualHealthCheck()
		if err != nil {
			c.logError("Failed to run health check: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nHealth check failed: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// Get all providers (get latest status)
	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to get providers: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Count healthy providers
	healthyCount := 0
	for _, p := range providers {
		if p.IsHealthy {
			healthyCount++
		}
	}

	// Calculate health rate
	totalProviders := len(providers)
	healthRate := 0.0
	if totalProviders > 0 {
		healthRate = float64(healthyCount) * 100 / float64(totalProviders)
	}

	// Prepare template data
	data := struct {
		Providers      []*schema.AiProvider
		CheckTime      string
		TotalProviders int
		HealthyCount   int
		HealthRate     float64
		CheckResults   []*HealthCheckResult // Add health check results
		JustRanCheck   bool                 // Whether a check was just executed
	}{
		Providers:      providers,
		CheckTime:      time.Now().Format("2006-01-02 15:04:05"),
		TotalProviders: totalProviders,
		HealthyCount:   healthyCount,
		HealthRate:     healthRate,
		CheckResults:   results,
		JustRanCheck:   runCheck,
	}

	var tmpl *template.Template

	// Try to read template from filesystem
	if result := utils.GetFirstExistedFile(
		"common/aibalance/templates/healthy_check.html",
		"templates/healthy_check.html",
		"../templates/healthy_check.html",
	); result != "" {
		rawTemp, err := os.ReadFile(result)
		if err != nil {
			c.logError("Failed to read healthy check template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
		tmpl, err = template.New("healthy_check").Parse(string(rawTemp))
		if err != nil {
			c.logError("Failed to parse healthy check template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	} else {
		// Use embedded file system template
		tmpl, err = template.ParseFS(templatesFS, "templates/healthy_check.html")
		if err != nil {
			c.logError("Failed to parse embedded healthy check template: %v", err)
			errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
			conn.Write([]byte(errorResponse))
			return
		}
	}

	// Create a buffer to save rendered HTML
	var htmlBuffer bytes.Buffer
	err = tmpl.Execute(&htmlBuffer, data)
	if err != nil {
		c.logError("Failed to execute healthy check template: %v", err)
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
	modelTypes := make(map[string]bool)

	for _, p := range providers {
		if p.WrapperName != "" {
			wrapperNames[p.WrapperName] = true
		}
		if p.ModelName != "" {
			modelNames[p.ModelName] = true
		}
		if p.TypeName != "" {
			modelTypes[p.TypeName] = true
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

	modelTypesList := make([]string, 0, len(modelTypes))
	for typeName := range modelTypes {
		modelTypesList = append(modelTypesList, typeName)
	}

	// Add some common model types if they don't exist in the database
	commonTypes := []string{"ChatCompletion", "TextCompletion", "Embedding"}
	for _, typeName := range commonTypes {
		if _, exists := modelTypes[typeName]; !exists {
			modelTypesList = append(modelTypesList, typeName)
		}
	}

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
			if p.ModelName != "" {
				modelSet[p.ModelName] = true
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
	} else if uriIns.Path == "/portal/healthy-check" {
		c.serveHealthyCheckPage(conn, request)
	} else if uriIns.Path == "/portal/single-provider-health-check" {
		c.serveSingleProviderHealthCheckPage(conn, request)
	} else if uriIns.Path == "/portal/delete-providers" && request.Method == "POST" {
		c.processDeleteProviders(conn, request)
	} else if uriIns.Path == "/portal/logout" {
		c.handleLogout(conn, request)
	} else {
		// Default return home page
		c.servePortalWithAuth(conn)
	}
}

// serveSingleProviderHealthCheckPage handles requests for health check of a single provider
func (c *ServerConfig) serveSingleProviderHealthCheckPage(conn net.Conn, request *http.Request) {
	c.logInfo("Serving single provider health check page")

	// Parse query parameters to get provider ID
	query := request.URL.Query()
	providerIDStr := query.Get("id")
	if providerIDStr == "" {
		c.logError("Missing provider ID")
		errorResponse := "HTTP/1.1 400 Bad Request\r\n\r\nMissing provider ID parameter"
		conn.Write([]byte(errorResponse))
		return
	}

	// Convert ID to number
	providerIDInt, err := strconv.ParseUint(providerIDStr, 10, 64)
	if err != nil {
		c.logError("Invalid provider ID: %s, %v", providerIDStr, err)
		errorResponse := fmt.Sprintf("HTTP/1.1 400 Bad Request\r\n\r\nInvalid provider ID: %s", providerIDStr)
		conn.Write([]byte(errorResponse))
		return
	}
	providerID := uint(providerIDInt)

	// Execute health check
	result, err := RunSingleProviderHealthCheck(providerID)
	if err != nil {
		c.logError("Failed to run health check for provider %d: %v", providerID, err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nHealth check failed: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Prepare JSON response
	responseData := struct {
		Success      bool   `json:"success"`
		ProviderID   uint   `json:"provider_id"`
		ProviderName string `json:"provider_name"`
		IsHealthy    bool   `json:"is_healthy"`
		ResponseTime int64  `json:"response_time"`
		Message      string `json:"message"`
		CheckTime    string `json:"check_time"`
	}{
		Success:      true,
		ProviderID:   providerID,
		ProviderName: result.Provider.WrapperName,
		IsHealthy:    result.IsHealthy,
		ResponseTime: result.ResponseTime,
		CheckTime:    time.Now().Format("2006-01-02 15:04:05"),
	}

	if result.Error != nil {
		responseData.Message = result.Error.Error()
	} else if result.IsHealthy {
		responseData.Message = "Health check successful, provider is healthy"
	} else {
		responseData.Message = "Health check completed, provider is not healthy"
	}

	// Convert to JSON
	jsonData, err := json.Marshal(responseData)
	if err != nil {
		c.logError("Failed to encode response data: %v", err)
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

// processDeleteProviders handles requests to delete AI providers
func (c *ServerConfig) processDeleteProviders(conn net.Conn, request *http.Request) {
	c.logInfo("Processing delete providers request")

	// Limit request body size to prevent potential DOS attacks
	request.Body = http.MaxBytesReader(nil, request.Body, 1024*1024)

	// Parse request body
	var requestData struct {
		ProviderIDs []uint `json:"provider_ids"`
	}

	err := json.NewDecoder(request.Body).Decode(&requestData)
	if err != nil {
		c.logError("Failed to parse delete providers request: %v", err)
		responseData := map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to parse request: %v", err),
		}
		c.sendJSONResponse(conn, responseData, http.StatusBadRequest)
		return
	}

	// Validate provider IDs
	if len(requestData.ProviderIDs) == 0 {
		c.logError("No provider IDs specified for deletion")
		responseData := map[string]interface{}{
			"success": false,
			"message": "No providers specified for deletion",
		}
		c.sendJSONResponse(conn, responseData, http.StatusBadRequest)
		return
	}

	// Record delete operation
	c.logInfo("Deleting %d providers: %v", len(requestData.ProviderIDs), requestData.ProviderIDs)

	// Execute delete
	var failedIDs []uint
	for _, id := range requestData.ProviderIDs {
		err := DeleteAiProviderByID(id)
		if err != nil {
			c.logError("Failed to delete provider ID %d: %v", id, err)
			failedIDs = append(failedIDs, id)
		}
	}

	// Create response
	if len(failedIDs) > 0 {
		responseData := map[string]interface{}{
			"success":    false,
			"message":    fmt.Sprintf("Failed to delete some providers: %v", failedIDs),
			"failed_ids": failedIDs,
		}
		c.sendJSONResponse(conn, responseData, http.StatusInternalServerError)
	} else {
		responseData := map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Successfully deleted %d providers", len(requestData.ProviderIDs)),
		}
		c.sendJSONResponse(conn, responseData, http.StatusOK)
	}
}

// sendJSONResponse sends a JSON-formatted response
func (c *ServerConfig) sendJSONResponse(conn net.Conn, data interface{}, statusCode int) {
	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		c.logError("Failed to encode JSON response: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to encode data: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	// Prepare HTTP status line
	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = "Unknown"
	}

	// Prepare HTTP response header
	header := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, statusText) +
		"Content-Type: application/json; charset=utf-8\r\n" +
		"Content-Length: " + fmt.Sprintf("%d", len(jsonData)) + "\r\n" +
		"\r\n"

	// Write header and JSON content
	conn.Write([]byte(header))
	conn.Write(jsonData)
}
