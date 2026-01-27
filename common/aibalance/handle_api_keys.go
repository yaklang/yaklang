package aibalance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	uuid "github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// ==================== API Key Page Handlers ====================

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

	// Note: This function references api_keys.html template which may not exist
	// The main portal page handles API key display
	c.logInfo("API keys page data prepared with %d keys", len(data.APIKeys))
	c.servePortalWithAuth(conn)
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

	// Save API key to database
	allowedModelsStr := strings.Join(allowedModels, ",")
	err = SaveAiApiKey(apiKey, allowedModelsStr)
	if err != nil {
		c.logError("Failed to save API key to database: %v", err)
	}

	// Reload API keys into memory
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after creating key '%s': %v", apiKey, err)
	} else {
		c.logInfo("Successfully reloaded API keys into memory after creating key '%s'.", apiKey)
	}

	// Build result message
	c.logInfo("Successfully created API key: %s with %d allowed models", apiKey, len(allowedModels))

	// Redirect back to portal page
	header := "HTTP/1.1 303 See Other\r\n" +
		"Location: /portal/\r\n" +
		"\r\n"
	conn.Write([]byte(header))
}

// ==================== API Key Generation Handlers ====================

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

	// Call function to generate and store API key
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
	apiKey := uuid.New().String()
	allowedModelsStr := strings.Join(allowedModels, ",")

	newKeyData := &schema.AiApiKeys{
		APIKey:        apiKey,
		AllowedModels: allowedModelsStr,
		UsageCount:    0,
		SuccessCount:  0,
		FailureCount:  0,
		InputBytes:    0,
		OutputBytes:   0,
		LastUsedTime:  time.Time{},
	}

	err := SaveAiApiKey(newKeyData.APIKey, newKeyData.AllowedModels)
	if err != nil {
		c.logError("Failed to store new API key in database: %v", err)
		return "", fmt.Errorf("failed to store new API key: %w", err)
	}

	// Reload API keys from DB
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after adding a new one: %v", err)
	}

	return apiKey, nil
}

// ==================== API Key Status Handlers ====================

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

	// Reload API Key configuration into memory
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after %s key ID %d: %v", action, id, err)
	} else {
		c.logInfo("Successfully reloaded API keys into memory after %s key ID %d.", action, id)
	}

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
		IDs []string `json:"ids"`
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
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body for batch %s API keys: %v", action, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	if len(reqBody.IDs) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "No API key IDs specified",
		})
		return
	}

	// Convert string IDs to uint
	uintIDs := make([]uint, 0, len(reqBody.IDs))
	for _, idStr := range reqBody.IDs {
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.logError("Invalid API key ID '%s' in batch %s: %v", idStr, action, err)
			continue
		}
		uintIDs = append(uintIDs, uint(id))
	}

	if len(uintIDs) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "No valid API key IDs provided",
		})
		return
	}

	// Batch update API key status
	affectedCount, err := BatchUpdateAiApiKeyStatus(uintIDs, activate)
	if err != nil {
		c.logError("Failed to batch %s API keys: %v", action, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to %s API keys", action),
		})
		return
	}

	c.logInfo("Successfully %sd %d API keys", action, affectedCount)

	// Reload API keys into memory
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after batch %s: %v", action, err)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":       true,
		"message":       fmt.Sprintf("Successfully %sd %d API keys", action, affectedCount),
		"affectedCount": affectedCount,
	})
}

// ==================== API Key Allowed Models Handlers ====================

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
		c.logError("Invalid API key ID '%s' for update allowed models: %v", idStr, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid API key ID format",
		})
		return
	}

	// Parse request body
	var reqBody struct {
		AllowedModels []string `json:"allowed_models"`
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body for allowed models update: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body for allowed models update: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	// Update allowed models
	allowedModelsStr := strings.Join(reqBody.AllowedModels, ",")
	err = UpdateAiApiKeyAllowedModels(uint(id), allowedModelsStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

// ==================== API Key Delete Handlers ====================

// handleDeleteAPIKey handles requests to delete a single API key
func (c *ServerConfig) handleDeleteAPIKey(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Processing delete API key request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Extract API key ID from URL path
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		c.logError("Invalid path format for delete API key: %s", path)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request path",
		})
		return
	}

	idStr := parts[len(parts)-1]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.logError("Invalid API key ID '%s' for delete: %v", idStr, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid API key ID format",
		})
		return
	}

	// Delete the API key
	err = DeleteAiApiKeyByID(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.logError("API key not found for ID %d during delete: %v", id, err)
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "API key not found",
			})
		} else {
			c.logError("Failed to delete API key (ID: %d): %v", id, err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": "Failed to delete API key",
			})
		}
		return
	}

	c.logInfo("Successfully deleted API key (ID: %d)", id)

	// Reload API keys into memory
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after deleting key ID %d: %v", id, err)
	} else {
		c.logInfo("Successfully reloaded API keys into memory after deleting key ID %d.", id)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "API key deleted successfully",
	})
}

