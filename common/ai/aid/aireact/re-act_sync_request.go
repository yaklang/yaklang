package aireact

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// handleSyncMessage 处理同步消息 - 现在仅作为分发器
func (r *ReAct) handleSyncMessage(event *ypb.AIInputEvent) error {
	switch event.SyncType {
	case SYNC_TYPE_QUEUE_INFO:
		return r.HandleSyncTypeQueueInfoEvent(event)
	case SYNC_TYPE_KNOWLEDGE:
		return r.HandleSyncTypeKnowledgeEvent(event)
	case SYNC_TYPE_REACT_JUMP_QUEUE:
		return r.HandleSyncTypeReactJumpQueueEvent(event)
	case SYNC_TYPE_REACT_CANCEL_CURRENT_TASK:
		return r.HandleSyncTypeReactCancelCurrentTaskEvent(event)
	case SYNC_TYPE_REACT_REMOVE_TASK:
		return r.HandleSyncTypeReactRemoveTaskEvent(event)
	default:
		return fmt.Errorf("unsupported sync type: %s", event.SyncType)
	}
}

func (r *ReAct) RegisterReActSyncEvent() {
	r.config.InputEventManager.RegisterSyncCallback(SYNC_TYPE_QUEUE_INFO, r.HandleSyncTypeQueueInfoEvent)
	r.config.InputEventManager.RegisterSyncCallback(SYNC_TYPE_KNOWLEDGE, r.HandleSyncTypeKnowledgeEvent)
	r.config.InputEventManager.RegisterSyncCallback(SYNC_TYPE_REACT_JUMP_QUEUE, r.HandleSyncTypeReactJumpQueueEvent)
	r.config.InputEventManager.RegisterSyncCallback(SYNC_TYPE_REACT_CANCEL_CURRENT_TASK, r.HandleSyncTypeReactCancelCurrentTaskEvent)
	r.config.InputEventManager.RegisterSyncCallback(SYNC_TYPE_REACT_REMOVE_TASK, r.HandleSyncTypeReactRemoveTaskEvent)
}

func (r *ReAct) UnRegisterReActSyncEvent() {
	r.config.InputEventManager.UnRegisterSyncCallback(SYNC_TYPE_QUEUE_INFO)
	r.config.InputEventManager.UnRegisterSyncCallback(SYNC_TYPE_KNOWLEDGE)
	r.config.InputEventManager.UnRegisterSyncCallback(SYNC_TYPE_REACT_JUMP_QUEUE)
	r.config.InputEventManager.UnRegisterSyncCallback(SYNC_TYPE_REACT_CANCEL_CURRENT_TASK)
	r.config.InputEventManager.UnRegisterSyncCallback(SYNC_TYPE_REACT_REMOVE_TASK)
}

// 单独拆分的 handler 函数

func (r *ReAct) HandleSyncTypeQueueInfoEvent(event *ypb.AIInputEvent) error {
	// 获取队列信息并通过事件发送
	queueInfo := r.GetQueueInfo()
	// 通过 Emitter 发送队列信息事件
	r.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "queue_info", queueInfo)
	return nil
}

func (r *ReAct) HandleSyncTypeKnowledgeEvent(event *ypb.AIInputEvent) error {
	// 同步某个任务已经获取到的知识
	taskID := r.GetCurrentTask().GetId() // 默认使用当前任务ID
	if r.config.EnhanceKnowledgeManager == nil {
		// 检查知识管理器是否配置, 如果没有则报错记录但不会返回错误
		r.EmitError("knowledge manager is not configured")
		return nil
	}
	if event.SyncJsonInput != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(event.SyncJsonInput), &params); err == nil {
			if id, ok := params["taskid"].(string); ok && id != "" {
				taskID = id
			}
		}
	}
	knowledgeList := r.config.EnhanceKnowledgeManager.GetKnowledgeByTaskID(taskID)
	if len(knowledgeList) <= 0 {
		log.Error("no knowledge found")
	}
	r.EmitKnowledgeListAboutTask(taskID, knowledgeList)
	return nil
}

