package coordinator_legacy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
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

	source := &mockReactRuntimeSource{reactID: "react-1"}
	// The common aggregator must not discover the legacy global registry itself.
	require.Empty(t, aid.BuildTaskRuntimeReport(source).PlanExecutions)
	report := aid.BuildTaskRuntimeReport(source, CollectPlanExecutionSnapshots(nil)...)
	require.Len(t, report.PlanExecutions, 1)
	require.Equal(t, "coord-1", report.PlanExecutions[0].CoordinatorID)
	require.Contains(t, report.PlanExecutions[0].ActiveTaskIDs, "1")

	var childEntry *aid.TaskRuntimeEntry
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
