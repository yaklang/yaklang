package aibalance

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
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
	// wrapper 级仅维护描述/标签/老 TrafficMultiplier（字节流量倍数）。Token 维度的四维倍率
	// 已迁移到「实际模型(内部转发名)」维度，见 handleUpdateModelMultiplier，不在此处编辑。
	// 关键词: handleUpdateModelMeta, wrapper 描述/标签/老倍数, 不含 4 维 token 倍率
	var reqBody struct {
		ModelName         string   `json:"model_name"`
		Description       string   `json:"description"`
		Tags              string   `json:"tags"`
		TrafficMultiplier *float64 `json:"traffic_multiplier,omitempty"` // 老字段：字节流量倍数（保留兼容）
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

	// 4 维 Token 倍率传 -1（不更新）：wrapper 级不再编辑 token 倍率，避免与实际模型计费混淆。
	if err := SaveModelMetaWithMultipliers(
		reqBody.ModelName, reqBody.Description, reqBody.Tags,
		trafficMul, -1, -1, -1, -1,
	); err != nil {
		c.logError("Failed to save model meta: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to save metadata: %v", err),
		})
		return
	}

	c.logInfo("Successfully updated metadata for model %s (traffic_mul=%.2f)",
		reqBody.ModelName, trafficMul)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Model metadata updated successfully",
	})
}

// ==================== Model Multiplier (倍率双标识 + 批量应用) Handlers ====================

// multiplierBody 是倍率四维入参的通用结构。指针为 nil 表示「不更新该维」。
// IsFree 指针为 nil 表示「不更新免费标志」。
// 关键词: multiplierBody, 倍率四维入参, IsFree 免费标志入参
type multiplierBody struct {
	InputTokenMultiplier    *float64 `json:"input_token_multiplier,omitempty"`
	OutputTokenMultiplier   *float64 `json:"output_token_multiplier,omitempty"`
	CacheCreationMultiplier *float64 `json:"cache_creation_multiplier,omitempty"`
	CacheHitMultiplier      *float64 `json:"cache_hit_multiplier,omitempty"`
	IsFree                  *bool    `json:"is_free,omitempty"`
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

// pickIsFree 把 *bool 转成 SaveModelMultiplierWithFree 约定的 int 入参：
//   - nil   -> -1（不更新 IsFree）
//   - false -> 0
//   - true  -> 1
//
// 关键词: pickIsFree, nil 不更新免费标志
func pickIsFree(p *bool) int {
	if p == nil {
		return -1
	}
	if *p {
		return 1
	}
	return 0
}

// handleUpdateModelMultiplier writes the four-dimensional multiplier for one actual model
// (internal_model_name). 计费以实际模型为唯一标识，同一实际模型单价一致。
// 关键词: handleUpdateModelMultiplier, 实际模型倍率写入
func (c *ServerConfig) handleUpdateModelMultiplier(conn net.Conn, request *http.Request) {
	c.logInfo("Processing update model multiplier request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	var reqBody struct {
		InternalModelName string `json:"internal_model_name"`
		multiplierBody
	}
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}
	if reqBody.InternalModelName == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "internal_model_name is required",
		})
		return
	}

	inputMul := pickMultiplier(reqBody.InputTokenMultiplier)
	outputMul := pickMultiplier(reqBody.OutputTokenMultiplier)
	cacheCreateMul := pickMultiplier(reqBody.CacheCreationMultiplier)
	cacheHitMul := pickMultiplier(reqBody.CacheHitMultiplier)
	isFree := pickIsFree(reqBody.IsFree)

	if err := SaveModelMultiplierWithFree(
		reqBody.InternalModelName,
		inputMul, outputMul, cacheCreateMul, cacheHitMul, isFree,
	); err != nil {
		c.logError("Failed to save model multiplier: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to save multiplier: %v", err),
		})
		return
	}

	c.logInfo("Successfully saved model multiplier for internal=%s (input=%.2f output=%.2f cache_create=%.2f cache_hit=%.2f is_free=%d)",
		reqBody.InternalModelName, inputMul, outputMul, cacheCreateMul, cacheHitMul, isFree)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Model multiplier saved successfully",
	})
}

// handleDeleteModelMultiplier removes an actual model's multiplier, making it fall back
// to the global default.
// 关键词: handleDeleteModelMultiplier, 实际模型倍率清除, 回落全局默认
func (c *ServerConfig) handleDeleteModelMultiplier(conn net.Conn, request *http.Request) {
	c.logInfo("Processing delete model multiplier request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	var reqBody struct {
		InternalModelName string `json:"internal_model_name"`
	}
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}
	if reqBody.InternalModelName == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "internal_model_name is required",
		})
		return
	}

	if err := DeleteModelMultiplier(reqBody.InternalModelName); err != nil {
		c.logError("Failed to delete model multiplier: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to delete multiplier: %v", err),
		})
		return
	}

	c.logInfo("Successfully deleted model multiplier for internal=%s", reqBody.InternalModelName)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Model multiplier deleted successfully",
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

