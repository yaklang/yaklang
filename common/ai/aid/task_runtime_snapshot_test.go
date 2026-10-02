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

func TestRecentTextOutputsFromTimeline_LimitAndOrder(t *testing.T) {
	timeline := aicommon.NewTimeline(nil, nil)
	for i := 0; i < 15; i++ {
		timeline.PushText(int64(i+1), "msg-"+strconv.Itoa(i))
	}
	outputs := RecentTaskTextOutputs(timeline, 10)
	require.Len(t, outputs, 10)
	require.Contains(t, outputs[len(outputs)-1].Content, "msg-")
}
