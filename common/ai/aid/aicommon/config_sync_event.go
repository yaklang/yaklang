package aicommon

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	SYNC_TYPE_PLAN                 string = "plan"
	SYNC_TYPE_CONSUMPTION          string = "consumption"
	SYNC_TYPE_PING                 string = "ping"
	SYNC_TYPE_UPDATE_CONFIG        string = "set_config"
	SYNC_TYPE_PROCESS_EVENT        string = "sync_process_event"
	SYNC_TYPE_TIMELINE                    = "timeline"
	SYNC_TYPE_MEMORY_CONTEXT              = "memory_sync"
	SYNC_TYPE_SKIP_SUBTASK_IN_PLAN        = "skip_subtask_in_plan"
	SYNC_TYPE_REDO_SUBTASK_IN_PLAN        = "redo_subtask_in_plan"
	SYNC_TYPE_PLAN_EXEC_TASKS             = "plan_exec_tasks"
	SYNC_TYPE_USER_INTERVENTION           = "user_intervention"

	ProcessID           string = "process_id"
	SyncProcessEeventID        = "sync_process_event_id"
)

func (c *Config) HandleSyncConsumptionEvent(e *ypb.AIInputEvent) error {
	c.EmitSyncJSON(
		schema.EVENT_TYPE_CONSUMPTION,
		"system",
		map[string]any{
			"input_consumption":  c.GetInputConsumption(),
			"output_consumption": c.GetOutputConsumption(),
			"consumption_uuid":   c.GetConsumptionUUID(),
			"tier_consumption":   c.GetTierConsumptionSnapshot(),
		},
		e.SyncID,
	)
	return nil
}

func (c *Config) HandleSyncPongEvent(e *ypb.AIInputEvent) error {
	c.EmitSyncJSON(schema.EVENT_TYPE_PONG, "system", map[string]any{
		"now":         time.Now().Format(time.RFC3339),
		"now_unix":    time.Now().Unix(),
		"now_unix_ms": time.Now().UnixMilli(),
	},
		e.SyncID,
	)
	return nil
}

func (c *Config) HandleSyncUserIntervention(event *ypb.AIInputEvent) error {
	if event.SyncJsonInput == "" {
		c.EmitError("sync json input is empty")
		return nil
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(event.SyncJsonInput), &params); err != nil {
		c.EmitError("invalid sync json input: %v", err)
		return nil
	}
	content := aitool.InvokeParams(params).GetString("content")
	if content == "" {
		c.EmitError("content is empty in sync json input")
		return nil
	}

	c.Timeline.PushText(c.AcquireId(), "[User Intervention] "+content)
	if _, err := c.AppendUserInputHistory(content, time.Now()); err != nil {
		c.EmitError("append user intervention history failed: %v", err)
	}

	c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "user_intervention", map[string]interface{}{
		"content": content,
	}, event.SyncID)
	return nil
}

func (c *Config) HandleSyncTimelineEvent(event *ypb.AIInputEvent) error {
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

	if c.Timeline == nil {
		c.EmitError("timeline is null")
	}

	total := c.Timeline.GetIdToTimelineItem().Len()
	if limit <= 0 {
		limit = total
	}

	// 通过 Emitter 发送时间线信息事件
	c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "timeline", map[string]interface{}{
		"total_entries": total,
		"limit":         limit,
		"entries":       c.Timeline.ToTimelineItemOutputLastN(limit),
		"dump":          c.Timeline.Dump(),
	},
		event.SyncID,
	)
	return nil
}

func (c *Config) HandleSyncPlanExecTasksEvent(event *ypb.AIInputEvent) error {
	sessionID := c.PersistentSessionId
	limit := -1
	if event.SyncJsonInput != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(event.SyncJsonInput), &params); err == nil {
			if sid, ok := params["session_id"].(string); ok && sid != "" {
				sessionID = sid
			}
			if l, ok := params["limit"].(float64); ok && l > 0 {
				limit = int(l)
			}
		}
	}

	db := c.GetDB()
	if db == nil {
		c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "plan_exec_tasks", map[string]interface{}{
			"session_id": sessionID,
			"total":      0,
			"records":    []map[string]interface{}{},
			"error":      "db is nil",
		}, event.SyncID)
		return nil
	}
	if sessionID == "" {
		c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "plan_exec_tasks", map[string]interface{}{
			"session_id": sessionID,
			"total":      0,
			"records":    []map[string]interface{}{},
		}, event.SyncID)
		return nil
	}

	query := db.Model(&schema.AISessionPlanAndExec{}).Where("session_id = ?", sessionID).Order("updated_at desc")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var records []schema.AISessionPlanAndExec
	if err := query.Find(&records).Error; err != nil {
		c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "plan_exec_tasks", map[string]interface{}{
			"session_id": sessionID,
			"total":      0,
			"records":    []map[string]interface{}{},
			"error":      err.Error(),
		}, event.SyncID)
		return nil
	}

	items := make([]map[string]interface{}, 0, len(records))
	for _, r := range records {
		items = append(items, map[string]interface{}{
			"session_id":      r.SessionID,
			"coordinator_id":  r.CoordinatorID,
			"task_tree":       r.TaskTree,
			"task_progress":   r.TaskProgress,
			"created_at":      r.CreatedAt,
			"updated_at":      r.UpdatedAt,
			"created_at_unix": r.CreatedAt.Unix(),
			"updated_at_unix": r.UpdatedAt.Unix(),
		})
	}

	c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "plan_exec_tasks", map[string]interface{}{
		"session_id": sessionID,
		"total":      len(items),
		"records":    items,
	}, event.SyncID)
	return nil
}

