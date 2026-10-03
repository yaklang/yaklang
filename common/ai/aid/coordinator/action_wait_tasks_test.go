package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionWaitTasks(t *testing.T) {
	f := newActionFixture(t, true)
	f.loop.Set("coordinator_observed_user_revision", uint64(999))
	f.invoke("wait_tasks", map[string]any{"task_ids": []string{"a"}, "mode": "all"}, false)
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), `"reason":"changed"`)
	f.loop.Set("coordinator_observed_user_revision", f.c.Snapshot().UserRevision)
	f.invoke("wait_tasks", map[string]any{"task_ids": []string{"missing"}}, true)
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	f.invoke("wait_tasks", map[string]any{"task_ids": []string{"a"}, "mode": "all", "timeout_seconds": 1}, false)
	require.True(t, f.c.Snapshot().Attempts["a"].Seen)
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), "A verified")
}
