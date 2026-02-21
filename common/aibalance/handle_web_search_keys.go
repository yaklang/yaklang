package aibalance

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

// handleGetWebSearchApiKeys lists all web search API keys, with optional filtering by searcher_type
func (c *ServerConfig) handleGetWebSearchApiKeys(conn net.Conn, request *http.Request) {
	c.logInfo("processing get web search api keys request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Check for optional searcher_type filter from query params
	searcherType := request.URL.Query().Get("searcher_type")

	var keys []*schema.WebSearchApiKey
	var err error
	if searcherType != "" {
		keys, err = GetWebSearchApiKeysByType(searcherType)
	} else {
		keys, err = GetAllWebSearchApiKeys()
	}

	if err != nil {
		c.logError("failed to get web search api keys: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get web search api keys"})
		return
	}

	// Convert to API response format (mask the API key)
	keyData := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		maskedKey := maskAPIKey(k.APIKey)
		keyData = append(keyData, map[string]interface{}{
			"id":             k.ID,
			"searcher_type":  k.SearcherType,
			"api_key":        maskedKey,
			"base_url":       k.BaseURL,
			"proxy":          k.Proxy,
			"active":         k.Active,
			"success_count":  k.SuccessCount,
			"failure_count":  k.FailureCount,
			"total_requests": k.TotalRequests,
			"last_used_time": k.LastUsedTime.Format("2006-01-02 15:04:05"),
			"last_latency":   k.LastLatency,
			"is_healthy":     k.IsHealthy,
			"created_at":     k.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"keys":    keyData,
		"total":   len(keyData),
	})
}

// handleCreateWebSearchApiKey creates one or more web search API keys
// Supports batch creation: api_keys field accepts newline-separated keys
func (c *ServerConfig) handleCreateWebSearchApiKey(conn net.Conn, request *http.Request) {
	c.logInfo("processing create web search api key request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		SearcherType string `json:"searcher_type"`
		APIKeys      string `json:"api_keys"` // newline-separated keys for batch creation
		BaseURL      string `json:"base_url"`
		Proxy        string `json:"proxy"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	// Validate searcher type
	if reqBody.SearcherType == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "searcher_type is required",
		})
		return
	}
	validTypes := map[string]bool{"brave": true, "tavily": true, "chatglm": true, "bocha": true, "unifuncs": true}
	if !validTypes[reqBody.SearcherType] {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "searcher_type must be 'brave', 'tavily', 'chatglm', 'bocha' or 'unifuncs'",
		})
		return
	}

	// Parse API keys: split by newline, trim whitespace, skip empty lines
	var apiKeys []string
	for _, line := range strings.Split(reqBody.APIKeys, "\n") {
		key := strings.TrimSpace(line)
		if key != "" {
			apiKeys = append(apiKeys, key)
		}
	}

	if len(apiKeys) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "at least one api_key is required",
		})
		return
	}

	// Batch create
	var addedCount int
	var errors []string
	for _, apiKey := range apiKeys {
		key := &schema.WebSearchApiKey{
			SearcherType: reqBody.SearcherType,
			APIKey:       apiKey,
			BaseURL:      reqBody.BaseURL,
			Proxy:        reqBody.Proxy,
			Active:       true,
			IsHealthy:    true,
		}
		if err := SaveWebSearchApiKey(key); err != nil {
			c.logError("failed to save web search api key: %v", err)
			errors = append(errors, fmt.Sprintf("failed to save key %s: %v", maskAPIKey(apiKey), err))
			continue
		}
		addedCount++
	}

	c.logInfo("batch created %d web search api keys (type=%s, total_submitted=%d)", addedCount, reqBody.SearcherType, len(apiKeys))
	response := map[string]interface{}{
		"success": addedCount > 0,
		"message": fmt.Sprintf("added %d key(s)", addedCount),
		"added":   addedCount,
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}
	c.writeJSONResponse(conn, http.StatusOK, response)
}

// handleDeleteWebSearchApiKey deletes a web search API key by ID from URL path
func (c *ServerConfig) handleDeleteWebSearchApiKey(conn net.Conn, request *http.Request, path string) {
	c.logInfo("processing delete web search api key request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	if err := DeleteWebSearchApiKeyByID(uint(id)); err != nil {
		c.logError("failed to delete web search api key ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("failed to delete web search api key: %v", err),
		})
		return
	}

	c.logInfo("successfully deleted web search api key ID=%d", id)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "web search api key deleted successfully",
	})
}

// handleUpdateWebSearchApiKey updates a web search API key by ID from URL path
func (c *ServerConfig) handleUpdateWebSearchApiKey(conn net.Conn, request *http.Request, path string) {
	c.logInfo("processing update web search api key request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
		Proxy   string `json:"proxy"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	key, err := GetWebSearchApiKeyByID(uint(id))
	if err != nil {
		c.logError("web search api key not found ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": "web search api key not found",
		})
		return
	}

	// Update fields if provided
	if reqBody.APIKey != "" {
		key.APIKey = reqBody.APIKey
	}
	if reqBody.BaseURL != "" {
		key.BaseURL = reqBody.BaseURL
	}
	// Allow clearing proxy by sending empty string explicitly
	key.Proxy = reqBody.Proxy

	if err := UpdateWebSearchApiKey(key); err != nil {
		c.logError("failed to update web search api key ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("failed to update web search api key: %v", err),
		})
		return
	}

	c.logInfo("successfully updated web search api key ID=%d", id)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "web search api key updated successfully",
	})
}

