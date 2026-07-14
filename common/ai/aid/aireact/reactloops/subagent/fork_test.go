package subagent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestBuildForkTaskID_StableSegment(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("parent-abc", "x", context.Background(), aicommon.NewDummyEmitter(), true)
	id := BuildForkTaskID(task, ForkJob{
		Order:      1,
		Identifier: "sql_injection",
	})
	require.Contains(t, id, "parent-abc-sub-sql_injection-")
}

func TestNormalizeForkConcurrency(t *testing.T) {
	require.Equal(t, 5, normalizeForkConcurrency(0, 8))
	require.Equal(t, 2, normalizeForkConcurrency(0, 2))
	require.Equal(t, 10, normalizeForkConcurrency(99, 20))
}

func TestForkSubTaskCompletionDoesNotCancelJobCtx(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-phase2", "scan", context.Background(), aicommon.NewDummyEmitter(), true)
	jobCtx, jobCancel := context.WithCancel(parent.GetContext())
	defer jobCancel()

	subTask := aicommon.NewSubTaskBaseWithOptions(
		parent,
		"parent-phase2-sub-cmd_injection-test",
		"category scan",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(jobCtx),
	)
	require.NotSame(t, jobCtx, subTask.GetContext())

	subTask.SetStatus(aicommon.AITaskState_Completed)

	select {
	case <-jobCtx.Done():
		t.Fatal("jobCtx must stay alive when forked sub-task completes; only defer jobCancel should end the worker scope")
	default:
	}
}

func TestNestedSubTaskUsesParentTaskId(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-scan-sql", "audit", context.Background(), nil)
	nested := newNestedSubTask(parent, "fast-context")
	require.NotNil(t, nested)
	require.Equal(t, parent.GetId(), nested.GetId())
	require.Equal(t, parent.GetId(), nested.GetIndex())
	require.NotSame(t, parent.GetContext(), nested.GetContext(), "nested scope must use a derived context")
}

func TestNestedSubTaskContextCancelledOnComplete(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-scan-sql", "audit", context.Background(), nil)
	nested := newNestedSubTask(parent, "fast-context")

	nested.SetStatus(aicommon.AITaskState_Completed)

	select {
	case <-parent.GetContext().Done():
		t.Fatal("parent task context must stay alive when nested scope completes")
	default:
	}
	select {
	case <-nested.GetContext().Done():
	default:
		t.Fatal("nested scope context should be cancelled after completion")
	}
}

// ---------------------------------------------------------------------------
// RunKeepAlive tests: verify the keep-alive ticker fires immediately, then
// periodically, and stops cleanly when the stop function is called.
// These tests use a very short keepAliveInterval override to keep the test
// fast. Since keepAliveInterval is a package-level const (15s) we cannot
// override it, so instead we verify the structural contract: immediate fire,
// stop is clean, nil is safe. The periodic-fire contract at 15s is too slow
// for a unit test; the integration tests in the reactloops package verify
// the keep-alive mechanism end-to-end with a custom ticker.
// ---------------------------------------------------------------------------

// TestRunKeepAlive_NilIsNoOp verifies that RunKeepAlive with a nil
// KeepAliveFunc returns a no-op stop function that does not panic.
func TestRunKeepAlive_NilIsNoOp(t *testing.T) {
	stop := RunKeepAlive(nil)
	require.NotNil(t, stop)
	require.NotPanics(t, func() {
		stop()
	})
}

// TestRunKeepAlive_FiresImmediately verifies that RunKeepAlive calls the
// keep-alive function once very early on start (before the first ticker
// interval), so the parent tick is refreshed from the very beginning of the
// sub-agent wait. The immediate call happens inside the spawned goroutine,
// so we allow a brief scheduling window.
func TestRunKeepAlive_FiresImmediately(t *testing.T) {
	var calls int64
	keepAlive := func() {
		atomic.AddInt64(&calls, 1)
	}

	stop := RunKeepAlive(keepAlive)
	// The immediate call happens inside the goroutine; allow a brief
	// scheduling window for it to execute.
	require.Eventually(t, func() bool {
		return atomic.LoadInt64(&calls) >= 1
	}, 100*time.Millisecond, 2*time.Millisecond,
		"keep-alive should fire immediately on start, before any ticker interval")
	stop()
}

// TestRunKeepAlive_StopHaltsCalls verifies that after the stop function
// returns, no further keep-alive calls occur. The stop function must block
// until the goroutine has fully exited.
func TestRunKeepAlive_StopHaltsCalls(t *testing.T) {
	var calls int64
	keepAlive := func() {
		atomic.AddInt64(&calls, 1)
	}

	stop := RunKeepAlive(keepAlive)
	// Let the immediate call settle.
	time.Sleep(5 * time.Millisecond)
	before := atomic.LoadInt64(&calls)

	stop()

	// Wait a bit and confirm no new calls after stop.
	time.Sleep(20 * time.Millisecond)
	after := atomic.LoadInt64(&calls)
	require.Equal(t, before, after,
		"keep-alive calls must not continue after stop()")
}
