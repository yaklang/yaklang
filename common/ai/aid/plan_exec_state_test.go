package aid

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

func newTestCoordinator(t *testing.T) *Coordinator {
	t.Helper()
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true), aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return nil, nil
	}))
	return &Coordinator{
		Config:    cfg,
		userInput: "test-user-input",
	}
}

func TestPlanAndExecProgressCounts(t *testing.T) {
	c := newTestCoordinator(t)

	root := c.generateAITaskWithName("root", "root-goal")
	sub1 := c.generateAITaskWithName("s1", "g1")
	sub2 := c.generateAITaskWithName("s2", "g2")
	sub2a := c.generateAITaskWithName("s2a", "g2a")

	sub1.ParentTask = root
	sub2.ParentTask = root
	sub2a.ParentTask = sub2
	root.Subtasks = []*AiTask{sub1, sub2}
	sub2.Subtasks = []*AiTask{sub2a}

	root.GenerateIndex()

	sub1.SetStatus(aicommon.AITaskState_Completed)
	sub2a.SetStatus(aicommon.AITaskState_Skipped)

	r := &runtime{
		RootTask: root,
		TaskLink: DFSOrderAiTask(root),
		cursor:   2,
	}
	c.runtime = r

	progress := c.buildPlanAndExecProgress(root, sub2, "executing")

	require.Equal(t, 4, progress.TotalTasks)
	require.Equal(t, 1, progress.CompletedTasks)
	require.Equal(t, 1, progress.SkippedTasks)
	require.Equal(t, 0, progress.AbortedTasks)
	require.Equal(t, 2, progress.CurrentIndex)
	require.Equal(t, sub2.Index, progress.CurrentTaskIndex)
	require.Equal(t, sub2.Name, progress.CurrentTask)
	require.Equal(t, sub2.Goal, progress.CurrentGoal)
	require.Equal(t, "executing", progress.Phase)
}

func TestPlanExecTaskTreeRecovery(t *testing.T) {
	c := newTestCoordinator(t)

	root := c.generateAITaskWithName("root", "root-goal")
	sub1 := c.generateAITaskWithName("s1", "g1")
	sub2 := c.generateAITaskWithName("s2", "g2")
	sub2a := c.generateAITaskWithName("s2a", "g2a")

	sub1.ParentTask = root
	sub2.ParentTask = root
	sub2a.ParentTask = sub2
	root.Subtasks = []*AiTask{sub1, sub2}
	sub2.Subtasks = []*AiTask{sub2a}

	root.GenerateIndex()

	sub1.TaskSummary = "done-1"
	sub1.SetStatus(aicommon.AITaskState_Completed)
	sub2a.SetStatus(aicommon.AITaskState_Processing)

	var recovered recoveredTask
	raw := utils.Jsonify(root)
	require.NoError(t, json.Unmarshal(raw, &recovered))

	recRoot := c.buildRecoveredTaskTree(&recovered, nil)
	require.NotNil(t, recRoot)
	require.Equal(t, root.Index, recRoot.Index)
	require.Len(t, recRoot.Subtasks, 2)

	recSub1 := recRoot.Subtasks[0]
	recSub2 := recRoot.Subtasks[1]
	require.NotNil(t, recSub1.AIStatefulTaskBase)
	require.NotNil(t, recSub2.AIStatefulTaskBase)
	require.Equal(t, recRoot, recSub1.ParentTask)
	require.Equal(t, recRoot, recSub2.ParentTask)

	require.Equal(t, aicommon.AITaskState_Completed, recSub1.GetStatus())
	require.Equal(t, "done-1", recSub1.TaskSummary)

	require.Len(t, recSub2.Subtasks, 1)
	recSub2a := recSub2.Subtasks[0]
	require.Equal(t, recSub2, recSub2a.ParentTask)
	require.Equal(t, aicommon.AITaskState_Processing, recSub2a.GetStatus())
}

func TestPlanExecTaskSummaryRoundTrip(t *testing.T) {
	c := newTestCoordinator(t)

	root := c.generateAITaskWithName("root", "root-goal")
	root.StatusSummary = "status-summary"
	root.TaskSummary = "task-summary"
	root.ShortSummary = "short-summary"
	root.LongSummary = "long-summary"
	root.GenerateIndex()

	var recovered recoveredTask
	raw := utils.Jsonify(root)
	require.NoError(t, json.Unmarshal(raw, &recovered))

	recRoot := c.buildRecoveredTaskTree(&recovered, nil)
	require.NotNil(t, recRoot)
	require.Equal(t, "status-summary", recRoot.StatusSummary)
	require.Equal(t, "task-summary", recRoot.TaskSummary)
	require.Equal(t, "short-summary", recRoot.ShortSummary)
	require.Equal(t, "long-summary", recRoot.LongSummary)
}

func TestPlanExecTaskSummaryLegacyFallback(t *testing.T) {
	c := newTestCoordinator(t)

	recovered := &recoveredTask{
		Index:    "1",
		Name:     "root",
		Goal:     "goal",
		Progress: string(aicommon.AITaskState_Completed),
		Summary:  "legacy-summary",
	}

	recRoot := c.buildRecoveredTaskTree(recovered, nil)
	require.NotNil(t, recRoot)
	require.Equal(t, "legacy-summary", recRoot.TaskSummary)
}