// handleBatchDeleteAPIKeys handles requests to delete multiple API keys
func (c *ServerConfig) handleBatchDeleteAPIKeys(conn net.Conn, request *http.Request) {
	c.logInfo("Processing batch delete API keys request")

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
		c.logError("Failed to read request body for batch delete API keys: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body for batch delete API keys: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	if len(reqBody.IDs) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "No API key IDs specified",
		})
		return
	}

	// Convert string IDs to uint
	uintIDs := make([]uint, 0, len(reqBody.IDs))
	for _, idStr := range reqBody.IDs {
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.logError("Invalid API key ID '%s' in batch delete: %v", idStr, err)
			continue
		}
		uintIDs = append(uintIDs, uint(id))
	}

	if len(uintIDs) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "No valid API key IDs provided",
		})
		return
	}

	// Delete the API keys
	affectedCount, err := BatchDeleteAiApiKeys(uintIDs)
	if err != nil {
		c.logError("Failed to batch delete API keys: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to delete API keys",
		})
		return
	}

	c.logInfo("Successfully deleted %d API keys", affectedCount)

	// Reload API keys into memory
	err = c.LoadAPIKeysFromDB()
	if err != nil {
		c.logError("Failed to reload API keys into memory after batch delete: %v", err)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":      true,
		"message":      fmt.Sprintf("Successfully deleted %d API keys", affectedCount),
		"deletedCount": affectedCount,
	})
}

// ==================== API Key Traffic Limit Handlers ====================

// handleUpdateAPIKeyTrafficLimit handles requests to update API key traffic limit settings
func (c *ServerConfig) handleUpdateAPIKeyTrafficLimit(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Processing update API key traffic limit request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Extract API key ID from URL path
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		c.logError("Invalid path format for update traffic limit: %s", path)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request path",
		})
		return
	}

	idStr := parts[len(parts)-1]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.logError("Invalid API key ID '%s' for traffic limit update: %v", idStr, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid API key ID format",
		})
		return
	}

	// Parse request body
	var reqBody struct {
		TrafficLimit int64 `json:"traffic_limit"`
		Enable       bool  `json:"enable"`
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body for traffic limit update: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to parse request body for traffic limit update: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	// Update the traffic limit
	err = UpdateAiApiKeyTrafficLimit(uint(id), reqBody.TrafficLimit, reqBody.Enable)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "API key not found",
			})
		} else {
			c.logError("Failed to update traffic limit for API key (ID: %d): %v", id, err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": "Failed to update traffic limit",
			})
		}
		return
	}

	c.logInfo("Successfully updated traffic limit for API key (ID: %d)", id)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Traffic limit updated successfully",
	})
}

// handleResetAPIKeyTraffic handles requests to reset API key traffic used counter
func (c *ServerConfig) handleResetAPIKeyTraffic(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Processing reset API key traffic request: %s", path)

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Extract API key ID from URL path
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		c.logError("Invalid path format for reset traffic: %s", path)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request path",
		})
		return
	}

	idStr := parts[len(parts)-1]
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.logError("Invalid API key ID '%s' for traffic reset: %v", idStr, err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid API key ID format",
		})
		return
	}

	// Reset the traffic used
	err = ResetAiApiKeyTrafficUsed(uint(id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
				"success": false,
				"message": "API key not found",
			})
		} else {
			c.logError("Failed to reset traffic for API key (ID: %d): %v", id, err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": "Failed to reset traffic",
			})
		}
		return
	}

	c.logInfo("Successfully reset traffic for API key (ID: %d)", id)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Traffic reset successfully",
	})
}

// ==================== API Key Pagination Handlers ====================

// handleGetAPIKeysPaginated handles requests to get API keys with pagination
func (c *ServerConfig) handleGetAPIKeysPaginated(conn net.Conn, request *http.Request) {
	c.logInfo("Processing get API keys paginated request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Parse query parameters
	query := request.URL.Query()

	page := 1
	if pageStr := query.Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 20
	if pageSizeStr := query.Get("pageSize"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	sortBy := query.Get("sortBy")
	if sortBy == "" {
		sortBy = "created_at"
	}

	sortOrder := query.Get("sortOrder")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Get paginated API keys
	keys, total, err := GetAiApiKeysPaginated(page, pageSize, sortBy, sortOrder)
	if err != nil {
		c.logError("Failed to get paginated API keys: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to get API keys",
		})
		return
	}

	// Format the response
	keyDataList := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		displayKey := key.APIKey
		if len(displayKey) > 8 {
			displayKey = displayKey[:4] + "..." + displayKey[len(displayKey)-4:]
		}

		keyData := map[string]interface{}{
			"id":                   key.ID,
			"api_key":              key.APIKey,
			"display_key":          displayKey,
			"allowed_models":       key.AllowedModels,
			"input_bytes":          key.InputBytes,
			"output_bytes":         key.OutputBytes,
			"usage_count":          key.UsageCount,
			"success_count":        key.SuccessCount,
			"failure_count":        key.FailureCount,
			"active":               key.Active,
			"traffic_limit":        key.TrafficLimit,
			"traffic_used":         key.TrafficUsed,
			"traffic_limit_enable": key.TrafficLimitEnable,
			"created_at":           key.CreatedAt.Format("2006-01-02 15:04:05"),
		}

		if !key.LastUsedTime.IsZero() {
			keyData["last_used_time"] = key.LastUsedTime.Format("2006-01-02 15:04:05")
		}

		keyDataList = append(keyDataList, keyData)
	}

	// Calculate pagination info
	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    keyDataList,
		"pagination": map[string]interface{}{
			"page":       page,
			"pageSize":   pageSize,
			"total":      total,
			"totalPages": totalPages,
			"sortBy":     sortBy,
			"sortOrder":  sortOrder,
		},
	})
}
