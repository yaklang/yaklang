package aiengine

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// NewInputEvent constructs the same input envelope accepted by StartReAct.
// Yak callers can use it to reply to a specific review endpoint without a Go bridge.
func NewInputEvent(fields map[string]any) (*ypb.AIInputEvent, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode AI input event: %w", err)
	}
	event := new(ypb.AIInputEvent)
	if err := json.Unmarshal(raw, event); err != nil {
		return nil, fmt.Errorf("decode AI input event: %w", err)
	}
	return event, nil
}
