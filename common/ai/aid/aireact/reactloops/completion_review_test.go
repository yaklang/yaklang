package reactloops

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func reviewedFinishAction(t *testing.T) *aicommon.Action {
	t.Helper()
	action, err := aicommon.ExtractAction(`{"@action":"finish","completion_review":{
		"goal_evidence":"The requested config value is verified in observation-1.",
		"discovery_audit":"observation-1 contains only the requested config; no additional relevant object was discovered.",
		"closure_audit":"todo-1, when present, is resolved by observation-1; no deferred blocker remains."
	}}`, "finish")
	require.NoError(t, err)
	return action
}

func requireCompletionCheckpoint(t *testing.T, loop *ReActLoop, task aicommon.AIStatefulTask) {
	t.Helper()
	op := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, reviewedFinishAction(t), op)
	require.True(t, op.IsContinued())
	require.Contains(t, op.GetFeedback().String(), "[COMPLETION REVIEW REQUIRED]")
}

func requireReviewedFinish(t *testing.T, loop *ReActLoop, task aicommon.AIStatefulTask) {
	t.Helper()
	op := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, reviewedFinishAction(t), op)
	terminated, err := op.IsTerminated()
	require.NoError(t, err)
	require.True(t, terminated)
}

func TestCompletionReviewCannotBeBypassedByRepeatedBareFinish(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	for attempt := 0; attempt < 3; attempt++ {
		op := NewActionHandlerOperator(task)
		loopAction_Finish.ActionHandler(loop, nil, op)
		require.True(t, op.IsContinued())
	}
	requireReviewedFinish(t, loop, task)
}

func TestCompletionReviewRejectsMissingOrNonStringAuditFields(t *testing.T) {
	for _, field := range []string{"goal_evidence", "discovery_audit", "closure_audit"} {
		for _, value := range []any{nil, "", " \n", true, 1, []any{"claimed"}} {
			t.Run(field+"/"+stringMustJSON(value), func(t *testing.T) {
				loop, _, _, task := newTodoGateTestLoop(t, nil)
				requireCompletionCheckpoint(t, loop, task)
				review := map[string]any{"goal_evidence": "observation-1", "discovery_audit": "todo-1", "closure_audit": "observed result"}
				review[field] = value
				raw, err := json.Marshal(map[string]any{"@action": "finish", "completion_review": review})
				require.NoError(t, err)
				action, err := aicommon.ExtractAction(string(raw), "finish")
				require.NoError(t, err)
				op := NewActionHandlerOperator(task)
				loopAction_Finish.ActionHandler(loop, action, op)
				require.True(t, op.IsContinued())
			})
		}
	}
}

func stringMustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func TestCompletionReviewInvalidatedByWorkEvenWithoutTodoDelta(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	requireCompletionCheckpoint(t, loop, task)
	// Tool output, failure, answer or clarification may reveal an untracked
	// object. The execution path calls this after every non-finish handler.
	loop.invalidateCompletionReview(task)
	requireCompletionCheckpoint(t, loop, task)
	requireReviewedFinish(t, loop, task)
}

func TestCompletionReviewInvalidatedByTodoChangeOnFinish(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	requireCompletionCheckpoint(t, loop, task)
	// Simulate additional work and a delta applied before the finish handler.
	setCurrentTodo(t, cfg, task, "todo-1")
	results := cfg.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), &aicommon.TodoDelta{
		Close: []aicommon.TodoClose{{ID: "todo-1", Outcome: aicommon.TodoOutcomeResolved, Reason: "verified observation-1", Refs: []string{"observation-1"}}},
	})
	require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))
	requireCompletionCheckpoint(t, loop, task)
	requireReviewedFinish(t, loop, task)
}

func TestCompletionReviewInvalidatedByExternalEvidence(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	requireCompletionCheckpoint(t, loop, task)
	cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{{Op: "add", ID: "new-object", Content: "observation-2 reveals an additional active configuration"}})
	requireCompletionCheckpoint(t, loop, task)
	requireReviewedFinish(t, loop, task)
}

func TestCompletionReviewIsScopedToCurrentTask(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	requireCompletionCheckpoint(t, loop, task)
	other := aicommon.NewStatefulTaskBase("other", "other input", context.Background(), cfg.GetEmitter(), true)
	loop.SetCurrentTask(other)
	requireCompletionCheckpoint(t, loop, other)
	loop.SetCurrentTask(task)
	requireReviewedFinish(t, loop, task)
}

func TestCompletionReviewInvalidTodoSidecarCannotExit(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	requireCompletionCheckpoint(t, loop, task)
	action := reviewedFinishAction(t)
	action.Set("todo_delta", map[string]any{"update": []any{map[string]any{"id": "never-added", "text": "unregistered discovery"}}})
	validateTodoDeltaBeforeActionVerifier(loop, action)
	require.NotEmpty(t, action.GetString("_todo_delta_error"))
	require.ErrorContains(t, loopAction_Finish.ActionVerifier(loop, action), "cannot finish with invalid TODO maintenance")
	op := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, action, op)
	require.True(t, op.IsContinued())
	require.Contains(t, op.GetFeedback().String(), "Ignored invalid todo_delta")
	requireCompletionCheckpoint(t, loop, task)
	requireReviewedFinish(t, loop, task)
}

func TestCompletionReviewUsesBoundedVerifierRetriesAfterCheckpoint(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	bare, err := aicommon.ExtractAction(`{"@action":"finish"}`, "finish")
	require.NoError(t, err)
	require.NoError(t, loopAction_Finish.ActionVerifier(loop, bare), "first attempt must reach the host checkpoint")
	requireCompletionCheckpoint(t, loop, task)
	require.ErrorContains(t, loopAction_Finish.ActionVerifier(loop, bare), "completion_review.goal_evidence")
	require.NoError(t, loopAction_Finish.ActionVerifier(loop, reviewedFinishAction(t)))
}
