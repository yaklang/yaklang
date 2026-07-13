package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestSetUserCancelled_MarkIsSetAndReadable(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "input", nil, nil, true)
	require.False(t, task.IsUserCancelled(), "fresh task should not be user-cancelled")

	task.SetUserCancelled()
	assert.True(t, task.IsUserCancelled(), "SetUserCancelled should mark the task")
}

func TestCancel_CancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := NewStatefulTaskBase("task-1", "input", ctx, nil, true)

	// Before cancel, context should be alive.
	select {
	case <-task.GetContext().Done():
		t.Fatal("context should not be done before Cancel")
	default:
	}

	task.Cancel("test reason")

	// After cancel, context should be done quickly.
	select {
	case <-task.GetContext().Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("context should be done after Cancel")
	}
}

func TestCancel_RecordsReason(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "input", nil, nil, true)
	task.Cancel("user requested cancellation; do NOT retry")

	require.Equal(t, "user requested cancellation; do NOT retry", task.GetCancelReason(),
		"cancel reason should be recorded")
}

func TestCancel_FirstReasonWins(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "input", nil, nil, true)
	task.Cancel("first reason")
	task.Cancel("second reason")

	assert.Equal(t, "first reason", task.GetCancelReason(),
		"first cancel reason should not be overwritten")
}

func TestCancel_NilReasonDoesNotOverwrite(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "input", nil, nil, true)
	task.Cancel("explicit reason")
	task.Cancel() // no reason

	assert.Equal(t, "explicit reason", task.GetCancelReason(),
		"cancel without reason should not overwrite existing reason")
}

func TestSubTaskCancel_PropagatesToChildContext(t *testing.T) {
	// Simulate the sub-agent fork chain:
	//   parentTask.ctx → jobCtx → subTask.ctx
	// subTask.Cancel() should cancel subTask.ctx.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parentTask := NewStatefulTaskBase("parent", "parent input", ctx, nil, true)
	jobCtx, jobCancel := context.WithCancel(parentTask.GetContext())
	defer jobCancel()

	subTask := NewSubTaskBaseWithOptions(parentTask, "sub-1", "sub input",
		WithStatefulTaskBaseContext(jobCtx),
	)

	// Before cancel, subTask context is alive.
	require.False(t, isCtxDone(subTask.GetContext()))

	// Cancel the sub-task.
	subTask.Cancel("user cancelled sub agent")

	// subTask context should be done quickly.
	require.True(t, isCtxDoneWithin(subTask.GetContext(), 500*time.Millisecond),
		"subTask.Cancel should cancel subTask.GetContext()")

	// parentTask context should NOT be cancelled by subTask.Cancel.
	require.False(t, isCtxDoneWithin(parentTask.GetContext(), 100*time.Millisecond),
		"subTask.Cancel should not cancel parentTask context")

	// jobCtx should NOT be cancelled by subTask.Cancel (it has its own cancel).
	require.False(t, isCtxDoneWithin(jobCtx, 100*time.Millisecond),
		"subTask.Cancel should not cancel jobCtx")
}

func TestSubTaskCancel_ChildConfigContextCancels(t *testing.T) {
	// Verify the fix: passing subTask.GetContext() to WithContext means
	// subTask.Cancel() cancels the child config's context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parentTask := NewStatefulTaskBase("parent", "parent input", ctx, nil, true)
	jobCtx, jobCancel := context.WithCancel(parentTask.GetContext())
	defer jobCancel()

	subTask := NewSubTaskBaseWithOptions(parentTask, "sub-1", "sub input",
		WithStatefulTaskBaseContext(jobCtx),
	)

	// Simulate what BuildForkReactInvoker does: WithContext(subTask.GetContext()).
	// WithContext creates a child context via context.WithCancel.
	childCtx, childCancel := context.WithCancel(subTask.GetContext())
	defer childCancel()

	// Before cancel, child context is alive.
	require.False(t, isCtxDone(childCtx))

	// Cancel the sub-task (as user would do via sync event).
	subTask.Cancel("user cancelled sub agent")
	subTask.SetUserCancelled()

	// The child config's context should be cancelled because it was derived
	// from subTask.GetContext(), not from jobCtx.
	require.True(t, isCtxDoneWithin(childCtx, 500*time.Millisecond),
		"child config context (derived from subTask.GetContext) should be cancelled by subTask.Cancel")
}

// --- helpers ---

func isCtxDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func isCtxDoneWithin(ctx context.Context, timeout time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestAIChatToAICallbackType_PropagatesContext(t *testing.T) {
	// Verify that AIChatToAICallbackType injects the caller config's context
	// into the AI gateway options via aispec.WithContext, and that the
	// injected context is a descendant of the caller's context — i.e.
	// cancelling the caller context propagates to the downstream context.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := NewTestConfig(ctx)

	sawContext := make(chan context.Context, 1)
	chatFn := func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		ac := aispec.NewDefaultAIConfig()
		for _, opt := range opts {
			opt(ac)
		}
		if ac.Context != nil {
			sawContext <- ac.Context
		}
		// Block until the injected context is done.
		<-ac.Context.Done()
		return "", ac.Context.Err()
	}

	cb := AIChatToAICallbackType(chatFn)
	rsp, err := cb(cfg, NewAIRequest("ping"))
	require.NoError(t, err)
	require.NotNil(t, rsp)

	// Cancel the caller context — the downstream context should be cancelled too.
	cancel()
	drainAIResponse(t, rsp)

	select {
	case gotCtx := <-sawContext:
		// The injected context must be a descendant of the caller context,
		// so cancelling the caller context should have cancelled it too.
		assert.ErrorIs(t, gotCtx.Err(), context.Canceled,
			"downstream context should be cancelled after caller context is cancelled — context inheritance/propagation is broken")
	default:
		t.Fatal("AIChatToAICallbackType did not inject context into aispec opts")
	}
}
