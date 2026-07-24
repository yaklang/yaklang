package aivizhttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// handleSessionEvents 返回 session 的历史事件 (分页)
// GET /sessions/{sessionId}/events?page=1&limit=50&type=tool_call_start
func (s *VizHTTPServer) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	page := int64(1)
	limit := int64(50)
	if v, err := strconv.ParseInt(r.URL.Query().Get("page"), 10, 64); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64); err == nil && v > 0 && v <= 500 {
		limit = v
	}

	filter := &ypb.AIEventFilter{SessionID: sessionID}
	if eventTypes := r.URL.Query().Get("type"); eventTypes != "" {
		filter.EventType = strings.Split(eventTypes, ",")
	}

	paging := &ypb.Paging{
		Page:    page,
		Limit:   limit,
		OrderBy: "id",
		Order:   "asc",
	}

	paginator, events, err := yakit.QueryAIEventPaging(s.db, filter, paging)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query events failed: "+err.Error())
		return
	}

	items := make([]EventItem, 0, len(events))
	for _, e := range events {
		if e == nil {
			continue
		}
		items = append(items, eventToItem(e))
	}

	totalCount := int64(paginator.TotalRecord)
	writeJSON(w, http.StatusOK, EventListResponse{
		Events:  items,
		Total:   totalCount,
		Page:    page,
		Limit:   limit,
		HasMore: page*limit < totalCount,
	})
}

// handleSSEStream 提供实时事件流
// GET /sessions/{sessionId}/stream
// 两种模式:
//   - session 正在运行: 通过 aireact.SubscribeRunningSession 进程内订阅, 实时收到事件
//   - session 已结束: 从 DB 回放历史事件
func (s *VizHTTPServer) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// 先发送已有的历史事件 (确保不遗漏订阅前已产生的)
	s.replayHistoricalSSE(w, flusher, sessionID)

	// 尝试进程内订阅实时事件
	if s.subscribeLiveSSE(w, flusher, r, sessionID) {
		return
	}

	// session 已结束, 历史回放完成, 发送结束标记
	writeSSEEvent(w, "system", map[string]string{"type": "ended", "message": "session not running, historical replay complete"})
	flusher.Flush()
}

// replayHistoricalSSE 从 DB 回放 session 的历史事件
func (s *VizHTTPServer) replayHistoricalSSE(w io.Writer, flusher http.Flusher, sessionID string) {
	if s.db == nil {
		return
	}
	var events []*schema.AiOutputEvent
	s.db.Where("session_id = ?", sessionID).Order("id asc").Limit(500).Find(&events)
	for _, e := range events {
		if e == nil {
			continue
		}
		writeSSEEvent(w, "event", eventToItem(e))
	}
	flusher.Flush()
}

// subscribeLiveSSE 尝试通过 aireact 进程内注册表订阅运行中的 session.
// 返回 true 表示 session 正在运行且已建立实时订阅; false 表示 session 未运行.
func (s *VizHTTPServer) subscribeLiveSSE(w http.ResponseWriter, flusher http.Flusher, r *http.Request, sessionID string) bool {
	unsubscribe, ok := aireact.SubscribeRunningSession(sessionID, func(e *schema.AiOutputEvent) {
		if e == nil {
			return
		}
		writeSSEEvent(w, "event", eventToItem(e))
		flusher.Flush()
	})
	if !ok {
		return false
	}
	defer unsubscribe()

	writeSSEEvent(w, "system", map[string]string{"type": "live", "message": "subscribed to running session"})
	flusher.Flush()

	<-r.Context().Done()
	return true
}

// writeSSEEvent 写入一个 SSE 事件
func writeSSEEvent(w io.Writer, eventType string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
}

// eventToItem 将 schema.AiOutputEvent 转换为 EventItem
func eventToItem(e *schema.AiOutputEvent) EventItem {
	item := EventItem{
		ID:              e.ID,
		Type:            string(e.Type),
		NodeId:          e.NodeId,
		Timestamp:       e.Timestamp,
		TaskIndex:       e.TaskIndex,
		TaskId:          e.TaskId,
		TaskUUID:        e.TaskUUID,
		TaskName:        e.TaskSemanticLabel,
		CoordinatorId:   e.CoordinatorId,
		IsSubAgent:      false,
		CallToolID:      e.CallToolID,
		IsStream:        e.IsStream,
		IsReason:        e.IsReason,
		IsJson:          e.IsJson,
		ContentType:     e.ContentType,
		AIService:       e.AIService,
		AIModelName:     e.AIModelName,
		EventUUID:       e.EventUUID,
		RecoveryIndexID: e.RecoveryIndexID,
	}
	if e.Content != nil {
		item.Content = string(e.Content)
	}
	if e.StreamDelta != nil {
		item.StreamDelta = string(e.StreamDelta)
	}
	// Sub-agent identity and parent link: every event from a subtask carries the
	// task's name, sub-agent flag and optional parent task id in the structured
	// react_task_created payload. We try to recover a human-readable label and the
	// parent link from that payload when it is available.
	if e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "react_task_created" && e.Content != nil {
		var payload map[string]interface{}
		if err := json.Unmarshal(e.Content, &payload); err == nil {
			if name, ok := payload["react_task_name"].(string); ok && name != "" {
				item.TaskName = name
			}
			if is, ok := payload["react_task_is_sub_agent"].(bool); ok {
				item.IsSubAgent = is
			}
			if parent, ok := payload["react_parent_task_id"].(string); ok && parent != "" {
				item.ParentTaskID = parent
			}
		}
	}
	if item.TaskName == "" && item.TaskId != "" {
		item.TaskName = item.TaskId
	}
	return item
}
