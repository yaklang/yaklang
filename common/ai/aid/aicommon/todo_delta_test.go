package aicommon

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestNormalizeTodoDeltaRequiresCloseReason(t *testing.T) {
	action := NewSimpleAction("finish", aitool.InvokeParams{"todo_delta": map[string]any{
		"close": []any{map[string]any{"id": "todo-1", "outcome": "resolved"}},
	}})
	_, err := NormalizeTodoDelta(action)
	require.ErrorContains(t, err, "reason is required")
}

func TestNormalizeTodoDeltaIgnoresEmptyObjectAndRejectsInvalidRefs(t *testing.T) {
	delta, err := NormalizeTodoDelta(NewSimpleAction("finish", aitool.InvokeParams{"todo_delta": map[string]any{}}))
	require.NoError(t, err)
	require.Nil(t, delta)

	// The incremental stream callback can represent an empty JSON object as an
	// empty slice before the canonical object callback finishes. Keep this
	// harmless parser shape tolerant as well.
	delta, err = NormalizeTodoDelta(NewSimpleAction("finish", aitool.InvokeParams{"todo_delta": []any{}}))
	require.NoError(t, err)
	require.Nil(t, delta)

	_, err = NormalizeTodoDelta(NewSimpleAction("finish", aitool.InvokeParams{"todo_delta": map[string]any{
		"close": []any{map[string]any{"id": "todo-1", "outcome": "dismissed", "reason": "attempted twice and both requests failed", "refs": []any{"tool-call-1", 2}}},
	}}))
	require.ErrorContains(t, err, "only strings")
}

func TestNormalizeTodoDeltaAcceptsTypedObjectArrays(t *testing.T) {
	action := NewSimpleAction("tool", aitool.InvokeParams{"todo_delta": map[string]any{
		"add": []map[string]any{{"id": "todo-1", "text": "inspect response"}},
	}})
	delta, err := NormalizeTodoDelta(action)
	require.NoError(t, err)
	require.Equal(t, []TodoAdd{{ID: "todo-1", Text: "inspect response"}}, delta.Add)
}

func TestTodoDeltaMarshalPreservesExplicitNullCurrent(t *testing.T) {
	raw, err := json.Marshal(TodoDelta{CurrentSet: true})
	require.NoError(t, err)
	require.JSONEq(t, `{"current":null}`, string(raw))
}

func TestTodoDeltaApplyOrderAndUniqueCurrent(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task-a", TaskIndex: "1"}
	current := "todo-b"
	delta := &TodoDelta{
		CurrentSet: true, Current: &current,
		Add:    []TodoAdd{{ID: "todo-a", Text: "A"}, {ID: "todo-b", Text: "B"}},
		Update: []TodoUpdate{{ID: "todo-a", Text: "A2"}},
		Close:  []TodoClose{{ID: "todo-a", Outcome: TodoOutcomeResolved, Reason: "A completed", Refs: []string{"tool-call-1"}}},
	}
	results := store.ApplyTodoDelta(scope, delta)
	for _, result := range results {
		require.True(t, result.Success, result.Reason)
	}
	open, focused, closed := store.CanonicalSnapshot(scope)
	require.Len(t, open, 1)
	require.Equal(t, "todo-b", focused)
	require.Equal(t, "A completed", closed[0].Reason)
	require.Equal(t, []string{"tool-call-1"}, closed[0].Refs)
}

func TestTodoDeltaScopesAreIsolated(t *testing.T) {
	store := NewVerificationTodoStore()
	for _, taskID := range []string{"a", "b"} {
		current := "same"
		results := store.ApplyTodoDelta(VerificationTodoScope{TaskID: taskID}, &TodoDelta{CurrentSet: true, Current: &current, Add: []TodoAdd{{ID: "same", Text: taskID}}})
		require.True(t, results[0].Success)
	}
	_, currentA, _ := store.CanonicalSnapshot(VerificationTodoScope{TaskID: "a"})
	_, currentB, _ := store.CanonicalSnapshot(VerificationTodoScope{TaskID: "b"})
	require.Equal(t, "same", currentA)
	require.Equal(t, "same", currentB)
}

