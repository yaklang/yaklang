package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentTaskTodoPayloadKeepsLegacyProjectionAndCanonicalState(t *testing.T) {
	cfg := NewConfig(context.Background())
	task := NewStatefulTaskBase("task-event", "exercise TODO event projection", context.Background(), nil, true)
	scope := BuildVerificationTodoScope(task)

	current := "todo-2"
	delta := &TodoDelta{
		CurrentSet: true,
		Current:    &current,
		Add: []TodoAdd{
			{ID: "todo-1", Text: "closed work"},
			{ID: "todo-2", Text: "current work"},
			{ID: "todo-3", Text: "later work"},
		},
		Close: []TodoClose{{
			ID: "todo-1", Outcome: TodoOutcomeResolved,
			Reason: "request returned token_used", Refs: []string{"tool-call-48"},
		}},
	}
	results := cfg.ApplyTodoDelta(scope, delta)
	for _, result := range results {
		require.True(t, result.Success, result.Reason)
	}

	payload := BuildCurrentTaskTodoListPayload(cfg, task, 7, false, TodoDeltaToOperations(delta))
	require.Equal(t, 1, payload.Stats.Pending)
	require.Equal(t, 1, payload.Stats.Doing)
	require.Equal(t, 1, payload.Stats.Done)
	require.Len(t, payload.Items, 3)
	require.Len(t, payload.OpenTodos, 2)
	require.Equal(t, "todo-2", payload.CurrentTodoID)
	require.Len(t, payload.ClosedTodos, 1)
	require.Equal(t, "request returned token_used", payload.ClosedTodos[0].Reason)
	require.Equal(t, []string{"tool-call-48"}, payload.ClosedTodos[0].Refs)

	var closedProjection VerificationTodoItem
	for _, item := range payload.Items {
		if item.ID == "todo-1" {
			closedProjection = item
		}
	}
	require.Equal(t, VerificationTodoStatusDone, closedProjection.Status)
	require.Equal(t, TodoOutcomeResolved, closedProjection.Outcome)
	require.Equal(t, payload.ClosedTodos[0].Reason, closedProjection.Reason)
	require.Equal(t, payload.ClosedTodos[0].Refs, closedProjection.Refs)
}
