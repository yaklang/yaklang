package aicommon

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

func TestVerificationTodoRenderKeepsAllItemsBelowBudget(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task", TaskIndex: "1.2"}
	current := "working"
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, &TodoDelta{
		Add: []TodoAdd{
			{ID: "done", Text: "completed target"},
			{ID: "working", Text: "current target"},
			{ID: "pending", Text: "next target"},
			{ID: "blocked", Text: "blocked target"},
		},
		CurrentSet: true, Current: &current,
	})))
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, &TodoDelta{Close: []TodoClose{
		{ID: "done", Outcome: TodoOutcomeResolved, Reason: "verified", Refs: []string{"tool-call-1"}},
		{ID: "blocked", Outcome: TodoOutcomeDeferred, Reason: "waiting for access"},
	}})))

	rendered := store.RenderWithCurrentScope(scope)
	for _, id := range []string{"working", "pending", "blocked", "done"} {
		require.Contains(t, rendered, "[id: "+id+"]")
	}
	require.Less(t, strings.Index(rendered, "[CURRENT] [id: working]"), strings.Index(rendered, "[ ] [id: pending]"))
	require.Less(t, strings.Index(rendered, "[ ] [id: pending]"), strings.Index(rendered, "[deferred] [id: blocked]"))
	require.Less(t, strings.Index(rendered, "[deferred] [id: blocked]"), strings.Index(rendered, "[resolved] [id: done]"))
	require.Contains(t, rendered, "completed target; reason: verified; refs: tool-call-1")
	require.NotContains(t, rendered, "预算不足，省略")
}

func TestVerificationTodoRenderOmitsOnlyResolvedWhenOverBudget(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task", TaskIndex: "1"}
	current := "working"
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, &TodoDelta{
		Add: []TodoAdd{
			{ID: "working", Text: "current target"},
			{ID: "pending", Text: "next target"},
			{ID: "resolved-old", Text: strings.Repeat("resolved detail ", VerificationTodoSnapshotLimit)},
			{ID: "deferred", Text: "blocked target"},
			{ID: "dismissed", Text: "excluded target"},
		},
		CurrentSet: true, Current: &current,
	})))
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, &TodoDelta{Close: []TodoClose{
		{ID: "resolved-old", Outcome: TodoOutcomeResolved, Reason: "verified"},
		{ID: "deferred", Outcome: TodoOutcomeDeferred, Reason: "waiting for access"},
		{ID: "dismissed", Outcome: TodoOutcomeDismissed, Reason: "excluded by scope"},
	}})))

	rendered := store.RenderWithCurrentScope(scope)
	require.LessOrEqual(t, ytoken.CalcTokenCount(rendered), VerificationTodoSnapshotLimit)
	for _, id := range []string{"working", "pending", "deferred", "dismissed"} {
		require.Contains(t, rendered, "[id: "+id+"]")
	}
	require.NotContains(t, rendered, "[id: resolved-old]")
	require.Contains(t, rendered, "预算不足，省略 1 项 resolved")
}

func TestVerificationTodoRenderOmitsLargestResolvedFirst(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task"}
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, &TodoDelta{Add: []TodoAdd{
		{ID: "open", Text: "work remains"},
		{ID: "short-1", Text: "completed first check"},
		{ID: "short-2", Text: "completed second check"},
		{ID: "large", Text: "archived observations"},
	}})))
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, &TodoDelta{Close: []TodoClose{
		{ID: "short-1", Outcome: TodoOutcomeResolved, Reason: "verified"},
		{ID: "short-2", Outcome: TodoOutcomeResolved, Reason: "verified"},
		{ID: "large", Outcome: TodoOutcomeResolved, Reason: strings.Repeat("historical record ", VerificationTodoSnapshotLimit)},
	}})))

	rendered := store.RenderWithCurrentScope(scope)
	require.LessOrEqual(t, ytoken.CalcTokenCount(rendered), VerificationTodoSnapshotLimit)
	require.Contains(t, rendered, "[id: open]")
	require.Contains(t, rendered, "[id: short-1]")
	require.Contains(t, rendered, "[id: short-2]")
	require.NotContains(t, rendered, "[id: large]")
	require.Contains(t, rendered, "预算不足，省略 1 项 resolved")
}

func TestVerificationTodoRenderHardTruncatesOnlyAfterResolvedAreGone(t *testing.T) {
	store := NewVerificationTodoStore()
	scope := VerificationTodoScope{TaskID: "task"}
	current := "working"
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, &TodoDelta{
		Add: []TodoAdd{
			{ID: "working", Text: strings.Repeat("open detail ", VerificationTodoSnapshotLimit*2)},
			{ID: "resolved", Text: "completed target"},
		},
		CurrentSet: true, Current: &current,
	})))
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(scope, &TodoDelta{Close: []TodoClose{
		{ID: "resolved", Outcome: TodoOutcomeResolved, Reason: "verified"},
	}})))

	rendered := store.RenderWithCurrentScope(scope)
	require.LessOrEqual(t, ytoken.CalcTokenCount(rendered), VerificationTodoSnapshotLimit)
	require.Contains(t, rendered, "[CURRENT] [id: working]")
	require.Contains(t, rendered, "预算不足，省略 1 项 resolved")
	require.NotContains(t, rendered, "[id: resolved]")
	require.True(t, strings.HasSuffix(rendered, "..."), "oversized unresolved text should be hard-truncated")
}

func TestVerificationTodoRenderEmptyStateIsExplicit(t *testing.T) {
	scope := VerificationTodoScope{TaskID: "task", TaskIndex: "1"}
	rendered := NewSessionPromptState().GetVerificationTodoRendered(scope)
	require.Contains(t, rendered, "CURRENT TASK [task_index=1, task_id=task]")
	require.Contains(t, rendered, "空清单不代表任务完成")
}

func TestVerificationTodoRenderEmptyCurrentScopeKeepsOtherTasksVisible(t *testing.T) {
	store := NewVerificationTodoStore()
	other := VerificationTodoScope{TaskID: "other", TaskIndex: "2"}
	require.Empty(t, FormatTodoDeltaValidationError(store.ApplyTodoDelta(other, &TodoDelta{
		Add: []TodoAdd{{ID: "other-work", Text: "still in progress"}},
	})))

	rendered := store.RenderWithCurrentScope(VerificationTodoScope{TaskID: "current", TaskIndex: "1"})
	require.Contains(t, rendered, "当前任务无 TODO；空清单不代表任务完成")
	require.Contains(t, rendered, "OTHER TASK (read-only) [task_index=2, task_id=other]")
	require.Contains(t, rendered, "[id: other-work]")
}
