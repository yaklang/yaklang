package aid

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type EventType string

const (
	EVENT_TYPE_STREAM     EventType = "stream"
	EVENT_TYPE_STRUCTURED EventType = "structured"

	// Token 开销情况
	EVENT_TYPE_CONSUMPTION EventType = "consumption" // token consumption include `{"input_"}`

	// 探活
	EVENT_TYPE_PONG EventType = "pong" // ping response ping-pong is a check for alive item

	// 压力值
	EVENT_TYPE_PRESSURE EventType = "pressure" // pressure for ai context percent

	EVENT_TYPE_AI_FIRST_BYTE_COST_MS EventType = "ai_first_byte_cost_ms" // first byte cost
	EVENT_TYPE_AI_TOTAL_COST_MS      EventType = "ai_total_cost_ms"      // first byte cost

	// AI 请求用户交互
	EVENT_TYPE_REQUIRE_USER_INTERACTIVE = "require_user_interactive"

	// risk control prompt is the prompt for risk control
	// contains score, reason, and other information to help uesr interactivation
	EVENT_TYPE_RISK_CONTROL_PROMPT = "risk_control_prompt"

	EVENT_TOOL_CALL_START       = "tool_call_start"       // tool call start event, used to emit the tool call start information
	EVENT_TOOL_CALL_STATUS      = "tool_call_status"      // tool call status event, used to emit the tool call status information
	EVENT_TOOL_CALL_USER_CANCEL = "tool_call_user_cancel" // tool call user cancel event, used to emit the tool call user cancel information
	EVENT_TOOL_CALL_DONE        = "tool_call_done"        // tool call end event, used to emit the tool call end information
	EVENT_TOOL_CALL_ERROR       = "tool_call_error"       // tool call error event, used to emit the tool call error information

	EVENT_TYPE_PLAN                    EventType = "plan"
	EVENT_TYPE_SELECT                  EventType = "select"
	EVENT_TYPE_PERMISSION_REQUIRE      EventType = "permission_require"
	EVENT_TYPE_TASK_REVIEW_REQUIRE     EventType = "task_review_require"
	EVENT_TYPE_PLAN_REVIEW_REQUIRE     EventType = "plan_review_require"
	EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE EventType = "tool_use_review_require"

	EVENT_TYPE_TOOL_CALL_WATCHER EventType = "tool_call_watcher" // tool call watcher event, used to emit the tool call watcher information. user can cancel this tool call

	EVENT_TYPE_REVIEW_RELEASE EventType = "review_release"

	EVENT_TYPE_INPUT EventType = "input"

	EVENT_TYPE_AID_CONFIG = "aid_config" // aid config event, used to emit the current config information
)

type Event struct {
	CoordinatorId string
	Type          EventType

	NodeId      string
	IsSystem    bool
	IsStream    bool
	IsReason    bool
	StreamDelta []byte
	IsJson      bool
	Content     []byte

	Timestamp int64

	// task index
	TaskIndex string
	// disable markdown render
	DisableMarkdown bool
}

func (e *Event) GetInteractiveId() string {
	if e.IsJson {
		var i map[string]any
		if err := json.Unmarshal(e.Content, &i); err == nil {
			// 检查事件类型是否为需要交互的类型
			switch e.Type {
			case EVENT_TYPE_PLAN_REVIEW_REQUIRE,
				EVENT_TYPE_TASK_REVIEW_REQUIRE,
				EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE,
				EVENT_TYPE_PERMISSION_REQUIRE,
				EVENT_TYPE_REQUIRE_USER_INTERACTIVE,
				EVENT_TYPE_TOOL_CALL_WATCHER,
				EVENT_TYPE_REVIEW_RELEASE:
				if id, ok := i["id"].(string); ok {
					return id
				}
			}
		}
	}
	return ""
}

func (e *Event) String() string {
	var parts []string

	if e.CoordinatorId != "" {
		parts = append(parts, fmt.Sprintf("id: %s", utils.ShrinkString(e.CoordinatorId, 10)))
	}
	if e.Type != "" {
		parts = append(parts, fmt.Sprintf("[type:%s]", e.Type))
	}
	if e.NodeId != "" {
		parts = append(parts, fmt.Sprintf("[node:%v]", e.NodeId))
	}
	if e.IsSystem {
		parts = append(parts, "system:true")
	}
	if e.IsStream {
		parts = append(parts, "stream:true")
	}
	if e.IsReason {
		parts = append(parts, "reason:true")
	}
	if len(e.StreamDelta) > 0 {
		parts = append(parts, fmt.Sprintf("delta:%v", string(e.StreamDelta)))
	}
	if e.IsJson {
		parts = append(parts, "json:true")
	}
	if len(e.Content) > 0 {
		parts = append(parts, fmt.Sprintf("data:%s", string(e.Content)))
	}

	return fmt.Sprintf("event: %s", strings.Join(parts, ", "))
}

