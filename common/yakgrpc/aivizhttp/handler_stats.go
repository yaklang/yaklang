package aivizhttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/schema"
)

// handleSessionStats 返回 session 的统计信息
// GET /sessions/{sessionId}/stats
// 聚合 consumption/pressure/ai_call_summary 事件
func (s *VizHTTPServer) handleSessionStats(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	stats := StatsResponse{SessionID: sessionID}

	// 总事件数
	s.db.Model(&schema.AiOutputEvent{}).Where("session_id = ?", sessionID).Count(&stats.TotalEvents)

	// 工具调用数：按 distinct call_tool_id 计数（每个工具调用会产生 start/param/
	// result/done/log 等多条事件，不能直接数 call_tool_id != '' 的事件数）。
	s.db.Model(&schema.AiOutputEvent{}).
		Where("session_id = ? AND call_tool_id != ''", sessionID).
		Select("COUNT(DISTINCT call_tool_id)").
		Count(&stats.ToolCallCount)

	// 流式事件数
	s.db.Model(&schema.AiOutputEvent{}).Where("session_id = ? AND is_stream = ?", sessionID, true).Count(&stats.StreamCount)

	// AI 调用数 + token 用量 + 模型明细。
	// 优先用 consumption 事件（携带 input/output tokens）；若不存在（code_security_audit
	// 等编排型 session 不发 consumption），回退到 prompt_profile（携带 prompt_tokens =
	// input tokens）+ stream_start（每次 AI 调用一个，携带 model 名）。
	var consumptionEvents []*schema.AiOutputEvent
	s.db.Where("session_id = ? AND type = ?", sessionID, "consumption").Find(&consumptionEvents)

	modelUsage := make(map[string]*ModelUsage)
	if len(consumptionEvents) > 0 {
		for _, e := range consumptionEvents {
			if e == nil || e.Content == nil {
				continue
			}
			var c map[string]interface{}
			if err := json.Unmarshal(e.Content, &c); err != nil {
				continue
			}
			inputTokens := getInt64(c, "input_tokens")
			if inputTokens == 0 {
				inputTokens = getInt64(c, "input")
			}
			outputTokens := getInt64(c, "output_tokens")
			if outputTokens == 0 {
				outputTokens = getInt64(c, "output")
			}
			stats.InputTokens += inputTokens
			stats.OutputTokens += outputTokens
			stats.TotalTokens += inputTokens + outputTokens
			model := e.AIModelName
			if model == "" {
				model = "unknown"
			}
			mu, ok := modelUsage[model]
			if !ok {
				mu = &ModelUsage{Model: model}
				modelUsage[model] = mu
			}
			mu.InputTokens += inputTokens
			mu.OutputTokens += outputTokens
			mu.CallCount++
		}
		stats.AICallCount = int64(len(consumptionEvents))
	} else {
		// 回退：prompt_profile 提供 input tokens，stream_start 提供调用数+模型名。
		var profileEvents []*schema.AiOutputEvent
		s.db.Where("session_id = ? AND type = ?", sessionID, "prompt_profile").Find(&profileEvents)
		for _, e := range profileEvents {
			if e == nil || e.Content == nil {
				continue
			}
			var c map[string]interface{}
			if err := json.Unmarshal(e.Content, &c); err != nil {
				continue
			}
			stats.InputTokens += getInt64(c, "prompt_tokens")
		}
		stats.TotalTokens = stats.InputTokens + stats.OutputTokens

		var streamStarts []*schema.AiOutputEvent
		s.db.Where("session_id = ? AND type = ?", sessionID, "stream_start").Find(&streamStarts)
		for _, e := range streamStarts {
			// Only count real AI reasoning streams — those with a model name.
			// stream_start events without ai_model_name are tool output streams
			// (tool-read_file-stdout, tool-grep-stderr, ...) or status streams
			// (code-audit-scan, todo_added), not AI calls.
			model := e.AIModelName
			if model == "" {
				continue
			}
			stats.AICallCount++
			mu, ok := modelUsage[model]
			if !ok {
				mu = &ModelUsage{Model: model}
				modelUsage[model] = mu
			}
			mu.CallCount++
		}
	}

	// 查询 pressure 事件 (上下文压力)
	var maxPressure float64
	var pressureEvents []*schema.AiOutputEvent
	s.db.Where("session_id = ? AND type = ?", sessionID, "pressure").Find(&pressureEvents)
	for _, e := range pressureEvents {
		if e == nil || e.Content == nil {
			continue
		}
		var c map[string]interface{}
		if err := json.Unmarshal(e.Content, &c); err != nil {
			continue
		}
		if p, ok := c["pressure"]; ok {
			if pf, ok := p.(float64); ok && pf > maxPressure {
				maxPressure = pf
			}
		}
		if p, ok := c["percent"]; ok {
			if pf, ok := p.(float64); ok && pf > maxPressure {
				maxPressure = pf
			}
		}
	}
	stats.ContextPressure = maxPressure

	// 查询 ai_call_summary 事件 (延迟统计)
	var summaryEvents []*schema.AiOutputEvent
	s.db.Where("session_id = ? AND type = ?", sessionID, "ai_call_summary").Find(&summaryEvents)
	var totalFirstByteMs, totalCostMs int64
	for _, e := range summaryEvents {
		if e == nil || e.Content == nil {
			continue
		}
		var c map[string]interface{}
		if err := json.Unmarshal(e.Content, &c); err != nil {
			continue
		}
		totalFirstByteMs += getInt64(c, "first_byte_cost_ms")
		totalCostMs += getInt64(c, "total_cost_ms")
	}
	if len(summaryEvents) > 0 {
		stats.FirstByteCostMs = totalFirstByteMs / int64(len(summaryEvents))
		stats.TotalCostMs = totalCostMs / int64(len(summaryEvents))
	}

	// 模型用量明细
	for _, mu := range modelUsage {
		stats.ModelBreakdown = append(stats.ModelBreakdown, *mu)
	}

	writeJSON(w, http.StatusOK, stats)
}

// getInt64 从 map 中安全提取 int64
func getInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int64(n)
		case int64:
			return n
		case int:
			return int64(n)
		case json.Number:
			if i, err := n.Int64(); err == nil {
				return i
			}
		}
	}
	return 0
}
