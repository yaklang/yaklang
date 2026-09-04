package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCancelTaskRejectsFinishedTask(t *testing.T) {
	emitter := aicommon.NewEmitter("cancel-finished-task", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return event, nil
	})
	task := aicommon.NewStatefulTaskBase("finished-task", "done", nil, emitter, true)
	task.SetStatus(aicommon.AITaskState_Completed)
	react := &ReAct{
		Emitter:      emitter,
		RuntimeTasks: []aicommon.AIStatefulTask{task},
	}

	err := react.HandleSyncTypeCancelTaskEvent(&ypb.AIInputEvent{
		SyncID:        "cancel-sync-id",
		SyncJsonInput: `{"task_id":"finished-task"}`,
	})

	require.NoError(t, err)
	require.False(t, task.IsUserCancelled())
	require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
}
