package aicommon

import (
	"github.com/yaklang/yaklang/common/schema"
)

// TodoListUpdatePayload is the JSON payload of EVENT_TYPE_TODO_LIST_UPDATE.
// It carries the full structured TODO snapshot (so the frontend can render
// the panel as-is without rebuilding from history) plus the operations
// applied in this round and minimal context (task / iteration).
//
// 关键词: TodoListUpdatePayload, EVENT_TYPE_TODO_LIST_UPDATE, 全局 TODO
type TodoListUpdatePayload struct {
	Items          []VerificationTodoItem `json:"items"`
	Stats          VerificationTodoStats  `json:"stats"`
	AppliedOps     []VerifyNextMovement   `json:"applied_ops"`
	Satisfied      bool                   `json:"satisfied"`
	IterationIndex int                    `json:"iteration_index"`
	TaskID         string                 `json:"task_id,omitempty"`
}

// EmitTodoListUpdate emits a structured TODO list update event under
// schema.EVENT_TYPE_TODO_LIST_UPDATE with NodeId "todo_list".
//
// 关键词: EmitTodoListUpdate, frontend TODO 面板, 全局 TODO 通道
func (r *Emitter) EmitTodoListUpdate(payload TodoListUpdatePayload) (*schema.AiOutputEvent, error) {
	if r == nil {
		return nil, nil
	}
	if payload.Items == nil {
		payload.Items = []VerificationTodoItem{}
	}
	if payload.AppliedOps == nil {
		payload.AppliedOps = []VerifyNextMovement{}
	}
	return r.EmitJSON(schema.EVENT_TYPE_TODO_LIST_UPDATE, "todo_list", payload)
}
