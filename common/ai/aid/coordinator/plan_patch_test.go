package coordinator

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func planningFixture(t *testing.T) *Controller {
	t.Helper()
	p, err := ParsePlan(nestedPresetPlan, "# 检查计划\n先核对来源。\n", nil)
	require.NoError(t, err)
	c := New(context.Background(), &testHost{plan: p}, 1)
	c.patchDir = t.TempDir()
	t.Cleanup(c.Close)
	_, err = c.CreatePlan(context.Background(), nestedPresetPlan, p.Document)
	require.NoError(t, err)
	return c
}

func taskOperation(kind, id string, changes any) map[string]any {
	op := map[string]any{"operator": kind, "task_id": id}
	if changes != nil {
		op["changes"] = changes
	}
	return op
}

func TestPlanPhaseModifyTransactionsAndStableIDs(t *testing.T) {
	c := planningFixture(t)
	before := c.Snapshot()
	patch := "--- plan_document.md\n+++ plan_document.md\n@@ -1,2 +1,3 @@\n # 检查计划\n 先核对来源。\n+记录证据。\n"
	receipt, err := c.ModifyPlan(context.Background(), map[string]any{"document_patch": patch, "tasks_patch": []any{
		taskOperation("update", before.Plan.Tasks[0].ID, map[string]any{"subtask_identifier": "environment_check"}),
		taskOperation("update", groupID(t, before.Plan, "sources"), map[string]any{"depends_on": []string{"environment_check"}}),
	}})
	require.NoError(t, err)
	require.Equal(t, []string{"document", "tasks"}, receipt.Components)
	artifact, err := os.ReadFile(receipt.PatchArtifact)
	require.NoError(t, err)
	require.Equal(t, patch, string(artifact))
	after := c.Snapshot()
	require.Equal(t, before.Plan.Document+"记录证据。\n", after.Plan.Document)
	for i, task := range before.Plan.Tasks {
		require.Equal(t, task.ID, after.Plan.Tasks[i].ID)
	}
	require.Equal(t, before.Plan.Tasks[0].ID, after.Plan.Tasks[1].DependsOn[0])
	receipt, err = c.ModifyPlan(context.Background(), map[string]any{"document": after.Plan.Document})
	require.NoError(t, err)
	require.Equal(t, "unchanged", receipt.Status)
	require.Equal(t, after, c.Snapshot())
	// Intermediate dangling reference is fixed by a later operation in the same batch.
	_, err = c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{
		taskOperation("update", after.Plan.Tasks[1].ID, map[string]any{"depends_on": []string{"new_source"}}),
		map[string]any{"operator": "add", "task": map[string]any{"subtask_name": "新增来源", "subtask_goal": "读取新增来源", "subtask_identifier": "new_source"}},
	}})
	require.NoError(t, err)
	added := c.Snapshot().Plan.Tasks[4]
	require.NotEmpty(t, added.ID)
	_, err = c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{
		taskOperation("delete", added.ID, nil), taskOperation("update", after.Plan.Tasks[1].ID, map[string]any{"depends_on": []string{}}),
	}})
	require.NoError(t, err)
	require.Len(t, c.Snapshot().Plan.Tasks, 4)
	var replacement map[string]any
	require.NoError(t, json.Unmarshal([]byte(nestedPresetPlan), &replacement))
	_, err = c.ModifyPlan(context.Background(), map[string]any{"document": "完整新文档", "tasks": replacement})
	require.NoError(t, err)
	require.Equal(t, "完整新文档", c.Snapshot().Plan.Document)
}

func groupID(t *testing.T, p *Plan, identifier string) string {
	t.Helper()
	var root PlanNode
	require.NoError(t, json.Unmarshal(p.Tree, &root))
	id := ""
	root.walk(func(n *PlanNode) {
		if n.Identifier == identifier {
			id = n.TaskID
		}
	})
	require.NotEmpty(t, id)
	return id
}

