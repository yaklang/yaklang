package aivizhttp

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// handleListSessions 返回所有 AI session 列表
// GET /sessions
func (s *VizHTTPServer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := yakit.QueryAllAISessionMetaOrderByUpdated(s.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query sessions failed: "+err.Error())
		return
	}

	result := make([]SessionSummary, 0, len(sessions))
	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		summary := SessionSummary{
			SessionID:  sess.SessionID,
			Title:      sess.Title,
			Source:     sess.Source,
			CreatedAt:  sess.CreatedAt,
			UpdatedAt:  sess.UpdatedAt,
			LastUsedAt: sess.LastUsedAt,
		}
		// 统计事件数
		var count int64
		s.db.Model(&schema.AiOutputEvent{}).Where("session_id = ?", sess.SessionID).Count(&count)
		summary.EventCount = count
		// 判断是否活跃: 优先检查内存注册表, 回退到时间戳启发式
		summary.IsAlive = isSessionLive(sess.SessionID) || time.Since(sess.UpdatedAt) < 5*time.Minute
		result = append(result, summary)
	}

	writeJSON(w, http.StatusOK, SessionListResponse{
		Sessions: result,
		Total:    len(result),
	})
}

// handleSessionDetail 返回单个 session 详情 (含事件统计)
// GET /sessions/{sessionId}
func (s *VizHTTPServer) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	// 查 session 元数据
	var sess schema.AISession
	if err := s.db.Where("session_id = ?", sessionID).First(&sess).Error; err != nil {
		writeError(w, http.StatusNotFound, "session not found: "+err.Error())
		return
	}

	// 统计事件
	var eventCount int64
	s.db.Model(&schema.AiOutputEvent{}).Where("session_id = ?", sessionID).Count(&eventCount)

	// 统计工具调用
	var toolCallCount int64
	s.db.Model(&schema.AiOutputEvent{}).Where("session_id = ? AND call_tool_id != ''", sessionID).Count(&toolCallCount)

	// 统计流式事件
	var streamCount int64
	s.db.Model(&schema.AiOutputEvent{}).Where("session_id = ? AND is_stream = ?", sessionID, true).Count(&streamCount)

	detail := map[string]interface{}{
		"session_id":      sess.SessionID,
		"title":           sess.Title,
		"source":          sess.Source,
		"created_at":      sess.CreatedAt,
		"updated_at":      sess.UpdatedAt,
		"last_used_at":    sess.LastUsedAt,
		"event_count":     eventCount,
		"tool_call_count": toolCallCount,
		"stream_count":    streamCount,
		"is_alive":        isSessionLive(sessionID) || time.Since(sess.UpdatedAt) < 5*time.Minute,
	}

	writeJSON(w, http.StatusOK, detail)
}
