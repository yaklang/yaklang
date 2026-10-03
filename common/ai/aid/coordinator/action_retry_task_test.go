package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionRetryTask(t *testing.T) {
	f := newActionFixture(t, true)
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	a := awaitResult(t, f.c, "a")
	require.NoError(t, f.c.ReviewTask("a", a.ID, "accept", "original result checked"))
	args := map[string]any{"task_id": "a", "attempt_id": a.ID + 1, "reason": "recheck source e1"}
	f.invoke("retry_task", args, true)
	require.Equal(t, a.ID, f.c.Snapshot().Attempts["a"].ID)
	args["attempt_id"] = a.ID
	f.invoke("retry_task", args, false)
	next := awaitResult(t, f.c, "a")
	require.Greater(t, next.ID, a.ID)
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), "recheck source e1")
}
