package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ---------------------------------------------------------------------------
// Registry unit tests
// ---------------------------------------------------------------------------

func TestMiniAITaskRegistry_RegisterAndGet(t *testing.T) {
	reg := NewMiniAITaskRegistry()

	called := false
	handler := func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		called = true
		return "ok", nil
	}

	reg.Register("test_task", handler)

	got, ok := reg.Get("test_task")
	require.True(t, ok, "handler should be found after register")
	require.NotNil(t, got)

	result, err := got(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Equal(t, "ok", result)
	require.True(t, called)
}

func TestMiniAITaskRegistry_Unregister(t *testing.T) {
	reg := NewMiniAITaskRegistry()
	reg.Register("test_task", func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		return nil, nil
	})

	_, ok := reg.Get("test_task")
	require.True(t, ok)

	reg.Unregister("test_task")

	_, ok = reg.Get("test_task")
	require.False(t, ok, "handler should not be found after unregister")
}

func TestMiniAITaskRegistry_GetUnknown(t *testing.T) {
	reg := NewMiniAITaskRegistry()
	_, ok := reg.Get("nonexistent")
	require.False(t, ok)
}

func TestMiniAITaskRegistry_Names(t *testing.T) {
	reg := NewMiniAITaskRegistry()
	reg.Register("a", func(ctx context.Context, _ *MiniAITaskContext, _ map[string]any) (any, error) { return nil, nil })
	reg.Register("b", func(ctx context.Context, _ *MiniAITaskContext, _ map[string]any) (any, error) { return nil, nil })
	reg.Register("c", func(ctx context.Context, _ *MiniAITaskContext, _ map[string]any) (any, error) { return nil, nil })

	names := reg.Names()
	require.Len(t, names, 3)
	nameMap := make(map[string]bool)
	for _, n := range names {
		nameMap[n] = true
	}
	require.True(t, nameMap["a"])
	require.True(t, nameMap["b"])
	require.True(t, nameMap["c"])
}

func TestMiniAITaskRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewMiniAITaskRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reg.Register(fmt.Sprintf("task_%d", idx), func(ctx context.Context, _ *MiniAITaskContext, _ map[string]any) (any, error) {
				return idx, nil
			})
		}(i)
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reg.Get(fmt.Sprintf("task_%d", idx))
		}(i)
	}

	wg.Wait()
	require.Len(t, reg.Names(), 50)
}

func TestReAct_RegisterMiniAITask(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("optimized text")),
	)
	require.NoError(t, err)

	customCalled := false
	r.RegisterMiniAITask("custom_test", func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		customCalled = true
		return map[string]any{"custom": true}, nil
	})

	handler, ok := r.miniAITaskRegistry.Get("custom_test")
	require.True(t, ok)
	_, err = handler(context.Background(), &MiniAITaskContext{ReAct: r, Config: r.config, Timeline: r.config.GetTimeline()}, nil)
	require.NoError(t, err)
	require.True(t, customCalled)

	r.UnregisterMiniAITask("custom_test")
	_, ok = r.miniAITaskRegistry.Get("custom_test")
	require.False(t, ok)
}

// ---------------------------------------------------------------------------
// Builtin handler tests
// ---------------------------------------------------------------------------

func TestBuiltinMiniAITasks_Registered(t *testing.T) {
	reg := NewMiniAITaskRegistry()
	RegisterBuiltinMiniAITasks(reg)

	_, ok := reg.Get("prompt_optimize")
	require.True(t, ok, "prompt_optimize should be registered")

	_, ok = reg.Get("timeline_summary")
	require.True(t, ok, "timeline_summary should be registered")
}

func TestHandlePromptOptimize_MissingPrompt(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("optimized")),
	)
	require.NoError(t, err)

	taskCtx := &MiniAITaskContext{
		ReAct:    r,
		Config:   r.config,
		Timeline: r.config.GetTimeline(),
	}

	_, err = handlePromptOptimize(context.Background(), taskCtx, map[string]any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt is required")
}

