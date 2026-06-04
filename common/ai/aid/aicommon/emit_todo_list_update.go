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
	TaskIndex      string                 `json:"task_index,omitempty"`
}

// BuildCurrentTaskTodoListPayload builds a TodoListUpdatePayload scoped to the
// given task. Only TODO items owned by that task (matching VerificationTodoScope)
// are included; sibling or legacy unscoped items from the session store are
// omitted. When task is nil or has no id, items and stats are empty.
//
// 关键词: BuildCurrentTaskTodoListPayload, 当前任务 TODO 快照, scope 过滤
func BuildCurrentTaskTodoListPayload(
	cfg AICallerConfigIf,
	task AIStatefulTask,
	iterationIndex int,
	satisfied bool,
	appliedOps []VerifyNextMovement,
) TodoListUpdatePayload {
	scope := BuildVerificationTodoScope(task)
	payload := TodoListUpdatePayload{
		Satisfied:      satisfied,
		IterationIndex: iterationIndex,
		TaskID:         scope.TaskID,
		TaskIndex:      scope.TaskIndex,
	}
	if len(appliedOps) > 0 {
		payload.AppliedOps = append([]VerifyNextMovement(nil), appliedOps...)
	}
	if cfg == nil || scope.IsZero() {
		payload.Items = []VerificationTodoItem{}
		payload.Stats = VerificationTodoStats{}
		return payload
	}
	payload.Items = cfg.SnapshotVerificationTodoItemsByScope(scope)
	payload.Stats = cfg.GetVerificationTodoStatsByScope(scope)
	return payload
}

func normalizeTodoListUpdatePayload(payload TodoListUpdatePayload) TodoListUpdatePayload {
	if payload.Items == nil {
		payload.Items = []VerificationTodoItem{}
	}
	if payload.AppliedOps == nil {
		payload.AppliedOps = []VerifyNextMovement{}
	}
	return payload
}

// EmitCurrentTaskTodoList emits EVENT_TYPE_CURRENT_TASK_TODO_LIST_UPDATE with
// only the current task's TODO snapshot (see BuildCurrentTaskTodoListPayload).
// Use this when the frontend should refresh the panel for one task without
// receiving TODO items owned by other tasks in the same session store.
//
// 关键词: EmitCurrentTaskTodoList, 当前任务 TODO 通道, scope 过滤 emit
func (r *Emitter) EmitCurrentTaskTodoList(
	cfg AICallerConfigIf,
	task AIStatefulTask,
	iterationIndex int,
	satisfied bool,
	appliedOps []VerifyNextMovement,
) (*schema.AiOutputEvent, error) {
	if r == nil {
		return nil, nil
	}
	payload := normalizeTodoListUpdatePayload(
		BuildCurrentTaskTodoListPayload(cfg, task, iterationIndex, satisfied, appliedOps),
	)
	return r.EmitJSON(schema.EVENT_TYPE_CURRENT_TASK_TODO_LIST_UPDATE, "current_task_todo_list", payload)
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
