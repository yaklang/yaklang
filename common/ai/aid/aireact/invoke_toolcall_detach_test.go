package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func detachTestRuntime(t *testing.T, native bool, tool *aitool.Tool, opts ...aicommon.ConfigOption) (*ReAct, *reactloops.ReActLoop, aicommon.AIStatefulTask) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	base := []aicommon.ConfigOption{aicommon.WithTools(tool), aicommon.WithAgreeYOLO(), aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return nil, errors.New("unexpected model request")
	})}
	cfg := aicommon.NewConfig(ctx, append(base, opts...)...)
	r := &ReAct{config: cfg, Emitter: cfg.Emitter}
	r.promptManager = NewPromptManager(r, t.TempDir())
	loop, err := reactloops.NewReActLoop("detach-test", r, reactloops.WithFunctionCallMode(native), reactloops.WithFunctionCallActionVariants(), reactloops.WithAllowAIForge(false), reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false), reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true))
	require.NoError(t, err)
	t.Cleanup(loop.Release)
	task := aicommon.NewStatefulTaskBase("detach-owner", "original owner", ctx, cfg.Emitter, true)
	loop.SetCurrentTask(task)
	return r, loop, task
}
func detachAwait(t *testing.T, ctx context.Context, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("execution did not reach expected phase:", ctx.Err())
	}
}
func detachAction(t *testing.T, loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, fields aitool.InvokeParams) (*reactloops.LoopActionHandlerOperator, <-chan struct{}) {
	t.Helper()
	var action *aicommon.Action
	if loop.FunctionCallModeEnabled() {
		action = aicommon.NewSimpleAction(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, fields)
	} else {
		fields["@action"] = schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL
		raw, err := json.Marshal(fields)
		require.NoError(t, err)
		action, err = aicommon.ExtractAction(string(raw), schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
		require.NoError(t, err)
	}
	handler, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	require.NoError(t, err)
	require.NoError(t, handler.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	done := make(chan struct{})
	go func() { defer close(done); handler.ActionHandler(loop, action, op) }()
	return op, done
}

func TestDirectToolDetachDuringExecution(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			started, trigger, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			ids := make(chan string, 8)
			tool, err := aitool.New("detach_fixture", aitool.WithDangerousNoNeedUserReview(true), aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
				calls.Add(1)
				fmt.Fprintln(stdout, "before detach")
				close(started)
				select {
				case <-trigger:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				var wg sync.WaitGroup
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						id, err := runtime.Detach()
						if err != nil {
							t.Error(err)
						}
						ids <- id
					}()
				}
				wg.Wait()
				fmt.Fprintln(stdout, "after detach")
				select {
				case <-finish:
					return "settled", nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}))
			require.NoError(t, err)
			r, loop, task := detachTestRuntime(t, native, tool)
			op, done := detachAction(t, loop, task, aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "test promotion"})
			detachAwait(t, task.GetContext(), started)
			select {
			case <-done:
				t.Fatal("foreground returned before detach")
			default:
			}
			close(trigger)
			detachAwait(t, task.GetContext(), done)
			require.Zero(t, op.GetExecutedToolCallCount(), "a receipt is not a completed tool")
			manager, err := loop.GetAsyncExecutionManager()
			require.NoError(t, err)
			list := manager.List()
			require.Len(t, list, 1)
			require.Contains(t, op.GetFeedback().String(), list[0].ID)
			for i := 0; i < 8; i++ {
				select {
				case id := <-ids:
					require.Equal(t, list[0].ID, id)
				case <-task.GetContext().Done():
					t.Fatal("detach stalled")
				}
			}
			live, err := manager.Inspect(task.GetContext(), list[0].ID)
			require.NoError(t, err)
			require.Contains(t, live.Observation.Stdout, "before detach")
			require.Empty(t, live.Observation.Result)
			require.Empty(t, task.GetAllToolCallResults())
			close(finish)
			summary, err := manager.Wait(task.GetContext(), list[0].ID)
			require.NoError(t, err)
			require.Equal(t, asyncexecmanager.StateCompleted, summary.State)
			snapshot, err := manager.Inspect(task.GetContext(), list[0].ID)
			require.NoError(t, err)
			require.Contains(t, snapshot.Observation.Stdout, "after detach")
			require.EqualValues(t, 1, calls.Load())
			require.Len(t, task.GetAllToolCallResults(), 1)
			require.True(t, r.config.GetAiToolManager().IsRecentlyUsedTool(tool.Name))
			var canonical aitool.ToolResult
			require.NoError(t, json.Unmarshal(snapshot.Observation.Result, &canonical))
			require.Equal(t, task.GetAllToolCallResults()[0].ID, canonical.ID)
		})
	}
}