// matchInternalModelPattern 判断实际模型名是否匹配批量模式：
//   - 含通配符(* ? [ ])时按 glob 匹配（如 "*kimi2.5*"）
//   - 不含通配符时按大小写不敏感子串匹配（如 "kimi2.5" 命中所有含该子串的 K2.5 模型）
//
// 关键词: matchInternalModelPattern, glob 与子串, 按模式批量
func matchInternalModelPattern(name, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if strings.ContainsAny(pattern, "*?[]") {
		return utils.MatchAnyOfGlob(name, pattern)
	}
	return utils.IContains(name, pattern)
}

// handleApplyModelMultiplierByPattern batch-applies the given four-dimensional multipliers
// to every actual model whose internal name matches the pattern (glob 或子串)。
// 这让「把某倍率应用到所有 K2.5 模型」之类的批量配置变得轻松。
// 关键词: handleApplyModelMultiplierByPattern, 按模式批量, 实际模型计费
func (c *ServerConfig) handleApplyModelMultiplierByPattern(conn net.Conn, request *http.Request) {
	c.logInfo("Processing apply model multiplier by pattern request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	var reqBody struct {
		Pattern string `json:"pattern"`
		multiplierBody
	}
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}
	if strings.TrimSpace(reqBody.Pattern) == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "pattern is required",
		})
		return
	}

	inputMul := pickMultiplier(reqBody.InputTokenMultiplier)
	outputMul := pickMultiplier(reqBody.OutputTokenMultiplier)
	cacheCreateMul := pickMultiplier(reqBody.CacheCreationMultiplier)
	cacheHitMul := pickMultiplier(reqBody.CacheHitMultiplier)
	isFree := pickIsFree(reqBody.IsFree)

	models, err := GetDistinctInternalModels()
	if err != nil {
		c.logError("Failed to enumerate internal models: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to enumerate internal models: %v", err),
		})
		return
	}

	applied := 0
	failed := 0
	matched := make([]string, 0)
	for _, m := range models {
		if !matchInternalModelPattern(m.InternalModelName, reqBody.Pattern) {
			continue
		}
		matched = append(matched, m.InternalModelName)
		if err := SaveModelMultiplierWithFree(
			m.InternalModelName, inputMul, outputMul, cacheCreateMul, cacheHitMul, isFree,
		); err != nil {
			failed++
			c.logWarn("apply-by-pattern failed for internal=%s: %v", m.InternalModelName, err)
			continue
		}
		applied++
	}

	c.logInfo("Apply model multiplier by pattern completed: pattern=%q matched=%d applied=%d failed=%d (input=%.2f output=%.2f cache_create=%.2f cache_hit=%.2f is_free=%d)",
		reqBody.Pattern, len(matched), applied, failed, inputMul, outputMul, cacheCreateMul, cacheHitMul, isFree)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Applied to %d actual models matching %q (%d failed)", applied, reqBody.Pattern, failed),
		"applied": applied,
		"failed":  failed,
		"matched": matched,
	})
}

// handleApplyModelMultiplierToModels batch-applies the given four-dimensional multipliers
// to an explicit list of actual models (internal_model_names), driven by UI 勾选。
// 关键词: handleApplyModelMultiplierToModels, 按勾选批量, 实际模型计费
func (c *ServerConfig) handleApplyModelMultiplierToModels(conn net.Conn, request *http.Request) {
	c.logInfo("Processing apply model multiplier to selected models request")

	if !c.checkAuth(request) {
		c.writeJSONResponse(conn, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	if request.Method != http.MethodPost {
		c.writeJSONResponse(conn, http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed, use POST"})
		return
	}

	var reqBody struct {
		InternalModelNames []string `json:"internal_model_names"`
		multiplierBody
	}
	if !c.readJSONBody(conn, request, &reqBody) {
		return
	}
	if len(reqBody.InternalModelNames) == 0 {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "internal_model_names is required",
		})
		return
	}

	inputMul := pickMultiplier(reqBody.InputTokenMultiplier)
	outputMul := pickMultiplier(reqBody.OutputTokenMultiplier)
	cacheCreateMul := pickMultiplier(reqBody.CacheCreationMultiplier)
	cacheHitMul := pickMultiplier(reqBody.CacheHitMultiplier)
	isFree := pickIsFree(reqBody.IsFree)

	applied := 0
	failed := 0
	for _, internal := range reqBody.InternalModelNames {
		if strings.TrimSpace(internal) == "" {
			continue
		}
		if err := SaveModelMultiplierWithFree(
			internal, inputMul, outputMul, cacheCreateMul, cacheHitMul, isFree,
		); err != nil {
			failed++
			c.logWarn("apply-to-models failed for internal=%s: %v", internal, err)
			continue
		}
		applied++
	}

	c.logInfo("Apply model multiplier to selected completed: applied=%d failed=%d total=%d (input=%.2f output=%.2f cache_create=%.2f cache_hit=%.2f is_free=%d)",
		applied, failed, len(reqBody.InternalModelNames), inputMul, outputMul, cacheCreateMul, cacheHitMul, isFree)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Applied to %d selected actual models (%d failed)", applied, failed),
		"applied": applied,
		"failed":  failed,
		"total":   len(reqBody.InternalModelNames),
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
