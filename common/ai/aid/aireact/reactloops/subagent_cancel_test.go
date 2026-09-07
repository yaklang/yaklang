package reactloops

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
)

// captureEmitter is a BaseEmitter that captures all emitted events for assertions.
type captureEmitter struct {
	mu      atomic.Pointer[[]*schema.AiOutputEvent]
	emitCnt atomic.Int64
}

func newCaptureEmitter() *captureEmitter {
	return &captureEmitter{}
}

func (c *captureEmitter) emit(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
	c.emitCnt.Add(1)
	current := c.mu.Load()
	if current == nil {
		empty := []*schema.AiOutputEvent{}
		c.mu.Store(&empty)
		current = &empty
	}
	updated := append(*current, e)
	c.mu.Store(&updated)
	return e, nil
}

func (c *captureEmitter) Events() []*schema.AiOutputEvent {
	ptr := c.mu.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

func (c *captureEmitter) EmitCnt() int64 {
	return c.emitCnt.Load()
}

// findSyncEvent searches captured events for one whose NodeId matches.
func findSyncEvent(events []*schema.AiOutputEvent, nodeID string) *schema.AiOutputEvent {
	for _, e := range events {
		if e.NodeId == nodeID {
			return e
		}
	}
	return nil
}

// --- Tests ---

// TestBuildSubAgentRuntime_SetsAsyncMode verifies that buildSubAgentRuntime marks
// the sub-task as async mode, so CancelTask's IsAsyncMode() branch handles it.
func TestBuildSubAgentRuntime_SetsAsyncMode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ce := newCaptureEmitter()
	rootEmitter := aicommon.NewEmitter("root", ce.emit)
	parentCfg := aicommon.NewConfig(ctx,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithEmitter(rootEmitter),
	)
	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")

	parentTask := aicommon.NewStatefulTaskBase("parent-async-mode-test", "scan", ctx, rootEmitter, true)
	parentTask.SetStatus(aicommon.AITaskState_Processing)

	// Prepare a fork timeline handle
	fork, err := parentTimeline.ForkForTask("sub-async-mode", "sub", parentCfg, parentCfg)
	require.NoError(t, err)
	require.NotNil(t, fork)
	handle := &TimelineHandle{mode: SubAgentTimelineFork, fork: fork, branch: fork.Branch}

	// Mock AIRuntimeInvokerGetter
	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(childCtx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		childCfg := aicommon.NewConfig(childCtx, opts...)
		mi := mockcfg.NewMockInvoker(childCtx)
		mi.SetConfig(childCfg)
		return mi, nil
	}

	parentInvoker := mockcfg.NewMockInvoker(ctx)
	parentInvoker.SetConfig(parentCfg)

	subTask, release, err := callBuildSubAgentRuntime(t, parentInvoker, parentTask, handle)
	require.NoError(t, err)
	require.NotNil(t, subTask)
	defer release()

	assert.True(t, subTask.IsAsyncMode(),
		"sub-task created by buildSubAgentRuntime must be in async mode so CancelTask uses the deferred path")
	assert.True(t, subTask.IsSubAgent(), "sub-task must be marked as sub-agent")
}

// TestFinalizeSubAgents_TriggersAsyncDeferCallbackOnUserCancel verifies that
// finalizeSubAgents calls CallAsyncDeferCallback for a user-cancelled sub-task,
// emitting the "react_task_cancelled" sync event only after the loop exits.
func TestFinalizeSubAgents_TriggersAsyncDeferCallbackOnUserCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ce := newCaptureEmitter()
	rootEmitter := aicommon.NewEmitter("root", ce.emit)

	parentTask := aicommon.NewStatefulTaskBase("parent-cancel-defer", "scan", ctx, rootEmitter, true)
	parentTask.SetStatus(aicommon.AITaskState_Processing)

	// Create a sub-task in async mode (as buildSubAgentRuntime does)
	subTask := aicommon.NewSubTaskBaseWithOptions(
		parentTask,
		"sub-cancel-defer-test",
		"test goal",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(ctx),
	)
	subTask.SetEmitter(rootEmitter)
	subTask.SetAsyncMode(true)

	// Simulate CancelTask's async branch: register deferred callback + user-cancel
	const syncID = "test-sync-cancel-123"
	subTask.SetAsyncDeferCallback(func(err error) {
		rootEmitter.EmitSyncEvent("react_task_cancelled", map[string]interface{}{
			"task_id":      subTask.GetId(),
			"user_input":   subTask.GetUserInput(),
			"cancelled_at": time.Now(),
		}, syncID)
		subTask.SetStatus(aicommon.AITaskState_Skipped)
	})
	subTask.SetUserCancelled()
	subTask.Cancel("user requested cancellation")

	// Before finalize, the sync event must NOT have been emitted yet
	require.Nil(t, findSyncEvent(ce.Events(), "react_task_cancelled"),
		"cancel sync event must not be emitted before finalizeSubAgents runs")

	// Simulate the loop exiting (context cancelled → loop returns error)
	<-subTask.GetContext().Done()
	execErr := subTask.GetContext().Err()

	// Build a minimal ExecutedSubAgent as executeSubAgents would
	executed := &ExecutedSubAgent{
		PreparedSubAgent: &PreparedSubAgent{
			Task:      subTask,
			StartedAt: time.Now(),
			Release:   func() {},
		},
		SubLoop:  nil,
		ExecErr:  execErr,
		Duration: 100 * time.Millisecond,
	}

	// Run finalizeSubAgents
	results := finalizeSubAgents([]*ExecutedSubAgent{executed}, SubAgentOptions{})

	// After finalize, the sync event must have been emitted
	require.Len(t, results, 1)
	event := findSyncEvent(ce.Events(), "react_task_cancelled")
	require.NotNil(t, event, "cancel sync event must be emitted after finalizeSubAgents")
	assert.Equal(t, syncID, event.SyncID, "sync event must carry the original SyncID")
	assert.Equal(t, aicommon.AITaskState_Skipped, subTask.GetStatus(),
		"sub-task status must be Skipped after deferred callback fires")
}

