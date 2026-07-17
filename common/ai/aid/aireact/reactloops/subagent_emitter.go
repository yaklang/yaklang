package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// BuildForwardingEmitter 通过 PushEventProcesser 从父 config 的 emitter（而非
// 父任务 emitter）派生子 Agent emitter。派生 emitter 共享父前端 sink，同时
// processor 会给每个事件的 TaskId 打上子任务 ID——前端据此聚合子 Agent 消息。
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

// BuildForwardingEmitterForTask 同时打上 TaskId 和 TaskUUID，使工具卡片嵌套在
// react_task_created 创建的子 Agent 卡片下。
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
