package aivizhttp

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/schema"
)

// handleSessionTools 返回 session 的工具调用汇总
// GET /sessions/{sessionId}/tools
// 按 CallToolID 配对 tool_call_start → done/error/result
func (s *VizHTTPServer) handleSessionTools(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	// 查询所有工具调用相关事件 — skip stream_delta BLOB (not needed for tool
	// summaries, cuts memory on sessions with thousands of tool events).
	var events []*schema.AiOutputEvent
	s.db.Select("id, type, call_tool_id, content, ai_model_name, timestamp").
		Where("session_id = ? AND call_tool_id != ''", sessionID).
		Order("id asc").
		Find(&events)

	// 按 CallToolID 分组
	toolMap := make(map[string]*ToolCallSummary)
	toolOrder := make([]string, 0) // 保持首次出现顺序

	for _, e := range events {
		if e == nil || e.CallToolID == "" {
			continue
		}

		tc, exists := toolMap[e.CallToolID]
		if !exists {
			tc = &ToolCallSummary{
				CallToolID: e.CallToolID,
				Status:     "running",
			}
			toolMap[e.CallToolID] = tc
			toolOrder = append(toolOrder, e.CallToolID)
		}

		switch string(e.Type) {
		case "tool_call_start":
			tc.StartTime = e.Timestamp
			// tool_call_start content: {"tool":{"name":"tree","description":"..."}}
			// The tool name lives under the nested "tool" object; also fall back to
			// top-level "tool_name"/"name" for older emitters.
			if toolObj := extractJSONObject(e.Content, "tool"); toolObj != nil {
				if name := jsonStringField(toolObj, "name"); name != "" {
					tc.ToolName = name
				}
			}
			if tc.ToolName == "" {
				if name := extractJSONField(e.Content, "tool_name"); name != "" {
					tc.ToolName = name
				}
			}
			if tc.ToolName == "" {
				if name := extractJSONField(e.Content, "name"); name != "" {
					tc.ToolName = name
				}
			}
		case "tool_call_reason":
			// content: {"reason":"..."} — extract the reason text, not the whole JSON.
			if r := extractJSONField(e.Content, "reason"); r != "" {
				tc.Reason = r
			} else if e.Content != nil {
				tc.Reason = string(e.Content)
			}
		case "tool_call_param":
			// content: {"params":{...}} — pretty-print the params object only.
			tc.Params = extractPrettyJSONField(e.Content, "params")
		case "tool_call_done":
			tc.Status = "done"
			tc.EndTime = e.Timestamp
			if tc.StartTime > 0 {
				tc.DurationMs = tc.EndTime - tc.StartTime
			}
		case "tool_call_error":
			tc.Status = "error"
			tc.EndTime = e.Timestamp
			if tc.StartTime > 0 {
				tc.DurationMs = tc.EndTime - tc.StartTime
			}
			tc.Error = extractPrettyJSONField(e.Content, "error")
		case "tool_call_user_cancel":
			tc.Status = "cancelled"
			tc.EndTime = e.Timestamp
			if tc.StartTime > 0 {
				tc.DurationMs = tc.EndTime - tc.StartTime
			}
		case "tool_call_result":
			// content: {"result":"..."} — extract the result text/object only.
			tc.Result = extractPrettyJSONField(e.Content, "result")
		case "tool_call_summary":
			if s := extractJSONField(e.Content, "summary"); s != "" && s != "null" {
				tc.Summary = s
			}
		}
	}

	// 按顺序输出
	result := make([]ToolCallSummary, 0, len(toolOrder))
	for _, id := range toolOrder {
		result = append(result, *toolMap[id])
	}

	// 按 start_time 降序排列 (最新在前)
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime > result[j].StartTime
	})

	writeJSON(w, http.StatusOK, ToolCallListResponse{
		ToolCalls: result,
		Total:     len(result),
	})
}

// extractJSONField 从 JSON content 中提取指定字符串字段值
func extractJSONField(content []byte, field string) string {
	if len(content) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(content, &m); err != nil {
		return ""
	}
	return jsonStringField(m, field)
}

// jsonStringField returns a string field from a decoded JSON object.
func jsonStringField(m map[string]interface{}, field string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[field]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// extractJSONObject returns a nested object field from decoded JSON content.
func extractJSONObject(content []byte, field string) map[string]interface{} {
	if len(content) == 0 {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(content, &m); err != nil {
		return nil
	}
	if v, ok := m[field]; ok {
		if obj, ok := v.(map[string]interface{}); ok {
			return obj
		}
	}
	return nil
}

// extractPrettyJSONField extracts a field (string or object) from JSON content
// and returns a compact/pretty string representation of just that field's value,
// stripping the surrounding envelope (e.g. call_tool_id). Falls back to "".
func extractPrettyJSONField(content []byte, field string) string {
	if len(content) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(content, &m); err != nil {
		return ""
	}
	v, ok := m[field]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}, []interface{}:
		b, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			return ""
		}
		return string(b)
	default:
		return ""
	}
}
