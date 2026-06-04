package aicommon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

// TestEmitter_EmitTodoListUpdate_PayloadShape 验证 EmitTodoListUpdate 发出的
// 事件:
//  1. type == EVENT_TYPE_TODO_LIST_UPDATE
//  2. nodeId == "todo_list"
//  3. content 是 JSON, 字段顺序与计划一致 (items / stats / applied_ops /
//     iteration_index / task_id), nil slice 被规范化为 [].
//
// 关键词: EmitTodoListUpdate 事件 schema, 全局 TODO 通道, 前端面板契约
func TestEmitter_EmitTodoListUpdate_PayloadShape(t *testing.T) {
	var captured *schema.AiOutputEvent
	emitter := NewEmitter("test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = e
		return e, nil
	})

	payload := TodoListUpdatePayload{
		Items: []VerificationTodoItem{
			{ID: "verify_target", Content: "复现错误码", Status: VerificationTodoStatusPending, CreatedAt: 1, UpdatedAt: 1},
		},
		Stats: VerificationTodoStats{Pending: 1},
		AppliedOps: []VerifyNextMovement{
			{Op: "add", ID: "verify_target", Content: "复现错误码"},
		},
		Satisfied:      false,
		IterationIndex: 7,
		TaskID:         "task-abc",
	}

	event, err := emitter.EmitTodoListUpdate(payload)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.NotNil(t, captured)
	require.Equal(t, schema.EVENT_TYPE_TODO_LIST_UPDATE, captured.Type)
	require.Equal(t, "todo_list", captured.NodeId)
	require.True(t, captured.IsJson)

	var decoded TodoListUpdatePayload
	require.NoError(t, json.Unmarshal(captured.Content, &decoded))
	require.Len(t, decoded.Items, 1)
	require.Equal(t, "verify_target", decoded.Items[0].ID)
	require.Equal(t, VerificationTodoStatusPending, decoded.Items[0].Status)
	require.Equal(t, 1, decoded.Stats.Pending)
	require.Len(t, decoded.AppliedOps, 1)
	require.Equal(t, "add", decoded.AppliedOps[0].Op)
	require.Equal(t, 7, decoded.IterationIndex)
	require.Equal(t, "task-abc", decoded.TaskID)
	require.False(t, decoded.Satisfied)
}

// TestEmitter_EmitTodoListUpdate_NormalizesNilSlices 验证 nil items / nil
// applied_ops 会被替换为空切片, 前端拿到的 JSON 始终是 [] 而不是 null。
//
// 关键词: nil slice 归一化, 前端容错
func TestEmitter_EmitTodoListUpdate_NormalizesNilSlices(t *testing.T) {
	var captured *schema.AiOutputEvent
	emitter := NewEmitter("test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = e
		return e, nil
	})

	_, err := emitter.EmitTodoListUpdate(TodoListUpdatePayload{
		Satisfied:      true,
		IterationIndex: 0,
	})
	require.NoError(t, err)
	require.NotNil(t, captured)
	body := string(captured.Content)
	require.Contains(t, body, `"items":[]`)
	require.Contains(t, body, `"applied_ops":[]`)
}

// TestEmitter_EmitCurrentTaskTodoList_ScopedItemsOnly 验证 EmitCurrentTaskTodoList
// 只携带当前 task scope 下的 items/stats, 不包含兄弟任务或 legacy 无 scope 项。
func TestEmitter_EmitCurrentTaskTodoList_ScopedItemsOnly(t *testing.T) {
	cfg := NewConfig(context.Background())
	taskOne := NewStatefulTaskBase("task-1", "one", nil, nil, true)
	taskTwo := NewStatefulTaskBase("task-2", "two", nil, nil, true)

	scopeOne := BuildVerificationTodoScope(taskOne)
	scopeTwo := BuildVerificationTodoScope(taskTwo)
	cfg.ApplyVerificationTodoOps(scopeOne, false, []VerifyNextMovement{
		{Op: "add", ID: "a", Content: "task one todo"},
	})
	cfg.ApplyVerificationTodoOps(scopeTwo, false, []VerifyNextMovement{
		{Op: "add", ID: "b", Content: "task two todo"},
	})
	cfg.ApplyVerificationTodoOps(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "legacy", Content: "legacy todo"},
	})

	var captured *schema.AiOutputEvent
	emitter := NewEmitter("test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = e
		return e, nil
	})

	_, err := emitter.EmitCurrentTaskTodoList(cfg, taskOne, 3, false, []VerifyNextMovement{
		{Op: "add", ID: "a", Content: "task one todo"},
	})
	require.NoError(t, err)
	require.NotNil(t, captured)

	require.Equal(t, schema.EVENT_TYPE_CURRENT_TASK_TODO_LIST_UPDATE, captured.Type)
	require.Equal(t, "current_task_todo_list", captured.NodeId)

	var decoded TodoListUpdatePayload
	require.NoError(t, json.Unmarshal(captured.Content, &decoded))
	require.Equal(t, "task-1", decoded.TaskID)
	require.Len(t, decoded.Items, 1)
	require.Equal(t, "a", decoded.Items[0].ID)
	require.Equal(t, 1, decoded.Stats.Pending)
	require.Equal(t, 3, decoded.IterationIndex)

	allItems := cfg.SnapshotVerificationTodoItems()
	require.Len(t, allItems, 3, "session store should still hold all tasks' TODOs")
}

func TestEmitter_EmitTodoListUpdates_EmitsBothEventTypes(t *testing.T) {
	cfg := NewConfig(context.Background())
	task := NewStatefulTaskBase("task-emit-pair", "pair", nil, nil, true)
	scope := BuildVerificationTodoScope(task)
	cfg.ApplyVerificationTodoOps(scope, false, []VerifyNextMovement{
		{Op: "add", ID: "x", Content: "scoped todo"},
	})

	var captured []schema.EventType
	emitter := NewEmitter("test-emitter", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e.Type)
		return e, nil
	})

	emitter.EmitTodoListUpdates(cfg, task, TodoListUpdatePayload{
		Items:          cfg.SnapshotVerificationTodoItems(),
		Stats:          cfg.GetVerificationTodoStats(),
		AppliedOps:     []VerifyNextMovement{{Op: "add", ID: "x", Content: "scoped todo"}},
		IterationIndex: 1,
		TaskID:         task.GetId(),
	})

	require.Contains(t, captured, schema.EVENT_TYPE_TODO_LIST_UPDATE)
	require.Contains(t, captured, schema.EVENT_TYPE_CURRENT_TASK_TODO_LIST_UPDATE)
}
