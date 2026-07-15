package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// BuildForwardingEmitter derives a sub-agent emitter from the parent config emitter
// (not the parent task emitter) via PushEventProcesser. The derived emitter shares
// the parent's frontend sink while a processor stamps every event's TaskId with the
// sub-task id — the marker the frontend uses to aggregate sub-agent messages.
func BuildForwardingEmitter(parentEmitter *aicommon.Emitter, subTaskID string) *aicommon.Emitter {
	if parentEmitter == nil {
		return aicommon.NewDummyEmitter()
	}
	return parentEmitter.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil && subTaskID != "" {
			event.TaskId = subTaskID
		}
		return event
	})
}

// BuildForwardingEmitterForTask stamps both TaskId and TaskUUID so tool cards nest
// under the sub-agent card created by react_task_created.
func BuildForwardingEmitterForTask(parentEmitter *aicommon.Emitter, task aicommon.AIStatefulTask) *aicommon.Emitter {
	if task == nil {
		return BuildForwardingEmitter(parentEmitter, "")
	}
	taskID := task.GetId()
	taskUUID := task.GetUUID()
	emitter := BuildForwardingEmitter(parentEmitter, taskID)
	if taskUUID == "" {
		return emitter
	}
	return emitter.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil {
			event.TaskUUID = taskUUID
		}
		return event
	})
}
