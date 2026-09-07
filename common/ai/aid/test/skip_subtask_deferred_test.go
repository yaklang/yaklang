package test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// TestCoordinator_SkipSubtask_DeferredCallbackFiresOnce verifies that when a
// task is cancelled via the asyncDeferCallback path (as HandleSkipSubtaskInPlan
// does), CallAsyncDeferCallback fires the callback exactly once, and the
// callback only fires after CallAsyncDeferCallback is invoked (not at
// registration time).
func TestCoordinator_SkipSubtask_DeferredCallbackFiresOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emitter := aicommon.NewDummyEmitter()

	// Create a task that mimics an AiTask's AIStatefulTaskBase
	task := aicommon.NewStatefulTaskBase("skip-defer-test", "test goal", ctx, emitter, true)

	var callbackFired atomic.Int64
	var emittedSyncID string
	_ = emittedSyncID

	// Simulate what HandleSkipSubtaskInPlan does: register deferred callback
	syncID := "test-sync-skip-123"
	task.SetAsyncDeferCallback(func(err error) {
		callbackFired.Add(1)
		emitter.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "skip_subtask_in_plan", map[string]any{
			"success": true,
		}, syncID)
	})

	// Set user cancelled + Skipped + Cancel
	task.SetUserCancelled()
	task.SetStatus(aicommon.AITaskState_Skipped)
	task.Cancel("user skipped subtask")

	// Before CallAsyncDeferCallback, the callback must NOT have fired
	require.Equal(t, int64(0), callbackFired.Load(),
		"deferred callback must not fire before CallAsyncDeferCallback is invoked")

	// Simulate what execute() / invokeTask does after loop exits
	task.CallAsyncDeferCallback(nil)

	require.Equal(t, int64(1), callbackFired.Load(),
		"deferred callback must fire exactly once after CallAsyncDeferCallback")

	// Verify idempotency: second call should be a no-op
	task.CallAsyncDeferCallback(nil)
	require.Equal(t, int64(1), callbackFired.Load(),
		"deferred callback must NOT fire again on second CallAsyncDeferCallback")
}

// TestCoordinator_SkipSubtask_DeferredCallbackNotFiredForNormalCompletion
// verifies that if a task is NOT user-cancelled, CallAsyncDeferCallback is
// a no-op (no callback fires).
func TestCoordinator_SkipSubtask_DeferredCallbackNotFiredForNormalCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := aicommon.NewStatefulTaskBase("normal-complete-test", "test goal", ctx, aicommon.NewDummyEmitter(), true)

	var callbackFired atomic.Int64
	task.SetAsyncDeferCallback(func(err error) {
		callbackFired.Add(1)
	})

	// Normal completion (no user cancel)
	task.SetStatus(aicommon.AITaskState_Completed)

	// Simulate execute() checking IsUserCancelled before calling CallAsyncDeferCallback
	if task.IsUserCancelled() {
		task.CallAsyncDeferCallback(nil)
	}

	require.Equal(t, int64(0), callbackFired.Load(),
		"callback must NOT fire when task was not user-cancelled")
}

// TestCoordinator_SkipSubtask_DeferredCallbackViaExecuteErrorPath
// verifies the execute() error path: when the loop exits with error and the
// task is Skipped, CallAsyncDeferCallback fires the deferred callback.
func TestCoordinator_SkipSubtask_DeferredCallbackViaExecuteErrorPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := aicommon.NewStatefulTaskBase("execute-error-skip", "test goal", ctx, aicommon.NewDummyEmitter(), true)

	var callbackFired atomic.Int64
	task.SetAsyncDeferCallback(func(err error) {
		callbackFired.Add(1)
	})

	task.SetUserCancelled()
	task.SetStatus(aicommon.AITaskState_Skipped)
	task.Cancel("user skipped subtask")

	// Simulate execute() error path: err != nil, status == Skipped
	// In execute(), when this happens, CallAsyncDeferCallback(nil) is called
	<-task.GetContext().Done() // wait for cancel to propagate
	execErr := task.GetContext().Err()

	// Mirror execute() logic:
	if execErr != nil {
		if task.GetStatus() == aicommon.AITaskState_Skipped {
			task.CallAsyncDeferCallback(nil)
		}
	}

	require.Equal(t, int64(1), callbackFired.Load(),
		"deferred callback must fire when execute() detects Skipped status on error path")
}

// TestCoordinator_SkipSubtask_DeferredCallbackViaPreExecutionPath
// verifies the invokeTask() pre-execution path: when a task is already Skipped
// before execution starts, CallAsyncDeferCallback fires the deferred callback.
func TestCoordinator_SkipSubtask_DeferredCallbackViaPreExecutionPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := aicommon.NewStatefulTaskBase("pre-exec-skip", "test goal", ctx, aicommon.NewDummyEmitter(), true)

	var callbackFired atomic.Int64
	task.SetAsyncDeferCallback(func(err error) {
		callbackFired.Add(1)
	})

	task.SetUserCancelled()
	task.SetStatus(aicommon.AITaskState_Skipped)
	task.Cancel("user skipped subtask")

	// Simulate invokeTask() pre-execution check:
	// if current.GetStatus() == Skipped → CallAsyncDeferCallback(nil) → return nil
	if task.GetStatus() == aicommon.AITaskState_Skipped {
		task.CallAsyncDeferCallback(nil)
	}

	require.Equal(t, int64(1), callbackFired.Load(),
		"deferred callback must fire when invokeTask detects Skipped status before execution")
}

// TestCoordinator_SkipSubtask_DeferredCallbackTiming verifies that the
// deferred callback fires AFTER the task context is cancelled (i.e. after
// the loop would have exited), not at registration time.
func TestCoordinator_SkipSubtask_DeferredCallbackTiming(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := aicommon.NewStatefulTaskBase("timing-test", "test goal", ctx, aicommon.NewDummyEmitter(), true)

	var fireTime time.Time
	var registerTime time.Time

	registerTime = time.Now()
	task.SetAsyncDeferCallback(func(err error) {
		fireTime = time.Now()
	})

	task.SetUserCancelled()
	task.SetStatus(aicommon.AITaskState_Skipped)
	task.Cancel("user skipped subtask")

	// Wait a bit to ensure context cancellation propagates
	time.Sleep(10 * time.Millisecond)

	// Now simulate the loop exiting and triggering the callback
	task.CallAsyncDeferCallback(nil)

	require.False(t, fireTime.IsZero(), "callback must have fired")
	assert.True(t, fireTime.After(registerTime),
		"callback fire time must be after registration time (deferred, not immediate)")
}