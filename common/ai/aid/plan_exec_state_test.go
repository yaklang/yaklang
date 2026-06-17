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

func newStateTask(c *Coordinator, name string) *AiTask {
	return c.generateAITaskWithName(name, name+"-goal")
}

func TestPlanAndExecProgressCountsExecutableLeavesAndStages(t *testing.T) {
	c := newTestCoordinator(t)

	root := newStateTask(c, "root")
	sub1 := newStateTask(c, "s1")
	group := newStateTask(c, "group")
	sub2 := newStateTask(c, "s2")

	sub1.ParentTask = root
	group.ParentTask = root
	sub2.ParentTask = group
	group.Subtasks = []*AiTask{sub2}
	root.Subtasks = []*AiTask{sub1, group}

	c.standardizeTaskTree(root)

	sub1.SetStatus(aicommon.AITaskState_Completed)
	sub2.SetStatus(aicommon.AITaskState_Processing)

	graph, err := buildStrictExecutableTaskGraph(root)
	require.NoError(t, err)

	r := &runtime{
		RootTask:          root,
		execGraph:         graph,
		currentStage:      0,
		stageAnchorTaskID: sub1.Index,
		activeTaskIndexes: []string{sub2.Index},
	}
	c.runtime = r

	progress := c.buildPlanAndExecProgress(root, nil, "executing")

	require.Equal(t, 2, progress.TotalTasks)
	require.Equal(t, 1, progress.CompletedTasks)
	require.Equal(t, 0, progress.SkippedTasks)
	require.Equal(t, 0, progress.AbortedTasks)
	require.Equal(t, 1, progress.TotalStages)
	require.Equal(t, 0, progress.CompletedStages)
	require.Equal(t, 0, progress.CurrentStage)
	require.Equal(t, 2, progress.CurrentIndex)
	require.Equal(t, sub2.Index, progress.CurrentTaskIndex)
	require.Equal(t, sub2.Name, progress.CurrentTask)
	require.Equal(t, sub2.Goal, progress.CurrentGoal)
	require.Equal(t, []string{sub2.Index}, progress.ActiveTaskIndexes)
	require.Equal(t, "executing", progress.Phase)
}

func TestPlanExecTaskTreeRecoveryRoundTripPreservesDependsOnAndSemanticIdentifier(t *testing.T) {
	c := newTestCoordinator(t)

	root := newStateTask(c, "root")
	root.SetSemanticIdentifier("root-semantic")
	root.DependsOn = []string{"external-ref"}
	child := newStateTask(c, "child")
	child.ParentTask = root
	child.SetSemanticIdentifier("child-semantic")
	child.DependsOn = []string{"root-semantic"}
	root.Subtasks = []*AiTask{child}
	c.standardizeTaskTree(root)
	child.SetStatus(aicommon.AITaskState_Processing)

	var recovered recoveredTask
	raw := utils.Jsonify(root)
	require.NoError(t, json.Unmarshal(raw, &recovered))

	recRoot := c.buildRecoveredTaskTree(&recovered, nil)
	require.NotNil(t, recRoot)
	require.Equal(t, "root-semantic", recRoot.SemanticIdentifier)
	require.Equal(t, []string{"external-ref"}, recRoot.DependsOn)
	require.Len(t, recRoot.Subtasks, 1)
	require.Equal(t, "child-semantic", recRoot.Subtasks[0].SemanticIdentifier)
	require.Equal(t, []string{"root-semantic"}, recRoot.Subtasks[0].DependsOn)
	require.Equal(t, aicommon.AITaskState_Processing, recRoot.Subtasks[0].GetStatus())
}

func TestPlanExecTaskTreeRecoveryRoundTripPreservesTaskId(t *testing.T) {
	c := newTestCoordinator(t)

	root := newStateTask(c, "root")
	child := newStateTask(c, "child")
	child.ParentTask = root
	root.Subtasks = []*AiTask{child}
	c.standardizeTaskTree(root)

	rootTaskID := root.TaskId
	childTaskID := child.TaskId
	require.NotEmpty(t, rootTaskID)
	require.NotEmpty(t, childTaskID)

	var recovered recoveredTask
	raw := utils.Jsonify(root)
	require.NoError(t, json.Unmarshal(raw, &recovered))
	require.Equal(t, rootTaskID, recovered.TaskId)
	require.Equal(t, childTaskID, recovered.Subtasks[0].TaskId)

	recRoot := c.buildRecoveredTaskTree(&recovered, nil)
	require.NotNil(t, recRoot)
	c.standardizeTaskTree(recRoot)
	require.Equal(t, rootTaskID, recRoot.TaskId)
	require.Equal(t, rootTaskID, recRoot.GetId())
	require.Len(t, recRoot.Subtasks, 1)
	require.Equal(t, childTaskID, recRoot.Subtasks[0].TaskId)
	require.Equal(t, childTaskID, recRoot.Subtasks[0].GetId())
}

func TestPlanExecTaskTreeRecoveryLegacyTaskIdFallbackUsesIndex(t *testing.T) {
	c := newTestCoordinator(t)

	recovered := &recoveredTask{
		Index:    "1-2",
		Name:     "legacy-child",
		Goal:     "legacy goal",
		Progress: string(aicommon.AITaskState_Created),
	}

	recRoot := c.buildRecoveredTaskTree(recovered, nil)
	require.NotNil(t, recRoot)
	require.Equal(t, "pe-task-1-2", recRoot.TaskId)
	require.Equal(t, "pe-task-1-2", recRoot.GetId())
}