// TestMUSTPASS_TopologicalSortSubtasks_NoDeps tests sorting of tasks without any
// dependency relationships – original order should be preserved.
func TestMUSTPASS_TopologicalSortSubtasks_NoDeps(t *testing.T) {
c := newTestCoordinator(t)
a := c.generateAITaskWithName("a", "goal-a")
b := c.generateAITaskWithName("b", "goal-b")
cc := c.generateAITaskWithName("c", "goal-c")

sorted := topologicalSortSubtasks([]*AiTask{a, b, cc})
require.Len(t, sorted, 3)
require.Equal(t, "a", sorted[0].Name)
require.Equal(t, "b", sorted[1].Name)
require.Equal(t, "c", sorted[2].Name)
}

// TestMUSTPASS_TopologicalSortSubtasks_LinearChain tests that a linear chain
// a -> b -> c is correctly ordered as a, b, c.
func TestMUSTPASS_TopologicalSortSubtasks_LinearChain(t *testing.T) {
c := newTestCoordinator(t)
a := c.generateAITaskWithName("a", "goal-a")
b := c.generateAITaskWithName("b", "goal-b")
cc := c.generateAITaskWithName("c", "goal-c")
b.DependsOn = []string{"a"}
cc.DependsOn = []string{"b"}

// Input in reverse dependency order to ensure sorting actually does work.
sorted := topologicalSortSubtasks([]*AiTask{cc, b, a})
require.Len(t, sorted, 3)
require.Equal(t, "a", sorted[0].Name)
require.Equal(t, "b", sorted[1].Name)
require.Equal(t, "c", sorted[2].Name)
}

// TestMUSTPASS_TopologicalSortSubtasks_DiamondDeps tests a diamond dependency
// pattern: b and c both depend on a; d depends on both b and c.
func TestMUSTPASS_TopologicalSortSubtasks_DiamondDeps(t *testing.T) {
c := newTestCoordinator(t)
a := c.generateAITaskWithName("a", "goal-a")
b := c.generateAITaskWithName("b", "goal-b")
cc := c.generateAITaskWithName("c", "goal-c")
d := c.generateAITaskWithName("d", "goal-d")
b.DependsOn = []string{"a"}
cc.DependsOn = []string{"a"}
d.DependsOn = []string{"b", "c"}

// Provide tasks in an unordered fashion.
sorted := topologicalSortSubtasks([]*AiTask{d, cc, b, a})
require.Len(t, sorted, 4)

idxOf := func(name string) int {
for i, t := range sorted {
if t.Name == name {
return i
}
}
return -1
}
require.Less(t, idxOf("a"), idxOf("b"), "a must precede b")
require.Less(t, idxOf("a"), idxOf("c"), "a must precede c")
require.Less(t, idxOf("b"), idxOf("d"), "b must precede d")
require.Less(t, idxOf("c"), idxOf("d"), "c must precede d")
}

// TestMUSTPASS_TopologicalSortSubtasks_CycleSafe verifies that a cyclic
// dependency graph does not cause an infinite loop – all tasks are returned.
func TestMUSTPASS_TopologicalSortSubtasks_CycleSafe(t *testing.T) {
c := newTestCoordinator(t)
a := c.generateAITaskWithName("a", "goal-a")
b := c.generateAITaskWithName("b", "goal-b")
a.DependsOn = []string{"b"}
b.DependsOn = []string{"a"} // cycle

sorted := topologicalSortSubtasks([]*AiTask{a, b})
// No tasks should be silently dropped.
require.Len(t, sorted, 2)
}

// TestMUSTPASS_TopologicalSortSubtasks_ExternalDepIgnored ensures that
// dependencies that reference tasks outside the current slice are simply ignored
// and do not block the task from being scheduled.
func TestMUSTPASS_TopologicalSortSubtasks_ExternalDepIgnored(t *testing.T) {
c := newTestCoordinator(t)
a := c.generateAITaskWithName("a", "goal-a")
a.DependsOn = []string{"nonexistent"}

sorted := topologicalSortSubtasks([]*AiTask{a})
require.Len(t, sorted, 1)
require.Equal(t, "a", sorted[0].Name)
}

// TestMUSTPASS_UpdateTaskLink_RespectsDepends verifies that after updateTaskLink
// the task execution list follows the topological ordering dictated by DependsOn.
func TestMUSTPASS_UpdateTaskLink_RespectsDepends(t *testing.T) {
c := newTestCoordinator(t)

root := c.generateAITaskWithName("root", "root-goal")
taskA := c.generateAITaskWithName("a", "goal-a")
taskB := c.generateAITaskWithName("b", "goal-b")
taskC := c.generateAITaskWithName("c", "goal-c")

// b depends on a; c depends on b – linear chain in reverse input order.
taskB.DependsOn = []string{"a"}
taskC.DependsOn = []string{"b"}
root.Subtasks = []*AiTask{taskC, taskB, taskA} // deliberately reversed

root.GenerateIndex()

r := &runtime{RootTask: root}
r.updateTaskLink()

// Collect names from the linked list (skip root at position 0).
var names []string
for i := 1; i < r.TaskLink.Len(); i++ {
task, ok := r.TaskLink.Get(i)
require.True(t, ok)
names = append(names, task.Name)
}

require.Equal(t, []string{"a", "b", "c"}, names,
"tasks must appear in dependency order (a before b before c)")
}
