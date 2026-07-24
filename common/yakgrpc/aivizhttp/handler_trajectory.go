package aivizhttp

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/schema"
)

// handleSessionTrajectory returns a coarse execution tree (loops / phases / subagents)
// for the given session, built from raw AiOutputEvent events.
// GET /sessions/{sessionId}/trajectory
func (s *VizHTTPServer) handleSessionTrajectory(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["sessionId"]
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	// Fetch the full event stream for the session. The trajectory tree is
	// reconstructed from loop_marker / react_task_created / prompt_profile
	// events scattered across the entire session; a capped page (e.g. 2000)
	// silently drops later phases — e.g. a code_security_audit session with
	// ~20k events would lose all but the first 3 of 8 Phase-2 category scans.
	// Matching handler_timeline.go, we pull everything ordered by id asc.
	// Only select columns BuildTrajectory actually reads — skip stream_delta
	// (the largest BLOB) to cut memory/transfer on sessions with tens of
	// thousands of events.
	var events []*schema.AiOutputEvent
	s.db.Select("id, created_at, updated_at, deleted_at, coordinator_id, type, node_id, task_id, task_uuid, task_index, call_tool_id, content, timestamp, session_id, ai_model_name, task_semantic_label").
		Where("session_id = ?", sessionID).Order("id asc").Find(&events)

	root := BuildTrajectory(sessionID, events)
	writeJSON(w, http.StatusOK, TrajectoryResponse{
		SessionID: sessionID,
		Root:      root,
	})
}
