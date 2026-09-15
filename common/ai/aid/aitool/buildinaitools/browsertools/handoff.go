package browsertools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

const handoffPollInterval = 750 * time.Millisecond

type browserHandoffEvent struct {
	HandoffID   string `json:"handoffId"`
	CallToolID  string `json:"callToolId,omitempty"`
	DeviceID    string `json:"deviceId"`
	Reason      string `json:"reason"`
	Message     string `json:"message,omitempty"`
	State       string `json:"state"`
	RequestedAt int64  `json:"requestedAt,omitempty"`
	ResolvedAt  int64  `json:"resolvedAt,omitempty"`
	TabID       int    `json:"tabId"`
	FrameID     int    `json:"frameId"`
	DocumentID  string `json:"documentId,omitempty"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Origin      string `json:"origin,omitempty"`
}

func stringValue(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func intValue(values map[string]interface{}, key string) int {
	return int(aitool.InvokeParams(values).GetInteger(key))
}

func handoffEvent(result interface{}, deviceID, callToolID string) (browserHandoffEvent, bool) {
	values, ok := result.(map[string]interface{})
	if !ok {
		return browserHandoffEvent{}, false
	}
	target, ok := values["target"].(map[string]interface{})
	if !ok {
		return browserHandoffEvent{}, false
	}
	event := browserHandoffEvent{
		HandoffID:   stringValue(values, "id"),
		CallToolID:  callToolID,
		DeviceID:    strings.TrimSpace(deviceID),
		Reason:      stringValue(values, "reason"),
		Message:     stringValue(values, "message"),
		State:       stringValue(values, "state"),
		RequestedAt: int64(aitool.InvokeParams(values).GetInteger("requestedAt")),
		ResolvedAt:  int64(aitool.InvokeParams(values).GetInteger("resolvedAt")),
		TabID:       intValue(target, "tabId"),
		FrameID:     intValue(target, "frameId"),
		DocumentID:  stringValue(target, "documentId"),
		Title:       stringValue(target, "title"),
		URL:         stringValue(target, "grantedUrl"),
		Origin:      stringValue(target, "origin"),
	}
	return event, event.HandoffID != "" && event.DeviceID != "" && event.State != ""
}

func emitHandoffEvent(runtimeConfig *aitool.ToolRuntimeConfig, event browserHandoffEvent) error {
	if runtimeConfig == nil || runtimeConfig.EmitUIEvent == nil {
		return nil
	}
	return runtimeConfig.EmitUIEvent(schema.EVENT_TYPE_BROWSER_HANDOFF, event.HandoffID, event)
}

func callAgentCapability(
	ctx context.Context,
	caller Caller,
	deviceID string,
	target Target,
	method string,
	params aitool.InvokeParams,
	timeout time.Duration,
	withTarget bool,
	runtimeConfig *aitool.ToolRuntimeConfig,
) (interface{}, error) {
	result, err := CallCapability(ctx, caller, deviceID, target, method, params, timeout, withTarget)
	if err != nil || method != "browser.handoff.request" {
		return result, err
	}
	callToolID := ""
	if runtimeConfig != nil {
		callToolID = runtimeConfig.RuntimeID
	}
	event, ok := handoffEvent(result, deviceID, callToolID)
	if !ok {
		return nil, fmt.Errorf("browser handoff returned invalid metadata")
	}
	if err := emitHandoffEvent(runtimeConfig, event); err != nil {
		return nil, fmt.Errorf("emit browser handoff UI event: %w", err)
	}
	if runtimeConfig == nil || runtimeConfig.EmitUIEvent == nil {
		return result, nil
	}

	ticker := time.NewTicker(handoffPollInterval)
	defer ticker.Stop()
	for event.State == "waiting_for_user" {
		select {
		case <-ctx.Done():
			cancelContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_, _ = CallCapability(
				cancelContext,
				caller,
				deviceID,
				Target{},
				"browser.handoff.resolve",
				aitool.InvokeParams{"handoffId": event.HandoffID, "outcome": "cancelled"},
				2*time.Second,
				false,
			)
			cancel()
			return nil, ctx.Err()
		case <-ticker.C:
		}
		result, err = CallCapability(
			ctx, caller, deviceID, Target{}, "browser.handoff.status", aitool.InvokeParams{}, ReadTimeout, false,
		)
		if err != nil {
			return nil, err
		}
		next, valid := handoffEvent(result, deviceID, runtimeConfig.RuntimeID)
		if !valid || next.HandoffID != event.HandoffID {
			return nil, fmt.Errorf("browser handoff state no longer matches request %q", event.HandoffID)
		}
		event = next
	}
	if err := emitHandoffEvent(runtimeConfig, event); err != nil {
		return nil, fmt.Errorf("emit completed browser handoff UI event: %w", err)
	}
	return result, nil
}
