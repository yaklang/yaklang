package aireact

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
	default:
		return fmt.Errorf("unsupported sync type: %s", event.SyncType)
	}
}