// TestFinalizeSubAgents_NoCallbackForNormalCompletion verifies that
// finalizeSubAgents does NOT trigger CallAsyncDeferCallback when the sub-task
// was not user-cancelled (normal completion).
func TestFinalizeSubAgents_NoCallbackForNormalCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ce := newCaptureEmitter()
	rootEmitter := aicommon.NewEmitter("root", ce.emit)

	parentTask := aicommon.NewStatefulTaskBase("parent-normal", "scan", ctx, rootEmitter, true)
	parentTask.SetStatus(aicommon.AITaskState_Processing)

	subTask := aicommon.NewSubTaskBaseWithOptions(
		parentTask,
		"sub-normal-completion",
		"test goal",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(ctx),
	)
	subTask.SetEmitter(rootEmitter)
	subTask.SetAsyncMode(true)

	// Register a deferred callback that should NOT be called for normal completion
	var callbackCalled atomic.Bool
	subTask.SetAsyncDeferCallback(func(err error) {
		callbackCalled.Store(true)
	})

	// Simulate normal completion (not user-cancelled)
	subTask.SetStatus(aicommon.AITaskState_Completed)

	executed := &ExecutedSubAgent{
		PreparedSubAgent: &PreparedSubAgent{
			Task:      subTask,
			StartedAt: time.Now(),
			Release:   func() {},
		},
		SubLoop:  nil,
		ExecErr:  nil,
		Duration: 100 * time.Millisecond,
	}

	results := finalizeSubAgents([]*ExecutedSubAgent{executed}, SubAgentOptions{})

	require.Len(t, results, 1)
	assert.False(t, callbackCalled.Load(),
		"asyncDeferCallback must NOT be called when sub-task was not user-cancelled")
	assert.Equal(t, "completed", results[0].Record.Status,
		"record status must be 'completed' for normal completion")
}

// TestBuildSubAgentResult_StatusIsCancelledForUserCancelled verifies that
// BuildSubAgentResult sets status to "cancelled" (not "failed") when the sub-task
// was user-cancelled.
func TestBuildSubAgentResult_StatusIsCancelledForUserCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootEmitter := aicommon.NewDummyEmitter()
	parentTask := aicommon.NewStatefulTaskBase("parent-result-cancel", "scan", ctx, rootEmitter, true)

	subTask := aicommon.NewSubTaskBaseWithOptions(
		parentTask,
		"sub-cancel-result-test",
		"test goal",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(ctx),
	)
	subTask.SetUserCancelled()
	subTask.SetAsyncMode(true)

	executed := &ExecutedSubAgent{
		PreparedSubAgent: &PreparedSubAgent{
			Job: SubAgentJob{
				Order:      1,
				Identifier: "test-cancel-result",
				Goal:       "test goal",
				LoopName:   schema.AI_REACT_LOOP_NAME_DEFAULT,
			},
			Task:      subTask,
			StartedAt: time.Now(),
			Release:   func() {},
		},
		SubLoop:  nil,
		ExecErr:  context.Canceled,
		Duration: 50 * time.Millisecond,
	}

	result := BuildSubAgentResult(executed, SubAgentOptions{})

	require.NotNil(t, result)
	assert.Equal(t, "cancelled", result.Record.Status,
		"status must be 'cancelled' for a user-cancelled sub-task")
	assert.Empty(t, result.Record.Error,
		"error field must be cleared for a cancelled sub-task")
}

