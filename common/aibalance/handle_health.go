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
)

// ==================== Health Check Handlers ====================

// serveHealthCheckAPI handles health check API requests
func (c *ServerConfig) serveHealthCheckAPI(conn net.Conn, request *http.Request) {
	c.logInfo("Handling health check API request")

	// Parse request body to check if a specific provider is requested
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

	// If a specific provider is requested
	if requestData.ProviderID > 0 {
		result, err := RunSingleProviderHealthCheck(requestData.ProviderID)
		if err != nil {
			c.logError("Health check failed for provider ID=%d: %v", requestData.ProviderID, err)
			c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("Health check failed: %v", err),
			})
			return
		}

		var errorMsg string
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}

		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"id":           result.Provider.ID,
				"name":         result.Provider.WrapperName,
				"healthy":      result.IsHealthy,
				"responseTime": result.ResponseTime,
				"error":        errorMsg,
			},
		})
		return
	}

	// Return all providers' health status
	providers, err := GetAllAiProviders()
	if err != nil {
		c.logError("Failed to get providers: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Failed to get providers",
		})
		return
	}

	providerHealth := make([]map[string]interface{}, 0, len(providers))
	for _, p := range providers {
		lastRequestTime := ""
		if !p.LastRequestTime.IsZero() {
			lastRequestTime = p.LastRequestTime.Format("2006-01-02 15:04:05")
		}
		providerHealth = append(providerHealth, map[string]interface{}{
			"id":                p.ID,
			"wrapper_name":      p.WrapperName,
			"type_name":         p.TypeName,
			"is_healthy":        p.IsHealthy,
			"last_latency":      p.LastLatency,
			"last_request_time": lastRequestTime,
		})
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":   true,
		"providers": providerHealth,
	})
}

// handleCheckSingleHealth checks health for a single provider
func (c *ServerConfig) handleCheckSingleHealth(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Processing single health check request: %s", path)

	// Extract provider ID from path
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

	// Execute health check
	result, err := RunSingleProviderHealthCheck(uint(providerID))
	if err != nil {
		c.logError("Provider health check failed ID=%d: %v", providerID, err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Health check failed: %v", err),
		})
		return
	}

	// Build response
	var errorMsg string
	if result.Error != nil {
		errorMsg = result.Error.Error()
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Provider %d health check completed", providerID),
		"data": map[string]interface{}{
			"id":           result.Provider.ID,
			"name":         result.Provider.WrapperName,
			"healthy":      result.IsHealthy,
			"responseTime": result.ResponseTime,
			"error":        errorMsg,
		},
	})
}

// handleCheckAllHealth checks health for all providers
func (c *ServerConfig) handleCheckAllHealth(conn net.Conn, request *http.Request) {
	c.logInfo("Processing check all health request")

	// Execute health check
	results, err := RunManualHealthCheck()
	if err != nil {
		c.logError("Health check failed: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Health check failed: %v", err),
		})
		return
	}

	// Count results
	totalCount := len(results)
	healthyCount := 0
	for _, result := range results {
		if result != nil && result.IsHealthy {
			healthyCount++
		}
	}

	// Calculate health rate
	healthRate := 0.0
	if totalCount > 0 {
		healthRate = float64(healthyCount) * 100 / float64(totalCount)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":      true,
		"message":      fmt.Sprintf("Health check completed: %d/%d providers healthy", healthyCount, totalCount),
		"totalCount":   totalCount,
		"healthyCount": healthyCount,
		"healthRate":   healthRate,
		"checkTime":    time.Now().Format("2006-01-02 15:04:05"),
	})
}
