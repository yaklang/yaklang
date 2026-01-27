package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

// ==================== Provider Page Handlers ====================

// serveAddProviderPage serves the page for adding new providers
func (c *ServerConfig) serveAddProviderPage(conn net.Conn, request *http.Request) {
	c.logInfo("Serving add provider page")

	// Get unique wrapper names from existing providers for autocomplete
	providers, err := GetAllAiProviders()
	wrapperNames := make([]string, 0)
	if err == nil {
		wrapperSet := make(map[string]bool)
		for _, p := range providers {
			if p.WrapperName != "" && !wrapperSet[p.WrapperName] {
				wrapperSet[p.WrapperName] = true
				wrapperNames = append(wrapperNames, p.WrapperName)
			}
		}
	}

	// Get available AI types from aispec
	aiTypes := aispec.RegisteredAIGateways()

	// Prepare template data
	data := struct {
		WrapperNames []string
		AITypes      []string
	}{
		WrapperNames: wrapperNames,
		AITypes:      aiTypes,
	}

	// Render template
	var htmlBuffer bytes.Buffer
	tmpl, err := template.ParseFS(templatesFS, "templates/index.html")
	if err != nil {
		c.logError("Failed to parse add provider template: %v", err)
		errorResponse := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\n\r\nFailed to read template: %v", err)
		conn.Write([]byte(errorResponse))
		return
	}

	err = tmpl.Execute(&htmlBuffer, data)
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

// processAddProviders handles requests to add one or more providers
// Supports both JSON and form-urlencoded formats
func (c *ServerConfig) processAddProviders(conn net.Conn, request *http.Request) {
	c.logInfo("Processing add providers request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	type ProviderData struct {
		WrapperName  string
		ModelName    string
		TypeName     string
		DomainOrURL  string
		APIKey       string
		NoHTTPS      bool
		ProviderMode string
	}

	var providers []ProviderData

	contentType := request.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Parse form data
		if err := request.ParseForm(); err != nil {
			c.logError("Failed to parse form: %v", err)
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Failed to parse form data"})
			return
		}

		wrapperName := request.FormValue("wrapper_name")
		modelName := request.FormValue("model_name")
		typeName := request.FormValue("model_type") // Frontend uses model_type
		if typeName == "" {
			typeName = request.FormValue("type_name")
		}
		domainOrURL := request.FormValue("domain_or_url")
		providerMode := request.FormValue("provider_mode")
		noHTTPS := request.FormValue("no_https") == "on"
		apiKeysStr := request.FormValue("api_keys")

		// If model_name is empty, use wrapper_name
		if modelName == "" {
			modelName = wrapperName
		}

		// Parse API keys (one per line)
		apiKeyLines := strings.Split(apiKeysStr, "\n")
		for _, apiKey := range apiKeyLines {
			apiKey = strings.TrimSpace(apiKey)
			if apiKey == "" {
				continue
			}
			providers = append(providers, ProviderData{
				WrapperName:  wrapperName,
				ModelName:    modelName,
				TypeName:     typeName,
				DomainOrURL:  domainOrURL,
				APIKey:       apiKey,
				NoHTTPS:      noHTTPS,
				ProviderMode: providerMode,
			})
		}
	} else {
		// Parse JSON body
		bodyBytes, err := io.ReadAll(request.Body)
		if err != nil {
			c.logError("Failed to read request body: %v", err)
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Failed to read request body"})
			return
		}
		defer request.Body.Close()

		var reqBody struct {
			Providers []struct {
				WrapperName  string `json:"wrapper_name"`
				ModelName    string `json:"model_name"`
				TypeName     string `json:"type_name"`
				DomainOrURL  string `json:"domain_or_url"`
				APIKey       string `json:"api_key"`
				NoHTTPS      bool   `json:"no_https"`
				ProviderMode string `json:"provider_mode"`
			} `json:"providers"`
		}

		if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
			c.logError("Failed to parse request body: %v", err)
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
			return
		}

		for _, p := range reqBody.Providers {
			modelName := p.ModelName
			if modelName == "" {
				modelName = p.WrapperName
			}
			providers = append(providers, ProviderData{
				WrapperName:  p.WrapperName,
				ModelName:    modelName,
				TypeName:     p.TypeName,
				DomainOrURL:  p.DomainOrURL,
				APIKey:       p.APIKey,
				NoHTTPS:      p.NoHTTPS,
				ProviderMode: p.ProviderMode,
			})
		}
	}

	if len(providers) == 0 {
		c.logError("No providers specified")
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "No providers specified"})
		return
	}

	// Process each provider
	var addedCount int
	var errors []string

	for _, p := range providers {
		if p.WrapperName == "" || p.TypeName == "" {
			errors = append(errors, "Provider missing wrapper_name or type_name")
			continue
		}

		// Create provider record
		provider := &schema.AiProvider{
			WrapperName:  p.WrapperName,
			ModelName:    p.ModelName,
			TypeName:     p.TypeName,
			DomainOrURL:  p.DomainOrURL,
			APIKey:       p.APIKey,
			NoHTTPS:      p.NoHTTPS,
			ProviderMode: p.ProviderMode,
			IsHealthy:    true, // Default to healthy
		}

		if err := SaveAiProvider(provider); err != nil {
			c.logError("Failed to save provider %s: %v", p.WrapperName, err)
			errors = append(errors, fmt.Sprintf("Failed to save %s: %v", p.WrapperName, err))
			continue
		}

		addedCount++
		c.logInfo("Successfully added provider: %s (model: %s, type: %s)", p.WrapperName, p.ModelName, p.TypeName)
	}

	// Reload providers into memory
	if addedCount > 0 {
		if err := LoadProvidersFromDatabase(c); err != nil {
			c.logError("Failed to reload providers: %v", err)
		}
	}

	// Prepare response
	response := map[string]interface{}{
		"success": addedCount > 0,
		"message": fmt.Sprintf("Added %d provider(s)", addedCount),
		"added":   addedCount,
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	c.writeJSONResponse(conn, http.StatusOK, response)
}

