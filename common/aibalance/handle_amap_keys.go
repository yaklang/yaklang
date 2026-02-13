package aibalance

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

const (
	// maxAmapApiKeyLength is the maximum allowed length for a single Amap API key
	maxAmapApiKeyLength = 128
	// maxAmapBatchSize is the maximum number of keys allowed in a single batch add
	maxAmapBatchSize = 100
	// maxAmapRequestBodySize is the maximum allowed request body size (64KB)
	maxAmapRequestBodySize = 64 * 1024
)

// amapApiKeyPattern validates that an API key contains only safe characters (alphanumeric and hyphens)
var amapApiKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`)

// sanitizeForOutput escapes HTML special characters in strings to prevent XSS
// when the frontend might insert them into the DOM
func sanitizeForOutput(s string) string {
	return html.EscapeString(s)
}

// validateAmapApiKey validates a single Amap API key format
func validateAmapApiKey(key string) error {
	if len(key) == 0 {
		return fmt.Errorf("empty key")
	}
	if len(key) > maxAmapApiKeyLength {
		return fmt.Errorf("key too long (max %d chars)", maxAmapApiKeyLength)
	}
	if !amapApiKeyPattern.MatchString(key) {
		return fmt.Errorf("key contains invalid characters (only alphanumeric, hyphens and underscores allowed)")
	}
	return nil
}

// handleGetAmapApiKeys lists all amap API keys
func (c *ServerConfig) handleGetAmapApiKeys(conn net.Conn, request *http.Request) {
	c.logInfo("processing get amap api keys request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	keys, err := GetAllAmapApiKeys()
	if err != nil {
		c.logError("failed to get amap api keys: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get amap api keys"})
		return
	}

	keyData := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		maskedKey := maskAPIKey(k.APIKey)
		healthCheckTimeStr := ""
		if !k.HealthCheckTime.IsZero() {
			healthCheckTimeStr = k.HealthCheckTime.Format("2006-01-02 15:04:05")
		}
		lastUsedTimeStr := ""
		if !k.LastUsedTime.IsZero() {
			lastUsedTimeStr = k.LastUsedTime.Format("2006-01-02 15:04:05")
		}
		keyData = append(keyData, map[string]interface{}{
			"id":                  k.ID,
			"api_key":             sanitizeForOutput(maskedKey),
			"active":              k.Active,
			"is_healthy":          k.IsHealthy,
			"health_check_time":   healthCheckTimeStr,
			"last_check_error":    sanitizeForOutput(k.LastCheckError),
			"success_count":       k.SuccessCount,
			"failure_count":       k.FailureCount,
			"consecutive_failures": k.ConsecutiveFailures,
			"total_requests":      k.TotalRequests,
			"last_used_time":      lastUsedTimeStr,
			"last_latency":        k.LastLatency,
			"created_at":          k.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"keys":    keyData,
		"total":   len(keyData),
	})
}

// handleCreateAmapApiKey creates one or more amap API keys
// Supports batch creation: api_keys field accepts newline-separated keys
func (c *ServerConfig) handleCreateAmapApiKey(conn net.Conn, request *http.Request) {
	c.logInfo("processing create amap api key request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Limit request body size to prevent abuse
	limitedReader := io.LimitReader(request.Body, maxAmapRequestBodySize+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		c.logError("failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer request.Body.Close()

	if len(bodyBytes) > maxAmapRequestBodySize {
		c.writeJSONResponse(conn, http.StatusRequestEntityTooLarge, map[string]string{
			"error": "request body too large",
		})
		return
	}

	var reqBody struct {
		APIKeys string `json:"api_keys"` // newline-separated keys for batch creation
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	if reqBody.APIKeys == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": "api_keys is required",
		})
		return
	}

	// Split by newlines and trim
	lines := strings.Split(reqBody.APIKeys, "\n")
	var apiKeys []string
	for _, line := range lines {
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

	// Enforce batch size limit
	if len(apiKeys) > maxAmapBatchSize {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("too many keys in batch (max %d)", maxAmapBatchSize),
		})
		return
	}

	// Validate each key format
	var addedCount int
	var validationErrors []string
	for _, apiKey := range apiKeys {
		if err := validateAmapApiKey(apiKey); err != nil {
			validationErrors = append(validationErrors, fmt.Sprintf("invalid key %s: %v", sanitizeForOutput(maskAPIKey(apiKey)), err))
			continue
		}

		key := &schema.AmapApiKey{
			APIKey:    apiKey,
			Active:    true,
			IsHealthy: true,
		}
		if err := SaveAmapApiKey(key); err != nil {
			c.logError("failed to save amap api key: %v", err)
			validationErrors = append(validationErrors, fmt.Sprintf("failed to save key %s: database error", sanitizeForOutput(maskAPIKey(apiKey))))
			continue
		}
		addedCount++
	}

	c.logInfo("batch created %d amap api keys (total_submitted=%d)", addedCount, len(apiKeys))
	response := map[string]interface{}{
		"success":     addedCount > 0,
		"added_count": addedCount,
		"total":       len(apiKeys),
	}
	if len(validationErrors) > 0 {
		response["errors"] = validationErrors
	}
	c.writeJSONResponse(conn, http.StatusOK, response)
}

// handleDeleteAmapApiKey deletes an amap API key by ID
func (c *ServerConfig) handleDeleteAmapApiKey(conn net.Conn, request *http.Request, path string) {
	c.logInfo("processing delete amap api key request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractAmapIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	if err := DeleteAmapApiKeyByID(uint(id)); err != nil {
		c.logError("failed to delete amap api key ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("failed to delete: %v", err),
		})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "key deleted",
	})
}

// handleToggleAmapApiKeyStatus toggles the active status of an amap API key
func (c *ServerConfig) handleToggleAmapApiKeyStatus(conn net.Conn, request *http.Request, path string) {
	c.logInfo("processing toggle amap api key status request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractAmapIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	key, err := GetAmapApiKeyByID(uint(id))
	if err != nil {
		c.logError("amap api key not found ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": "key not found",
		})
		return
	}

	newActive := !key.Active
	if err := UpdateAmapApiKeyStatus(uint(id), newActive); err != nil {
		c.logError("failed to toggle amap api key status ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("failed to toggle: %v", err),
		})
		return
	}

	status := "deactivated"
	if newActive {
		status = "activated"
	}
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("key %s", status),
		"active":  newActive,
	})
}

// handleResetAmapApiKeyHealth resets the health status of an amap API key
func (c *ServerConfig) handleResetAmapApiKeyHealth(conn net.Conn, request *http.Request, path string) {
	c.logInfo("processing reset amap api key health request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractAmapIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	if err := ResetAmapApiKeyHealth(uint(id)); err != nil {
		c.logError("failed to reset amap api key health ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("failed to reset: %v", err),
		})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "health status reset",
	})
}

// handleTestAmapApiKey tests a single amap API key by calling the weather API
func (c *ServerConfig) handleTestAmapApiKey(conn net.Conn, request *http.Request, path string) {
	c.logInfo("processing test amap api key request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	id, err := extractAmapIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("invalid ID: %v", err),
		})
		return
	}

	sk, err := GetAmapApiKeyByID(uint(id))
	if err != nil {
		c.logError("amap api key not found ID=%d: %v", id, err)
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": "amap api key not found",
		})
		return
	}

	c.logInfo("testing amap api key ID=%d", sk.ID)

	healthy, latencyMs, checkErr := CheckSingleAmapApiKey(sk.APIKey)

	checkErrorStr := ""
	if checkErr != nil {
		checkErrorStr = checkErr.Error()
	}

	// Update health status
	if dbErr := UpdateAmapApiKeyHealthStatus(sk.ID, healthy, latencyMs, checkErrorStr); dbErr != nil {
		c.logError("failed to update amap key health status: %v", dbErr)
	}

	// Update stats
	if dbErr := UpdateAmapApiKeyStats(sk.ID, healthy, latencyMs); dbErr != nil {
		c.logError("failed to update amap key stats: %v", dbErr)
	}

	if !healthy {
		c.logError("amap api key test failed ID=%d: %v", id, checkErr)
		errMsg := "unknown error"
		if checkErr != nil {
			errMsg = sanitizeForOutput(checkErr.Error())
		}
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success":    false,
			"message":    fmt.Sprintf("test failed: %s", errMsg),
			"latency_ms": latencyMs,
			"is_healthy": false,
		})
		return
	}

	c.logInfo("amap api key test passed ID=%d, latency=%dms", id, latencyMs)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":    true,
		"message":    "test passed",
		"latency_ms": latencyMs,
		"is_healthy": true,
	})
}

// handleCheckAllAmapApiKeys manually triggers a health check for all amap API keys
func (c *ServerConfig) handleCheckAllAmapApiKeys(conn net.Conn, request *http.Request) {
	c.logInfo("processing check all amap api keys request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	results, err := CheckAllAmapApiKeys()
	if err != nil {
		c.logError("failed to check all amap api keys: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("check failed: %v", err),
		})
		return
	}

	healthyCount := 0
	unhealthyCount := 0
	for _, r := range results {
		if r.IsHealthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	// Sanitize result strings for safe frontend display
	sanitizedResults := make([]map[string]interface{}, 0, len(results))
	for _, r := range results {
		sanitizedResults = append(sanitizedResults, map[string]interface{}{
			"key_id":     r.KeyID,
			"api_key":    sanitizeForOutput(r.APIKey),
			"is_healthy": r.IsHealthy,
			"latency_ms": r.LatencyMs,
			"error":      sanitizeForOutput(r.Error),
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":         true,
		"results":         sanitizedResults,
		"total":           len(results),
		"healthy_count":   healthyCount,
		"unhealthy_count": unhealthyCount,
	})
}

// handleGetAmapConfig returns the global amap config
func (c *ServerConfig) handleGetAmapConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing get amap config request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	config, err := GetAmapConfig()
	if err != nil {
		c.logError("failed to get amap config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get amap config"})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":              true,
		"allow_free_user_amap": config.AllowFreeUserAmap,
		"total_amap_requests":  config.TotalAmapRequests,
	})
}

// handleSetAmapConfig updates the global amap config
func (c *ServerConfig) handleSetAmapConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing set amap config request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	limitedReader := io.LimitReader(request.Body, maxAmapRequestBodySize+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}
	defer request.Body.Close()

	if len(bodyBytes) > maxAmapRequestBodySize {
		c.writeJSONResponse(conn, http.StatusRequestEntityTooLarge, map[string]string{
			"error": "request body too large",
		})
		return
	}

	var reqBody struct {
		AllowFreeUserAmap *bool `json:"allow_free_user_amap"`
	}

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	config, err := GetAmapConfig()
	if err != nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get current config"})
		return
	}

	if reqBody.AllowFreeUserAmap != nil {
		config.AllowFreeUserAmap = *reqBody.AllowFreeUserAmap
	}

	if err := SaveAmapConfig(config); err != nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to save config"})
		return
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "config updated",
	})
}

// extractAmapIDFromPath extracts the numeric ID from a path segment.
// Handles paths like /portal/amap/keys/123/test, /portal/amap/keys/123
func extractAmapIDFromPath(path string) (int, error) {
	// Remove trailing slash
	path = strings.TrimRight(path, "/")

	// Split path and find the ID segment
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "keys" && i+1 < len(parts) {
			idStr := parts[i+1]
			id, err := strconv.Atoi(idStr)
			if err != nil {
				return 0, fmt.Errorf("invalid id: %s", sanitizeForOutput(idStr))
			}
			if id <= 0 {
				return 0, fmt.Errorf("invalid id: must be a positive integer")
			}
			return id, nil
		}
	}
	return 0, fmt.Errorf("no id found in path")
}

