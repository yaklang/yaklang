package aibalance

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// ==================== Model Metadata Handlers ====================

// handleUpdateModelMeta handles requests to update model metadata
func (c *ServerConfig) handleUpdateModelMeta(conn net.Conn, request *http.Request) {
	c.logInfo("Processing update model meta request")

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
		ModelName         string   `json:"model_name"`
		Description       string   `json:"description"`
		Tags              string   `json:"tags"`
		TrafficMultiplier *float64 `json:"traffic_multiplier,omitempty"` // Use pointer to detect if provided
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body for model meta update: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Failed to read request body",
		})
		return
	}
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("Failed to unmarshal request body for model meta update: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request body format",
		})
		return
	}

	if reqBody.ModelName == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Model name is required",
		})
		return
	}

	// Determine traffic multiplier value (-1 means don't update)
	multiplier := -1.0
	if reqBody.TrafficMultiplier != nil {
		multiplier = *reqBody.TrafficMultiplier
		if multiplier < 0 {
			multiplier = 1.0 // Default negative to 1.0
		}
	}

	// Save to DB with traffic multiplier
	if err := SaveModelMetaWithMultiplier(reqBody.ModelName, reqBody.Description, reqBody.Tags, multiplier); err != nil {
		c.logError("Failed to save model meta: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to save metadata: %v", err),
		})
		return
	}

	c.logInfo("Successfully updated metadata for model %s (multiplier: %.2f)", reqBody.ModelName, multiplier)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Model metadata updated successfully",
	})
}
