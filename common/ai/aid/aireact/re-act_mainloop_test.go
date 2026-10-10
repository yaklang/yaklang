package aireact

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestUpdateRuntimeTasksEmitsStandbyStatusOnce(t *testing.T) {
	for _, status := range []aicommon.AITaskState{
		aicommon.AITaskState_Completed,
		aicommon.AITaskState_Aborted,
		aicommon.AITaskState_Skipped,
	} {
		t.Run(string(status), func(t *testing.T) {
			react := &ReAct{}
			var events []*schema.AiOutputEvent
			react.Emitter = aicommon.NewEmitter("runtime-tasks-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
				require.True(t, react.UpdateRuntimeTaskMutex.TryLock(), "status callbacks must run outside the runtime task lock")
				react.UpdateRuntimeTaskMutex.Unlock()
				require.Empty(t, react.GetRuntimeTasks(), "runtime tasks must be cleaned before emitting the status")
				events = append(events, event)
				return event, nil
			})

			react.updateRuntimeTasks()
			require.Empty(t, events, "an initially idle runtime has no completed task to report")

			for range 2 {
				task := aicommon.NewStatefulTaskBase("task", "input", context.Background(), nil)
				t.Cleanup(func() { task.Cancel() })
				task.SetStatus(status)
				react.RuntimeTasks = []aicommon.AIStatefulTask{task}
				react.updateRuntimeTasks()
				react.updateRuntimeTasks()
			}
			require.Len(t, events, 2, "each transition to idle should emit exactly one status")
			for _, event := range events {
				require.Equal(t, schema.EVENT_TYPE_STRUCTURED, event.Type)
				require.Equal(t, "status", event.NodeId)
				require.True(t, event.IsJson)
				var payload aicommon.StatusPayload
				require.NoError(t, json.Unmarshal(event.Content, &payload))
				require.Equal(t, reactloops.ReActLoadingStatusKey, payload.Key)
				require.Equal(t, "本轮任务已结束，等待新指令", payload.Value)
				require.Equal(t, aicommon.StatusStateWaiting, payload.State)
				require.Equal(t, &schema.I18n{Zh: payload.Value, En: "Task ended. Ready for your next instruction."}, payload.ValueI18n)
			}
		})
	}
}

func TestUpdateRuntimeTasksKeepsUnfinishedTasks(t *testing.T) {
	for _, status := range []aicommon.AITaskState{
		aicommon.AITaskState_Created,
		aicommon.AITaskState_Queueing,
		aicommon.AITaskState_Processing,
	} {
		t.Run(string(status), func(t *testing.T) {
			finished := aicommon.NewStatefulTaskBase("finished", "input", context.Background(), nil)
			t.Cleanup(func() { finished.Cancel() })
			finished.SetStatus(aicommon.AITaskState_Completed)
			unfinished := aicommon.NewStatefulTaskBase("unfinished", "input", context.Background(), nil)
			t.Cleanup(func() { unfinished.Cancel() })
			unfinished.SetStatus(status)
			var events []*schema.AiOutputEvent
			react := &ReAct{
				Emitter: aicommon.NewEmitter("runtime-tasks-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
					events = append(events, event)
					return event, nil
				}),
				RuntimeTasks: []aicommon.AIStatefulTask{finished, unfinished},
			}

			react.updateRuntimeTasks()
			require.Equal(t, []aicommon.AIStatefulTask{unfinished}, react.GetRuntimeTasks())
			require.Empty(t, events, "unfinished tasks must prevent the standby status")
		})
	}
}
