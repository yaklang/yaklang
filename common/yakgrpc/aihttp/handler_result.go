package aihttp

import (
	"net/http"
)

// handleGetRunResult handles GET /agent/run/{run_id}
// Returns the final result of a run, useful for resumption after disconnection
func (gw *AIAgentHTTPGateway) handleGetRunResult(w http.ResponseWriter, r *http.Request) {
	runID := getRunID(r)
	if runID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "run_id is required")
		return
	}

	session, ok := gw.runManager.GetSession(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}

	result := &RunResult{
		RunID:         session.RunID,
		TaskID:        session.TaskID,
		Status:        session.Status,
		StartTime:     session.StartTime,
		EndTime:       session.EndTime,
		CoordinatorID: session.CoordinatorID,
		Events:        session.GetEvents(),
		Error:         session.Error,
	}

	writeJSON(w, result)
}
