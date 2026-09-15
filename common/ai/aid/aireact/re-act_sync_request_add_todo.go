package aireact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SYNC_TYPE_ADD_TODO 是通过 sync 事件向当前 task 添加 TODO 条目的事件类型。
// 客户端发送 AIInputEvent{IsSyncMessage:true, SyncType:"add_todo_sync",
// SyncJsonInput: `{"text":"...","id":"...","set_current":true}`}.
const SYNC_TYPE_ADD_TODO = "add_todo_sync"

// addTodoRequest 是 add_todo sync 事件的请求体。
type addTodoRequest struct {
	// Text 是待办条目文本，需覆盖三要素（具体目标、来源、验收方法）。
	Text string `json:"text"`
	// ID 是可选的自定义条目 ID；省略时引擎自动生成 todo-N。
	ID string `json:"id,omitempty"`
	// SetCurrent 为 true 时将新增条目设为当前焦点（current）。
	// 如果 ID 省略，引擎会先 add 生成 ID，再设为 current。
	SetCurrent bool `json:"set_current,omitempty"`
}

// HandleSyncTypeAddTodoEvent 处理通过 sync 事件添加 TODO 的请求。
//
// 客户端发起:
//
//	AIInputEvent{
//	  IsSyncMessage: true,
//	  SyncType:      "add_todo_sync",
//	  SyncJsonInput: `{"text":"对 example.com 执行端口扫描...","set_current":true}`,
//	  SyncID:        "xxx",
//	}
//
// 服务端处理:
//  1. 解析 text（必填）和可选的 id / set_current
//  2. 构建 TodoDelta{Add: [{ID, Text}]}
//  3. 通过 ApplyTodoDeltaAndEmit 写入 SessionPromptState 并 emit 前端更新
//  4. 如果 set_current 且 add 成功，再发一个 current delta
//  5. 在 timeline 记录 "user_added_todo"
//  6. EmitSyncJSON 返回结果
func (r *ReAct) HandleSyncTypeAddTodoEvent(event *ypb.AIInputEvent) error {
	var req addTodoRequest
	if err := json.Unmarshal([]byte(event.SyncJsonInput), &req); err != nil {
		r.emitAddTodoError(event.SyncID, fmt.Sprintf("parse params failed: %v", err))
		return nil
	}

	if strings.TrimSpace(req.Text) == "" {
		r.emitAddTodoError(event.SyncID, "text is required")
		return nil
	}

	task := r.GetCurrentTask()
	scope := aicommon.BuildVerificationTodoScope(task)

	// 构建 add delta
	delta := &aicommon.TodoDelta{
		Add: []aicommon.TodoAdd{
			{ID: req.ID, Text: req.Text},
		},
	}

	// 如果 set_current 且有自定义 ID，在同一 delta 中设置 current
	if req.SetCurrent && req.ID != "" {
		id := req.ID
		delta.Current = &id
		delta.CurrentSet = true
	}

	// 应用 delta
	results := aicommon.ApplyTodoDeltaAndEmit(
		r.config, r.Emitter, task,
		scope, 0, delta,
		func(entryType, content string) {
			r.AddToTimeline(entryType, content)
		},
	)

	// 检查 add 是否成功
	var addedID string
	for _, res := range results {
		if res.Operation.Op == "add" {
			if res.Success {
				addedID = res.Operation.ID
			} else {
				r.emitAddTodoError(event.SyncID, fmt.Sprintf("add todo failed: %s", res.Reason))
				return nil
			}
		}
	}

	// 如果 set_current 但 ID 是自动生成的，需要第二次 delta 设置 current
	if req.SetCurrent && req.ID == "" && addedID != "" {
		currentDelta := &aicommon.TodoDelta{
			Current:    &addedID,
			CurrentSet: true,
		}
		aicommon.ApplyTodoDeltaAndEmit(
			r.config, r.Emitter, task,
			scope, 0, currentDelta,
			func(entryType, content string) {
				r.AddToTimeline(entryType, content)
			},
		)
	}

	// timeline 记录
	r.AddToTimeline("user_added_todo", fmt.Sprintf("User added TODO: %s", req.Text))

	log.Infof("user added todo via sync: id=%s, set_current=%v", addedID, req.SetCurrent)

	// 返回成功响应
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "add_todo", map[string]any{
		"success":     true,
		"todo_id":     addedID,
		"text":        req.Text,
		"set_current": req.SetCurrent,
	}, event.SyncID)
	return nil
}

// emitAddTodoError 发送 add_todo 的错误响应。
func (r *ReAct) emitAddTodoError(syncID, errMsg string) {
	_, _ = r.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "add_todo", map[string]any{
		"success": false,
		"error":   errMsg,
	}, syncID)
}
