package aireact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHandleSyncTypeTaskSnapshotEvent_ReturnsMaterializedTask(t *testing.T) {
	var captured *schema.AiOutputEvent
	ins, err := NewTestReAct(aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
		if event != nil && event.NodeId == aicommon.SessionTaskSnapshotNodeID {
			captured = event
		}
	}))
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase("task-snapshot-sync", "input", context.Background(), nil, true)
	task.SetName("sync task")
	ins.config.ResetSessionSnapshotExecution(task.GetName(), "processing", time.Now())
	ins.config.MaterializeSessionSnapshot(task, &aicommon.SessionSnapshot{
		Execution: ins.config.BuildSessionSnapshotExecution(task),
	})
	aicommon.ApplyTodoDeltaAndEmit(ins.config, nil, task, aicommon.BuildVerificationTodoScope(task), 1,
		&aicommon.TodoDelta{Add: []aicommon.TodoAdd{{ID: "todo-1", Text: "task details todo"}}}, nil)
	ins.config.FinalizeSessionSnapshotExecution("completed", time.Now())
	ins.config.MaterializeSessionSnapshot(task, &aicommon.SessionSnapshot{
		Execution: ins.config.BuildSessionSnapshotExecution(task),
	})

	input, err := json.Marshal(map[string]string{"task_id": task.GetId()})
	require.NoError(t, err)
	require.NoError(t, ins.HandleSyncTypeTaskSnapshotEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncID:        "task-snapshot-sync-id",
		SyncJsonInput: string(input),
	}))

	require.NotNil(t, captured)
	require.True(t, captured.IsSync)
	require.Equal(t, "task-snapshot-sync-id", captured.SyncID)
	require.NotContains(t, string(captured.Content), `"round"`)
	require.NotContains(t, string(captured.Content), `"current_round"`)
	require.NotContains(t, string(captured.Content), `"total_rounds"`)
	require.Contains(t, string(captured.Content), `"execution_rounds"`)
	var snapshot aicommon.SessionTaskSnapshot
	require.NoError(t, json.Unmarshal(captured.Content, &snapshot))
	require.Equal(t, task.GetId(), snapshot.TaskID)
	require.Equal(t, "completed", snapshot.Status)
	require.True(t, snapshot.IsFinal)
	require.NotNil(t, snapshot.Todo)
	require.Len(t, snapshot.Todo.Items, 1)
	require.Equal(t, "task details todo", snapshot.Todo.Items[0].Content)
}

func TestSessionSnapshotRounds_ExcludesQueuedTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true))
	queue := NewTaskQueue("snapshot-rounds")
	for _, id := range []string{"first", "second"} {
		task := aicommon.NewStatefulTaskBase(id, id, ctx, nil, true)
		task.SetStatus(aicommon.AITaskState_Queueing)
		require.NoError(t, queue.Append(task))
	}

	idle := reactloops.BuildSessionSnapshot(cfg, nil, nil)
	require.Zero(t, idle.CurrentRound)
	require.Zero(t, idle.TotalRounds)

	first := queue.GetFirst()
	require.NotNil(t, first)
	first.SetStatus(aicommon.AITaskState_Processing)
	for range 2 {
		snapshot := reactloops.BuildSessionSnapshot(cfg, nil, first)
		require.Equal(t, 1, snapshot.CurrentRound)
		require.Equal(t, 1, snapshot.TotalRounds, "repeated emits must not count queued tasks")
		require.Len(t, snapshot.Tasks, 1)
	}
	require.Equal(t, 1, queue.Len())
	require.Nil(t, cfg.GetSessionTaskSnapshot("second"))

	first.SetStatus(aicommon.AITaskState_Completed)
	reactloops.BuildSessionSnapshot(cfg, nil, first)
	second := queue.GetFirst()
	require.NotNil(t, second)
	second.SetStatus(aicommon.AITaskState_Processing)
	snapshot := reactloops.BuildSessionSnapshot(cfg, nil, second)
	require.Equal(t, 2, snapshot.CurrentRound)
	require.Equal(t, 2, snapshot.TotalRounds)
	require.True(t, queue.IsEmpty())
}

func TestHandleSyncTypeTaskSnapshotEvent_ReturnsSyncError(t *testing.T) {
	var captured *schema.AiOutputEvent
	ins, err := NewTestReAct(aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
		if event != nil && event.NodeId == aicommon.SessionTaskSnapshotNodeID {
			captured = event
		}
	}))
	require.NoError(t, err)
	require.NoError(t, ins.HandleSyncTypeTaskSnapshotEvent(&ypb.AIInputEvent{
		IsSyncMessage: true, SyncID: "missing-task-sync", SyncJsonInput: `{"task_id":"does-not-exist"}`,
	}))
	require.NotNil(t, captured)
	require.True(t, captured.IsSync)
	require.Equal(t, "missing-task-sync", captured.SyncID)
	require.Contains(t, string(captured.Content), "not found")
}
