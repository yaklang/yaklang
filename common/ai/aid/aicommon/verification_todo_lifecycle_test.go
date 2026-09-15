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

// TestTodoLifecycleDurations verifies that EnrichItemsLifecycle fills the
// right duration fields for DOING, PENDING, and closed items.
func TestTodoLifecycleDurations(t *testing.T) {
	now := int64(10000)

	// DOING item: created at 8000, focused at 9000
	items := []VerificationTodoItem{{
		ID: "todo-doing", Status: VerificationTodoStatusDoing,
		CreatedTs: 8000, FocusStartedTs: 9000,
	}}
	EnrichItemsLifecycle(items, now)
	require.Equal(t, int64(2000), items[0].AgeSeconds, "age = now - created")
	require.Equal(t, int64(1000), items[0].FocusSeconds, "focus = now - focus_started")
	require.Zero(t, items[0].SurvivalSeconds, "survival zero while open")

	// PENDING item: created at 5000, never focused
	items = []VerificationTodoItem{{
		ID: "todo-pending", Status: VerificationTodoStatusPending,
		CreatedTs: 5000,
	}}
	EnrichItemsLifecycle(items, now)
	require.Equal(t, int64(5000), items[0].AgeSeconds)
	require.Zero(t, items[0].FocusSeconds, "pending has no focus")
	require.Zero(t, items[0].SurvivalSeconds)

	// CLOSED item: created at 3000, focused at 6000, closed at 9000
	items = []VerificationTodoItem{{
		ID: "todo-closed", Status: VerificationTodoStatusDone,
		CreatedTs: 3000, FocusStartedTs: 6000, ClosedTs: 9000,
	}}
	EnrichItemsLifecycle(items, now)
	require.Equal(t, int64(6000), items[0].SurvivalSeconds, "survival = closed - created")
	require.Equal(t, int64(3000), items[0].FocusSeconds, "focus = closed - focus_started")
	require.Equal(t, int64(7000), items[0].AgeSeconds, "age = now - created")

	// CLOSED item never focused: focus zero, survival still computed
	items = []VerificationTodoItem{{
		ID: "todo-closed-nf", Status: VerificationTodoStatusDone,
		CreatedTs: 3000, ClosedTs: 9000,
	}}
	EnrichItemsLifecycle(items, now)
	require.Equal(t, int64(6000), items[0].SurvivalSeconds)
	require.Zero(t, items[0].FocusSeconds)
}

// TestTodoLifecycleInEmitPayload verifies that BuildCurrentTaskTodoListPayload
// fills lifecycle durations directly onto each item.
func TestTodoLifecycleInEmitPayload(t *testing.T) {
	cfg := NewConfig(nil)
	task := NewStatefulTaskBase("task-emit", "emit lifecycle test", nil, nil, true)
	scope := BuildVerificationTodoScope(task)

	current := "todo-e1"
	deltaAdd := &TodoDelta{
		CurrentSet: true,
		Current:    &current,
		Add: []TodoAdd{
			{ID: "todo-e1", Text: "emit one"},
			{ID: "todo-e2", Text: "emit two"},
		},
	}
	for _, r := range cfg.ApplyTodoDelta(scope, deltaAdd) {
		require.True(t, r.Success, r.Reason)
	}

	// Sleep so that survival and focus durations are measurable (at least 1 second).
	time.Sleep(1100 * time.Millisecond)

	deltaClose := &TodoDelta{
		Close: []TodoClose{{
			ID: "todo-e2", Outcome: TodoOutcomeResolved,
			Reason: "already done", Refs: []string{"r1"},
		}},
	}
	for _, r := range cfg.ApplyTodoDelta(scope, deltaClose) {
		require.True(t, r.Success, r.Reason)
	}

	payload := BuildCurrentTaskTodoListPayload(cfg, task, 1, false, nil)
	require.NotEmpty(t, payload.Items)

	// The doing item should have non-zero focus_seconds directly on itself
	for _, item := range payload.Items {
		if item.Status == VerificationTodoStatusDoing {
			require.NotZero(t, item.FocusSeconds,
				"doing item should have non-zero focus_seconds")
			require.NotZero(t, item.AgeSeconds,
				"doing item should have non-zero age_seconds")
		}
		if item.Status == VerificationTodoStatusDone {
			require.NotZero(t, item.SurvivalSeconds,
				"done item should have non-zero survival_seconds")
		}
	}

	// open_todos should also have durations filled
	for _, open := range payload.OpenTodos {
		require.NotZero(t, open.AgeSeconds, "open todo should have age_seconds")
		if open.ID == payload.CurrentTodoID {
			require.NotZero(t, open.FocusSeconds, "current open todo should have focus_seconds")
		}
	}

	// closed_todos should also have durations filled
	for _, closed := range payload.ClosedTodos {
		require.NotZero(t, closed.SurvivalSeconds, "closed todo should have survival_seconds")
		require.NotZero(t, closed.AgeSeconds, "closed todo should have age_seconds")
	}
}
