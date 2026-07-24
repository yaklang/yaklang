package aivizhttp

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/schema"
)

// handleSessionContext 返回 backend 投影后的 context 时间线
// GET /sessions/{sessionId}/context
func (s *VizHTTPServer) handleSessionContext(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	// 拉取该 session 的全部事件。context 投影需要完整的 recovery block 与
	// tool_call start/result/done 配对，截断（如固定 2000 条）会让后出现的
	// tool_call 只有 start 没有 done、tool_name 变成 unknown，Phase 2/3/4 的
	// 子 agent 内容也会整段丢失。与 handler_timeline.go 一致，全量按 id asc 拉取。
	var events []*schema.AiOutputEvent
	s.db.Where("session_id = ?", sessionID).Order("id asc").Find(&events)

	// 投影
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)
	resp.SessionID = sessionID

	writeJSON(w, http.StatusOK, resp)
}