// ==================== Provider Autocomplete Handler ====================

// serveAutoCompleteData serves autocomplete data for provider forms
func (c *ServerConfig) serveAutoCompleteData(conn net.Conn, request *http.Request) {
	c.logInfo("Serving autocomplete data")

	// Get unique wrapper names, model names, and domain/URLs from existing providers
	providers, err := GetAllAiProviders()
	wrapperNames := make([]string, 0)
	modelNames := make([]string, 0)
	domainOrURLs := make([]string, 0)
	if err == nil {
		wrapperSet := make(map[string]bool)
		modelSet := make(map[string]bool)
		domainSet := make(map[string]bool)
		for _, p := range providers {
			if p.WrapperName != "" && !wrapperSet[p.WrapperName] {
				wrapperSet[p.WrapperName] = true
				wrapperNames = append(wrapperNames, p.WrapperName)
			}
			if p.ModelName != "" && !modelSet[p.ModelName] {
				modelSet[p.ModelName] = true
				modelNames = append(modelNames, p.ModelName)
			}
			if p.DomainOrURL != "" && !domainSet[p.DomainOrURL] {
				domainSet[p.DomainOrURL] = true
				domainOrURLs = append(domainOrURLs, p.DomainOrURL)
			}
		}
	}

	// Get AI types from aispec (registered gateway types)
	aiTypes := aispec.RegisteredAIGateways()

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"wrapper_names":  wrapperNames,
		"model_names":    modelNames,
		"model_types":    aiTypes, // Use model_types for frontend compatibility
		"domain_or_urls": domainOrURLs,
	})
}

// ==================== Provider Delete Handlers ====================

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

// handleDeleteMultipleProviders handles requests to delete multiple providers
func (c *ServerConfig) handleDeleteMultipleProviders(conn net.Conn, request *http.Request) {
	c.logInfo("Processing delete multiple providers request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Parse request body
	var reqBody struct {
		IDs []string `json:"ids"`
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Failed to read request body"})
		return
	}
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
		return
	}

	if len(reqBody.IDs) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "No provider IDs specified"})
		return
	}

	// Delete each provider
	var deletedCount int
	var errors []string

	for _, idStr := range reqBody.IDs {
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Invalid ID: %s", idStr))
			continue
		}

		if err := DeleteAiProviderByID(uint(id)); err != nil {
			errors = append(errors, fmt.Sprintf("Failed to delete ID %s: %v", idStr, err))
			continue
		}

		deletedCount++
	}

	// Reload providers
	if deletedCount > 0 {
		if err := LoadProvidersFromDatabase(c); err != nil {
			c.logError("Failed to reload providers: %v", err)
		}
	}

	c.logInfo("Successfully deleted %d providers", deletedCount)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":      deletedCount > 0,
		"message":      fmt.Sprintf("Deleted %d provider(s)", deletedCount),
		"deletedCount": deletedCount,
		"errors":       errors,
	})
}

