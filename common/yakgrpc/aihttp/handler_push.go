package aihttp

import (
	"net/http"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// handlePushEvent handles POST /agent/run/{run_id}/events/push
// This allows clients to send input events to a running AI session
func (gw *AIAgentHTTPGateway) handlePushEvent(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "bad_request", "session is already completed")
		return
	}

	var req PushEventRequest
	if err := readJSON(r, &req); err != nil {
		log.Debugf("Failed to parse push event request: %v", err)
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: "+err.Error())
		return
	}

	// Create input event based on request type
	inputEvent := &ypb.AIInputEvent{}

	switch req.Type {
	case "interactive":
		// Interactive response to AI query
		inputEvent.IsInteractiveMessage = true
		inputEvent.InteractiveId = req.InteractiveID
		inputEvent.InteractiveJSONInput = req.Input

	case "free_input":
		// Free-form user input
		inputEvent.IsFreeInput = true
		inputEvent.FreeInput = req.FreeInput

	case "sync":
		// Sync message
		inputEvent.IsSyncMessage = true
		inputEvent.SyncType = req.Type
		inputEvent.SyncJsonInput = req.Input

	default:
		// Default to free input
		inputEvent.IsFreeInput = true
		inputEvent.FreeInput = req.FreeInput
		if inputEvent.FreeInput == "" {
			inputEvent.FreeInput = req.Input
		}
	}

	// Send the input event
	session.SendInput(inputEvent)

	log.Infof("Pushed event to run: %s, type: %s", runID, req.Type)

	writeJSON(w, PushEventResponse{
		Success: true,
		Message: "event pushed successfully",
	})
}
