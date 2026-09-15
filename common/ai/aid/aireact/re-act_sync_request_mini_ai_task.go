package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SYNC_TYPE_AI_MINI_TASK 是 mini AI 任务的 sync 事件类型。
// 客户端通过发送此类型的 sync 事件触发轻量 AI 微操作。
const SYNC_TYPE_AI_MINI_TASK = "ai_mini_task"

// miniAITaskDefaultTimeout 是 mini AI handler 的默认超时时间。
const miniAITaskDefaultTimeout = 60 * time.Second

// HandleSyncTypeAIMiniTaskEvent 是 ai_mini_task sync 事件的统一入口。
//
// 客户端发起:
//
//	AIInputEvent{
//	  IsSyncMessage: true,
//	  SyncType:      "ai_mini_task",
//	  SyncJsonInput: `{"task_name":"prompt_optimize","prompt":"..."}`,
//	  SyncID:        "xxx",
//	}
//
// 服务端处理:
//  1. 从 SyncJsonInput 解析 task_name 和 params
//  2. 从 ReAct.miniAITaskRegistry 查找 handler
//  3. 构建 MiniAITaskContext
//  4. 带超时 + panic 防护执行 handler
//  5. EmitSyncJSON 回传结果或错误
func (r *ReAct) HandleSyncTypeAIMiniTaskEvent(event *ypb.AIInputEvent) error {
	// 1. 解析 SyncJsonInput
	var rawParams map[string]any
	if event.SyncJsonInput != "" {
		if err := json.Unmarshal([]byte(event.SyncJsonInput), &rawParams); err != nil {
			r.emitMiniAITaskError(event.SyncID, "", fmt.Sprintf("parse params failed: %v", err), nil)
			return nil
		}
	}

	taskName := getStringParam(rawParams, "task_name")
	if taskName == "" {
		r.emitMiniAITaskError(event.SyncID, "", "task_name is required", nil)
		return nil
	}

	// 2. 从 registry 查找 handler
	handler, ok := r.miniAITaskRegistry.Get(taskName)
	if !ok {
		var available []MiniTaskDescriptor
		if r.miniAITaskRegistry != nil {
			available = r.miniAITaskRegistry.Descriptors()
		}
		r.emitMiniAITaskError(event.SyncID, taskName,
			fmt.Sprintf("unknown mini ai task: %s", taskName), available)
		return nil
	}

	// 3. 构建上下文（移除 task_name，剩余作为 handler params）
	handlerParams := make(map[string]any)
	for k, v := range rawParams {
		if k != "task_name" {
			handlerParams[k] = v
		}
	}

	taskCtx := &MiniAITaskContext{
		ReAct:    r,
		Config:   r.config,
		Timeline: r.config.GetTimeline(),
		Task:     r.GetCurrentTask(),
		SyncID:   event.SyncID,
	}

	// 4. 带超时 + panic 防护执行
	ctx, cancel := context.WithTimeout(r.config.GetContext(), miniAITaskDefaultTimeout)
	defer cancel()

	var result any
	var handlerErr error
	func() {
		defer func() {
			if p := recover(); p != nil {
				handlerErr = fmt.Errorf("mini ai task %s panic: %v", taskName, p)
				log.Errorf("mini ai task %s panic: %v", taskName, p)
			}
		}()
		result, handlerErr = handler(ctx, taskCtx, handlerParams)
	}()

	// 5. EmitSyncJSON 回传
	response := map[string]any{
		"task_name": taskName,
	}
	if handlerErr != nil {
		response["error"] = handlerErr.Error()
	} else {
		response["result"] = result
	}

	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "ai_mini_task", response, event.SyncID)
	return nil
}

// emitMiniAITaskError 发送 mini AI task 的错误响应。
func (r *ReAct) emitMiniAITaskError(syncID, taskName, errMsg string, available []MiniTaskDescriptor) {
	response := map[string]any{
		"error": errMsg,
	}
	if taskName != "" {
		response["task_name"] = taskName
	}
	if available != nil {
		response["available"] = available
	}
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "ai_mini_task", response, syncID)
}
