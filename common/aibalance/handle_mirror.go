// handle_mirror.go - aibalance Mirror Rules portal HTTP handlers
//
// 路由总览 (全部要求 admin 鉴权, OPS 不开放):
//   GET    /portal/api/mirror-rules              列出全部规则 (启用/禁用都返回)
//   POST   /portal/api/mirror-rules              创建规则
//   PUT    /portal/api/mirror-rules/{id}         更新规则
//   DELETE /portal/api/mirror-rules/{id}         删除规则
//   POST   /portal/api/mirror-rules/{id}/toggle  启/停 (body: {"enabled": bool})
//   GET    /portal/api/mirror-rules/{id}/logs    获取内存中的最近 N 条调用日志
//   POST   /portal/api/mirror-rules/{id}/test    用示例 snapshot 同步试运行脚本
//
// 关键词: handle_mirror, mirror rules HTTP API, portal mirror tab

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

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	maxMirrorRequestBodySize = 256 * 1024  // 256KB, 脚本可能较长但远小于该阈值
	maxMirrorScriptLength    = 200 * 1024  // 200KB, 单脚本最大长度
	maxMirrorNameLength      = 128
	maxMirrorActionToolLen   = 256
)

// mirrorRulePayload 是 handler 写入 / 接收 JSON 时使用的中间结构.
// 字段命名与前端 / API 文档一致 (snake_case).
type mirrorRulePayload struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	ConditionType  string `json:"condition_type"`
	ActionName     string `json:"action_name"`
	ToolName       string `json:"tool_name"`
	CallbackScript string `json:"callback_script"`
	Concurrency    int    `json:"concurrency"`
	QueueSize      int    `json:"queue_size"`
	TimeoutMs      int64  `json:"timeout_ms"`
}

