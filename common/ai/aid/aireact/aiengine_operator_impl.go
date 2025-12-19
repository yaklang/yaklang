package aireact

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 以下方法实现 aicommon.AIEngineOperator 接口的便捷包装方法
// ReAct 的核心方法 SendInputEvent, Wait, IsFinished 已在 re-act.go 中实现

// SendFreeInput sends free text input to the ReAct instance
// This is a convenience wrapper around SendInputEvent
func (r *ReAct) SendFreeInput(input string) error {
	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   input,
	}
	return r.SendInputEvent(event)
}

// SendInteractiveResponse sends an interactive response to the ReAct instance
// Used to reply to AI questions or operations that require user confirmation
func (r *ReAct) SendInteractiveResponse(response string) error {
	event := &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveJSONInput: response,
	}
	return r.SendInputEvent(event)
}

// SendStartEvent sends a start event to initialize the ReAct instance
func (r *ReAct) SendStartEvent(params *ypb.AIStartParams) error {
	event := &ypb.AIInputEvent{
		IsStart: true,
		Params:  params,
	}
	return r.SendInputEvent(event)
}

// SendSyncEvent sends a sync event to request specific synchronization information
func (r *ReAct) SendSyncEvent(syncType string, jsonInput string) error {
	event := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      syncType,
		SyncJsonInput: jsonInput,
	}
	return r.SendInputEvent(event)
}

// SendConfigHotpatch sends a configuration hot-patch event
// Used to dynamically update the AI engine configuration at runtime
func (r *ReAct) SendConfigHotpatch(config map[string]interface{}) error {
	event := &ypb.AIInputEvent{
		IsConfigHotpatch: true,
	}
	return r.SendInputEvent(event)
}

