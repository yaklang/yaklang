package aicommon

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

const (
	SYNC_TYPE_PLAN           string = "plan"
	SYNC_TYPE_CONSUMPTION    string = "consumption"
	SYNC_TYPE_PING           string = "ping"
	SYNC_TYPE_UPDATE_CONFIG  string = "set_config"
	SYNC_TYPE_PROCESS_EVENT  string = "sync_process_event"
	SYNC_TYPE_TIMELINE              = "timeline"
	SYNC_TYPE_MEMORY_CONTEXT        = "memory_sync"

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
			"consumption_uuid":   c.consumptionUUID,
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

func (c *Config) HandleSyncUpdataConfigEvent(event *ypb.AIInputEvent) error {
	updateConfig := map[string]interface{}{}
	if event.Params.GetAIService() != "" {
		err := c.LoadAIServiceByName(event.Params.GetAIService())
		if err != nil {
			c.EmitError("load ai service failed: %v", err)
		}
	}
	if event.Params.GetReviewPolicy() != "" {
		c.AgreePolicy = AgreePolicyType(event.Params.GetReviewPolicy())
		c.HotPatchBroadcaster.Submit(WithAgreePolicy(c.AgreePolicy))
		updateConfig["review_policy"] = event.Params.GetReviewPolicy()
	}
	c.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "update_config", updateConfig, event.SyncID)
	return nil
}

func (c *Config) HandleSyncMemoryContextEvent(event *ypb.AIInputEvent) error {
	// 获取 memory session ID
	var memorySessionID string

	if c.MemoryTriage != nil {
		memorySessionID = c.MemoryTriage.GetSessionID()
	}

	// 收集 memoryPool 中的所有 MemoryEntity
	var memoryInfos []*MemoryEntity
	var totalSize int
	if c.MemoryPool != nil {
		for _, memoryEntity := range c.MemoryPool.Values() {
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
		"memory_pool_limit": c.MemoryPoolSize,
		"memories":          memoryInfos,
	}

	// 通过 Emitter 发送 EVENT_TYPE_MEMORY_CONTEXT 事件
	c.EmitSyncJSON(schema.EVENT_TYPE_MEMORY_CONTEXT, "memory_context", responseData, event.SyncID)
	return nil
}

func (c *Config) RegisterBasicSyncHandlers() {
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_CONSUMPTION, c.HandleSyncConsumptionEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_PING, c.HandleSyncPongEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_TIMELINE, c.HandleSyncTimelineEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_UPDATE_CONFIG, c.HandleSyncUpdataConfigEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_MEMORY_CONTEXT, c.HandleSyncMemoryContextEvent)
}