func (c *Config) HandleSyncUpdataConfigEvent(event *ypb.AIInputEvent) error {
	updateConfig := map[string]interface{}{}
	var legacyHotpatchEvent *ypb.AIInputEvent
	if event.Params.GetAIService() != "" {
		legacyHotpatchEvent = &ypb.AIInputEvent{
			HotpatchType: HotPatchType_AIService,
			Params: &ypb.AIStartParams{
				AIService:   event.Params.GetAIService(),
				AIModelName: event.Params.GetAIModelName(),
			},
		}
	}
	if event.Params.GetReviewPolicy() != "" {
		legacyHotpatchEvent = &ypb.AIInputEvent{
			HotpatchType: HotPatchType_AgreePolicy,
			Params: &ypb.AIStartParams{
				ReviewPolicy: event.Params.GetReviewPolicy(),
			},
		}
	}
	c.ProcessHotPatchMessage(legacyHotpatchEvent)
	c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "update_config", updateConfig, event.SyncID)
	return nil
}

func (c *Config) HandleSyncMemoryContextEvent(event *ypb.AIInputEvent) error {
	// 获取 memory session ID
	var memorySessionID string

	if c.MemoryTriage != nil {
		memorySessionID = c.MemoryTriage.GetSessionID()
	}

	// 收集 memoryPool 中的所有 MemoryEntity，并统计超过 0.7 分的各维度数量
	var memoryInfos []map[string]interface{}
	var totalSize int

	// score_overview 统计超过 0.7 分的各维度数量
	scoreOverview := map[string]int{
		"C_total": 0,
		"O_total": 0,
		"R_total": 0,
		"E_total": 0,
		"P_total": 0,
		"A_total": 0,
		"T_total": 0,
	}
	const scoreThreshold = 0.7

	if c.MemoryPool != nil {
		for _, memoryEntity := range c.MemoryPool.Values() {
			if memoryEntity != nil {
				// 构建带有 created_at_timestamp 的 memory 信息
				memoryInfo := map[string]interface{}{
					"id":                   memoryEntity.Id,
					"created_at":           memoryEntity.CreatedAt,
					"created_at_timestamp": memoryEntity.CreatedAt.Unix(),
					"content":              memoryEntity.Content,
					"tags":                 memoryEntity.Tags,
					"c_score":              memoryEntity.C_Score,
					"o_score":              memoryEntity.O_Score,
					"r_score":              memoryEntity.R_Score,
					"e_score":              memoryEntity.E_Score,
					"p_score":              memoryEntity.P_Score,
					"a_score":              memoryEntity.A_Score,
					"t_score":              memoryEntity.T_Score,
					"core_pact_vector":     memoryEntity.CorePactVector,
					"potential_questions":  memoryEntity.PotentialQuestions,
				}
				memoryInfos = append(memoryInfos, memoryInfo)
				totalSize += len(memoryEntity.Content)

				// 统计超过 0.7 分的各维度
				if memoryEntity.C_Score > scoreThreshold {
					scoreOverview["C_total"]++
				}
				if memoryEntity.O_Score > scoreThreshold {
					scoreOverview["O_total"]++
				}
				if memoryEntity.R_Score > scoreThreshold {
					scoreOverview["R_total"]++
				}
				if memoryEntity.E_Score > scoreThreshold {
					scoreOverview["E_total"]++
				}
				if memoryEntity.P_Score > scoreThreshold {
					scoreOverview["P_total"]++
				}
				if memoryEntity.A_Score > scoreThreshold {
					scoreOverview["A_total"]++
				}
				if memoryEntity.T_Score > scoreThreshold {
					scoreOverview["T_total"]++
				}
			}
		}
	}

	// 构建响应数据
	responseData := map[string]interface{}{
		"memory_session_id": memorySessionID,
		"total_memories":    len(memoryInfos),
		"total_size":        totalSize,
		"memory_pool_limit": c.MemoryPoolSize,
		"score_overview":    scoreOverview,
		"memories":          memoryInfos,
	}

	// 通过 Emitter 发送 EVENT_TYPE_MEMORY_CONTEXT 事件
	c.EmitSyncJSON(schema.EVENT_TYPE_MEMORY_CONTEXT, "memory_context", responseData, event.SyncID)
	return nil
}

func (c *Config) RegisterBasicSyncHandlers() {
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_CONSUMPTION, c.HandleSyncConsumptionEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_PING, c.HandleSyncPongEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_USER_INTERVENTION, c.HandleSyncUserIntervention)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_TIMELINE, c.HandleSyncTimelineEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_UPDATE_CONFIG, c.HandleSyncUpdataConfigEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_MEMORY_CONTEXT, c.HandleSyncMemoryContextEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_PLAN_EXEC_TASKS, c.HandleSyncPlanExecTasksEvent)
}