func TestTodoDeltaCloseCurrentClearsFocus(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task"}
	current := "todo-1"
	results := store.ApplyTodoDelta(scope, &TodoDelta{CurrentSet: true, Current: &current, Add: []TodoAdd{{ID: "todo-1", Text: "work"}}})
	require.True(t, results[0].Success)
	results = store.ApplyTodoDelta(scope, &TodoDelta{Close: []TodoClose{{ID: "todo-1", Outcome: TodoOutcomeDeferred, Reason: "attempted the available path; external input is required to continue"}}})
	require.True(t, results[0].Success)
	open, focused, closed := store.CanonicalSnapshot(scope)
	require.Empty(t, open)
	require.Empty(t, focused)
	require.Equal(t, TodoOutcomeDeferred, closed[0].Outcome)
}

func TestTodoDeltaCurrentCanSwitchAndExplicitlyClear(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task"}
	first := "todo-1"
	results := store.ApplyTodoDelta(scope, &TodoDelta{
		CurrentSet: true, Current: &first,
		Add: []TodoAdd{{ID: "todo-1", Text: "first"}, {ID: "todo-2", Text: "second"}},
	})
	for _, result := range results {
		require.True(t, result.Success, result.Reason)
	}

	second := "todo-2"
	results = store.ApplyTodoDelta(scope, &TodoDelta{CurrentSet: true, Current: &second})
	require.True(t, results[0].Success, results[0].Reason)
	_, current, _ := store.CanonicalSnapshot(scope)
	require.Equal(t, "todo-2", current)

	results = store.ApplyTodoDelta(scope, &TodoDelta{CurrentSet: true})
	require.True(t, results[0].Success, results[0].Reason)
	_, current, _ = store.CanonicalSnapshot(scope)
	require.Empty(t, current)
}

func TestTodoDeltaPureNoOpIsAcceptedAndNormalizedAway(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task"}
	current := "todo-1"
	initial := &TodoDelta{
		CurrentSet: true,
		Current:    &current,
		Add:        []TodoAdd{{ID: "todo-1", Text: "inspect current state"}},
	}
	results := store.ApplyTodoDelta(scope, initial)
	for _, result := range results {
		require.True(t, result.Success, result.Reason)
	}

	repeated := &TodoDelta{CurrentSet: true, Current: &current}
	results = store.ApplyTodoDelta(scope, repeated)
	require.Len(t, results, 1)
	require.True(t, results[0].Success, results[0].Reason)
	require.True(t, results[0].NoOp)
	require.False(t, repeated.HasChanges(), "pure no-op must not reach event or timeline emitters")

	open, focused, closed := store.CanonicalSnapshot(scope)
	require.Len(t, open, 1)
	require.Equal(t, "todo-1", focused)
	require.Empty(t, closed)
}

func TestTodoDeltaIdenticalRepeatedAddIsIdempotent(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task"}
	first := &TodoDelta{Add: []TodoAdd{{ID: "todo-1", Text: "inspect current state"}}}
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, first)))

	repeated := &TodoDelta{Add: []TodoAdd{{ID: "todo-1", Text: "inspect current state"}}}
	results := store.ApplyTodoDelta(scope, repeated)
	require.Len(t, results, 1)
	require.True(t, results[0].Success, results[0].Reason)
	require.True(t, results[0].NoOp)
	require.False(t, repeated.HasChanges())

	open, _, _ := store.CanonicalSnapshot(scope)
	require.Len(t, open, 1)
}

func TestTodoDeltaRejectsCurrentOutsideTaskScope(t *testing.T) {
	store := NewVerificationTodoStore()
	current := "todo-other"
	results := store.ApplyTodoDelta(VerificationTodoScope{TaskID: "task-a"}, &TodoDelta{CurrentSet: true, Current: &current})
	require.Len(t, results, 1)
	require.False(t, results[0].Success)
	require.Contains(t, results[0].Reason, "current task scope")
}

