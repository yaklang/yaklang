package aibalance

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// handleGetRateLimitConfig returns the global rate-limit configuration.
func (c *ServerConfig) handleGetRateLimitConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing get rate limit config request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	config, err := GetRateLimitConfig()
	if err != nil {
		c.logError("failed to get rate limit config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get rate limit config"})
		return
	}

	var modelOverrides map[string]int64
	if config.ModelRPMOverrides != "" {
		json.Unmarshal([]byte(config.ModelRPMOverrides), &modelOverrides)
	}
	if modelOverrides == nil {
		modelOverrides = make(map[string]int64)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"config": map[string]interface{}{
			"default_rpm":         config.DefaultRPM,
			"free_user_delay_sec": config.FreeUserDelaySec,
			"model_rpm_overrides": modelOverrides,
		},
	})
}

// handleSetRateLimitConfig updates the global rate-limit configuration.
func (c *ServerConfig) handleSetRateLimitConfig(conn net.Conn, request *http.Request) {
	c.logInfo("processing set rate limit config request")

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
		DefaultRPM        *int64            `json:"default_rpm,omitempty"`
		FreeUserDelaySec  *int64            `json:"free_user_delay_sec,omitempty"`
		ModelRPMOverrides map[string]int64  `json:"model_rpm_overrides,omitempty"`
	}
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		c.logError("failed to parse request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid request format"})
		return
	}

	config, err := GetRateLimitConfig()
	if err != nil {
		c.logError("failed to get rate limit config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to get rate limit config"})
		return
	}

	if reqBody.DefaultRPM != nil {
		config.DefaultRPM = *reqBody.DefaultRPM
	}
	if reqBody.FreeUserDelaySec != nil {
		config.FreeUserDelaySec = *reqBody.FreeUserDelaySec
	}
	if reqBody.ModelRPMOverrides != nil {
		overridesJSON, _ := json.Marshal(reqBody.ModelRPMOverrides)
		config.ModelRPMOverrides = string(overridesJSON)
	}

	if err := SaveRateLimitConfig(config); err != nil {
		c.logError("failed to save rate limit config: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "failed to save rate limit config"})
		return
	}

	// Apply to in-memory rate limiter
	c.applyRateLimitConfig(config)

	c.logInfo("successfully updated rate limit config: default_rpm=%d, free_user_delay=%ds",
		config.DefaultRPM, config.FreeUserDelaySec)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "rate limit config updated successfully",
	})
}

// handleGetRateLimitStatus returns real-time rate-limit status (queue count, etc).
func (c *ServerConfig) handleGetRateLimitStatus(conn net.Conn, request *http.Request) {
	c.logInfo("processing get rate limit status request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var queueCount int64
	var defaultRPM int64
	if c.chatRateLimiter != nil {
		queueCount = c.chatRateLimiter.GetQueueCount()
		defaultRPM = c.chatRateLimiter.defaultRPM.Load()
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":       true,
		"queue_count":   queueCount,
		"default_rpm":   defaultRPM,
	})
}

// applyRateLimitConfig applies the DB config to the in-memory rate limiter.
func (c *ServerConfig) applyRateLimitConfig(cfg *schema.AiBalanceRateLimitConfig) {
	if cfg == nil {
		return
	}
	if c.chatRateLimiter != nil {
		c.chatRateLimiter.SetDefaultRPM(cfg.DefaultRPM)
		c.chatRateLimiter.ClearModelRPM()
		if cfg.ModelRPMOverrides != "" {
			var overrides map[string]int64
			if err := json.Unmarshal([]byte(cfg.ModelRPMOverrides), &overrides); err == nil {
				for model, rpm := range overrides {
					model = strings.TrimSpace(model)
					if model != "" && rpm > 0 {
						c.chatRateLimiter.SetModelRPM(model, rpm)
					}
				}
			}
		}
	}
	c.freeUserDelaySec = cfg.FreeUserDelaySec
}