// readMirrorBody 限定大小 + 反序列化为 mirrorRulePayload.
//
// 关键词: readMirrorBody, mirror handler JSON 解析, body 大小限制
func readMirrorBody(request *http.Request) (*mirrorRulePayload, error) {
	limited := io.LimitReader(request.Body, maxMirrorRequestBodySize+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	defer request.Body.Close()
	if len(raw) > maxMirrorRequestBodySize {
		return nil, fmt.Errorf("request body too large (max %d bytes)", maxMirrorRequestBodySize)
	}
	var p mirrorRulePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return &p, nil
}

// validateMirrorPayload 校验业务字段, 不动 ID / Enabled.
//
// 关键词: validateMirrorPayload, mirror condition 白名单校验
func validateMirrorPayload(p *mirrorRulePayload) error {
	if p == nil {
		return fmt.Errorf("nil payload")
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > maxMirrorNameLength {
		return fmt.Errorf("name too long (max %d)", maxMirrorNameLength)
	}
	if !ValidMirrorConditionTypes[p.ConditionType] {
		return fmt.Errorf("invalid condition_type: %s", p.ConditionType)
	}
	if len(p.ActionName) > maxMirrorActionToolLen {
		return fmt.Errorf("action_name too long")
	}
	if len(p.ToolName) > maxMirrorActionToolLen {
		return fmt.Errorf("tool_name too long")
	}
	switch p.ConditionType {
	case MirrorConditionActionEq:
		if strings.TrimSpace(p.ActionName) == "" {
			return fmt.Errorf("action_name is required for condition action_eq")
		}
	case MirrorConditionActionCallToolEq:
		if strings.TrimSpace(p.ToolName) == "" {
			return fmt.Errorf("tool_name is required for condition action_call_tool_eq")
		}
	}
	if strings.TrimSpace(p.CallbackScript) == "" {
		return fmt.Errorf("callback_script is required")
	}
	if len(p.CallbackScript) > maxMirrorScriptLength {
		return fmt.Errorf("callback_script too long (max %d bytes)", maxMirrorScriptLength)
	}
	if p.Concurrency < 0 || p.Concurrency > 256 {
		return fmt.Errorf("concurrency must be 0..256")
	}
	if p.QueueSize < 0 || p.QueueSize > 1<<20 {
		return fmt.Errorf("queue_size must be 0..%d", 1<<20)
	}
	if p.TimeoutMs < 0 || p.TimeoutMs > 10*60*1000 {
		return fmt.Errorf("timeout_ms must be 0..600000")
	}
	return nil
}

// mirrorRuleToMap 把 DB 实体序列化为前端用的 map; 同时附带运行时态.
//
// 关键词: mirrorRuleToMap, portal mirror rule 视图模型
func (c *ServerConfig) mirrorRuleToMap(rule *schema.AiMirrorRule) map[string]interface{} {
	if rule == nil {
		return nil
	}
	m := map[string]interface{}{
		"id":              rule.ID,
		"name":            rule.Name,
		"enabled":         rule.Enabled,
		"condition_type":  rule.ConditionType,
		"action_name":     rule.ActionName,
		"tool_name":       rule.ToolName,
		"callback_script": rule.CallbackScript,
		"concurrency":     rule.Concurrency,
		"queue_size":      rule.QueueSize,
		"timeout_ms":      rule.TimeoutMs,
		"total_triggered": rule.TotalTriggered,
		"total_success":   rule.TotalSuccess,
		"total_failed":    rule.TotalFailed,
		"total_dropped":   rule.TotalDropped,
		"created_at":      rule.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if !rule.LastTriggeredAt.IsZero() {
		m["last_triggered_at"] = rule.LastTriggeredAt.Format("2006-01-02 15:04:05")
	} else {
		m["last_triggered_at"] = ""
	}
	// 运行时态
	if c.MirrorManager != nil {
		if st := c.MirrorManager.GetStatus(rule.ID); st != nil {
			m["queue_length"] = st.QueueLength
			m["queue_capacity"] = st.QueueCapacity
			m["active_workers"] = st.ActiveWorkers
			m["is_running"] = true
		} else {
			m["queue_length"] = 0
			m["queue_capacity"] = 0
			m["active_workers"] = 0
			m["is_running"] = false
		}
	}
	return m
}

// parseMirrorIDFromPath 从形如 /portal/api/mirror-rules/{id}[/sub] 的路径取出 id.
//
// 关键词: parseMirrorIDFromPath, RESTful path 参数提取
func parseMirrorIDFromPath(path string) (uint, error) {
	trimmed := strings.TrimPrefix(path, "/portal/api/mirror-rules/")
	if trimmed == "" || trimmed == path {
		return 0, fmt.Errorf("missing id in path")
	}
	parts := strings.SplitN(trimmed, "/", 2)
	idStr := parts[0]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id: %s", idStr)
	}
	return uint(id), nil
}

// ==================== Handlers ====================

// handleGetMirrorMeta: GET /portal/api/mirror-rules/_meta
//
// 返回前端构造编辑表单需要的"自描述"信息:
//   - default_script: 创建规则时默认填入的脚本模板 (含 YAK_MAIN 自测块)
//   - data_spec:      handle(data) 字段表 (字段名 / 类型 / 含义 / 示例)
//   - condition_types: 触发条件枚举 + 中文说明 (前端 select 文案来源)
//
// 路径以 _meta 开头, 避免与 /mirror-rules/{id} 冲突 (id 必为数字).
//
// 关键词: handleGetMirrorMeta, mirror rules /_meta API, portal 自描述,
//        default script template 来自后端, data spec 渲染
func (c *ServerConfig) handleGetMirrorMeta(conn net.Conn, request *http.Request) {
	c.logInfo("mirror: get meta")
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":        true,
		"default_script": DefaultMirrorScript(),
		"data_spec":      MirrorDataSpec(),
		"condition_types": []map[string]string{
			{"value": MirrorConditionAlways, "label": "每次成功请求 (always)"},
			{"value": MirrorConditionActionEq, "label": "@action 等于指定值 (action_eq)"},
			{"value": MirrorConditionAnyToolcall, "label": "任意 OpenAI tool_calls 出现 (any_toolcall)"},
			{"value": MirrorConditionActionCallToolEq, "label": "call-tool 类 @action 且工具名匹配 (action_call_tool_eq), Action 名称可选"},
		},
	})
}