// ==================== Provider Validation Handler ====================

// handleValidateProvider validates a provider configuration
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

	// Parse request - support both form data and JSON
	var typeName, domainOrURL, apiKey, modelName string
	var noHTTPS bool

	contentType := request.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Parse form data
		if err := request.ParseForm(); err != nil {
			c.logError("Failed to parse form: %v", err)
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Failed to parse form data"})
			return
		}
		typeName = request.FormValue("model_type")
		domainOrURL = request.FormValue("domain_or_url")
		// Use api_key_to_validate for validation (first API key from the list)
		apiKey = request.FormValue("api_key_to_validate")
		if apiKey == "" {
			apiKey = request.FormValue("api_key")
		}
		modelName = request.FormValue("model_name")
		noHTTPS = request.FormValue("no_https") == "on"
	} else {
		// Parse JSON body
		var reqBody struct {
			TypeName    string `json:"type_name"`
			ModelType   string `json:"model_type"`
			DomainOrURL string `json:"domain_or_url"`
			APIKey      string `json:"api_key"`
			ModelName   string `json:"model_name"`
			NoHTTPS     bool   `json:"no_https"`
		}

		bodyBytes, err := io.ReadAll(request.Body)
		if err != nil {
			c.logError("Failed to read request body: %v", err)
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Failed to read request body"})
			return
		}
		defer request.Body.Close()

		if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
			c.logError("Failed to parse request body: %v", err)
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
			return
		}

		typeName = reqBody.TypeName
		if typeName == "" {
			typeName = reqBody.ModelType
		}
		domainOrURL = reqBody.DomainOrURL
		apiKey = reqBody.APIKey
		modelName = reqBody.ModelName
		noHTTPS = reqBody.NoHTTPS
	}

	// Validate required fields
	if typeName == "" || apiKey == "" {
		c.logError("Missing required fields: type_name or api_key")
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Missing required fields: type_name and api_key are required",
		})
		return
	}

	c.logInfo("Validating provider: type=%s, domain=%s, model=%s", typeName, domainOrURL, modelName)

	// Create a temporary provider for validation
	provider := &Provider{
		TypeName:    typeName,
		DomainOrURL: domainOrURL,
		APIKey:      apiKey,
		ModelName:   modelName,
		NoHTTPS:     noHTTPS,
	}

	// Try to get a client and make a simple request
	client, err := provider.GetAIClient(nil, nil)
	if err != nil {
		c.logError("Failed to create client for validation: %v", err)
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to create client: %v", err),
		})
		return
	}

	// Try a simple chat completion
	_, err = client.Chat("Hello", aispec.WithTimeout(10))
	if err != nil {
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Validation failed: %v", err),
		})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Provider validated successfully",
	})
}

// ==================== Provider API Handler ====================

// serveProvidersAPI serves the providers API for getting provider data
func (c *ServerConfig) serveProvidersAPI(conn net.Conn, request *http.Request) {
	c.logInfo("Serving providers API")

	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "Failed to get providers"})
		return
	}

	// Convert to API response format
	providerData := make([]map[string]interface{}, 0, len(providers))
	for _, p := range providers {
		providerData = append(providerData, map[string]interface{}{
			"id":             p.ID,
			"wrapper_name":   p.WrapperName,
			"model_name":     p.ModelName,
			"type_name":      p.TypeName,
			"domain_or_url":  p.DomainOrURL,
			"is_healthy":     p.IsHealthy,
			"last_latency":   p.LastLatency,
			"success_count":  p.SuccessCount,
			"failure_count":  p.FailureCount,
			"total_requests": p.TotalRequests,
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":   true,
		"providers": providerData,
	})
}
