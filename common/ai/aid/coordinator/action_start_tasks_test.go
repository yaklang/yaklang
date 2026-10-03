package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionStartTasks(t *testing.T) {
	f := newActionFixture(t, true)
	f.invoke("start_tasks", map[string]any{"task_ids": []string{"b"}}, true)
	require.Equal(t, Pending, f.c.Snapshot().Attempts["b"].State)
	f.invoke("start_tasks", map[string]any{"task_ids": []string{"a"}}, false)
	a := awaitResult(t, f.c, "a")
	require.Equal(t, AwaitingReview, a.State)
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), `"attempt_id":1`)
	f2 := newActionFixture(t, true)
	WithPlanningOnly()(f2.loop)
	f2.invoke("start_tasks", nil, true)
	require.Equal(t, Pending, f2.c.Snapshot().Attempts["a"].State)
}
