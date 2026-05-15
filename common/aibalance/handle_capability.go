package aibalance

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handle_capability.go 为 portal 提供"工具调用能力探测"的 HTTP 入口.
//
// 关键词: aibalance Tool Calls Capability Probe Portal API, /portal/probe-tool-calls/:id
//
// 路由 (在 portal.go HandlePortalRequest 注册):
//   POST /portal/probe-tool-calls/<providerID>   -> handleProbeToolCallsSingle
//   POST /portal/probe-tool-calls-all            -> handleProbeToolCallsAll
//
// 响应体 JSON:
//   {
//     "success":  true|false,
//     "message":  "...",
//     "data": {
//       "id":               123,
//       "wrapper_name":     "z-deepseek-v4-pro",
//       "round1_mode":      "react",
//       "round2_mode":      "react",
//       "probed_at":        "2026-05-15T13:45:01+08:00",
//       "error":            ""        // 或 "round2: timeout ..."
//     }
//   }

// handleProbeToolCallsSingle 对单个 provider 触发一次工具调用能力探测.
// 关键词: portal probe-tool-calls single, manual capability probe
func (c *ServerConfig) handleProbeToolCallsSingle(conn net.Conn, request *http.Request, path string) {
	c.logInfo("Processing tool-calls capability probe request: %s", path)

	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request path, expect /portal/probe-tool-calls/<providerID>",
		})
		return
	}
	idStr := parts[len(parts)-1]
	providerID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Invalid provider ID: %q", idStr),
		})
		return
	}

	dbProvider, err := GetAiProviderByID(uint(providerID))
	if err != nil {
		c.writeJSONResponse(conn, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Provider id=%d not found: %v", providerID, err),
		})
		return
	}

	result, saveErr := ProbeAndSaveByProviderID(uint(providerID))
	if result == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Probe failed and produced no result: %v", saveErr),
		})
		return
	}
	resp := map[string]interface{}{
		"success": saveErr == nil,
		"message": fmt.Sprintf("Tool calls capability probe completed for provider %d", providerID),
		"data": map[string]interface{}{
			"id":           providerID,
			"wrapper_name": dbProvider.WrapperName,
			"model_name":   dbProvider.ModelName,
			"type_name":    dbProvider.TypeName,
			"round1_mode":  result.Round1Mode,
			"round2_mode":  result.Round2Mode,
			"probed_at":    result.ProbedAt.Format(time.RFC3339),
			"error":        result.Error,
		},
	}
	if saveErr != nil {
		resp["message"] = fmt.Sprintf("Probe done but persistence failed: %v", saveErr)
	}
	c.writeJSONResponse(conn, http.StatusOK, resp)
}

// handleProbeToolCallsAll 对所有 provider 批量触发能力探测.
// 关键词: portal probe-tool-calls all, batch capability probe
func (c *ServerConfig) handleProbeToolCallsAll(conn net.Conn, request *http.Request) {
	c.logInfo("Processing tool-calls capability probe (all) request")
	providers, err := GetAllAiProviders()
	if err != nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to load providers: %v", err),
		})
		return
	}

	type itemResult struct {
		ID          uint   `json:"id"`
		WrapperName string `json:"wrapper_name"`
		ModelName   string `json:"model_name"`
		Round1Mode  string `json:"round1_mode"`
		Round2Mode  string `json:"round2_mode"`
		ProbedAt    string `json:"probed_at"`
		Error       string `json:"error,omitempty"`
		Skipped     bool   `json:"skipped,omitempty"`
	}

	results := make([]itemResult, 0, len(providers))
	for _, p := range providers {
		// 不健康的 provider 跳过, 探测会必失败浪费时间
		if !p.IsHealthy {
			results = append(results, itemResult{
				ID:          p.ID,
				WrapperName: p.WrapperName,
				ModelName:   p.ModelName,
				Skipped:     true,
				Error:       "skipped: provider is not healthy",
			})
			continue
		}
		probeResult, saveErr := ProbeAndSaveByProviderID(p.ID)
		item := itemResult{
			ID:          p.ID,
			WrapperName: p.WrapperName,
			ModelName:   p.ModelName,
		}
		if probeResult != nil {
			item.Round1Mode = probeResult.Round1Mode
			item.Round2Mode = probeResult.Round2Mode
			item.ProbedAt = probeResult.ProbedAt.Format(time.RFC3339)
			item.Error = probeResult.Error
		}
		if saveErr != nil && item.Error == "" {
			item.Error = saveErr.Error()
		}
		results = append(results, item)
	}

	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Batch tool-calls capability probe completed for %d providers", len(providers)),
		"data":    results,
	})
}
