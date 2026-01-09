package aihttp

import (
	"net/http"

	"github.com/yaklang/yaklang/common/log"
)

// handleCancelRun handles POST /agent/run/{run_id}/cancel
func (gw *AIAgentHTTPGateway) handleCancelRun(w http.ResponseWriter, r *http.Request) {
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

	if session.IsDone() {
		writeJSON(w, CancelRunResponse{
			RunID:   runID,
			Status:  session.Status,
			Message: "session was already completed",
		})
		return
	}

	// Cancel the session
	gw.runManager.CancelSession(runID)
	log.Infof("Cancelled run: %s", runID)

	writeJSON(w, CancelRunResponse{
		RunID:   runID,
		Status:  RunStatusCancelled,
		Message: "run cancelled successfully",
	})
}