// handleToggleWebSearchApiKeyStatus activates or deactivates a web search API key
func (c *ServerConfig) handleToggleWebSearchApiKeyStatus(conn net.Conn, request *http.Request, path string, activate bool) {
	action := "activate"
	if !activate {
		action = "deactivate"
	}
	c.logInfo("processing %s web search api key request: %s", action, path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	if err := UpdateWebSearchApiKeyStatus(uint(id), activate); err != nil {
		c.logError("failed to %s web search api key ID=%d: %v", action, id, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("failed to %s web search api key: %v", action, err),
		})
		return
	}

	c.logInfo("successfully %sd web search api key ID=%d", action, id)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("web search api key %sd successfully", action),
	})
}

// handleResetWebSearchApiKeyHealth resets the health status of a web search API key
func (c *ServerConfig) handleResetWebSearchApiKeyHealth(conn net.Conn, request *http.Request, path string) {
	c.logInfo("processing reset web search api key health request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	if err := ResetWebSearchApiKeyHealth(uint(id)); err != nil {
		c.logError("failed to reset web search api key health ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("failed to reset health: %v", err),
		})
		return
	}

	c.logInfo("successfully reset web search api key health ID=%d", id)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "web search api key health reset successfully",
	})
}

// handleTestWebSearchApiKey tests a web search API key by performing a simple search
func (c *ServerConfig) handleTestWebSearchApiKey(conn net.Conn, request *http.Request, path string) {
	c.logInfo("processing test web search api key request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	sk, err := GetWebSearchApiKeyByID(uint(id))
	if err != nil {
		c.logError("web search api key not found ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": "web search api key not found",
		})
		return
	}

	c.logInfo("testing web search api key ID=%d, type=%s", sk.ID, sk.SearcherType)

	// Perform a simple test search
	testReq := &WebSearchRequest{
		Query:        "test",
		SearcherType: sk.SearcherType,
		Page:         1,
		PageSize:     3,
		MaxResults:   3,
	}

	startTime := time.Now()
	results, searchErr := c.doWebSearch(sk, testReq)
	latencyMs := time.Since(startTime).Milliseconds()

	if searchErr != nil {
		c.logError("web search api key test failed ID=%d: %v", id, searchErr)
		// Update stats: failure
		UpdateWebSearchApiKeyStats(sk.ID, false, latencyMs)
		// Increment global web search counter
		if incErr := IncrementWebSearchConfigTotalRequests(); incErr != nil {
			c.logError("failed to increment web search counter during test: %v", incErr)
		}
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success":    false,
			"message":    fmt.Sprintf("test failed: %v", searchErr),
			"latency_ms": latencyMs,
		})
		return
	}

	// Update stats: success
	UpdateWebSearchApiKeyStats(sk.ID, true, latencyMs)
	// Increment global web search counter
	if incErr := IncrementWebSearchConfigTotalRequests(); incErr != nil {
		c.logError("failed to increment web search counter during test: %v", incErr)
	}

	c.logInfo("web search api key test passed ID=%d, returned %d results, latency=%dms", id, len(results), latencyMs)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":       true,
		"message":       fmt.Sprintf("test passed, returned %d results", len(results)),
		"latency_ms":    latencyMs,
		"result_count":  len(results),
	})
}

// ==================== Web Search Global Config ====================

// handleGetWebSearchConfig returns the global web search config
func (c *ServerConfig) handleGetWebSearchConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing get web search config request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	config, err := GetWebSearchConfig()
	if err != nil {
		c.logError("failed to get web search config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get web search config"})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"config": map[string]interface{}{
			"proxy":                      config.Proxy,
			"allow_free_user_web_search": config.AllowFreeUserWebSearch,
		},
	})
}

// handleSetWebSearchConfig updates the global web search config
func (c *ServerConfig) handleSetWebSearchConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing set web search config request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer request.Body.Close()

	var reqBody struct {
		Proxy                  string `json:"proxy"`
		AllowFreeUserWebSearch *bool  `json:"allow_free_user_web_search,omitempty"` // pointer to distinguish absent from false
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	config, err := GetWebSearchConfig()
	if err != nil {
		c.logError("failed to get web search config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get web search config"})
		return
	}

	config.Proxy = strings.TrimSpace(reqBody.Proxy)
	if reqBody.AllowFreeUserWebSearch != nil {
		config.AllowFreeUserWebSearch = *reqBody.AllowFreeUserWebSearch
	}
	if err := SaveWebSearchConfig(config); err != nil {
		c.logError("failed to save web search config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to save web search config"})
		return
	}

	// Update in-memory config
	c.WebSearchProxy = config.Proxy

	c.logInfo("successfully updated web search config, proxy=%s, allow_free_user=%v", config.Proxy, config.AllowFreeUserWebSearch)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "web search config updated successfully",
	})
}

// ==================== Helper Functions ====================

// extractIDFromPath extracts the numeric ID from the last segment of a URL path
func extractIDFromPath(path string) (uint64, error) {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("empty path")
	}
	idStr := parts[len(parts)-1]
	return strconv.ParseUint(idStr, 10, 32)
}

// maskAPIKey masks an API key for display, showing only first 4 and last 4 chars
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "****" + apiKey[len(apiKey)-4:]
}
