package reactloops

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

// SetActionExecutionValue stores runtime data in this loop, keyed by invocation
// identity rather than action name (which may occur several times in a response).
// Action is only an identity key; its parsed parameters remain unchanged.
// A nil value clears the key. This never waits for streamed model parameters.
// Async work must capture what it needs before the action handler returns.
func (r *ReActLoop) SetActionExecutionValue(action *aicommon.Action, key string, value any) {
	if r == nil || action == nil {
		return
	}
	r.actionExecutionMu.Lock()
	defer r.actionExecutionMu.Unlock()
	values := r.actionExecutionValues[action]
	if value == nil {
		delete(values, key)
		if len(values) == 0 {
			delete(r.actionExecutionValues, action)
		}
		return
	}
	if r.actionExecutionValues == nil {
		r.actionExecutionValues = make(map[*aicommon.Action]map[string]any)
	}
	if values == nil {
		values = make(map[string]any)
		r.actionExecutionValues[action] = values
	}
	values[key] = value
}

func (r *ReActLoop) GetActionExecutionValue(action *aicommon.Action, key string) any {
	if r == nil || action == nil {
		return nil
	}
	r.actionExecutionMu.Lock()
	defer r.actionExecutionMu.Unlock()
	return r.actionExecutionValues[action][key]
}

// ClearActionExecutionValues releases references after execution, rejection or
// a failed transaction. It affects only this invocation in this loop.
func (r *ReActLoop) ClearActionExecutionValues(action *aicommon.Action) {
	if r == nil {
		return
	}
	r.actionExecutionMu.Lock()
	defer r.actionExecutionMu.Unlock()
	delete(r.actionExecutionValues, action)
}

func (r *ReActLoop) clearCallsExecutionValues(calls []LoopCall) {
	for _, call := range calls {
		r.ClearActionExecutionValues(call.Action)
	}
}
