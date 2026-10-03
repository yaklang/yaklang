package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionCancelTasks(t *testing.T) {
	f := newActionFixture(t, true)
	entered := make(chan struct{})
	release := make(chan struct{})
	// Keep the worker alive after cancellation to verify the stopping boundary.
	f.c.host.(*testHost).execute = func(ctx context.Context, _ Attempt) (Result, error) {
		close(entered)
		<-ctx.Done()
		<-release
		return Result{}, ctx.Err()
	}
	t.Cleanup(func() { close(release) })
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	f.invoke("cancel_tasks", map[string]any{"task_ids": []string{"missing"}, "reason": "invalid target"}, true)
	require.Equal(t, Running, f.c.Snapshot().Attempts["a"].State)
	f.invoke("cancel_tasks", map[string]any{"task_ids": []string{"a"}, "reason": "user changed scope"}, false)
	require.Equal(t, Cancelling, f.c.Snapshot().Attempts["a"].State)
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), "user changed scope")
}
