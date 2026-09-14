package aicommon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTodoLifecycleTimestampsOnApply verifies that applyTodoDelta records
// wall-clock timestamps at the right transitions:
//   - add  -> CreatedTs set
//   - current set -> FocusStartedTs set on the focused item
//   - close -> ClosedTs set, CreatedTs and FocusStartedTs carried forward
func TestTodoLifecycleTimestampsOnApply(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task-lifecycle"}

	current := "todo-1"
	delta := &TodoDelta{
		CurrentSet: true,
		Current:    &current,
		Add:        []TodoAdd{{ID: "todo-1", Text: "work one"}},
	}
	results := store.ApplyTodoDelta(scope, delta)
	for _, r := range results {
		require.True(t, r.Success, r.Reason)
	}

	open, currentID, _ := store.CanonicalSnapshot(scope)
	require.Equal(t, "todo-1", currentID)
	require.Len(t, open, 1)
	require.NotZero(t, open[0].CreatedTs, "CreatedTs should be set on add")
	require.NotZero(t, open[0].FocusStartedTs, "FocusStartedTs should be set when made current")

	// Add a second todo and switch focus to it
	current2 := "todo-2"
	delta2 := &TodoDelta{
		CurrentSet: true,
		Current:    &current2,
		Add:        []TodoAdd{{ID: "todo-2", Text: "work two"}},
	}
	results2 := store.ApplyTodoDelta(scope, delta2)
	for _, r := range results2 {
		require.True(t, r.Success, r.Reason)
	}

	open2, currentID2, _ := store.CanonicalSnapshot(scope)
	require.Equal(t, "todo-2", currentID2)
	require.Len(t, open2, 2)

	// todo-2 should have FocusStartedTs set; todo-1 retains its old FocusStartedTs
	var todo2 *TodoOpenItem
	for i := range open2 {
		if open2[i].ID == "todo-2" {
			todo2 = &open2[i]
		}
	}
	require.NotNil(t, todo2)
	require.NotZero(t, todo2.FocusStartedTs)

	// Close todo-2 and verify ClosedTs + carried CreatedTs + FocusStartedTs
	time.Sleep(1100 * time.Millisecond) // ensure measurable gap
	delta3 := &TodoDelta{
		Close: []TodoClose{{
			ID: "todo-2", Outcome: TodoOutcomeResolved,
			Reason: "done with two", Refs: []string{"ref-1"},
		}},
	}
	results3 := store.ApplyTodoDelta(scope, delta3)
	for _, r := range results3 {
		require.True(t, r.Success, r.Reason)
	}

	_, _, closed := store.CanonicalSnapshot(scope)
	require.Len(t, closed, 1)
	require.NotZero(t, closed[0].CreatedTs, "CreatedTs carried from open item")
	require.NotZero(t, closed[0].FocusStartedTs, "FocusStartedTs carried from open item")
	require.NotZero(t, closed[0].ClosedTs, "ClosedTs set on close")
	require.GreaterOrEqual(t, closed[0].ClosedTs, closed[0].FocusStartedTs, "ClosedTs should be >= FocusStartedTs")
}

// TestTodoLifecycleCompute verifies ComputeTodoLifecycle derives the right
// durations for open DOING, open PENDING, and closed items.
func TestTodoLifecycleCompute(t *testing.T) {
	now := int64(10000)

	// DOING item: created at 8000, focused at 9000
	doing := VerificationTodoItem{
		ID: "todo-doing", Status: VerificationTodoStatusDoing,
		CreatedTs: 8000, FocusStartedTs: 9000,
	}
	lc := ComputeTodoLifecycle(doing, now)
	require.Equal(t, int64(2000), lc.AgeSeconds, "age = now - created")
	require.Equal(t, int64(1000), lc.FocusSeconds, "focus = now - focus_started")
	require.Zero(t, lc.SurvivalSeconds, "survival zero while open")

	// PENDING item: created at 5000, never focused
	pending := VerificationTodoItem{
		ID: "todo-pending", Status: VerificationTodoStatusPending,
		CreatedTs: 5000,
	}
	lc = ComputeTodoLifecycle(pending, now)
	require.Equal(t, int64(5000), lc.AgeSeconds)
	require.Zero(t, lc.FocusSeconds, "pending has no focus")
	require.Zero(t, lc.SurvivalSeconds)

	// CLOSED item: created at 3000, focused at 6000, closed at 9000
	closed := VerificationTodoItem{
		ID: "todo-closed", Status: VerificationTodoStatusDone,
		CreatedTs: 3000, FocusStartedTs: 6000, ClosedTs: 9000,
	}
	lc = ComputeTodoLifecycle(closed, now)
	require.Equal(t, int64(6000), lc.SurvivalSeconds, "survival = closed - created")
	require.Equal(t, int64(3000), lc.FocusSeconds, "focus = closed - focus_started")
	require.Equal(t, int64(7000), lc.AgeSeconds, "age = now - created")

	// CLOSED item never focused: focus zero, survival still computed
	closedNoFocus := VerificationTodoItem{
		ID: "todo-closed-nf", Status: VerificationTodoStatusDone,
		CreatedTs: 3000, ClosedTs: 9000,
	}
	lc = ComputeTodoLifecycle(closedNoFocus, now)
	require.Equal(t, int64(6000), lc.SurvivalSeconds)
	require.Zero(t, lc.FocusSeconds)
}

// TestTodoLifecycleInEmitPayload verifies that BuildCurrentTaskTodoListPayload
// populates the Lifecycles field.
func TestTodoLifecycleInEmitPayload(t *testing.T) {
	cfg := NewConfig(nil)
	task := NewStatefulTaskBase("task-emit", "emit lifecycle test", nil, nil, true)
	scope := BuildVerificationTodoScope(task)

	current := "todo-e1"
	delta := &TodoDelta{
		CurrentSet: true,
		Current:    &current,
		Add: []TodoAdd{
			{ID: "todo-e1", Text: "emit one"},
			{ID: "todo-e2", Text: "emit two"},
		},
		Close: []TodoClose{{
			ID: "todo-e2", Outcome: TodoOutcomeResolved,
			Reason: "already done", Refs: []string{"r1"},
		}},
	}
	results := cfg.ApplyTodoDelta(scope, delta)
	for _, r := range results {
		require.True(t, r.Success, r.Reason)
	}

	// Sleep so that focus duration is measurable (at least 1 second).
	time.Sleep(1100 * time.Millisecond)

	payload := BuildCurrentTaskTodoListPayload(cfg, task, 1, false, nil)
	require.NotEmpty(t, payload.Lifecycles, "Lifecycles should be populated")
	require.Len(t, payload.Lifecycles, len(payload.Items))

	// Find the doing item's lifecycle
	for i, item := range payload.Items {
		if item.Status == VerificationTodoStatusDoing {
			require.NotZero(t, payload.Lifecycles[i].FocusSeconds,
				"doing item should have non-zero focus duration")
		}
	}
}