func (r *ReAct) HandleSyncTypeReactJumpQueueEvent(event *ypb.AIInputEvent) error {
	// 插队任务：将指定 task_id 的任务移动到队列最前面，并取消当前任务
	var targetTaskId string

	// 从 SyncJsonInput 中解析 task_id
	if event.SyncJsonInput != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(event.SyncJsonInput), &params); err != nil {
			r.EmitError("failed to parse jump queue parameters: %v", err)
			return nil
		}

		if taskId, ok := params["task_id"].(string); ok && taskId != "" {
			targetTaskId = taskId
		} else {
			r.EmitError("task_id is required for jump queue operation")
			return nil
		}
	} else {
		r.EmitError("SyncJsonInput is required for jump queue operation")
		return nil
	}

	log.Infof("attempting to jump queue for task: %s", targetTaskId)

	// 尝试将指定任务移动到队列最前面
	taskMoved := r.taskQueue.MoveTaskToFirst(targetTaskId)
	if !taskMoved {
		r.EmitError("task %s not found in queue, cannot jump queue", targetTaskId)
		return nil
	}

	// 取消当前正在执行的任务（如果有的话）
	currentTask := r.GetCurrentTask()
	if currentTask != nil {
		log.Infof("cancelling current task %s to allow jump queue for task %s", currentTask.GetId(), targetTaskId)

		// 调用任务的 Cancel 方法，这会取消任务的 context
		currentTask.Cancel()

		// 设置任务状态为 Aborted
		currentTask.SetStatus(aicommon.AITaskState_Aborted)

		// 发送任务取消事件
		r.EmitStructured(REACT_TASK_cancelled, map[string]interface{}{
			"task_id":      currentTask.GetId(),
			"user_input":   currentTask.GetUserInput(),
			"cancelled_at": time.Now(),
			"reason":       "jump_queue",
		})

		log.Infof("current task %s has been cancelled for jump queue", currentTask.GetId())
	}

	// 发送插队成功事件
	queueInfo := r.GetQueueInfo()
	r.EmitStructured("react_task_jumped_queue", map[string]interface{}{
		"jumped_task_id": targetTaskId,
		"jumped_at":      time.Now(),
		"queue_info":     queueInfo,
	})

	log.Infof("task %s has successfully jumped to front of queue", targetTaskId)
	return nil
}

func (r *ReAct) HandleSyncTypeReactCancelCurrentTaskEvent(event *ypb.AIInputEvent) error {
	// 中断当前正在执行的任务
	currentTask := r.GetCurrentTask()
	if currentTask == nil {
		r.EmitError("no current task to cancel")
		return nil
	}

	log.Infof("cancelling current task: %s", currentTask.GetId())

	// 调用任务的 Cancel 方法，这会取消任务的 context
	currentTask.Cancel()

	// 设置任务状态为 Aborted
	currentTask.SetStatus(aicommon.AITaskState_Aborted)

	// 发送任务取消事件
	r.EmitStructured("react_task_cancelled", map[string]interface{}{
		"task_id":      currentTask.GetId(),
		"user_input":   currentTask.GetUserInput(),
		"cancelled_at": time.Now(),
	})

	log.Infof("current task %s has been cancelled", currentTask.GetId())
	return nil
}

func (r *ReAct) HandleSyncTypeReactRemoveTaskEvent(event *ypb.AIInputEvent) error {
	// 移除任务：从队列中移除指定 task_id 的任务
	var targetTaskId string

	// 从 SyncJsonInput 中解析 task_id
	if event.SyncJsonInput != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(event.SyncJsonInput), &params); err != nil {
			r.EmitError("failed to parse remove task parameters: %v", err)
			return nil
		}

		if taskId, ok := params["task_id"].(string); ok && taskId != "" {
			targetTaskId = taskId
		} else {
			r.EmitError("task_id is required for remove task operation")
			return nil
		}
	} else {
		r.EmitError("SyncJsonInput is required for remove task operation")
		return nil
	}

	log.Infof("attempting to remove task: %s", targetTaskId)

	// 尝试从队列中移除指定任务
	taskRemoved := r.taskQueue.RemoveTask(targetTaskId)
	if !taskRemoved {
		r.EmitError("task %s not found in queue, cannot remove", targetTaskId)
		return nil
	}

	log.Infof("task %s has been successfully removed from queue", targetTaskId)

	// 发送队列信息更新事件（调用拆分后的队列信息 handler）
	return r.HandleSyncTypeQueueInfoEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_QUEUE_INFO,
	})
}
