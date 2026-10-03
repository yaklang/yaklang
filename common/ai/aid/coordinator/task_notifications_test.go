package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestCoordinatorTaskDiscoveryWakesIdleWithoutUserIntervention(t *testing.T) {
	f := newActionFixture(t, true)
	f.c.host.(*testHost).execute = func(ctx context.Context, _ Attempt) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	a := f.c.Snapshot().Attempts["a"]
	after := f.c.eventCursor()
	userRevision := f.c.Snapshot().UserRevision
	started, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- f.c.waitWhenIdle(f.task.GetContext(), after, func() { close(started) }) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("coordinator did not suspend")
	}
	f.c.taskDiscovered("a", a.ID+1)
	select {
	case <-done:
		t.Fatal("stale attempt woke the planner")
	default:
	}
	_, err = reactloops.SaveSessionEvidence(f.cfg, "discovery", "confirmed discovery before task completion")
	require.NoError(t, err)
	f.c.taskDiscovered("a", a.ID)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("discovery did not wake planner")
	}
	require.Equal(t, Running, f.c.Snapshot().Attempts["a"].State)
	require.Equal(t, userRevision, f.c.Snapshot().UserRevision)
	require.Contains(t, observationPrompt(f), "confirmed discovery")
	// Notifications before wait are retained by the cursor, not lost channels.
	require.NoError(t, f.c.waitWhenIdle(f.task.GetContext(), after, nil))
}

func TestCoordinatorIdleWaitCancellationAndActionableWork(t *testing.T) {
	f := newActionFixture(t, true)
	f.c.host.(*testHost).execute = func(ctx context.Context, _ Attempt) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	started, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- f.c.waitWhenIdle(ctx, f.c.eventCursor(), func() { close(started) }) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("idle wait not started")
	}
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	f.c.mu.Lock()
	a := f.c.state.Attempts["b"]
	a.State = AwaitingReview
	f.c.state.Attempts["b"] = a
	f.c.mu.Unlock()
	require.NoError(t, f.c.waitWhenIdle(context.Background(), f.c.eventCursor(), nil), "pending review must be handled immediately")
}

func TestCoordinatorExplicitAllWaitAlsoWakesForDiscovery(t *testing.T) {
	f := newActionFixture(t, true)
	f.c.host.(*testHost).execute = func(ctx context.Context, _ Attempt) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	result := make(chan WaitResult, 1)
	go func() {
		r, _ := f.c.WaitTasksMode(f.task.GetContext(), []string{"a"}, time.Second, "all")
		result <- r
	}()
	// Explicit wait has no on-wait callback; repeat the event until it is armed.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(time.Second)
	for {
		select {
		case r := <-result:
			require.Equal(t, "discovery", r.Reason)
			return
		case <-ticker.C:
			f.c.taskDiscovered("a", f.c.Snapshot().Attempts["a"].ID)
		case <-deadline:
			t.Fatal("all wait ignored discovery")
		}
	}
}

func TestCoordinatorWorkerDiscoveryOnlyNotifiesEvidenceChanges(t *testing.T) {
	f := newActionFixture(t, false)
	worker, err := NewWorkerLoop(f.loop.GetInvoker())
	require.NoError(t, err)
	worker.SetCurrentTask(f.task)
	calls := 0
	worker.Set("coordinator_discovery_callback", func() { calls++ })
	handler, err := worker.GetActionHandler("save_evidence")
	require.NoError(t, err)
	for _, content := range []string{"confirmed source", "confirmed source", "updated source"} {
		action := actionForTest(t, "save_evidence", map[string]any{"evidence_id": "source", "evidence_content": content})
		require.NoError(t, handler.ActionVerifier(worker, action))
		op := reactloops.NewActionHandlerOperator(f.task)
		handler.ActionHandler(worker, action, op)
		done, err := op.IsTerminated()
		require.NoError(t, err)
		require.False(t, done)
	}
	require.Equal(t, 2, calls, "re-saving identical evidence must not wake the planner")
}