func TestHandlePromptOptimize_Success(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithSpeedPriorityAICallback(mockSpeedActionAI("prompt_optimize", map[string]any{
			"optimized_prompt": "对目标网站执行完整安全扫描",
			"reason":           "增加了具体的扫描步骤",
		})),
	)
	require.NoError(t, err)

	taskCtx := &MiniAITaskContext{
		ReAct:    r,
		Config:   r.config,
		Timeline: r.config.GetTimeline(),
	}

	result, err := handlePromptOptimize(context.Background(), taskCtx, map[string]any{
		"prompt": "帮我扫描这个网站",
	})
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "帮我扫描这个网站", resultMap["original"])
	require.Equal(t, "对目标网站执行完整安全扫描", resultMap["optimized_prompt"])
	require.Equal(t, "增加了具体的扫描步骤", resultMap["reason"])
}

func TestHandleTimelineSummary_NilTimeline(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("summary text")),
	)
	require.NoError(t, err)

	taskCtx := &MiniAITaskContext{
		ReAct:    r,
		Config:   r.config,
		Timeline: nil,
	}

	_, err = handleTimelineSummary(context.Background(), taskCtx, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timeline is not available")
}

func TestHandleTimelineSummary_Success(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithSpeedPriorityAICallback(mockSpeedActionAI("timeline_summary", map[string]any{
			"summary":    "已执行端口扫描，发现3个开放端口",
			"key_points": []any{"端口扫描完成", "发现3个开放端口"},
		})),
	)
	require.NoError(t, err)

	r.AddToTimeline("test_entry", "some timeline content")

	taskCtx := &MiniAITaskContext{
		ReAct:    r,
		Config:   r.config,
		Timeline: r.config.GetTimeline(),
	}

	result, err := handleTimelineSummary(context.Background(), taskCtx, nil)
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "已执行端口扫描，发现3个开放端口", resultMap["summary"])
	require.NotZero(t, resultMap["entry_count"])
}

// ---------------------------------------------------------------------------
// HandleSyncTypeAIMiniTaskEvent dispatch tests
// ---------------------------------------------------------------------------

func TestHandleSyncTypeAIMiniTaskEvent_UnknownTaskName(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	captured := captureSyncEvents(r)
	err = r.HandleSyncTypeAIMiniTaskEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{"task_name":"nonexistent_handler"}`,
		SyncID:        "test-sync-1",
	})
	require.NoError(t, err)

	events := captured()
	resp := findSyncResponse(t, events, "test-sync-1")
	require.Contains(t, resp["error"], "unknown mini ai task")
	available, ok := resp["available"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, available)
	// Each available entry should be a descriptor with name and description
	for _, item := range available {
		desc, ok := item.(map[string]any)
		require.True(t, ok, "available entry should be a map")
		require.NotEmpty(t, desc["name"])
	}
}

func TestHandleSyncTypeAIMiniTaskEvent_EmptyTaskName(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	captured := captureSyncEvents(r)
	err = r.HandleSyncTypeAIMiniTaskEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{"prompt":"some text"}`,
		SyncID:        "test-sync-2",
	})
	require.NoError(t, err)

	events := captured()
	resp := findSyncResponse(t, events, "test-sync-2")
	require.Contains(t, resp["error"], "task_name is required")
}

func TestHandleSyncTypeAIMiniTaskEvent_InvalidJSON(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	captured := captureSyncEvents(r)
	err = r.HandleSyncTypeAIMiniTaskEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{invalid json`,
		SyncID:        "test-sync-3",
	})
	require.NoError(t, err)

	events := captured()
	resp := findSyncResponse(t, events, "test-sync-3")
	require.Contains(t, resp["error"], "parse params failed")
}

func TestHandleSyncTypeAIMiniTaskEvent_CustomHandlerSuccess(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	r.RegisterMiniAITask("echo_test", func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		return map[string]any{
			"echoed": getStringParam(params, "msg"),
		}, nil
	})

	captured := captureSyncEvents(r)
	err = r.HandleSyncTypeAIMiniTaskEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{"task_name":"echo_test","msg":"hello world"}`,
		SyncID:        "test-sync-4",
	})
	require.NoError(t, err)

	events := captured()
	resp := findSyncResponse(t, events, "test-sync-4")
	require.Equal(t, "echo_test", resp["task_name"])
	result, ok := resp["result"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "hello world", result["echoed"])
}

