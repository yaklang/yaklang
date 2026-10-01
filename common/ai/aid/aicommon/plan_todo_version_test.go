package aicommon

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPlanTodoVersionReconfirmationAndAtomicClosure(t *testing.T) {
	store := NewVerificationTodoStore()
	old := VerificationTodoScope{TaskID: "node", TaskIndex: "1-1", PlanVersion: "plan-v1"}
	next := old
	next.PlanVersion = "plan-v2"
	apply := func(scope VerificationTodoScope, delta *TodoDelta) string {
		return FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, delta))
	}
	require.Empty(t, apply(old, &TodoDelta{Add: []TodoAdd{{ID: "work", Text: "verify goal"}}}))
	before := store.Clone()
	focus := "work"
	require.Contains(t, apply(next, &TodoDelta{CurrentSet: true, Current: &focus}), "plan definition changed")
	require.Equal(t, before, store, "rejected stale focus must not mutate the store")
	close := TodoClose{ID: "work", Outcome: TodoOutcomeResolved, Reason: "validated", Refs: []string{"evidence-1"}}
	require.Contains(t, apply(next, &TodoDelta{Close: []TodoClose{close}}), "plan definition changed")
	require.Equal(t, before, store, "rejected stale closure must be atomic")
	require.Contains(t, store.RenderWithCurrentScope(next), "旧计划 TODO")
	require.Contains(t, store.RenderWithCurrentScope(next), "plan_version=plan-v2")
	// A text-identical update is effective when confirming a new plan version.
	require.Empty(t, apply(next, &TodoDelta{Update: []TodoUpdate{{ID: "work", Text: "verify goal"}}, Close: []TodoClose{close}}))
	open, _, closed := store.CanonicalSnapshot(next)
	require.Empty(t, open)
	require.Len(t, closed, 1)
	require.Equal(t, "plan-v2", closed[0].PlanVersion)
	restored := UnmarshalVerificationTodoStore(store.Marshal())
	require.Equal(t, store, restored)
	later := next
	later.PlanVersion = "plan-v3"
	require.Contains(t, restored.RenderWithCurrentScope(later), "历史计划记录")
	// Legacy records have no version; they remain active until reviewed, too.
	legacy := NewVerificationTodoStore()
	require.Empty(t, FormatTodoDeltaValidationError(legacy.ApplyTodoDelta(VerificationTodoScope{TaskID: "node"}, &TodoDelta{Add: []TodoAdd{{ID: "legacy", Text: "old work"}}})))
	require.Contains(t, legacy.RenderWithCurrentScope(next), "旧计划 TODO")
}
