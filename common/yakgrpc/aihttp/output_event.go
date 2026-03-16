package aihttp

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func newSystemOutputEvent(eventType string) *ypb.AIOutputEvent {
	return &ypb.AIOutputEvent{
		Type:      eventType,
		IsSystem:  true,
		Timestamp: time.Now().Unix(),
		EventUUID: uuid.NewString(),
	}
}

func newResultOutputEvent(eventType string) *ypb.AIOutputEvent {
	event := newSystemOutputEvent(eventType)
	event.IsResult = true
	return event
}

func newFailedOutputEvent(err error) *ypb.AIOutputEvent {
	event := newResultOutputEvent(string(RunStatusFailed))
	if err == nil {
		return event
	}
	content, marshalErr := json.Marshal(map[string]string{"error": err.Error()})
	if marshalErr == nil {
		event.IsJson = true
		event.Content = content
	}
	return event
}

func normalizeOutputEvent(event *ypb.AIOutputEvent) *ypb.AIOutputEvent {
	if event == nil {
		return nil
	}
	if event.GetTimestamp() > 0 && event.GetEventUUID() != "" {
		return event
	}
	cloned := proto.Clone(event).(*ypb.AIOutputEvent)
	if cloned.GetTimestamp() <= 0 {
		cloned.Timestamp = time.Now().Unix()
	}
	if cloned.GetEventUUID() == "" {
		cloned.EventUUID = uuid.NewString()
	}
	return cloned
}

func isTerminalRunEventType(eventType string) bool {
	switch eventType {
	case string(RunStatusCompleted), string(RunStatusCancelled), string(RunStatusFailed), "error", "done":
		return true
	default:
		return false
	}
}