// TestBuildSubAgentResult_StatusIsFailedForNonUserError verifies that
// BuildSubAgentResult still reports "failed" for a non-user-cancelled error.
func TestBuildSubAgentResult_StatusIsFailedForNonUserError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootEmitter := aicommon.NewDummyEmitter()
	parentTask := aicommon.NewStatefulTaskBase("parent-result-fail", "scan", ctx, rootEmitter, true)

	subTask := aicommon.NewSubTaskBaseWithOptions(
		parentTask,
		"sub-fail-result-test",
		"test goal",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(ctx),
	)
	subTask.SetAsyncMode(true)

	executed := &ExecutedSubAgent{
		PreparedSubAgent: &PreparedSubAgent{
			Job: SubAgentJob{
				Order:      1,
				Identifier: "test-fail-result",
				Goal:       "test goal",
				LoopName:   schema.AI_REACT_LOOP_NAME_DEFAULT,
			},
			Task:      subTask,
			StartedAt: time.Now(),
			Release:   func() {},
		},
		SubLoop:  nil,
		ExecErr:  assert.AnError,
		Duration: 50 * time.Millisecond,
	}

	result := BuildSubAgentResult(executed, SubAgentOptions{})

	require.NotNil(t, result)
	assert.Equal(t, "failed", result.Record.Status,
		"status must be 'failed' for a non-user-cancelled error")
	assert.NotEmpty(t, result.Record.Error,
		"error field must be populated for a failed sub-task")
}

// TestCallAsyncDeferCallback_Idempotent verifies that
// CallAsyncDeferCallback only fires the callback once even if called
// multiple times.
func TestCallAsyncDeferCallback_Idempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := aicommon.NewStatefulTaskBase("idempotent-test", "test", ctx, aicommon.NewDummyEmitter(), true)

	var callCount atomic.Int64
	task.SetAsyncDeferCallback(func(err error) {
		callCount.Add(1)
	})

	// Call multiple times
	task.CallAsyncDeferCallback(nil)
	task.CallAsyncDeferCallback(nil)
	task.CallAsyncDeferCallback(nil)

	assert.Equal(t, int64(1), callCount.Load(),
		"asyncDeferCallback must only be called exactly once even with multiple CallAsyncDeferCallback invocations")
}

// TestFinalizeSubAgents_CallbackFiredOncePerTask verifies that when multiple
// sub-agents are cancelled, each gets exactly one callback fire.
func TestFinalizeSubAgents_CallbackFiredOncePerTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ce := newCaptureEmitter()
	rootEmitter := aicommon.NewEmitter("root", ce.emit)

	parentTask := aicommon.NewStatefulTaskBase("parent-multi-cancel", "scan", ctx, rootEmitter, true)
	parentTask.SetStatus(aicommon.AITaskState_Processing)

	var callbackCount atomic.Int64

	makeCancelledSub := func(id string) *ExecutedSubAgent {
		sub := aicommon.NewSubTaskBaseWithOptions(
			parentTask,
			id,
			"goal",
			aicommon.WithStatefulTaskBaseSubAgent(),
			aicommon.WithStatefulTaskBaseContext(ctx),
		)
		sub.SetEmitter(rootEmitter)
		sub.SetAsyncMode(true)
		sub.SetAsyncDeferCallback(func(err error) {
			callbackCount.Add(1)
			rootEmitter.EmitSyncEvent("react_task_cancelled", map[string]interface{}{
				"task_id": sub.GetId(),
			}, "sync-"+id)
			sub.SetStatus(aicommon.AITaskState_Skipped)
		})
		sub.SetUserCancelled()
		sub.Cancel("user cancel")

		return &ExecutedSubAgent{
			PreparedSubAgent: &PreparedSubAgent{
				Task:      sub,
				StartedAt: time.Now(),
				Release:   func() {},
			},
			SubLoop:  nil,
			ExecErr:  sub.GetContext().Err(),
			Duration: 50 * time.Millisecond,
		}
	}

	executed := []*ExecutedSubAgent{
		makeCancelledSub("sub-a"),
		makeCancelledSub("sub-b"),
		makeCancelledSub("sub-c"),
	}

	results := finalizeSubAgents(executed, SubAgentOptions{})

	require.Len(t, results, 3)
	assert.Equal(t, int64(3), callbackCount.Load(),
		"each cancelled sub-task must trigger its callback exactly once")

	// Verify each sync event was emitted
	// Verify sync events were emitted for all 3 cancelled sub-tasks
	cancelEvents := []*schema.AiOutputEvent{}
	for _, e := range ce.Events() {
		if e.NodeId == "react_task_cancelled" {
			cancelEvents = append(cancelEvents, e)
		}
	}
	assert.Len(t, cancelEvents, 3, "all 3 cancelled sub-tasks must emit a sync event")
}

// --- Helpers ---

// callBuildSubAgentRuntime wraps buildSubAgentRuntime for test access.
func callBuildSubAgentRuntime(
	t *testing.T,
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	handle *TimelineHandle,
) (aicommon.AIStatefulTask, func(), error) {
	t.Helper()
	invoker, task, release, err := buildSubAgentRuntime(
		parentInvoker,
		parentTask,
		SubAgentJob{
			Order:      1,
			Identifier: "test-async-mode",
			Goal:       "test goal",
			LoopName:   schema.AI_REACT_LOOP_NAME_DEFAULT,
		},
		handle,
		SubAgentOptions{
			TimelineMode: SubAgentTimelineFork,
		},
	)
	if err != nil {
		return nil, nil, err
	}
	_ = invoker
	return task, release, nil
}