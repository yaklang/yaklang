package coordinator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestNativeRegistryProjectsOwnerDAGAndActualWorkerTimeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parent := &aicommon.Config{Id: "registry-parent"}
	p, err := ParsePlan(`{"task_id":"root","name":"Plan","goal":"Check","subtasks":[{"task_id":"group","name":"Sources","goal":"Read","semantic_identifier":"sources","subtasks":[{"task_id":"a","name":"A","goal":"Read A","semantic_identifier":"a"}]},{"task_id":"b","name":"B","goal":"Compare","depends_on":["sources"]}]}`, "Document", nil)
	require.NoError(t, err)
	c := New(ctx, &testHost{}, 1)
	t.Cleanup(c.Close)
	require.NoError(t, c.LoadApproved(p))
	a := c.state.Attempts["a"]
	a.State, a.Result.Summary = Accepted, "A verified"
	c.state.Attempts["a"] = a
	b := c.state.Attempts["b"]
	b.State = Running
	c.state.Attempts["b"] = b
	workerTimeline := aicommon.NewTimeline(nil, nil)
	workerTimeline.PushText(1, "ACTUAL_WORKER_OUTPUT")
	s := &Session{Config: &aicommon.Config{Id: "registry-child", Timeline: aicommon.NewTimeline(nil, nil)}, controller: c, parent: parent, parentTaskID: "outer-task", workerTimelines: map[string]*aicommon.Timeline{"b": workerTimeline}}
	runningSessions.Store(s.Id, s)
	t.Cleanup(func() { runningSessions.Delete(s.Id) })
	require.Empty(t, CollectPlanExecutionSnapshots("unrelated-session", nil))
	views := CollectPlanExecutionSnapshots(parent.Id, nil)
	require.Len(t, views, 1)
	v := views[0]
	require.Equal(t, "outer-task", v.AsyncReactTaskID)
	require.Equal(t, 2, v.TotalTasks)
	require.Equal(t, 1, v.CompletedTasks)
	require.Equal(t, 2, v.CurrentStage, "stage follows dependencies, not tree nesting")
	require.Equal(t, []string{"b"}, v.ActiveTaskIDs)
	for _, task := range v.Tasks {
		if task.TaskID == "b" {
			require.True(t, task.HasTimelineFork)
			require.NotEmpty(t, task.RecentTextOutputs)
		}
	}
	view := s.ContextSnapshot("caller query", s.Snapshot())
	require.Equal(t, "A verified", view.CurrentTask.TaskSummary)
	view.RootTask.Subtasks[0].Subtasks[0].TaskSummary = "caller mutation"
	require.Equal(t, "A verified", s.Snapshot().Attempts["a"].Result.Summary)
	runningSessions.Delete(s.Id)
	require.Empty(t, CollectPlanExecutionSnapshots(parent.Id, nil))
}
