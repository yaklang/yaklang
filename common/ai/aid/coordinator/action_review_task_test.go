package coordinator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionReviewTask(t *testing.T) {
	f := newActionFixture(t, true)
	_, err := f.c.StartTasks([]string{"a"})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return f.c.Snapshot().Attempts["a"].State == AwaitingReview }, time.Second, time.Millisecond)
	args := map[string]any{"task_id": "a", "attempt_id": f.c.Snapshot().Attempts["a"].ID, "decision": "accept", "reason": "e1 confirms actual result"}
	f.flushResults()
	f.invoke("review_task", args, false)
	require.Equal(t, Accepted, f.c.Snapshot().Attempts["a"].State)
	f.cfg.Timeline.FreezeAll()
	require.Contains(t, f.cfg.GetSessionEvidenceRendered(), "e1 confirms actual result")
	f.invoke("start_tasks", map[string]any{"task_ids": []string{"b"}}, false)
}
