package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/schema"
)

func newTestCoordinatorForDelta(t *testing.T) *coordinator_legacy.Coordinator {
	t.Helper()
	ctx := context.Background()
	c, err := coordinator_legacy.NewCoordinatorContext(
		ctx,
		"delta-test",
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := config.NewAIResponse()
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	return c
}

func buildTaskTree(c *coordinator_legacy.Coordinator, names ...string) (*coordinator_legacy.AiTask, []*coordinator_legacy.AiTask) {
	root := &coordinator_legacy.AiTask{
		Coordinator: c,
		Name:        "Root",
		Goal:        "root goal",
	}
	root.AIStatefulTaskBase = aicommon.NewStatefulTaskBase("root", "root goal", context.Background(), nil)

	for _, name := range names {
		sub := &coordinator_legacy.AiTask{
			Coordinator: c,
			Name:        name,
			Goal:        fmt.Sprintf("goal of %s", name),
			ParentTask:  root,
		}
		sub.AIStatefulTaskBase = aicommon.NewStatefulTaskBase("task-"+name, sub.Goal, context.Background(), nil)
		root.Subtasks = append(root.Subtasks, sub)
	}
	root.GenerateIndex()
	return root, root.Subtasks
}

func getSubtaskNames(root *coordinator_legacy.AiTask) []string {
	var names []string
	for _, s := range root.Subtasks {
		names = append(names, s.Name)
	}
	return names
}

func TestApplyTaskDeltas_InsertAfter(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C")

	currentTask := subs[0] // task-A (1-1), simulating it just completed

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op:           coordinator_legacy.TaskDeltaInsertAfter,
			RefTaskIndex: "1-1",
			Tasks: []coordinator_legacy.TaskDeltaNewTask{
				{SubtaskName: "task-NEW", SubtaskGoal: "new goal"},
			},
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-NEW", "task-B", "task-C"}, names)

	// Verify indices were recalculated
	assert.Equal(t, "1-1", root.Subtasks[0].Index)
	assert.Equal(t, "1-2", root.Subtasks[1].Index)
	assert.Equal(t, "1-3", root.Subtasks[2].Index)
	assert.Equal(t, "1-4", root.Subtasks[3].Index)
}

func TestApplyTaskDeltas_InsertAfterMultipleTasks(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op:           coordinator_legacy.TaskDeltaInsertAfter,
			RefTaskIndex: "1-1",
			Tasks: []coordinator_legacy.TaskDeltaNewTask{
				{SubtaskName: "task-NEW1", SubtaskGoal: "new goal 1"},
				{SubtaskName: "task-NEW2", SubtaskGoal: "new goal 2"},
			},
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-NEW1", "task-NEW2", "task-B", "task-C"}, names)
}

func TestApplyTaskDeltas_Append(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op: coordinator_legacy.TaskDeltaAppend,
			Tasks: []coordinator_legacy.TaskDeltaNewTask{
				{SubtaskName: "task-APPENDED", SubtaskGoal: "appended goal"},
			},
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-B", "task-APPENDED"}, names)
	assert.Equal(t, "1-3", root.Subtasks[2].Index)
}

func TestApplyTaskDeltas_Remove(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C", "task-D")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op:           coordinator_legacy.TaskDeltaRemove,
			RefTaskIndex: "1-3", // task-C
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-B", "task-D"}, names)
	assert.Equal(t, "1-3", root.Subtasks[2].Index) // task-D is now 1-3
}

func TestApplyTaskDeltas_RemoveCannotRemoveCurrentOrCompleted(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C")

	currentTask := subs[0]

	// Try to remove current task (1-1) -- should be skipped
	deltas := []coordinator_legacy.TaskDelta{
		{
			Op:           coordinator_legacy.TaskDeltaRemove,
			RefTaskIndex: "1-1",
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-B", "task-C"}, names) // unchanged
}

func TestApplyTaskDeltas_Modify(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op:           coordinator_legacy.TaskDeltaModify,
			RefTaskIndex: "1-2",
			UpdatedName:  "task-B-modified",
			UpdatedGoal:  "modified goal",
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	assert.Equal(t, "task-B-modified", root.Subtasks[1].Name)
	assert.Equal(t, "modified goal", root.Subtasks[1].Goal)
}

func TestApplyTaskDeltas_ModifyGoalOnly(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op:           coordinator_legacy.TaskDeltaModify,
			RefTaskIndex: "1-3",
			UpdatedGoal:  "new goal for C",
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	assert.Equal(t, "task-C", root.Subtasks[2].Name) // unchanged
	assert.Equal(t, "new goal for C", root.Subtasks[2].Goal)
}

func TestApplyTaskDeltas_ModifyCannotModifyCurrent(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op:           coordinator_legacy.TaskDeltaModify,
			RefTaskIndex: "1-1",
			UpdatedGoal:  "should not change",
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	assert.Equal(t, "goal of task-A", root.Subtasks[0].Goal) // unchanged
}

func TestApplyTaskDeltas_ReplaceAll(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C", "task-D")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op: coordinator_legacy.TaskDeltaReplaceAll,
			Tasks: []coordinator_legacy.TaskDeltaNewTask{
				{SubtaskName: "task-X", SubtaskGoal: "goal X"},
				{SubtaskName: "task-Y", SubtaskGoal: "goal Y"},
			},
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-X", "task-Y"}, names)
	assert.Equal(t, "1-1", root.Subtasks[0].Index)
	assert.Equal(t, "1-2", root.Subtasks[1].Index)
	assert.Equal(t, "1-3", root.Subtasks[2].Index)
}

func TestApplyTaskDeltas_ReplaceAllOverridesOtherOps(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{Op: coordinator_legacy.TaskDeltaRemove, RefTaskIndex: "1-2"},
		{Op: coordinator_legacy.TaskDeltaAppend, Tasks: []coordinator_legacy.TaskDeltaNewTask{{SubtaskName: "ignored", SubtaskGoal: "ignored"}}},
		{Op: coordinator_legacy.TaskDeltaReplaceAll, Tasks: []coordinator_legacy.TaskDeltaNewTask{{SubtaskName: "only-this", SubtaskGoal: "only goal"}}},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "only-this"}, names)
}

func TestApplyTaskDeltas_MixedOperations(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C", "task-D", "task-E")
	// indices: 1-1, 1-2, 1-3, 1-4, 1-5

	currentTask := subs[0] // task-A

	deltas := []coordinator_legacy.TaskDelta{
		{Op: coordinator_legacy.TaskDeltaRemove, RefTaskIndex: "1-3"},                                                                                                             // remove task-C
		{Op: coordinator_legacy.TaskDeltaModify, RefTaskIndex: "1-4", UpdatedName: "task-D-mod", UpdatedGoal: "modified D goal"},                                                  // modify task-D
		{Op: coordinator_legacy.TaskDeltaInsertAfter, RefTaskIndex: "1-2", Tasks: []coordinator_legacy.TaskDeltaNewTask{{SubtaskName: "task-INS", SubtaskGoal: "inserted goal"}}}, // insert after task-B
		{Op: coordinator_legacy.TaskDeltaAppend, Tasks: []coordinator_legacy.TaskDeltaNewTask{{SubtaskName: "task-END", SubtaskGoal: "appended goal"}}},                           // append at end
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	// After remove 1-3 (task-C): [task-A, task-B, task-D, task-E]
	// After modify 1-4 (task-D -> task-D-mod): [task-A, task-B, task-D-mod, task-E]
	// After insert_after 1-2 (task-B): [task-A, task-B, task-INS, task-D-mod, task-E]
	// After append: [task-A, task-B, task-INS, task-D-mod, task-E, task-END]
	assert.Equal(t, []string{"task-A", "task-B", "task-INS", "task-D-mod", "task-E", "task-END"}, names)

	// Verify task-D was modified
	for _, s := range root.Subtasks {
		if s.Name == "task-D-mod" {
			assert.Equal(t, "modified D goal", s.Goal)
			break
		}
	}
}

func TestApplyTaskDeltas_RemoveMultiple(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B", "task-C", "task-D", "task-E")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{Op: coordinator_legacy.TaskDeltaRemove, RefTaskIndex: "1-3"}, // task-C
		{Op: coordinator_legacy.TaskDeltaRemove, RefTaskIndex: "1-5"}, // task-E
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-B", "task-D"}, names)
}

func TestApplyTaskDeltas_InsertAfterNonexistentIndex(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B")

	currentTask := subs[0]

	deltas := []coordinator_legacy.TaskDelta{
		{
			Op:           coordinator_legacy.TaskDeltaInsertAfter,
			RefTaskIndex: "1-99",
			Tasks:        []coordinator_legacy.TaskDeltaNewTask{{SubtaskName: "task-X", SubtaskGoal: "goal"}},
		},
	}

	err := currentTask.ApplyTaskDeltas(deltas)
	require.NoError(t, err)

	// Should skip the invalid index and leave tree unchanged
	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-B"}, names)
}

func TestApplyTaskDeltas_EmptyDeltas(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	root, subs := buildTaskTree(c, "task-A", "task-B")

	currentTask := subs[0]

	err := currentTask.ApplyTaskDeltas([]coordinator_legacy.TaskDelta{})
	require.NoError(t, err)

	names := getSubtaskNames(root)
	assert.Equal(t, []string{"task-A", "task-B"}, names) // unchanged
}

func TestApplyTaskDeltas_NoParent(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	task := &coordinator_legacy.AiTask{
		Coordinator: c,
		Name:        "lonely",
	}
	task.AIStatefulTaskBase = aicommon.NewStatefulTaskBase("lonely", "", context.Background(), nil)

	err := task.ApplyTaskDeltas([]coordinator_legacy.TaskDelta{
		{Op: coordinator_legacy.TaskDeltaAppend, Tasks: []coordinator_legacy.TaskDeltaNewTask{{SubtaskName: "x", SubtaskGoal: "y"}}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no parent")
}

func TestParseTaskDeltas(t *testing.T) {
	params := aitool.InvokeParams{
		"suggestion": "adjust_plan",
		"reason":     "test reason",
		"task_deltas": []interface{}{
			map[string]interface{}{
				"op":             "insert_after",
				"ref_task_index": "1-2",
				"tasks": []interface{}{
					map[string]interface{}{
						"subtask_name": "new-task",
						"subtask_goal": "new-goal",
					},
				},
			},
			map[string]interface{}{
				"op":             "remove",
				"ref_task_index": "1-3",
			},
			map[string]interface{}{
				"op":             "modify",
				"ref_task_index": "1-4",
				"updated_name":   "modified-name",
				"updated_goal":   "modified-goal",
			},
		},
	}

	deltas := coordinator_legacy.ParseTaskDeltas(params)
	require.Len(t, deltas, 3)

	assert.Equal(t, coordinator_legacy.TaskDeltaInsertAfter, deltas[0].Op)
	assert.Equal(t, "1-2", deltas[0].RefTaskIndex)
	require.Len(t, deltas[0].Tasks, 1)
	assert.Equal(t, "new-task", deltas[0].Tasks[0].SubtaskName)

	assert.Equal(t, coordinator_legacy.TaskDeltaRemove, deltas[1].Op)
	assert.Equal(t, "1-3", deltas[1].RefTaskIndex)

	assert.Equal(t, coordinator_legacy.TaskDeltaModify, deltas[2].Op)
	assert.Equal(t, "modified-name", deltas[2].UpdatedName)
	assert.Equal(t, "modified-goal", deltas[2].UpdatedGoal)
}

func TestParseTaskDeltas_Empty(t *testing.T) {
	params := aitool.InvokeParams{
		"suggestion": "continue",
		"reason":     "all good",
	}

	deltas := coordinator_legacy.ParseTaskDeltas(params)
	assert.Nil(t, deltas)
}

func TestGetPendingSiblingTasksInfo(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	_, subs := buildTaskTree(c, "task-A", "task-B", "task-C")

	currentTask := subs[0]
	info := currentTask.GetPendingSiblingTasksInfo()

	assert.Contains(t, info, "task-B")
	assert.Contains(t, info, "task-C")
	assert.Contains(t, info, "1-2")
	assert.Contains(t, info, "1-3")
	assert.NotContains(t, info, "task-A")
}

func TestGetPendingSiblingTasksInfo_NoPending(t *testing.T) {
	c := newTestCoordinatorForDelta(t)
	_, subs := buildTaskTree(c, "task-A")

	currentTask := subs[0]
	info := currentTask.GetPendingSiblingTasksInfo()

	assert.Equal(t, "(no pending tasks)", info)
}
