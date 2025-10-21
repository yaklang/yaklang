package aireact

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// handleSyncMessage 处理同步消息
func (r *ReAct) handleSyncMessage(event *ypb.AIInputEvent) error {
	switch event.SyncType {
	case SYNC_TYPE_QUEUE_INFO:
		// 获取队列信息并通过事件发送
		queueInfo := r.GetQueueInfo()

		// 通过 Emitter 发送队列信息事件
		r.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "queue_info", queueInfo)
		return nil

	case SYNC_TYPE_KNOWLEDGE:
		// 同步某个任务已经获取到的知识
		taskID := r.GetCurrentTask().GetId()         // 默认使用当前任务ID
		if r.config.enhanceKnowledgeManager == nil { // 检查知识管理器是否配置, 如果没有则报错记录但不会返回错误
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
		knowledgeList := r.config.enhanceKnowledgeManager.GetKnowledgeByTaskID(taskID)
		if len(knowledgeList) <= 0 {
			log.Error("no knowledge found")
		}
		r.EmitKnowledgeListAboutTask(taskID, knowledgeList)
		return nil
	case SYNC_TYPE_TIMELINE:

		var limit = -1
		// 从 SyncJsonInput 中解析参数
		if event.SyncJsonInput != "" {
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(event.SyncJsonInput), &params); err == nil {
				if l, ok := params["limit"].(float64); ok && l > 0 {
					limit = int(l)
				}
			}
		}

		total := r.getTimelineTotal()
		if limit <= 0 {
			limit = total
		}

		// 通过 Emitter 发送时间线信息事件
		r.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "timeline", map[string]interface{}{
			"total_entries": total,
			"limit":         limit,
			"entries":       r.getTimeline(limit),
			"dump":          r.DumpTimeline(),
		})
		return nil
	case SYNC_TYPE_UPDATE_CONFIG:
		if event.Params.GetAIService() != "" {
			chat, err := ai.LoadChater(event.Params.GetAIService())
			if err != nil {
				r.EmitError("load ai service failed: %v", err)
			} else {
				r.config.aiCallback = aicommon.AIChatToAICallbackType(chat)
			}
		}
		if event.Params.GetReviewPolicy() != "" {
			r.config.reviewPolicy = aicommon.AgreePolicyType(event.Params.GetReviewPolicy())
		}
		return nil
	case SYNC_TYPE_MEMORY_CONTEXT:
		// 获取 memory session ID
		var memorySessionID string
		if r.memoryTriage != nil {
			if aiMemTriage, ok := r.memoryTriage.(*aimem.AIMemoryTriage); ok {
				memorySessionID = aiMemTriage.GetSessionID()
			}
		}

		// 收集 memoryPool 中的所有 MemoryEntity
		var memoryInfos []*aimem.MemoryEntity
		var totalSize int
		if r.memoryPool != nil {
			for _, memoryEntity := range r.memoryPool.Values() {
				if memoryEntity != nil {
					memoryInfos = append(memoryInfos, memoryEntity)
					totalSize += len(memoryEntity.Content)
				}
			}
		}

		// 构建响应数据
		responseData := map[string]interface{}{
			"memory_session_id": memorySessionID,
			"total_memories":    len(memoryInfos),
			"total_size":        totalSize,
			"memory_pool_limit": r.config.memoryPoolSize,
			"memories":          memoryInfos,
		}

		// 通过 Emitter 发送 EVENT_TYPE_MEMORY_CONTEXT 事件
		r.EmitJSON(schema.EVENT_TYPE_MEMORY_CONTEXT, "memory_context", responseData)
		return nil
	case SYNC_TYPE_REACT_JUMP_QUEUE:
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
			r.EmitStructured("react_task_cancelled", map[string]interface{}{
				"task_id":      currentTask.GetId(),
				"user_input":   currentTask.GetUserInput(),
				"cancelled_at": time.Now(),
				"reason":       "jump_queue",
			})

			log.Infof("current task %s has been cancelled for jump queue", currentTask.GetId())
		}

		// 发送插队成功事件
		r.EmitStructured("react_task_jumped_queue", map[string]interface{}{
			"jumped_task_id": targetTaskId,
			"jumped_at":      time.Now(),
		})

		log.Infof("task %s has successfully jumped to front of queue", targetTaskId)
		return nil
	case SYNC_TYPE_REACT_CANCEL_CURRENT_TASK:
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
	default:
		return fmt.Errorf("unsupported sync type: %s", event.SyncType)
	}
}
