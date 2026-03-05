package aihttp

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handlePushEvent(w http.ResponseWriter, r *http.Request) {
	runID := mux.Vars(r)["run_id"]

	session, ok := gw.runManager.Get(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "run not found: "+runID)
		return
	}

	if session.Status != RunStatusRunning && session.Status != RunStatusPending {
		writeError(w, http.StatusConflict, "run is not active, current status: "+string(session.Status))
		return
	}

	var req PushEventRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	inputEvent := convertPushToInputEvent(req, runID)
	if !hasInputPayload(inputEvent) {
		writeError(w, http.StatusBadRequest, "input event is empty")
		return
	}
	session.PushInput(inputEvent)

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "accepted",
	})
}

func convertPushToInputEvent(req PushEventRequest, runID string) *ypb.AIInputEvent {
	event := &ypb.AIInputEvent{
		IsStart:          req.IsStart,
		IsConfigHotpatch: req.IsConfigHotpatch,
		HotpatchType:     req.HotpatchType,
		FocusModeLoop:    req.FocusModeLoop,
	}
	if req.Params != nil {
		event.Params = ConvertAIParamsToYPB(*req.Params, runID)
	}
	if len(req.AttachedFiles) > 0 {
		event.AttachedFilePath = append([]string(nil), req.AttachedFiles...)
	}

	isInteractive := req.IsInteractiveMessage || req.Type == "interactive"
	isFreeInput := req.IsFreeInput || req.Type == "free_input"
	isSync := req.IsSyncMessage || req.Type == "sync"

	if isInteractive {
		event.IsInteractiveMessage = true
		event.InteractiveId = req.InteractiveID
		event.InteractiveJSONInput = req.InteractiveJSONInput
	}
	if isFreeInput {
		event.IsFreeInput = true
		event.FreeInput = req.FreeInput
		if event.FreeInput == "" {
			event.FreeInput = req.Content
		}
	}
	if isSync {
		event.IsSyncMessage = true
		event.SyncType = req.SyncType
		event.SyncJsonInput = req.SyncJSONInput
		event.SyncID = req.SyncID
	}

	return event
}

func hasInputPayload(event *ypb.AIInputEvent) bool {
	if event == nil {
		return false
	}
	return event.IsConfigHotpatch ||
		event.IsInteractiveMessage ||
		event.IsSyncMessage ||
		event.IsFreeInput ||
		event.FocusModeLoop != "" ||
		len(event.AttachedFilePath) > 0
}