// handleListMirrorRules: GET /portal/api/mirror-rules
func (c *ServerConfig) handleListMirrorRules(conn net.Conn, request *http.Request) {
	c.logInfo("mirror: list rules")
	rules, err := ListMirrorRules()
	if err != nil {
		c.logError("list mirror rules failed: %v", err)
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]interface{}, 0, len(rules))
	for _, r := range rules {
		out = append(out, c.mirrorRuleToMap(r))
	}
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"rules":   out,
		"total":   len(out),
	})
}

// handleCreateMirrorRule: POST /portal/api/mirror-rules
func (c *ServerConfig) handleCreateMirrorRule(conn net.Conn, request *http.Request) {
	c.logInfo("mirror: create rule")
	p, err := readMirrorBody(request)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := validateMirrorPayload(p); err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rule := &schema.AiMirrorRule{
		Name:           strings.TrimSpace(p.Name),
		Enabled:        p.Enabled,
		ConditionType:  p.ConditionType,
		ActionName:     strings.TrimSpace(p.ActionName),
		ToolName:       strings.TrimSpace(p.ToolName),
		CallbackScript: p.CallbackScript,
		Concurrency:    p.Concurrency,
		QueueSize:      p.QueueSize,
		TimeoutMs:      p.TimeoutMs,
	}
	if err := CreateMirrorRule(rule); err != nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if c.MirrorManager != nil {
		if err := c.MirrorManager.ReloadRule(rule.ID); err != nil {
			c.logWarn("reload mirror rule failed: %v", err)
		}
	}
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"rule":    c.mirrorRuleToMap(rule),
	})
}

// handleUpdateMirrorRule: PUT /portal/api/mirror-rules/{id}
func (c *ServerConfig) handleUpdateMirrorRule(conn net.Conn, request *http.Request, path string) {
	id, err := parseMirrorIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.logInfo("mirror: update rule id=%d", id)
	p, err := readMirrorBody(request)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := validateMirrorPayload(p); err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rule := &schema.AiMirrorRule{
		Name:           strings.TrimSpace(p.Name),
		Enabled:        p.Enabled,
		ConditionType:  p.ConditionType,
		ActionName:     strings.TrimSpace(p.ActionName),
		ToolName:       strings.TrimSpace(p.ToolName),
		CallbackScript: p.CallbackScript,
		Concurrency:    p.Concurrency,
		QueueSize:      p.QueueSize,
		TimeoutMs:      p.TimeoutMs,
	}
	rule.ID = id
	if err := UpdateMirrorRule(rule); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{"error": "rule not found"})
			return
		}
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if c.MirrorManager != nil {
		if err := c.MirrorManager.ReloadRule(id); err != nil {
			c.logWarn("reload mirror rule failed: %v", err)
		}
	}
	refreshed, _ := GetMirrorRuleByID(id)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"rule":    c.mirrorRuleToMap(refreshed),
	})
}

// handleDeleteMirrorRule: DELETE /portal/api/mirror-rules/{id}
func (c *ServerConfig) handleDeleteMirrorRule(conn net.Conn, request *http.Request, path string) {
	id, err := parseMirrorIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.logInfo("mirror: delete rule id=%d", id)
	if err := DeleteMirrorRule(id); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{"error": "rule not found"})
			return
		}
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if c.MirrorManager != nil {
		c.MirrorManager.RemoveRule(id)
	}
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{"success": true})
}

// handleToggleMirrorRule: POST /portal/api/mirror-rules/{id}/toggle
func (c *ServerConfig) handleToggleMirrorRule(conn net.Conn, request *http.Request, path string) {
	id, err := parseMirrorIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	limited := io.LimitReader(request.Body, 4096)
	raw, _ := io.ReadAll(limited)
	defer request.Body.Close()
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
	}
	c.logInfo("mirror: toggle rule id=%d -> enabled=%v", id, body.Enabled)
	if err := ToggleMirrorRule(id, body.Enabled); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{"error": "rule not found"})
			return
		}
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if c.MirrorManager != nil {
		if err := c.MirrorManager.ReloadRule(id); err != nil {
			c.logWarn("reload mirror rule failed: %v", err)
		}
	}
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{"success": true, "enabled": body.Enabled})
}

