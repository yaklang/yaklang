package aihttp

import (
	"fmt"
	"net/http"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handlePushEvent(w http.ResponseWriter, r *http.Request) {
	gw.handleStreamInput(w, r, false)
}

func readAIInputEventRequest(r *http.Request, runID string) (*ypb.AIInputEvent, error) {
	body, err := readRawBody(r)
	if err != nil {
		return nil, err
	}

	var event ypb.AIInputEvent
	if err := readProtoJSONBytes(body, &event); err == nil {
		return &event, nil
	} else {
		var legacy PushEventRequest
		if legacyErr := readJSONBytes(body, &legacy); legacyErr == nil {
			return convertPushToInputEvent(legacy, runID), nil
		} else {
			return nil, fmt.Errorf("parse AIInputEvent failed: %v; parse legacy PushEventRequest failed: %v", err, legacyErr)
		}
	}
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