func TestHandleSyncTypeAIMiniTaskEvent_HandlerPanic(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	r.RegisterMiniAITask("panic_test", func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		panic("intentional panic for test")
	})

	captured := captureSyncEvents(r)
	err = r.HandleSyncTypeAIMiniTaskEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{"task_name":"panic_test"}`,
		SyncID:        "test-sync-5",
	})
	require.NoError(t, err, "panic should be recovered, not propagated")

	events := captured()
	resp := findSyncResponse(t, events, "test-sync-5")
	errMsg, ok := resp["error"].(string)
	require.True(t, ok)
	require.Contains(t, errMsg, "panic")
	require.Contains(t, errMsg, "intentional panic for test")
}

func TestHandleSyncTypeAIMiniTaskEvent_HandlerError(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	r.RegisterMiniAITask("error_test", func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		return nil, fmt.Errorf("handler deliberate error")
	})

	captured := captureSyncEvents(r)
	err = r.HandleSyncTypeAIMiniTaskEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{"task_name":"error_test"}`,
		SyncID:        "test-sync-6",
	})
	require.NoError(t, err)

	events := captured()
	resp := findSyncResponse(t, events, "test-sync-6")
	require.Equal(t, "error_test", resp["task_name"])
	require.Contains(t, resp["error"], "handler deliberate error")
}

func TestHandleSyncTypeAIMiniTaskEvent_SyncID_Preserved(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	r.RegisterMiniAITask("syncid_test", func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		return map[string]any{"sync_id": taskCtx.SyncID}, nil
	})

	captured := captureSyncEvents(r)
	expectedSyncID := "sync-id-abc-123"
	err = r.HandleSyncTypeAIMiniTaskEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{"task_name":"syncid_test"}`,
		SyncID:        expectedSyncID,
	})
	require.NoError(t, err)

	events := captured()
	for _, e := range events {
		if !e.IsSync || e.NodeId != "ai_mini_task" {
			continue
		}
		require.Equal(t, expectedSyncID, e.SyncID, "response SyncID should match request SyncID")
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Content), &payload))
		result, ok := payload["result"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, expectedSyncID, result["sync_id"])
		return
	}
	t.Fatal("expected sync response with preserved SyncID")
}

func TestHandleSyncTypeAIMiniTaskEvent_TaskNameStrippedFromParams(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	var receivedParams map[string]any
	r.RegisterMiniAITask("params_test", func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		receivedParams = params
		return nil, nil
	})

	err = r.HandleSyncTypeAIMiniTaskEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{"task_name":"params_test","extra_param":"value","num":42}`,
		SyncID:        "test-sync-7",
	})
	require.NoError(t, err)

	// task_name should be stripped from params
	_, hasTaskName := receivedParams["task_name"]
	require.False(t, hasTaskName, "task_name should be stripped from handler params")
	require.Equal(t, "value", receivedParams["extra_param"])
}

// ---------------------------------------------------------------------------
// Integration: full dispatch via SendInputEvent
// ---------------------------------------------------------------------------