type eventWriteProducer struct {
	isReason        bool
	isSystem        bool
	disableMarkdown bool
	coordinatorId   string
	nodeId          string
	taskIndex       string
	handler         func(event *Event)
	timeStamp       int64
}

func (e *eventWriteProducer) Write(b []byte) (int, error) {
	if e.handler == nil {
		log.Error("eventWriteProducer: Event handler is nil")
		return 0, nil
	}

	if len(b) == 0 {
		return 0, nil
	}

	event := &Event{
		CoordinatorId:   e.coordinatorId,
		NodeId:          e.nodeId,
		Type:            EVENT_TYPE_STREAM,
		IsSystem:        e.isSystem,
		IsReason:        e.isReason,
		IsStream:        true,
		StreamDelta:     utils.CopyBytes(b),
		Timestamp:       e.timeStamp, // the event in same stream should have the same timestamp
		TaskIndex:       e.taskIndex,
		DisableMarkdown: e.disableMarkdown,
	}
	e.handler(event)
	return len(b), nil
}

func (r *Config) emitJson(typeName EventType, nodeId string, i any) {
	event := &Event{
		CoordinatorId: r.id,
		Type:          typeName,
		NodeId:        nodeId,
		IsJson:        true,
		Content:       utils.Jsonify(i),
		Timestamp:     time.Now().Unix(),
	}
	r.emit(event)
}

func (r *Config) EmitStatus(key string, value any) {
	r.EmitStructured("status", map[string]any{
		"key":   key,
		"value": value,
	})
}

func (r *Config) EmitStructured(nodeId string, i any) {
	r.emitJson(EVENT_TYPE_STRUCTURED, nodeId, i)
}

func (r *Config) EmitRequirePermission(title string, description ...string) {
	reqs := map[string]any{
		"title":       title,
		"description": description,
	}
	r.emitJson(EVENT_TYPE_PERMISSION_REQUIRE, "permission", reqs)
}