func TestToolDefaultAsyncAndExplicitOverride(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, mode := range []string{"default", "explicit_sync", "explicit_async"} {
			t.Run(fmt.Sprintf("%t/%s", native, mode), func(t *testing.T) {
				started, finish := make(chan struct{}), make(chan struct{})
				tool, err := aitool.New("default_async_fixture", aitool.WithDefaultAsync(true), aitool.WithDangerousNoNeedUserReview(true), aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, _ *aitool.ToolRuntimeConfig, _, _ io.Writer) (any, error) {
					close(started)
					select {
					case <-finish:
						return "done", nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}))
				require.NoError(t, err)
				_, loop, task := detachTestRuntime(t, native, tool)
				fields := aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "default scheduling"}
				if mode != "default" {
					fields["async_directly_call_tool"] = mode == "explicit_async"
				}
				_, done := detachAction(t, loop, task, fields)
				detachAwait(t, task.GetContext(), started)
				manager, err := loop.GetAsyncExecutionManager()
				require.NoError(t, err)
				if mode == "explicit_sync" {
					require.Empty(t, manager.List())
					select {
					case <-done:
						t.Fatal("explicit sync detached")
					default:
					}
				} else {
					detachAwait(t, task.GetContext(), done)
					require.Len(t, manager.List(), 1)
				}
				close(finish)
				detachAwait(t, task.GetContext(), done)
				for _, entry := range manager.List() {
					_, err = manager.Wait(task.GetContext(), entry.ID)
					require.NoError(t, err)
				}
				require.Len(t, task.GetAllToolCallResults(), 1)
			})
		}
	}
}

func TestIntervalReviewPromotesRunningTool(t *testing.T) {
	finish, backgroundReviewed := make(chan struct{}), make(chan struct{})
	var calls, reviews atomic.Int32
	tool, err := aitool.New("review_detach_fixture", aitool.WithDangerousNoNeedUserReview(true), aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, _ *aitool.ToolRuntimeConfig, stdout, _ io.Writer) (any, error) {
		calls.Add(1)
		fmt.Fprintln(stdout, "progress must not suppress detach review")
		select {
		case <-finish:
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	require.NoError(t, err)
	_, loop, task := detachTestRuntime(t, false, tool, aicommon.WithDisableToolCallerIntervalReview(false), aicommon.WithToolCallerIntervalReviewDuration(10*time.Millisecond), aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		count := reviews.Add(1)
		decision, supported := "continue", "false"
		if count == 1 {
			decision, supported = "detach", "true"
		}
		if !strings.Contains(req.GetPrompt(), "Runtime supports detachment: "+supported) {
			return nil, fmt.Errorf("unexpected detach capability on review %d", count)
		}
		if count == 2 {
			close(backgroundReviewed)
		}
		rsp := cfg.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`{"@action":"interval-toolcall-review","decision":%q,"reason":"healthy background service","progress_summary":"ready","estimated_remaining_time":"unknown"}`, decision)))
		rsp.Close()
		return rsp, nil
	}))
	_, done := detachAction(t, loop, task, aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "interval promotion"})
	detachAwait(t, task.GetContext(), done)
	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	entries := manager.List()
	require.Len(t, entries, 1)
	detachAwait(t, task.GetContext(), backgroundReviewed)
	close(finish)
	_, err = manager.Wait(task.GetContext(), entries[0].ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	require.GreaterOrEqual(t, reviews.Load(), int32(1))
	require.Len(t, task.GetAllToolCallResults(), 1)
}

