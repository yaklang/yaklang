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

func TestPlanExecTaskAbortedRoundTrip(t *testing.T) {
	c := newTestCoordinator(t)

	root := c.generateAITaskWithName("root", "root-goal")
	sub := c.generateAITaskWithName("aborted-subtask", "aborted-goal")
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

func TestPrepareRecoveryStartTask(t *testing.T) {
	c := newTestCoordinator(t)

	root := c.generateAITaskWithName("root", "root-goal")
	sub1 := c.generateAITaskWithName("s1", "g1")
	sub2 := c.generateAITaskWithName("s2", "g2")
	sub2a := c.generateAITaskWithName("s2a", "g2a")
	sub3 := c.generateAITaskWithName("s3", "g3")

	sub1.ParentTask = root
	sub2.ParentTask = root
	sub3.ParentTask = root
	sub2a.ParentTask = sub2
	root.Subtasks = []*AiTask{sub1, sub2, sub3}
	sub2.Subtasks = []*AiTask{sub2a}

	root.GenerateIndex()

	require.NoError(t, prepareRecoveryStartTask(root, sub2a.Index))
	require.Equal(t, aicommon.AITaskState_Skipped, sub1.GetStatus())
	require.NotEqual(t, aicommon.AITaskState_Skipped, sub2.GetStatus(), "ancestor task should stay resumable")
	require.NotEqual(t, aicommon.AITaskState_Skipped, sub2a.GetStatus(), "start task should not be skipped")
	require.NotEqual(t, aicommon.AITaskState_Skipped, sub3.GetStatus(), "tasks after start task should not be touched")
}

func TestPrepareRecoveryStartTaskNotFound(t *testing.T) {
	c := newTestCoordinator(t)
	root := c.generateAITaskWithName("root", "root-goal")
	root.GenerateIndex()

	err := prepareRecoveryStartTask(root, "1-99")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}
