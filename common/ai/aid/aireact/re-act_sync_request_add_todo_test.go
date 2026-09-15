package aireact

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHandleSyncTypeAddTodoEvent_MissingText(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{}`,
		SyncID:        "add-todo-1",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-1")
	require.False(t, resp["success"].(bool))
	require.Contains(t, resp["error"], "text is required")
}

func TestHandleSyncTypeAddTodoEvent_InvalidJSON(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{invalid`,
		SyncID:        "add-todo-2",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-2")
	require.False(t, resp["success"].(bool))
	require.Contains(t, resp["error"], "parse params failed")
}

func TestHandleSyncTypeAddTodoEvent_Success(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"对 example.com 执行端口扫描，确认开放端口及服务版本"}`,
		SyncID:        "add-todo-3",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-3")
	require.True(t, resp["success"].(bool))
	require.NotEmpty(t, resp["todo_id"])
	require.Equal(t, "对 example.com 执行端口扫描，确认开放端口及服务版本", resp["text"])
}

func TestHandleSyncTypeAddTodoEvent_WithCustomID(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"test todo","id":"my-custom-id"}`,
		SyncID:        "add-todo-4",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-4")
	require.True(t, resp["success"].(bool))
	require.Equal(t, "my-custom-id", resp["todo_id"])
}

func TestHandleSyncTypeAddTodoEvent_SetCurrentWithCustomID(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"test todo with current","id":"current-todo-1","set_current":true}`,
		SyncID:        "add-todo-5",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-5")
	require.True(t, resp["success"].(bool))
	require.Equal(t, "current-todo-1", resp["todo_id"])
	require.True(t, resp["set_current"].(bool))
}

func TestHandleSyncTypeAddTodoEvent_SetCurrentAutoID(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"auto id current test","set_current":true}`,
		SyncID:        "add-todo-6",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-6")
	require.True(t, resp["success"].(bool))
	require.NotEmpty(t, resp["todo_id"])
	require.True(t, resp["set_current"].(bool))
}

func TestHandleSyncTypeAddTodoEvent_TodoPersistedInState(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	todoText := "验证持久化的待办条目"
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: fmt.Sprintf(`{"text":"%s"}`, todoText),
		SyncID:        "add-todo-7",
	})
	require.NoError(t, err)

	// Verify the TODO was actually written to SessionPromptState
	scope := aicommon.BuildVerificationTodoScope(r.GetCurrentTask())
	items := r.config.SnapshotVerificationTodoItemsByScope(scope)
	require.NotEmpty(t, items)

	var found bool
	for _, item := range items {
		if item.Content == todoText {
			found = true
			break
		}
	}
	require.True(t, found, "todo text should be persisted in session state")
}

func TestHandleSyncTypeAddTodoEvent_EmitsTodoListUpdate(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	var mu sync.Mutex
	var captured []*schema.AiOutputEvent
	r.config.EventHandler = func(e *schema.AiOutputEvent) {
		if e == nil {
			return
		}
		mu.Lock()
		captured = append(captured, e)
		mu.Unlock()
	}

	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"emission test todo"}`,
		SyncID:        "add-todo-8",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	// Should emit at least one todo_list_update or current_task_todo_list_update event
	var foundTodoUpdate bool
	for _, e := range captured {
		if e.Type == schema.EVENT_TYPE_TODO_LIST_UPDATE || e.Type == schema.EVENT_TYPE_CURRENT_TASK_TODO_LIST_UPDATE {
			foundTodoUpdate = true
			break
		}
	}
	require.True(t, foundTodoUpdate, "should emit todo list update event")
}

func TestHandleSyncTypeAddTodoEvent_ViaDispatch(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	var mu sync.Mutex
	var captured []*schema.AiOutputEvent
	r.config.EventHandler = func(e *schema.AiOutputEvent) {
		if e == nil {
			return
		}
		mu.Lock()
		captured = append(captured, e)
		mu.Unlock()
	}

	r.SendInputEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"dispatch integration todo"}`,
		SyncID:        "add-todo-dispatch",
	})

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var foundSyncResp bool
	for _, e := range captured {
		if !e.IsSync || e.SyncID != "add-todo-dispatch" || e.NodeId != "add_todo" {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Content), &payload))
		require.True(t, payload["success"].(bool))
		foundSyncResp = true
		break
	}
	require.True(t, foundSyncResp, "expected sync response from full dispatch path")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func captureSyncEventsForAddTodo(r *ReAct) func() []*schema.AiOutputEvent {
	var mu sync.Mutex
	var captured []*schema.AiOutputEvent

	r.config.EventHandler = func(e *schema.AiOutputEvent) {
		if e == nil {
			return
		}
		mu.Lock()
		captured = append(captured, e)
		mu.Unlock()
	}

	return func() []*schema.AiOutputEvent {
		mu.Lock()
		defer mu.Unlock()
		return append([]*schema.AiOutputEvent(nil), captured...)
	}
}

func findAddTodoSyncResponse(t *testing.T, events []*schema.AiOutputEvent, syncID string) map[string]any {
	t.Helper()
	for _, e := range events {
		if e == nil || !e.IsSync || e.SyncID != syncID || e.NodeId != "add_todo" {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Content), &payload))
		return payload
	}
	t.Fatalf("expected sync response with syncID=%s and nodeId=add_todo", syncID)
	return nil
}