func TestBatchDynamicDetachReleasesForegroundSlots(t *testing.T) {
	finish := make(chan struct{})
	var calls atomic.Int32
	tool, err := aitool.New("batch_detach_fixture", aitool.WithDangerousNoNeedUserReview(true), aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, _, _ io.Writer) (any, error) {
		calls.Add(1)
		if _, err := runtime.Detach(); err != nil {
			return nil, err
		}
		select {
		case <-finish:
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	require.NoError(t, err)
	r, loop, task := detachTestRuntime(t, false, tool)
	request := &aicommon.ToolCallGroupRequest{}
	for i := 0; i < 4; i++ {
		request.Calls = append(request.Calls, aicommon.ToolCallGroupCall{ToolName: tool.Name, Params: aitool.InvokeParams{}, Reason: "promote after acquiring a foreground slot"})
	}
	done := make(chan struct{})
	var result *aicommon.ToolCallGroupResult
	go func() {
		defer close(done)
		var err error
		result, err = r.ExecuteToolCallGroup(task.GetContext(), task, request)
		if err != nil {
			t.Error(err)
		}
	}()
	detachAwait(t, task.GetContext(), done)
	require.EqualValues(t, 4, calls.Load(), "four promoted calls must pass a three-slot foreground gate")
	require.Len(t, result.Outcomes, 4)
	for _, outcome := range result.Outcomes {
		require.Equal(t, aicommon.ToolCallStageSubmitted, outcome.Stage)
		require.NotEmpty(t, outcome.ExecuteID)
		require.Nil(t, outcome.Result)
	}
	close(finish)
	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	for _, outcome := range result.Outcomes {
		summary, err := manager.Wait(task.GetContext(), outcome.ExecuteID)
		require.NoError(t, err)
		require.Equal(t, asyncexecmanager.StateCompleted, summary.State)
	}
	require.Len(t, task.GetAllToolCallResults(), 4)
	for _, outcome := range result.Outcomes {
		require.Nil(t, outcome.Result, "background completion must not mutate returned receipts")
	}
}

func TestDetachedToolStopJoinsCleanup(t *testing.T) {
	cleanupStarted, releaseCleanup := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCleanup) }) }
	defer release()
	tool, err := aitool.New("detach_cleanup_fixture", aitool.WithDangerousNoNeedUserReview(true), aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, stdout, _ io.Writer) (any, error) {
		if _, err := runtime.Detach(); err != nil {
			return nil, err
		}
		<-ctx.Done()
		close(cleanupStarted)
		<-releaseCleanup
		fmt.Fprintln(stdout, "cleanup finished")
		return nil, ctx.Err()
	}))
	require.NoError(t, err)
	_, loop, task := detachTestRuntime(t, false, tool)
	_, done := detachAction(t, loop, task, aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "verify cleanup ownership"})
	detachAwait(t, task.GetContext(), done)
	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	entries := manager.List()
	require.Len(t, entries, 1)
	_, err = manager.Stop(entries[0].ID)
	require.NoError(t, err)
	detachAwait(t, task.GetContext(), cleanupStarted)
	waitCtx, cancel := context.WithTimeout(task.GetContext(), 20*time.Millisecond)
	defer cancel()
	stopping, err := manager.Wait(waitCtx, entries[0].ID)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, asyncexecmanager.StateStopping, stopping.State)
	release()
	final, err := manager.Wait(task.GetContext(), entries[0].ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, final.State)
	snapshot, err := manager.Inspect(task.GetContext(), entries[0].ID)
	require.NoError(t, err)
	require.Contains(t, snapshot.Observation.Stdout, "cleanup finished")
}

func TestDetachCapacityFailureKeepsForegroundOwner(t *testing.T) {
	attempted, finish := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	detachErr := make(chan error, 1)
	tool, err := aitool.New("detach_capacity_fixture", aitool.WithDangerousNoNeedUserReview(true), aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, _, _ io.Writer) (any, error) {
		calls.Add(1)
		_, err := runtime.Detach()
		detachErr <- err
		close(attempted)
		select {
		case <-finish:
			return "foreground result", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	require.NoError(t, err)
	_, loop, task := detachTestRuntime(t, false, tool)
	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		resource, err := asyncexecmanager.NewResource(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, func(context.Context) (asyncexecmanager.Observation, error) {
			return asyncexecmanager.Observation{}, nil
		})
		require.NoError(t, err)
		_, err = manager.Start(asyncexecmanager.Request{Resource: resource})
		require.NoError(t, err)
	}
	op, done := detachAction(t, loop, task, aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "capacity rejection"})
	detachAwait(t, task.GetContext(), attempted)
	require.ErrorIs(t, <-detachErr, asyncexecmanager.ErrCapacity)
	select {
	case <-done:
		t.Fatal("failed promotion released the foreground owner")
	default:
	}
	require.Len(t, manager.List(), 4)
	close(finish)
	detachAwait(t, task.GetContext(), done)
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, 1, op.GetExecutedToolCallCount())
	require.Len(t, task.GetAllToolCallResults(), 1)
}