func TestPlanExecTaskSummaryRoundTrip(t *testing.T) {
	c := newTestCoordinator(t)

	root := newStateTask(c, "root")
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

func TestPlanExecTaskAbortedRoundTrip(t *testing.T) {
	c := newTestCoordinator(t)

	root := newStateTask(c, "root")
	sub := newStateTask(c, "aborted-subtask")
	sub.ParentTask = root
	root.Subtasks = []*AiTask{sub}
	root.GenerateIndex()

	sub.SetStatus(aicommon.AITaskState_Aborted)
	sub.SetSummary("aborted-summary")

	var recovered recoveredTask
	raw := utils.Jsonify(root)
	require.NoError(t, json.Unmarshal(raw, &recovered))
	require.Len(t, recovered.Subtasks, 1)
	require.Equal(t, string(aicommon.AITaskState_Aborted), recovered.Subtasks[0].Progress)

	recRoot := c.buildRecoveredTaskTree(&recovered, nil)
	require.NotNil(t, recRoot)
	require.Len(t, recRoot.Subtasks, 1)
	require.Equal(t, aicommon.AITaskState_Aborted, recRoot.Subtasks[0].GetStatus())
	require.Equal(t, "aborted-summary", recRoot.Subtasks[0].TaskSummary)
}

func TestPrepareRecoveryStartTaskResetsSelectedNodeAndLaterNodesOnly(t *testing.T) {
	c := newTestCoordinator(t)

	root := newStateTask(c, "root")
	a := newStateTask(c, "a")
	b := newStateTask(c, "b")
	cLeaf := newStateTask(c, "c")
	d := newStateTask(c, "d")

	a.ParentTask = root
	b.ParentTask = root
	cLeaf.ParentTask = root
	d.ParentTask = root
	root.Subtasks = []*AiTask{a, b, cLeaf, d}
	c.standardizeTaskTree(root)

	a.SetStatus(aicommon.AITaskState_Completed)
	b.SetStatus(aicommon.AITaskState_Completed)
	cLeaf.SetStatus(aicommon.AITaskState_Completed)
	d.SetStatus(aicommon.AITaskState_Completed)

	require.NoError(t, prepareRecoveryStartTask(root, cLeaf.Index))

	require.Equal(t, aicommon.AITaskState_Completed, a.GetStatus(), "earlier completed executable task should stay completed")
	require.Equal(t, aicommon.AITaskState_Completed, b.GetStatus(), "earlier completed executable task in the same stage should stay completed")
	require.Equal(t, aicommon.AITaskState_Created, cLeaf.GetStatus(), "selected executable task should reset")
	require.Equal(t, aicommon.AITaskState_Created, d.GetStatus(), "later executable tasks should reset")
}

func TestPrepareRecoveryStartTaskMarksEarlierIncompleteLeavesSkipped(t *testing.T) {
	c := newTestCoordinator(t)

	root := newStateTask(c, "root")
	sub1 := newStateTask(c, "s1")
	group := newStateTask(c, "group")
	sub2 := newStateTask(c, "s2")
	sub3 := newStateTask(c, "s3")

	sub1.ParentTask = root
	group.ParentTask = root
	sub2.ParentTask = group
	sub3.ParentTask = root
	group.Subtasks = []*AiTask{sub2}
	root.Subtasks = []*AiTask{sub1, group, sub3}
	c.standardizeTaskTree(root)

	require.NoError(t, prepareRecoveryStartTask(root, sub2.Index))
	require.Equal(t, aicommon.AITaskState_Skipped, sub1.GetStatus())
	require.Equal(t, aicommon.AITaskState_Created, sub2.GetStatus())
	require.Equal(t, aicommon.AITaskState_Created, sub3.GetStatus())
}

func TestPrepareRecoveryStartTaskAutoDetectsFirstUnsuccessfulExecutableNode(t *testing.T) {
	c := newTestCoordinator(t)

	root := newStateTask(c, "root")
	done := newStateTask(c, "done")
	aborted := newStateTask(c, "aborted")
	completedLater := newStateTask(c, "completed-later")
	later := newStateTask(c, "later")

	done.ParentTask = root
	aborted.ParentTask = root
	completedLater.ParentTask = root
	later.ParentTask = root
	root.Subtasks = []*AiTask{done, aborted, completedLater, later}
	c.standardizeTaskTree(root)

	done.SetStatus(aicommon.AITaskState_Completed)
	aborted.SetStatus(aicommon.AITaskState_Aborted)
	completedLater.SetStatus(aicommon.AITaskState_Completed)
	later.SetStatus(aicommon.AITaskState_Created)

	require.NoError(t, prepareRecoveryStartTask(root, ""))
	require.Equal(t, aicommon.AITaskState_Completed, done.GetStatus())
	require.Equal(t, aicommon.AITaskState_Created, aborted.GetStatus())
	require.Equal(t, aicommon.AITaskState_Created, completedLater.GetStatus(), "later executable tasks should rewind even if they were completed")
	require.Equal(t, aicommon.AITaskState_Created, later.GetStatus())
}

func TestPrepareRecoveryStartTaskNotFound(t *testing.T) {
	c := newTestCoordinator(t)
	root := newStateTask(c, "root")
	root.GenerateIndex()

	err := prepareRecoveryStartTask(root, "1-99")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}
