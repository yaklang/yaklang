package aid

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

type mockReactRuntimeSource struct {
	reactID           string
	runtimeTasks      []aicommon.AIStatefulTask
	currentTask       aicommon.AIStatefulTask
	planExecutionTask aicommon.AIStatefulTask
	queueingTasks     []aicommon.AIStatefulTask
	sessionTimeline   *aicommon.Timeline
}

func (m *mockReactRuntimeSource) GetReActID() string { return m.reactID }
func (m *mockReactRuntimeSource) GetRuntimeTasks() []aicommon.AIStatefulTask {
	return m.runtimeTasks
}
func (m *mockReactRuntimeSource) GetCurrentTask() aicommon.AIStatefulTask {
	return m.currentTask
}
func (m *mockReactRuntimeSource) GetCurrentPlanExecutionTask() aicommon.AIStatefulTask {
	return m.planExecutionTask
}
func (m *mockReactRuntimeSource) GetQueueingTasks() []aicommon.AIStatefulTask {
	return m.queueingTasks
}
func (m *mockReactRuntimeSource) GetSessionTimeline() *aicommon.Timeline {
	return m.sessionTimeline
}

func TestBuildTaskRuntimeReport_AsyncAndExecutingReactTasks(t *testing.T) {
	timeline := aicommon.NewTimeline(nil, nil)
	for i := 0; i < 12; i++ {
		timeline.PushText(int64(i+1), "line-"+strconv.Itoa(i))
	}

	asyncTask := aicommon.NewStatefulTaskBase("async-1", "async input", context.Background(), nil, false)
	asyncTask.SetAsyncMode(true)
	asyncTask.SetStatus(aicommon.AITaskState_Processing)

	currentTask := aicommon.NewStatefulTaskBase("current-1", "current input", context.Background(), nil, false)
	currentTask.SetStatus(aicommon.AITaskState_Processing)

	queuedTask := aicommon.NewStatefulTaskBase("queued-1", "queued input", context.Background(), nil, false)
	queuedTask.SetStatus(aicommon.AITaskState_Queueing)

	report := BuildTaskRuntimeReport(&mockReactRuntimeSource{
		reactID:           "react-session-1",
		runtimeTasks:      []aicommon.AIStatefulTask{asyncTask},
		currentTask:       currentTask,
		planExecutionTask: asyncTask,
		queueingTasks:     []aicommon.AIStatefulTask{queuedTask},
		sessionTimeline:   timeline,
	})

	require.NotEmpty(t, report.GeneratedAt)
	require.Equal(t, "react-session-1", report.ReActID)
	require.NotEmpty(t, report.AsyncTasks)
	require.NotEmpty(t, report.ExecutingTasks)
	require.Len(t, report.QueuedReactTasks, 1)
	require.Equal(t, "queued-1", report.QueuedReactTasks[0].TaskID)

	foundAsync := false
	for _, entry := range report.AsyncTasks {
		if entry.TaskID == "async-1" {
			foundAsync = true
			require.True(t, entry.AsyncMode)
			require.LessOrEqual(t, len(entry.RecentTextOutputs), defaultRecentTextOutputLimit)
		}
	}
	require.True(t, foundAsync)
}

func TestBuildTaskRuntimeReport_PlanExecutionCoordinator(t *testing.T) {
	coordinator := &Coordinator{
		Config: aicommon.NewConfig(context.Background(), aicommon.WithID("coord-1")),
	}
	root := &AiTask{
		Index: "0",
		Name:  "root",
		Subtasks: []*AiTask{
			{
				Index:              "1",
				Name:               "child",
				AIStatefulTaskBase: aicommon.NewStatefulTaskBase("child-task", "child", context.Background(), nil, false),
			},
		},
	}
	root.Subtasks[0].SetStatus(aicommon.AITaskState_Processing)
	coordinator.rootTask = root
	coordinator.runtime = coordinator.createRuntime()
	coordinator.runtime.RootTask = root
	coordinator.runtime.setActiveStage(0, []*executableTaskNode{{
		id:    "1",
		task:  root.Subtasks[0],
		order: 0,
	}})

	registerRunningCoordinator(coordinator)
	defer unregisterRunningCoordinator("coord-1")

	childTimeline := aicommon.NewTimeline(nil, nil)
	childTimeline.PushText(1, "child-output")
	root.Subtasks[0].timelineFork = &aicommon.TimelineFork{
		Branch: childTimeline,
	}
	root.Subtasks[0].Coordinator = coordinator

	report := BuildTaskRuntimeReport(&mockReactRuntimeSource{reactID: "react-1"})
	require.Len(t, report.PlanExecutions, 1)
	require.Equal(t, "coord-1", report.PlanExecutions[0].CoordinatorID)
	require.Contains(t, report.PlanExecutions[0].ActiveTaskIndexes, "1")

	var childEntry *TaskRuntimeEntry
	for _, entry := range report.PlanExecutions[0].Tasks {
		if entry.TaskIndex == "1" {
			childEntry = &entry
			break
		}
	}
	require.NotNil(t, childEntry)
	require.True(t, childEntry.Executing)
	require.NotEmpty(t, childEntry.RecentTextOutputs)
	require.Equal(t, "child-output", childEntry.RecentTextOutputs[0].Content)
}

func TestRecentTextOutputsFromTimeline_LimitAndOrder(t *testing.T) {
	timeline := aicommon.NewTimeline(nil, nil)
	for i := 0; i < 15; i++ {
		timeline.PushText(int64(i+1), "msg-"+strconv.Itoa(i))
	}
	outputs := recentTextOutputsFromTimeline(timeline, 10)
	require.Len(t, outputs, 10)
	require.Contains(t, outputs[len(outputs)-1].Content, "msg-")
}
