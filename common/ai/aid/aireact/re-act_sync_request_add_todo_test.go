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

// ---------------------------------------------------------------------------
// Additional edge case tests
// ---------------------------------------------------------------------------

func TestHandleSyncTypeAddTodoEvent_EmptySyncJsonInput(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: "",
		SyncID:        "add-todo-empty",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-empty")
	require.False(t, resp["success"].(bool))
	// Empty SyncJsonInput causes JSON parse error
	require.Contains(t, resp["error"], "parse params failed")
}

func TestHandleSyncTypeAddTodoEvent_WhitespaceOnlyText(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"   "}`,
		SyncID:        "add-todo-ws",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-ws")
	require.False(t, resp["success"].(bool))
	require.Contains(t, resp["error"], "text is required")
}

func TestHandleSyncTypeAddTodoEvent_MultipleTodosCoexist(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	// Add first todo
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"first todo"}`,
		SyncID:        "multi-1",
	})
	require.NoError(t, err)

	// Add second todo
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"second todo"}`,
		SyncID:        "multi-2",
	})
	require.NoError(t, err)

	scope := aicommon.BuildVerificationTodoScope(r.GetCurrentTask())
	items := r.config.SnapshotVerificationTodoItemsByScope(scope)
	require.Len(t, items, 2, "both todos should coexist in session state")

	contents := make(map[string]bool)
	for _, item := range items {
		contents[item.Content] = true
	}
	require.True(t, contents["first todo"])
	require.True(t, contents["second todo"])
}

func TestHandleSyncTypeAddTodoEvent_SetCurrentActuallySetsInState(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"current verification todo","set_current":true}`,
		SyncID:        "add-todo-current-verify",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-current-verify")
	require.True(t, resp["success"].(bool))
	addedID := resp["todo_id"].(string)
	require.NotEmpty(t, addedID)

	// Verify current is actually set in canonical snapshot
	scope := aicommon.BuildVerificationTodoScope(r.GetCurrentTask())
	_, currentID, _ := r.config.SnapshotCanonicalTodos(scope)
	require.Equal(t, addedID, currentID, "current should point to the added todo")
}

func TestHandleSyncTypeAddTodoEvent_TimelineRecordsUserAddedTodo(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"timeline check todo"}`,
		SyncID:        "add-todo-timeline",
	})
	require.NoError(t, err)

	// Timeline should contain the user_added_todo entry
	dump := r.DumpTimeline()
	require.Contains(t, dump, "user_added_todo", "timeline should record user_added_todo entry")
	require.Contains(t, dump, "timeline check todo", "timeline should contain the todo text")
}

func TestHandleSyncTypeAddTodoEvent_SyncIDPreserved(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	expectedSyncID := "preserve-sync-id-test-123"
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"sync id test"}`,
		SyncID:        expectedSyncID,
	})
	require.NoError(t, err)

	events := captured()
	for _, e := range events {
		if e.IsSync && e.NodeId == "add_todo" {
			require.Equal(t, expectedSyncID, e.SyncID, "response SyncID should match request")
			return
		}
	}
	t.Fatal("expected sync response with preserved SyncID")
}

func TestHandleSyncTypeAddTodoEvent_ConcurrentAdds(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
				IsSyncMessage: true,
				SyncType:      SYNC_TYPE_ADD_TODO,
				SyncJsonInput: fmt.Sprintf(`{"text":"concurrent todo %d"}`, idx),
				SyncID:        fmt.Sprintf("concurrent-%d", idx),
			})
		}(i)
	}
	wg.Wait()

	scope := aicommon.BuildVerificationTodoScope(r.GetCurrentTask())
	items := r.config.SnapshotVerificationTodoItemsByScope(scope)
	require.Len(t, items, 10, "all 10 concurrent todos should be persisted")
}

func TestHandleSyncTypeAddTodoEvent_LongText(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	longText := "对目标网站 example.com 执行完整安全扫描，包括端口扫描、目录爆破、漏洞检测。" +
		"来源：用户要求。验收方法：扫描结果中列出所有开放端口及服务版本，" +
		"目录爆破发现的可访问路径需记录 HTTP 状态码，漏洞检测结果需包含漏洞类型和严重程度评级。"
	jsonInput := fmt.Sprintf(`{"text":%s}`, mustMarshalJSON(longText))

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: jsonInput,
		SyncID:        "add-todo-long",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-long")
	require.True(t, resp["success"].(bool))
	require.Equal(t, longText, resp["text"])
}

func TestHandleSyncTypeAddTodoEvent_DuplicateIDFails(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	// Add first todo with custom ID
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"first","id":"dup-id-1"}`,
		SyncID:        "dup-1",
	})
	require.NoError(t, err)

	// Add second todo with the same ID — should fail
	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"second","id":"dup-id-1"}`,
		SyncID:        "dup-2",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "dup-2")
	require.False(t, resp["success"].(bool), "duplicate ID add should fail")
	require.Contains(t, resp["error"], "add todo failed")

	// Verify only one todo exists
	scope := aicommon.BuildVerificationTodoScope(r.GetCurrentTask())
	items := r.config.SnapshotVerificationTodoItemsByScope(scope)
	require.Len(t, items, 1, "only the first todo should exist")
}

func TestHandleSyncTypeAddTodoEvent_EmptyTextWithID(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"","id":"some-id"}`,
		SyncID:        "add-todo-empty-text-id",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "add-todo-empty-text-id")
	require.False(t, resp["success"].(bool))
	require.Contains(t, resp["error"], "text is required")
}

func TestHandleSyncTypeAddTodoEvent_SetCurrentOnlyAppliesToFirstDelta(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
	)
	require.NoError(t, err)

	// Add first todo without set_current
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"first without current"}`,
		SyncID:        "seq-1",
	})
	require.NoError(t, err)

	// Add second todo WITH set_current
	captured := captureSyncEventsForAddTodo(r)
	err = r.HandleSyncTypeAddTodoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_ADD_TODO,
		SyncJsonInput: `{"text":"second with current","set_current":true}`,
		SyncID:        "seq-2",
	})
	require.NoError(t, err)

	events := captured()
	resp := findAddTodoSyncResponse(t, events, "seq-2")
	require.True(t, resp["success"].(bool))
	secondID := resp["todo_id"].(string)

	// Current should point to the second todo
	scope := aicommon.BuildVerificationTodoScope(r.GetCurrentTask())
	_, currentID, _ := r.config.SnapshotCanonicalTodos(scope)
	require.Equal(t, secondID, currentID, "current should be the second todo, not the first")
}

func mustMarshalJSON(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}
