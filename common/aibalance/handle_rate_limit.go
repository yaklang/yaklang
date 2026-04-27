package aibalance

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// defaultModelStatsMinRPM is the default minimum RPM threshold used by the
// "hot models" stats endpoint when the caller does not override it. The
// previous value of 20 was too high to surface most tuning candidates, so
// the threshold was lowered to 3 to expose anything beyond trivial traffic.
const defaultModelStatsMinRPM int64 = 3

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

	var modelDelayOverrides map[string]int64
	if config.ModelDelayOverrides != "" {
		json.Unmarshal([]byte(config.ModelDelayOverrides), &modelDelayOverrides)
	}
	if modelDelayOverrides == nil {
		modelDelayOverrides = make(map[string]int64)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"config": map[string]interface{}{
			"default_rpm":           config.DefaultRPM,
			"free_user_delay_sec":   config.FreeUserDelaySec,
			"model_rpm_overrides":   modelOverrides,
			"model_delay_overrides": modelDelayOverrides,
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
		DefaultRPM          *int64           `json:"default_rpm,omitempty"`
		FreeUserDelaySec    *int64           `json:"free_user_delay_sec,omitempty"`
		ModelRPMOverrides   map[string]int64 `json:"model_rpm_overrides,omitempty"`
		ModelDelayOverrides map[string]int64 `json:"model_delay_overrides,omitempty"`
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
	if reqBody.ModelDelayOverrides != nil {
		delayJSON, _ := json.Marshal(reqBody.ModelDelayOverrides)
		config.ModelDelayOverrides = string(delayJSON)
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

// handleGetRateLimitModelStats returns aggregated per-model RPM across all
// API keys for the current 60-second sliding window. Only models with
// RPM >= min_rpm (default 20) are returned. Admin only.
func (c *ServerConfig) handleGetRateLimitModelStats(conn net.Conn, request *http.Request) {
	c.logInfo("processing get rate limit model stats request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	minRPM := defaultModelStatsMinRPM
	if raw := strings.TrimSpace(request.URL.Query().Get("min_rpm")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed >= 1 {
			minRPM = parsed
		}
	}

	var models []ModelRPMStat
	if c.chatRateLimiter != nil {
		models = c.chatRateLimiter.GetModelRPMStats(minRPM)
	}
	if models == nil {
		models = []ModelRPMStat{}
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":        true,
		"models":         models,
		"min_rpm":        minRPM,
		"window_seconds": int64(rpmWindowDuration / 1e9),
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
		c.chatRateLimiter.ClearModelDelay()
		if cfg.ModelDelayOverrides != "" {
			var delayOverrides map[string]int64
			if err := json.Unmarshal([]byte(cfg.ModelDelayOverrides), &delayOverrides); err == nil {
				for model, delay := range delayOverrides {
					model = strings.TrimSpace(model)
					if model != "" && delay >= 0 {
						c.chatRateLimiter.SetModelDelay(model, delay)
					}
				}
			}
		}
	}
	c.freeUserDelaySec = cfg.FreeUserDelaySec
}
