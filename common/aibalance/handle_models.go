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
	// 关键词: handleUpdateModelMeta, 4 \u500d\u7387\u5b57\u6bb5
	var reqBody struct {
		ModelName               string   `json:"model_name"`
		Description             string   `json:"description"`
		Tags                    string   `json:"tags"`
		TrafficMultiplier       *float64 `json:"traffic_multiplier,omitempty"`        // 老字段：字节流量倍数（保留兼容）
		InputTokenMultiplier    *float64 `json:"input_token_multiplier,omitempty"`    // 输入 token 倍率
		OutputTokenMultiplier   *float64 `json:"output_token_multiplier,omitempty"`   // 输出 token 倍率
		CacheCreationMultiplier *float64 `json:"cache_creation_multiplier,omitempty"` // 缓存创建倍率
		CacheHitMultiplier      *float64 `json:"cache_hit_multiplier,omitempty"`      // 缓存命中倍率
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

	// 老 TrafficMultiplier：-1 表示不更新；负数兜底为 1.0
	trafficMul := -1.0
	if reqBody.TrafficMultiplier != nil {
		trafficMul = *reqBody.TrafficMultiplier
		if trafficMul < 0 {
			trafficMul = 1.0
		}
	}

	// 4 维 Token 倍率：nil 表示不更新（-1）；负数兜底为 0（=回落到默认/老倍率）
	pickMul := func(p *float64) float64 {
		if p == nil {
			return -1
		}
		if *p < 0 {
			return 0
		}
		return *p
	}
	inputMul := pickMul(reqBody.InputTokenMultiplier)
	outputMul := pickMul(reqBody.OutputTokenMultiplier)
	cacheCreateMul := pickMul(reqBody.CacheCreationMultiplier)
	cacheHitMul := pickMul(reqBody.CacheHitMultiplier)

	if err := SaveModelMetaWithMultipliers(
		reqBody.ModelName, reqBody.Description, reqBody.Tags,
		trafficMul, inputMul, outputMul, cacheCreateMul, cacheHitMul,
	); err != nil {
		c.logError("Failed to save model meta: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to save metadata: %v", err),
		})
		return
	}

	c.logInfo("Successfully updated metadata for model %s (traffic_mul=%.2f input=%.2f output=%.2f cache_create=%.2f cache_hit=%.2f)",
		reqBody.ModelName, trafficMul, inputMul, outputMul, cacheCreateMul, cacheHitMul)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Model metadata updated successfully",
	})
}
