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

// ==================== Model Multiplier (倍率双标识 + 批量应用) Handlers ====================

// multiplierBody 是倍率四维入参的通用结构。指针为 nil 表示「不更新该维」。
// 关键词: multiplierBody, 倍率四维入参
type multiplierBody struct {
	InputTokenMultiplier    *float64 `json:"input_token_multiplier,omitempty"`
	OutputTokenMultiplier   *float64 `json:"output_token_multiplier,omitempty"`
	CacheCreationMultiplier *float64 `json:"cache_creation_multiplier,omitempty"`
	CacheHitMultiplier      *float64 `json:"cache_hit_multiplier,omitempty"`
}

// pickMultiplier 把指针四维转成 SaveXxx 约定的 float 入参：
//   - nil   -> -1（不更新该维）
//   - < 0   -> 0（清空该维，回落下一层）
//   - 其它  -> 原值
//
// 关键词: pickMultiplier, nil 不更新, 负数清空
func pickMultiplier(p *float64) float64 {
	if p == nil {
		return -1
	}
	if *p < 0 {
		return 0
	}
	return *p
}

// handleUpdateModelOverride writes a (WrapperName + InternalModelName) multiplier override.
// 关键词: handleUpdateModelOverride, 双标识倍率覆盖写入
func (c *ServerConfig) handleUpdateModelOverride(conn net.Conn, request *http.Request) {
	c.logInfo("Processing update model override request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	var reqBody struct {
		WrapperName       string `json:"wrapper_name"`
		InternalModelName string `json:"internal_model_name"`
		multiplierBody
	}
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}
	if reqBody.WrapperName == "" || reqBody.InternalModelName == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "wrapper_name and internal_model_name are required",
		})
		return
	}

	inputMul := pickMultiplier(reqBody.InputTokenMultiplier)
	outputMul := pickMultiplier(reqBody.OutputTokenMultiplier)
	cacheCreateMul := pickMultiplier(reqBody.CacheCreationMultiplier)
	cacheHitMul := pickMultiplier(reqBody.CacheHitMultiplier)

	if err := SaveModelMultiplierOverride(
		reqBody.WrapperName, reqBody.InternalModelName,
		inputMul, outputMul, cacheCreateMul, cacheHitMul,
	); err != nil {
		c.logError("Failed to save model override: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to save override: %v", err),
		})
		return
	}

	c.logInfo("Successfully saved model override for wrapper=%s internal=%s (input=%.2f output=%.2f cache_create=%.2f cache_hit=%.2f)",
		reqBody.WrapperName, reqBody.InternalModelName, inputMul, outputMul, cacheCreateMul, cacheHitMul)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Model multiplier override saved successfully",
	})
}

// handleDeleteModelOverride removes a (WrapperName + InternalModelName) override,
// making it fall back to the wrapper-level default.
// 关键词: handleDeleteModelOverride, 双标识倍率覆盖删除, 回落 wrapper 默认
func (c *ServerConfig) handleDeleteModelOverride(conn net.Conn, request *http.Request) {
	c.logInfo("Processing delete model override request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	var reqBody struct {
		WrapperName       string `json:"wrapper_name"`
		InternalModelName string `json:"internal_model_name"`
	}
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}
	if reqBody.WrapperName == "" || reqBody.InternalModelName == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "wrapper_name and internal_model_name are required",
		})
		return
	}

	if err := DeleteModelMultiplierOverride(reqBody.WrapperName, reqBody.InternalModelName); err != nil {
		c.logError("Failed to delete model override: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to delete override: %v", err),
		})
		return
	}

	c.logInfo("Successfully deleted model override for wrapper=%s internal=%s",
		reqBody.WrapperName, reqBody.InternalModelName)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Model multiplier override deleted successfully",
	})
}

// handleSetGlobalDefaultMultiplier sets the singleton global default four-dimensional multipliers.
// 关键词: handleSetGlobalDefaultMultiplier, 全局默认倍率设置
func (c *ServerConfig) handleSetGlobalDefaultMultiplier(conn net.Conn, request *http.Request) {
	c.logInfo("Processing set global default multiplier request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	var reqBody multiplierBody
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}

	inputMul := pickMultiplier(reqBody.InputTokenMultiplier)
	outputMul := pickMultiplier(reqBody.OutputTokenMultiplier)
	cacheCreateMul := pickMultiplier(reqBody.CacheCreationMultiplier)
	cacheHitMul := pickMultiplier(reqBody.CacheHitMultiplier)

	if err := SaveGlobalMultiplierConfig(inputMul, outputMul, cacheCreateMul, cacheHitMul); err != nil {
		c.logError("Failed to save global default multiplier: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to save global default: %v", err),
		})
		return
	}

	c.logInfo("Successfully saved global default multiplier (input=%.2f output=%.2f cache_create=%.2f cache_hit=%.2f)",
		inputMul, outputMul, cacheCreateMul, cacheHitMul)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Global default multiplier saved successfully",
	})
}

// handleApplyMultiplierToAll batch-applies the given four-dimensional multipliers to every
// real model route (distinct WrapperName + InternalModelName) by writing override rows.
// This is the "一键铺到所有实际模型" capability that makes billing setup much less tedious.
// 关键词: handleApplyMultiplierToAll, 一键铺到所有实际模型, 批量写覆盖
func (c *ServerConfig) handleApplyMultiplierToAll(conn net.Conn, request *http.Request) {
	c.logInfo("Processing apply multiplier to all request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	var reqBody multiplierBody
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}

	inputMul := pickMultiplier(reqBody.InputTokenMultiplier)
	outputMul := pickMultiplier(reqBody.OutputTokenMultiplier)
	cacheCreateMul := pickMultiplier(reqBody.CacheCreationMultiplier)
	cacheHitMul := pickMultiplier(reqBody.CacheHitMultiplier)

	routes, err := GetDistinctModelRoutes()
	if err != nil {
		c.logError("Failed to enumerate model routes: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to enumerate model routes: %v", err),
		})
		return
	}

	applied := 0
	failed := 0
	for _, route := range routes {
		if err := SaveModelMultiplierOverride(
			route.WrapperName, route.InternalModelName,
			inputMul, outputMul, cacheCreateMul, cacheHitMul,
		); err != nil {
			failed++
			c.logWarn("apply-to-all failed for wrapper=%s internal=%s: %v",
				route.WrapperName, route.InternalModelName, err)
			continue
		}
		applied++
	}

	c.logInfo("Apply multiplier to all completed: applied=%d failed=%d total=%d (input=%.2f output=%.2f cache_create=%.2f cache_hit=%.2f)",
		applied, failed, len(routes), inputMul, outputMul, cacheCreateMul, cacheHitMul)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Applied to %d model routes (%d failed)", applied, failed),
		"applied": applied,
		"failed":  failed,
		"total":   len(routes),
	})
}

// readJSONBody reads and unmarshals the request body into dst. On any failure it writes
// a 400 response and returns false (caller should return immediately).
// 关键词: readJSONBody, 请求体读取解析复用
func (c *ServerConfig) readJSONBody(conn net.Conn, request *http.Request, dst interface{}) bool {
	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		c.logError("Failed to read request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Failed to read request body",
		})
		return false
	}
	defer request.Body.Close()

	if err := json.Unmarshal(bodyBytes, dst); err != nil {
		c.logError("Failed to unmarshal request body: %v", err)
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Invalid request body format",
		})
		return false
	}
	return true
}
