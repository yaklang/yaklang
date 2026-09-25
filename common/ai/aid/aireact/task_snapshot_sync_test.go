package aireact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
	var snapshot aicommon.SessionTaskSnapshot
	require.NoError(t, json.Unmarshal(captured.Content, &snapshot))
	require.Equal(t, task.GetId(), snapshot.TaskID)
	require.Equal(t, "completed", snapshot.Status)
	require.True(t, snapshot.IsFinal)
	require.NotNil(t, snapshot.Todo)
	require.Len(t, snapshot.Todo.Items, 1)
	require.Equal(t, "task details todo", snapshot.Todo.Items[0].Content)
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