func TestAlreadyAsyncDetachUsesOriginalExecutionID(t *testing.T) {
	ids := make(chan string, 2)
	finish := make(chan struct{})
	tool, err := aitool.New("already_detached_fixture", aitool.WithDangerousNoNeedUserReview(true), aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, _, _ io.Writer) (any, error) {
		for i := 0; i < 2; i++ {
			id, err := runtime.Detach()
			if err != nil {
				return nil, err
			}
			ids <- id
		}
		select {
		case <-finish:
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	require.NoError(t, err)
	_, loop, task := detachTestRuntime(t, true, tool)
	_, done := detachAction(t, loop, task, aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "already async", "async_directly_call_tool": true})
	detachAwait(t, task.GetContext(), done)
	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	entries := manager.List()
	require.Len(t, entries, 1)
	for i := 0; i < 2; i++ {
		select {
		case id := <-ids:
			require.Equal(t, entries[0].ID, id)
		case <-task.GetContext().Done():
			t.Fatal("missing detach ID")
		}
	}
	close(finish)
	_, err = manager.Wait(task.GetContext(), entries[0].ID)
	require.NoError(t, err)
	require.Len(t, manager.List(), 1)
}

func TestAsyncDirectAnswerTargetsOwningLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var callbacks atomic.Int32
	tool, err := aitool.New("direct_answer_async_fixture", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
		callbacks.Add(1)
		return "must not execute", nil
	}))
	require.NoError(t, err)
	recorder := new(batchHardeningReviewRecorder)
	runtime := newBatchHardeningReplayRuntime(t, fmt.Sprintf("async-direct-answer-%d", time.Now().UnixNano()), 0, nil, recorder, func(int, batchHardeningReviewMaterial) string { return `{"suggestion":"direct_answer"}` }, tool)
	owner := aicommon.NewStatefulTaskBase("review-owner", "original owner", ctx, runtime.Emitter, true)
	loop := reactloops.NewMinimalReActLoop(runtime.config, runtime)
	defer loop.Release()
	loop.SetCurrentTask(owner)
	other := reactloops.NewMinimalReActLoop(runtime.config, runtime)
	defer other.Release()
	other.SetCurrentTask(aicommon.NewStatefulTaskBase("other", "other owner", ctx, runtime.Emitter, true))
	receipt, err := loop.StartAsyncToolCall(tool.Name, aitool.InvokeParams{}, aicommon.WithToolCaller_Reason("review ownership"))
	require.NoError(t, err)
	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	summary, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, summary.State)
	detachAwait(t, ctx, loop.AsyncDirectAnswerSignal())
	select {
	case <-other.AsyncDirectAnswerSignal():
		t.Fatal("review escaped its owner loop")
	default:
	}
	require.Zero(t, callbacks.Load())
	require.Empty(t, owner.GetAllToolCallResults())
}

func TestToolDefaultAsyncBatchOverrides(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			finish := make(chan struct{})
			tool, err := aitool.New("default_async_batch_fixture", aitool.WithDefaultAsync(true), aitool.WithStringParam("mode"), aitool.WithDangerousNoNeedUserReview(true), aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, _ *aitool.ToolRuntimeConfig, _, _ io.Writer) (any, error) {
				if params["mode"] == "sync" {
					return "foreground result", nil
				}
				select {
				case <-finish:
					return "background result", nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}))
			require.NoError(t, err)
			_, loop, task := detachTestRuntime(t, native, tool)
			calls := []any{
				aitool.InvokeParams{"tool_name": tool.Name, "identifier": "default", "reason": "default background", "params": aitool.InvokeParams{"mode": "async"}},
				aitool.InvokeParams{"tool_name": tool.Name, "identifier": "sync", "reason": "explicit foreground", "params": aitool.InvokeParams{"mode": "sync"}, "async": false},
				aitool.InvokeParams{"tool_name": tool.Name, "identifier": "async", "reason": "explicit background", "params": aitool.InvokeParams{"mode": "async"}, "async": true},
			}
			_, done := detachAction(t, loop, task, aitool.InvokeParams{"directly_call_tool_params_group": calls})
			detachAwait(t, task.GetContext(), done)
			manager, err := loop.GetAsyncExecutionManager()
			require.NoError(t, err)
			entries := manager.List()
			require.Len(t, entries, 2, "explicit false must override the tool's default in batches")
			require.Len(t, task.GetAllToolCallResults(), 1)
			close(finish)
			for _, entry := range entries {
				summary, err := manager.Wait(task.GetContext(), entry.ID)
				require.NoError(t, err)
				require.Equal(t, asyncexecmanager.StateCompleted, summary.State)
			}
			require.Len(t, task.GetAllToolCallResults(), 3)
		})
	}
}
