package coordinator

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutionEditStopsAffectedWorkerBeforeRedispatch(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	first, second := nextStart(t, h), nextStart(t, h)
	if first.Task.ID != "a" {
		first, second = second, first
	}
	_, err := c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{map[string]any{"operator": "update", "task_id": "a", "changes": map[string]any{"goal": "新的A验收目标"}}}})
	require.NoError(t, err)
	a := nextStart(t, h)
	require.Equal(t, "a", a.Task.ID)
	require.Greater(t, a.ID, first.ID)
	require.Equal(t, first.Task.Goal, h.starts["a"][0].Task.Goal, "old frozen inputs were not rewritten")
	require.Equal(t, "新的A验收目标", a.Task.Goal)
	require.Equal(t, second.ID, c.Snapshot().Attempts["b"].ID)
	require.Equal(t, Cancelled, c.Snapshot().History["a"][0].State)
	before := c.Snapshot().NextMessage
	c.taskDiscovered("a", first.ID, "late-evidence")
	require.Equal(t, before, c.Snapshot().NextMessage)
}

func TestExecutionEditPersistenceFailureDoesNotResolveCancelledWork(t *testing.T) {
	c, h := executionFixture(t, false)
	require.NoError(t, c.SubmitPlan(context.Background()))
	nextStart(t, h)
	nextStart(t, h)
	original := c.Snapshot().Plan
	c.host = &executionCommitFailure{h}
	_, err := c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{map[string]any{"operator": "update", "task_id": "a", "changes": map[string]any{"goal": "must not commit"}}}})
	require.Error(t, err)
	require.Equal(t, original, c.Snapshot().Plan)
	require.Equal(t, Failed, c.Snapshot().Attempts["a"].State)
	require.False(t, c.ReportReady())
	require.Equal(t, Running, c.Snapshot().Attempts["b"].State)
}

type executionCommitFailure struct{ *executionHost }

func (*executionCommitFailure) CommitPlan(Snapshot) error { return context.DeadlineExceeded }
