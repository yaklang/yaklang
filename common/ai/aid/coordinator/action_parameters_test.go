package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorActionParametersAcceptExtraFields(t *testing.T) {
	f := newActionFixture(t, false)
	for name, params := range map[string]map[string]any{
		"create_plan":   {"plan": map[string]any{"name": "检查", "goal": "验收", "tasks": []any{map[string]any{"name": "核对", "goal": "保存证据", "identifier": "check"}}}, "plan_document": "文档"},
		"modify_plan":   {"document": "文档"},
		"submit_plan":   {},
		"wait_messages": {"timeout_seconds": 30},
		"inspect_task":  {"task_id": "a"},
		"review_task":   {"task_id": "a", "attempt_id": 1, "decision": "accept", "reason": "证据支持"},
		"retry_task":    {"task_id": "a", "attempt_id": 1, "reason": "重新核对"},
		"cancel_tasks":  {"task_ids": []string{"a"}, "reason": "停止"},
		"create_report": {"title": "报告", "document": "证据与限制"},
		"modify_report": {"document": "新正文"},
		"submit_report": {"summary": "已交付"},
	} {
		t.Run(name, func(t *testing.T) {
			params["human_readable_thought"] = "下一步"
			params["todo_delta"] = map[string]any{"current": "check"}
			params["future_business_context"] = map[string]any{"format_version": 2}
			params["plan_version"] = "legacy metadata"
			a := actionForTest(t, name, params)
			before := a.GetParams()
			h, err := f.loop.GetActionHandler(name)
			require.NoError(t, err)
			require.NoError(t, h.ActionVerifier(f.loop, a))
			require.Equal(t, before, a.GetParams(), "shared-loop metadata must remain available")
		})
	}
}

func TestCoordinatorActionParametersStillRequireValidKnownFields(t *testing.T) {
	f := newActionFixture(t, false)
	for name, params := range map[string]map[string]any{
		"missing_reason": {"task_id": "a", "attempt_id": 1, "decision": "accept"},
		"null_attempt":   {"task_id": "a", "attempt_id": nil, "decision": "accept", "reason": "核对"},
		"string_attempt": {"task_id": "a", "attempt_id": "1", "decision": "accept", "reason": "核对"},
	} {
		t.Run(name, func(t *testing.T) {
			params["future_reason"] = "cannot replace required reason"
			a := actionForTest(t, "review_task", params)
			h, err := f.loop.GetActionHandler("review_task")
			require.NoError(t, err)
			require.Error(t, h.ActionVerifier(f.loop, a))
		})
	}
	h, err := f.loop.GetActionHandler("cancel_tasks")
	require.NoError(t, err)
	require.Error(t, h.ActionVerifier(f.loop, actionForTest(t, "cancel_tasks", map[string]any{"task_ids": nil, "reason": "stop", "extra": true})))
}

func TestCoordinatorEditActionsIgnoreExtraFields(t *testing.T) {
	t.Run("plan", func(t *testing.T) {
		f := newActionFixture(t, false)
		f.invoke("create_plan", map[string]any{"plan": map[string]any{"name": "检查", "goal": "验收", "tasks": []any{map[string]any{"name": "核对", "goal": "保存证据", "identifier": "check"}}}, "plan_document": "旧正文"}, false)
		f.invoke("modify_plan", map[string]any{"document": "新正文", "human_readable_thought": "更新", "todo_delta": map[string]any{}, "extra": nil}, false)
		require.Equal(t, "新正文", f.c.Snapshot().Plan.Document)
		before := f.c.Snapshot()
		f.invoke("modify_plan", map[string]any{"extra": "not an edit"}, true)
		require.Equal(t, before, f.c.Snapshot())
	})
	t.Run("report", func(t *testing.T) {
		f := completedActionFixture(t)
		f.invoke("create_report", map[string]any{"title": "报告", "document": "旧正文"}, false)
		f.invoke("modify_report", map[string]any{"document": "新正文", "human_readable_thought": "更新", "todo_delta": map[string]any{}, "extra": nil}, false)
		require.Equal(t, "新正文", f.c.Snapshot().Report.Document)
		before := f.c.Snapshot()
		f.invoke("modify_report", map[string]any{"document": "覆盖", "document_patch": "patch", "extra": true}, true)
		require.Equal(t, before, f.c.Snapshot(), "extras must not bypass mutually exclusive edits")
	})
}