func (r *Config) EmitRequireReviewForTask(task *aiTask, id string) {
	reqs := map[string]any{
		"id":            id,
		"selectors":     TaskReviewSuggestions,
		"task":          task,
		"short_summary": task.ShortSummary,
		"long_summary":  task.LongSummary,
	}
	if ep, ok := r.epm.loadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.submitCheckpointRequest(ep.checkpoint, reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	r.emitInteractiveJson(id, EVENT_TYPE_TASK_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) EmitRequireReviewForPlan(rsp *PlanResponse, id string) {
	reqs := map[string]any{
		"id":        id,
		"selectors": r.getPlanReviewSuggestion(),
		"plans":     rsp,
	}
	if ep, ok := r.epm.loadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.submitCheckpointRequest(ep.checkpoint, reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	r.emitInteractiveJson(id, EVENT_TYPE_PLAN_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) EmitRequireReviewForToolUse(tool *aitool.Tool, params aitool.InvokeParams, id string) {
	reqs := map[string]any{
		"id":               id,
		"selectors":        ToolUseReviewSuggestions,
		"tool":             tool.Name,
		"tool_description": tool.Description,
		"params":           params,
	}
	if ep, ok := r.epm.loadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.submitCheckpointRequest(ep.checkpoint, reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	r.emitInteractiveJson(id, EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) emitInteractiveJson(id string, typeName EventType, nodeId string, i any) {
	event := &Event{
		CoordinatorId: r.id,
		Type:          typeName,
		NodeId:        nodeId,
		IsJson:        true,
		Content:       utils.Jsonify(i),
		Timestamp:     time.Now().Unix(),
	}
	r.memory.StoreInteractiveEvent(id, event)
	r.emit(event)
}

func (r *Config) emitInteractiveRelease(eventId string, invokeParams aitool.InvokeParams) {
	release := map[string]any{
		"id":     eventId,
		"params": invokeParams,
	}
	event := &Event{
		CoordinatorId: r.id,
		Type:          EVENT_TYPE_REVIEW_RELEASE,
		NodeId:        "review-release",
		IsJson:        true,
		Content:       utils.Jsonify(release),
	}
	r.emit(event)
}

func (r *Config) emitLogWithLevel(level, name, fmtlog string, items ...any) {
	message := fmtlog
	if len(items) > 0 {
		message = fmt.Sprintf(fmtlog, items...)
	}

	nodeName := name
	if name == "" {
		nodeName = level
	}

	r.EmitStructured(nodeName, map[string]any{
		"level":   level,
		"message": message,
	})
}

func (r *Config) EmitWarningWithName(name string, fmtlog string, items ...any) {
	r.emitLogWithLevel("warning", name, fmtlog, items...)
}

func (r *Config) EmitInfoWithName(name string, fmtlog string, items ...any) {
	r.emitLogWithLevel("info", name, fmtlog, items...)
}

func (r *Config) EmitErrorWithName(name string, fmtlog string, items ...any) {
	r.emitLogWithLevel("error", name, fmtlog, items...)
}

func (r *Config) EmitToolCallWatcher(toolCallID string, id string, tool *aitool.Tool, params aitool.InvokeParams) {
	reqs := map[string]any{
		"call_tool_id":     toolCallID,
		"id":               id,
		"tool":             tool.Name,
		"tool_description": tool.Description,
		"params":           params,
		"selectors":        ToolCallWatcher,
	}
	r.emitInteractiveJson(id, EVENT_TYPE_TOOL_CALL_WATCHER, "review-require", reqs)
}

func (r *Config) EmitToolCallStart(callToolId string, tool *aitool.Tool) {
	r.emitJson(EVENT_TOOL_CALL_START, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"tool": map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
		},
	})
}

func (r *Config) EmitToolCallStatus(callToolId string, status string) {
	r.emitJson(EVENT_TOOL_CALL_STATUS, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"status":       status,
	})
}

func (r *Config) EmitToolCallDone(callToolId string) {
	r.emitJson(EVENT_TOOL_CALL_DONE, callToolId, map[string]any{
		"call_tool_id": callToolId,
	})
}

func (r *Config) EmitToolCallError(callToolId string, err any) {
	r.emitJson(EVENT_TOOL_CALL_ERROR, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"error":        fmt.Sprintf("%v", err),
	})
}

func (r *Config) EmitToolCallUserCancel(callToolId string) {
	r.emitJson(EVENT_TOOL_CALL_USER_CANCEL, callToolId, map[string]any{
		"call_tool_id": callToolId,
	})
}

func (r *Config) EmitToolCallStd(toolName string, stdOut, stdErr io.Reader, taskIndex string) {
	startTime := time.Now()
	r.EmitStreamEventEx(fmt.Sprintf("tool-%v-stdout", toolName), startTime, stdOut, taskIndex, true)
	r.EmitStreamEventEx(fmt.Sprintf("tool-%v-stderr", toolName), startTime, stdErr, taskIndex, true)
}

func (r *Config) EmitStreamEvent(nodeId string, startTime time.Time, reader io.Reader, taskIndex string) {
	r.EmitStreamEventEx(nodeId, startTime, reader, taskIndex, false)
}

func (r *Config) EmitStreamEventEx(nodeId string, startTime time.Time, reader io.Reader, taskIndex string, disableMarkdown bool) {
	r.emitExStreamEvent(&streamEvent{
		disableMarkdown: disableMarkdown,
		startTime:       startTime,
		isSystem:        false,
		isReason:        false,
		reader:          reader,
		nodeId:          nodeId,
		taskIndex:       taskIndex,
	})
}

func (r *Config) EmitSystemStreamEvent(nodeId string, startTime time.Time, reader io.Reader, taskIndex string) {
	r.emitExStreamEvent(&streamEvent{
		startTime: startTime,
		isSystem:  true,
		isReason:  false,
		reader:    reader,
		nodeId:    nodeId,
		taskIndex: taskIndex,
	})
}

func (r *Config) EmitReasonStreamEvent(nodeId string, startTime time.Time, reader io.Reader, taskIndex string) {
	r.emitExStreamEvent(&streamEvent{
		startTime: startTime,
		isSystem:  false,
		isReason:  true,
		reader:    reader,
		nodeId:    nodeId,
		taskIndex: taskIndex,
	})
}

func (r *Config) EmitSystemReasonStreamEvent(nodeId string, startTime time.Time, reader io.Reader, taskIndex string) {
	r.emitExStreamEvent(&streamEvent{
		startTime: startTime,
		isSystem:  true,
		isReason:  true,
		reader:    reader,
		nodeId:    nodeId,
		taskIndex: taskIndex,
	})
}

func (r *Config) EmitCurrentConfigInfo() {
	r.emitJson(EVENT_TYPE_AID_CONFIG, "system", r.SimpleInfoMap())
}

func (r *Config) EmitInfo(fmtlog string, items ...any) {
	r.emitLogWithLevel("info", "system", fmtlog, items...)
}

func (r *Config) EmitPushTask(task *aiTask) {
	r.EmitStructured("system", map[string]any{
		"type": "push_task",
		"task": map[string]any{
			"index": task.Index,
			"name":  task.Name,
			"goal":  task.Goal,
		},
	})
}

func (r *Config) EmitPopTask(task *aiTask) {
	r.EmitStructured("system", map[string]any{
		"type": "pop_task",
		"task": map[string]any{
			"index": task.Index,
			"name":  task.Name,
			"goal":  task.Goal,
		},
	})
}

func (r *Config) EmitRiskControlPrompt(id string, result *RiskControlResult) {
	r.emitJson(EVENT_TYPE_RISK_CONTROL_PROMPT, `risk-control`, map[string]any{
		"id":     id,
		"score":  result.Score,
		"reason": result.Reason,
	})
}

func (r *Config) EmitUpdateTaskStatus(task *aiTask) {
	r.EmitStructured("system", map[string]any{
		"type": "update_task_status",
		"task": map[string]any{
			"index":        task.Index,
			"name":         task.Name,
			"goal":         task.Goal,
			"summary":      task.ShortSummary,
			"long_summary": task.LongSummary,
			"executing":    task.executing,
			"executed":     task.executed,
		},
	})
}

func (r *Config) EmitWarning(fmtlog string, items ...any) {
	r.emitLogWithLevel("warning", "system", fmtlog, items...)
}

func (r *Config) EmitError(fmtlog string, items ...any) {
	r.emitLogWithLevel("error", "system", fmtlog, items...)
}

func (r *Config) EmitPrompt(step string, prompt string) {
	r.EmitStructured("prompt", map[string]any{
		"system": false,
		"step":   step,
		"prompt": prompt,
	})
}

func (r *Config) EmitSystemPrompt(step string, prompt string) {
	r.EmitStructured("prompt", map[string]any{
		"system": true,
		"step":   step,
		"prompt": prompt,
	})
}

type streamEvent struct {
	startTime       time.Time
	isSystem        bool
	isReason        bool
	reader          io.Reader
	nodeId          string
	taskIndex       string
	disableMarkdown bool
}

func (r *Config) emitExStreamEvent(e *streamEvent) {
	r.streamWaitGroup.Add(1)
	go func() {
		defer r.streamWaitGroup.Done()

		io.Copy(&eventWriteProducer{
			coordinatorId:   r.id,
			nodeId:          e.nodeId,
			disableMarkdown: e.disableMarkdown,
			isSystem:        e.isSystem,
			isReason:        e.isReason,
			handler:         r.emit,
			timeStamp:       e.startTime.Unix(),
			taskIndex:       e.taskIndex,
		}, e.reader)
	}()
	return
}

func (r *Config) WaitForStream() {
	r.streamWaitGroup.Wait()
}

type SyncType string

const (
	SYNC_TYPE_PLAN        SyncType = "plan"
	SYNC_TYPE_CONSUMPTION SyncType = "consumption"
	SYNC_TYPE_PING        SyncType = "ping"
)

func ParseSyncType(s string) (SyncType, bool) {
	for _, t := range []SyncType{
		SYNC_TYPE_PLAN, SYNC_TYPE_CONSUMPTION, SYNC_TYPE_PING,
	} {
		if string(t) == s {
			return t, true
		}
	}
	return "", false
}

type InputEvent struct {
	Id string

	// 是否是同步信息
	IsSyncInfo bool
	// 同步类型 一般认为有 plan consumption
	SyncType SyncType

	IsInteractive bool
	Params        aitool.InvokeParams
}