func TestMiniAITaskViaInputEventDispatch(t *testing.T) {
	r, err := NewTestReAct(
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {}),
		aicommon.WithSpeedPriorityAICallback(mockSpeedTextAI("dummy")),
	)
	require.NoError(t, err)

	handlerCalled := false
	r.RegisterMiniAITask("dispatch_integration", func(ctx context.Context, taskCtx *MiniAITaskContext, params map[string]any) (any, error) {
		handlerCalled = true
		require.NotNil(t, taskCtx.ReAct)
		require.NotNil(t, taskCtx.Config)
		return map[string]any{"dispatched": true}, nil
	})

	captured := captureSyncEvents(r)
	r.SendInputEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_AI_MINI_TASK,
		SyncJsonInput: `{"task_name":"dispatch_integration"}`,
		SyncID:        "integration-1",
	})

	time.Sleep(500 * time.Millisecond)
	require.True(t, handlerCalled, "handler should have been called via dispatch")

	events := captured()
	for _, e := range events {
		if !e.IsSync || e.SyncID != "integration-1" || e.NodeId != "ai_mini_task" {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Content), &payload))
		require.Equal(t, "dispatch_integration", payload["task_name"])
		return
	}
	t.Fatal("expected sync response from full dispatch path")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// captureSyncEvents sets up an event handler that captures sync events and
// returns a function to retrieve them. Must be called after NewTestReAct.
func captureSyncEvents(r *ReAct) func() []*schema.AiOutputEvent {
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

// findSyncResponse finds and parses the sync response event matching the given syncID.
func findSyncResponse(t *testing.T, events []*schema.AiOutputEvent, syncID string) map[string]any {
	t.Helper()
	for _, e := range events {
		if e == nil || !e.IsSync || e.SyncID != syncID || e.NodeId != "ai_mini_task" {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Content), &payload))
		return payload
	}
	t.Fatalf("expected sync response with syncID=%s and nodeId=ai_mini_task", syncID)
	return nil
}

// mockSpeedTextAI creates a speed-priority AI callback that returns a plain text output.
func mockSpeedTextAI(output string) aicommon.AICallbackType {
	return func(caller aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := aicommon.NewUnboundAIResponse()
		rsp.SetModelInfo("mock-provider", "mock-speed-model")
		rsp.EmitOutputStream(strings.NewReader(output))
		rsp.Close()
		return rsp, nil
	}
}

// mockSpeedActionAI creates a speed-priority AI callback that returns a structured
// JSON action matching the given actionName and params, which LiteForge can parse.
func mockSpeedActionAI(actionName string, actionParams map[string]any) aicommon.AICallbackType {
	return func(caller aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := aicommon.NewUnboundAIResponse()
		rsp.SetModelInfo("mock-provider", "mock-speed-model")

		action := map[string]any{
			"@action": actionName,
		}
		for k, v := range actionParams {
			action[k] = v
		}
		b, _ := json.Marshal(action)
		rsp.EmitOutputStream(strings.NewReader(string(b)))
		rsp.Close()
		return rsp, nil
	}
}

// ---------------------------------------------------------------------------
// Descriptors / SessionSnapshot disclosure tests
// ---------------------------------------------------------------------------

func TestMiniAITaskRegistry_Descriptors(t *testing.T) {
	reg := NewMiniAITaskRegistry()
	RegisterBuiltinMiniAITasks(reg)

	descs := reg.Descriptors()
	require.NotEmpty(t, descs)

	// Should contain prompt_optimize and timeline_summary
	found := make(map[string]string)
	for _, d := range descs {
		found[d.Name] = d.Description
	}
	_, hasPromptOptimize := found["prompt_optimize"]
	require.True(t, hasPromptOptimize)
	_, hasTimelineSummary := found["timeline_summary"]
	require.True(t, hasTimelineSummary)

	// Descriptions should be non-empty
	require.NotEmpty(t, found["prompt_optimize"])
	require.NotEmpty(t, found["timeline_summary"])
}

func TestMiniAITaskRegistry_Descriptors_CustomHandlerWithoutBuiltinDesc(t *testing.T) {
	reg := NewMiniAITaskRegistry()
	reg.Register("custom_no_desc", func(ctx context.Context, _ *MiniAITaskContext, _ map[string]any) (any, error) {
		return nil, nil
	})

	descs := reg.Descriptors()
	require.Len(t, descs, 1)
	require.Equal(t, "custom_no_desc", descs[0].Name)
	// Should fall back to name as description when not in builtin map
	require.Equal(t, "custom_no_desc", descs[0].Description)
}