// handleGetMirrorRuleLogs: GET /portal/api/mirror-rules/{id}/logs
func (c *ServerConfig) handleGetMirrorRuleLogs(conn net.Conn, request *http.Request, path string) {
	id, err := parseMirrorIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if c.MirrorManager == nil {
		c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{"success": true, "logs": []any{}})
		return
	}
	logs := c.MirrorManager.GetRecentLogs(id)
	out := make([]map[string]interface{}, 0, len(logs))
	for _, l := range logs {
		out = append(out, map[string]interface{}{
			"timestamp":     l.Timestamp.Format(time.RFC3339),
			"req_id":        l.ReqID,
			"duration_ms":   l.DurationMs,
			"success":       l.Success,
			"error_message": l.ErrorMessage,
			"stdout":        l.Stdout,
		})
	}
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success": true,
		"logs":    out,
		"total":   len(out),
	})
}

// handleTestMirrorRule: POST /portal/api/mirror-rules/{id}/test
// body: {"script": "...optional override...", "snapshot": {...optional override...}}
//
// 不带 body 时: 用 DB 中规则的脚本 + 一个合成 snapshot 试运行.
// 带 script: 用提供的脚本试运行 (用于"未保存先测试").
// 带 snapshot: 用提供的 snapshot 字段覆盖默认值.
func (c *ServerConfig) handleTestMirrorRule(conn net.Conn, request *http.Request, path string) {
	id, err := parseMirrorIDFromPath(path)
	if err != nil {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	type testReq struct {
		Script   string         `json:"script"`
		Snapshot map[string]any `json:"snapshot"`
	}
	var req testReq
	limited := io.LimitReader(request.Body, maxMirrorRequestBodySize+1)
	raw, _ := io.ReadAll(limited)
	defer request.Body.Close()
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
	}
	script := strings.TrimSpace(req.Script)
	if script == "" {
		rule, err := GetMirrorRuleByID(id)
		if err != nil || rule == nil {
			c.writeJSONResponse(conn, http.StatusNotFound, map[string]string{"error": "rule not found"})
			return
		}
		script = rule.CallbackScript
	}
	if script == "" {
		c.writeJSONResponse(conn, http.StatusBadRequest, map[string]string{"error": "empty script"})
		return
	}
	snap := buildSampleSnapshot(req.Snapshot)
	timeoutMs := int64(30000)
	if c.MirrorManager == nil {
		c.writeJSONResponse(conn, http.StatusInternalServerError, map[string]string{"error": "mirror manager not initialized"})
		return
	}
	success, errMsg, duration := c.MirrorManager.RunOnceForTest(script, snap, timeoutMs)
	c.writeJSONResponse(conn, http.StatusOK, map[string]interface{}{
		"success":     true,
		"executed":    success,
		"error":       errMsg,
		"duration_ms": duration,
		"snapshot":    snap.ToScriptMap(),
	})
}

// buildSampleSnapshot 根据用户覆盖字段构造一个合理的示例 snapshot.
// 没有提供的字段填默认值, 便于试运行调试.
//
// 关键词: buildSampleSnapshot, mirror test 示例数据
func buildSampleSnapshot(override map[string]any) *MirrorSnapshot {
	snap := &MirrorSnapshot{
		ReqID:        "test-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		TimestampMs:  time.Now().UnixMilli(),
		Model:        "test-model",
		TypeName:     "openai",
		Domain:       "api.example.com",
		APIKeyFP:     APIKeyFingerprint("sample-key-for-portal-test"),
		IsFreeModel:  false,
		Stream:       true,
		ResponseText: `{"@action":"directly_answer","answer_payload":"hello world"}`,
		DurationMs:   1234,
		InputBytes:   100,
		OutputBytes:  200,
	}
	action, payload := ParseActionFromText(snap.ResponseText)
	snap.Action = action
	snap.ActionPayload = payload

	if override == nil {
		return snap
	}
	if v, ok := override["model"].(string); ok {
		snap.Model = v
	}
	if v, ok := override["response_text"].(string); ok {
		snap.ResponseText = v
		snap.Action, snap.ActionPayload = ParseActionFromText(v)
	}
	if v, ok := override["response_reason"].(string); ok {
		snap.ResponseReason = v
	}
	if v, ok := override["action"].(string); ok && v != "" {
		snap.Action = v
	}
	return snap
}
