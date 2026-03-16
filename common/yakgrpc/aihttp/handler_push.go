package aihttp

import (
	"net/http"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (gw *AIAgentHTTPGateway) handlePushEvent(w http.ResponseWriter, r *http.Request) {
	gw.handleStreamInput(w, r, false)
}

func readAIInputEventRequest(r *http.Request) (*ypb.AIInputEvent, error) {
	var event ypb.AIInputEvent
	if err := readProtoJSON(r, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func readOptionalAIInputEventRequest(r *http.Request) (*ypb.AIInputEvent, error) {
	body, err := readOptionalRawBody(r)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}

	var event ypb.AIInputEvent
	if err := readProtoJSONBytes(body, &event); err != nil {
		return nil, err
	}
	return &event, nil
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