func TestTodoDeltaIsAtomicAndGeneratedIDsRemainStable(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task"}
	invalid := &TodoDelta{
		Add:   []TodoAdd{{Text: "generated"}},
		Close: []TodoClose{{ID: "missing", Outcome: TodoOutcomeDismissed, Reason: "looked for the item but it does not exist in this scope"}},
	}
	results := store.ApplyTodoDelta(scope, invalid)
	require.Len(t, results, 2)
	require.False(t, results[0].Success)
	require.True(t, results[0].RolledBack)
	require.Contains(t, results[0].Reason, "DISMISSED[missing]")
	require.False(t, results[1].Success)
	require.False(t, results[1].RolledBack)
	require.Contains(t, results[1].Reason, "open todo ids: [todo-1]")
	require.Contains(t, FormatTodoDeltaValidationError(results), "DISMISSED[missing]")
	require.NotContains(t, FormatTodoDeltaValidationError(results), "rolled back")
	open, _, _ := store.CanonicalSnapshot(scope)
	require.Empty(t, open, "a rejected delta must not partially commit its add")

	valid := &TodoDelta{Add: []TodoAdd{{Text: "generated"}}}
	results = store.ApplyTodoDelta(scope, valid)
	require.True(t, results[0].Success)
	require.Equal(t, "todo-1", valid.Add[0].ID)
}

func TestSessionTodoDeltaValidationReportsConcreteFailedOperation(t *testing.T) {
	state := NewSessionPromptState()
	scope := VerificationTodoScope{TaskID: "task"}
	require.NoError(t, state.ValidateTodoDelta(scope, &TodoDelta{Add: []TodoAdd{{ID: "todo-1", Text: "work"}}}))
	state.ApplyTodoDelta(scope, &TodoDelta{Add: []TodoAdd{{ID: "todo-1", Text: "work"}}})

	current := "todo-1"
	err := state.ValidateTodoDelta(scope, &TodoDelta{
		Close:      []TodoClose{{ID: "todo-1", Outcome: TodoOutcomeResolved, Reason: "result recorded"}},
		CurrentSet: true,
		Current:    &current,
	})
	require.ErrorContains(t, err, "CURRENT[todo-1]")
	require.ErrorContains(t, err, "after add/update/close are applied")
	require.ErrorContains(t, err, "open todo ids: []")
	require.NotContains(t, err.Error(), "another operation failed")
}

func TestCanonicalTodoRestoreAdvancesGeneratedIDCounter(t *testing.T) {
	raw := `{"scopes":[{"task_id":"task","open_todos":[{"id":"todo-7","text":"existing"}],"closed_todos":[],"counter":0}]}`
	store := UnmarshalVerificationTodoStore(raw)
	delta := &TodoDelta{Add: []TodoAdd{{Text: "new"}}}
	results := store.ApplyTodoDelta(VerificationTodoScope{TaskID: "task"}, delta)
	require.True(t, results[0].Success)
	require.Equal(t, "todo-8", delta.Add[0].ID)
}

func TestLegacyTodoMigration(t *testing.T) {
	legacy, err := json.Marshal(map[string]any{"items": []VerificationTodoItem{
		{ID: "one", Content: "old", Status: VerificationTodoStatusDoing, UpdatedAt: 1, ScopeTaskID: "task"},
		{ID: "two", Content: "new", Status: VerificationTodoStatusDoing, UpdatedAt: 2, ScopeTaskID: "task"},
		{ID: "three", Content: "done", Status: VerificationTodoStatusDone, UpdatedAt: 3, ScopeTaskID: "task"},
		{ID: "four", Content: "deleted", Status: VerificationTodoStatusDeleted, UpdatedAt: 4, ScopeTaskID: "task"},
		{ID: "five", Content: "skipped", Status: VerificationTodoStatusSkipped, UpdatedAt: 5, ScopeTaskID: "task"},
	}})
	require.NoError(t, err)
	store := UnmarshalVerificationTodoStore(string(legacy))
	open, current, closed := store.CanonicalSnapshot(VerificationTodoScope{TaskID: "task"})
	require.Len(t, open, 2)
	require.Equal(t, "two", current)
	require.Equal(t, []TodoOutcome{TodoOutcomeResolved, TodoOutcomeDismissed, TodoOutcomeDeferred}, []TodoOutcome{closed[0].Outcome, closed[1].Outcome, closed[2].Outcome})
	for _, item := range closed {
		require.Contains(t, item.Reason, "older Yaklang session")
	}
}