func TestPlanPhaseModifyRejectsAndRollsBack(t *testing.T) {
	c := planningFixture(t)
	s := c.Snapshot()
	a, b := s.Plan.Tasks[0].ID, s.Plan.Tasks[1].ID
	tests := map[string]map[string]any{
		"empty": {}, "null_document": {"document": nil}, "wrong_document_type": {"document": 1}, "empty_document": {"document": " "},
		"empty_patch": {"document_patch": ""}, "document_conflict": {"document": "new", "document_patch": "patch"},
		"nil_object": {"tasks": map[string]any(nil)}, "null_tasks": {"tasks": nil}, "flat_tasks": {"tasks": []any{}}, "empty_tasks": {"tasks": map[string]any{}},
		"nil_batch": {"tasks_patch": []any(nil)}, "null_batch": {"tasks_patch": nil}, "empty_batch": {"tasks_patch": []any{}}, "wrong_batch": {"tasks_patch": "add"},
		"task_conflict": {"tasks": map[string]any{}, "tasks_patch": []any{}}, "old_version": {"plan_version": 1},
		"unknown_node":             {"tasks_patch": []any{taskOperation("update", "missing", map[string]any{"goal": "new"})}},
		"nil_dependencies":         {"tasks_patch": []any{taskOperation("update", a, map[string]any{"depends_on": []string(nil)})}},
		"nil_subtasks":             {"tasks_patch": []any{taskOperation("update", a, map[string]any{"subtasks": []any(nil)})}},
		"empty_changes":            {"tasks_patch": []any{taskOperation("update", a, map[string]any{})}},
		"identity_change":          {"tasks_patch": []any{taskOperation("update", a, map[string]any{"task_id": "other"})}},
		"state_change":             {"tasks_patch": []any{taskOperation("update", a, map[string]any{"progress": "completed"})}},
		"duplicate_identifier":     {"tasks_patch": []any{taskOperation("update", a, map[string]any{"identifier": "first"})}},
		"self_dependency":          {"tasks_patch": []any{taskOperation("update", a, map[string]any{"depends_on": []string{"scope"}})}},
		"cycle":                    {"tasks_patch": []any{taskOperation("update", a, map[string]any{"depends_on": []string{"first"}})}},
		"delete_dependency":        {"tasks_patch": []any{taskOperation("delete", a, nil)}},
		"dangling_rename":          {"tasks_patch": []any{taskOperation("update", a, map[string]any{"identifier": "renamed"})}},
		"atomic_document_rollback": {"document": "must roll back", "tasks_patch": []any{taskOperation("update", b, map[string]any{"depends_on": []string{"missing"}})}},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := c.ModifyPlan(context.Background(), input)
			require.Error(t, err)
			require.Equal(t, s, c.Snapshot())
		})
	}
	for name, patch := range map[string]string{
		"wrong_target":   "--- other.md\n+++ other.md\n@@ -1 +1 @@\n-x\n+y\n",
		"multiple_files": "--- plan_document.md\n+++ plan_document.md\n@@ -1,2 +1,2 @@\n # 检查计划\n 先核对来源。\n--- other.md\n+++ other.md\n@@ -1 +1 @@\n-x\n+y\n",
		"mismatch":       "--- plan_document.md\n+++ plan_document.md\n@@ -1 +1 @@\n-wrong\n+new\n",
		"empty_result":   "--- plan_document.md\n+++ plan_document.md\n@@ -1,2 +0,0 @@\n-# 检查计划\n-先核对来源。\n",
	} {
		t.Run(name, func(t *testing.T) {
			r, err := c.ModifyPlan(context.Background(), map[string]any{"document_patch": patch})
			require.Error(t, err)
			require.FileExists(t, r.PatchArtifact)
			require.Equal(t, s, c.Snapshot())
		})
	}
}

func TestPlanPhaseDocumentPatchExactOffsetsAndEOF(t *testing.T) {
	result, err := applyDocumentPatch("a\nb\nc\nd", "--- a/plan_document.md\n+++ b/plan_document.md\n@@ -1 +1,2 @@\n a\n+x\n@@ -4 +5 @@\n-d\n\\ No newline at end of file\n+D\n\\ No newline at end of file\n")
	require.NoError(t, err)
	require.Equal(t, "a\nx\nb\nc\nD", result)
	_, err = applyDocumentPatch("a\nb\n", "--- plan_document.md\n+++ plan_document.md\n@@ -2 +2 @@\n-a\n+A\n")
	require.Error(t, err, "no fuzzy relocation")
}

func TestPlanPhaseTaskGroupDeleteAndOrderedRefill(t *testing.T) {
	c := planningFixture(t)
	p := c.Snapshot().Plan
	g := groupID(t, p, "sources")
	_, err := c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{
		taskOperation("delete", p.Tasks[1].ID, nil), taskOperation("delete", p.Tasks[2].ID, nil),
		map[string]any{"operator": "add", "parent_task_id": g, "task": map[string]any{"name": "新来源", "goal": "核对", "identifier": "new"}},
	}})
	require.NoError(t, err)
	_, err = c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{
		taskOperation("delete", g, nil), taskOperation("update", p.Tasks[3].ID, map[string]any{"depends_on": []string{"scope"}}),
	}})
	require.NoError(t, err)
	require.Len(t, c.Snapshot().Plan.Tasks, 2)
}

func TestPlanPhaseTaskGroupUpdateAndOrderedRefill(t *testing.T) {
	c := planningFixture(t)
	g := groupID(t, c.Snapshot().Plan, "sources")
	_, err := c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{
		taskOperation("update", g, map[string]any{"subtasks": []any{}}),
		map[string]any{"operator": "add", "parent_task_id": g, "task": map[string]any{"name": "新来源", "goal": "核对", "identifier": "new"}},
	}})
	require.NoError(t, err, "temporary empty groups are valid when repaired in the same batch")
	before := c.Snapshot()
	_, err = c.ModifyPlan(context.Background(), map[string]any{"tasks_patch": []any{taskOperation("update", g, map[string]any{"subtasks": []any{}})}})
	require.Error(t, err, "a group cannot silently become an executable leaf")
	require.Equal(t, before, c.Snapshot())
}

func TestPlanPhaseDocumentPatchRejectsCoordinateOverflow(t *testing.T) {
	for _, hunk := range []string{
		"@@ -999999999999999999999999999999 +1 @@",
		"@@ -1,999999999999999999999999999999 +1 @@",
		"@@ -1 +999999999999999999999999999999 @@",
		"@@ -1 +1,999999999999999999999999999999 @@",
	} {
		_, err := applyDocumentPatch("a\n", "--- plan_document.md\n+++ plan_document.md\n"+hunk+"\n a\n")
		require.Error(t, err)
	}
}
