package aid

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type eventWriteProducer struct {
	isReason        bool
	isSystem        bool
	disableMarkdown bool
	coordinatorId   string
	nodeId          string
	taskIndex       string
	handler         func(event *schema.AiOutputEvent)
	timeStamp       int64
	eventWriterID   string
}

func (e *eventWriteProducer) Write(b []byte) (int, error) {
	if e.handler == nil {
		log.Error("eventWriteProducer: Event handler is nil")
		return 0, nil
	}

	if len(b) == 0 {
		return 0, nil
	}

	event := &schema.AiOutputEvent{
		CoordinatorId:   e.coordinatorId,
		NodeId:          e.nodeId,
		Type:            schema.EVENT_TYPE_STREAM,
		IsSystem:        e.isSystem,
		IsReason:        e.isReason,
		IsStream:        true,
		StreamDelta:     utils.CopyBytes(b),
		Timestamp:       e.timeStamp, // the event in same stream should have the same timestamp
		EventUUID:       e.eventWriterID,
		TaskIndex:       e.taskIndex,
		DisableMarkdown: e.disableMarkdown,
	}
	e.handler(event)
	return len(b), nil
}

func (r *Config) emitJson(typeName schema.EventType, nodeId string, i any) {
	event := &schema.AiOutputEvent{
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

func (r *Config) EmitStream(nodeId string, content string) {
	r.emit(&Event{
		CoordinatorId: r.id,
		Type:          EVENT_TYPE_STREAM,
		NodeId:        nodeId,
		IsJson:        true,
		IsStream:      true,
		StreamDelta:   []byte(content),
		Timestamp:     time.Now().Unix(),
	})
}

func (r *Config) EmitStructured(nodeId string, i any) {
	r.emitJson(schema.EVENT_TYPE_STRUCTURED, nodeId, i)
}

func (r *Config) EmitRequirePermission(title string, description ...string) {
	reqs := map[string]any{
		"title":       title,
		"description": description,
	}
	r.emitJson(schema.EVENT_TYPE_PERMISSION_REQUIRE, "permission", reqs)
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
	r.emitInteractiveJson(id, schema.EVENT_TYPE_TASK_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) EmitRequireReviewForPlan(rsp *PlanResponse, id string) {
	reqs := map[string]any{
		"id":        id,
		"selectors": r.getPlanReviewSuggestion(),
		"plans":     rsp,
		"plans_id":  uuid.New().String(),
	}
	if ep, ok := r.epm.loadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.submitCheckpointRequest(ep.checkpoint, reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	r.emitInteractiveJson(id, schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, "review-require", reqs)
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
	r.emitInteractiveJson(id, schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) emitInteractiveJson(id string, typeName schema.EventType, nodeId string, i any) {
	event := &schema.AiOutputEvent{
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
	event := &schema.AiOutputEvent{
		CoordinatorId: r.id,
		Type:          schema.EVENT_TYPE_REVIEW_RELEASE,
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
	r.emitInteractiveJson(id, schema.EVENT_TYPE_TOOL_CALL_WATCHER, "review-require", reqs)
}

func (r *Config) EmitToolCallStart(callToolId string, tool *aitool.Tool) {
	r.emitJson(schema.EVENT_TOOL_CALL_START, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"tool": map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
		},
	})
}

func (r *Config) EmitToolCallStatus(callToolId string, status string) {
	r.emitJson(schema.EVENT_TOOL_CALL_STATUS, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"status":       status,
	})
}

func (r *Config) EmitToolCallDone(callToolId string) {
	r.emitJson(schema.EVENT_TOOL_CALL_DONE, callToolId, map[string]any{
		"call_tool_id": callToolId,
	})
}

func (r *Config) EmitToolCallError(callToolId string, err any) {
	r.emitJson(schema.EVENT_TOOL_CALL_ERROR, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"error":        fmt.Sprintf("%v", err),
	})
}

func (r *Config) EmitToolCallUserCancel(callToolId string) {
	r.emitJson(schema.EVENT_TOOL_CALL_USER_CANCEL, callToolId, map[string]any{
		"call_tool_id": callToolId,
	})
}

func (r *Config) EmitToolCallSummary(callToolId string, summary string) {
	r.emitJson(schema.EVENT_TOOL_CALL_SUMMARY, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"summary":      summary,
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
	r.emitJson(schema.EVENT_TYPE_AID_CONFIG, "system", r.SimpleInfoMap())
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
	r.emitJson(schema.EVENT_TYPE_RISK_CONTROL_PROMPT, `risk-control`, map[string]any{
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
			eventWriterID:   ksuid.New().String(),
			taskIndex:       e.taskIndex,
		}, e.reader)
	}()
	return
}

func (c *Config) pushProcess(newProcess *schema.AiProcess) *Config {
	err := yakit.CreateAIProcess(consts.GetGormProjectDatabase(), newProcess)
	if err != nil {
		return nil
	}
	callBack := func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		event.Processes = append(event.Processes, newProcess)
		return event
	}
	return c.pushEventBeforeSave(callBack)
}

func (c *Config) pushEventBeforeSave(newHandler func(event *schema.AiOutputEvent) *schema.AiOutputEvent) *Config {
	var subConfig = new(Config)
	*subConfig = *c
	if subConfig.eventBeforeSave == nil {
		subConfig.eventBeforeSave = utils.NewStack[func(event *schema.AiOutputEvent) *schema.AiOutputEvent]()
	}
	subConfig.eventBeforeSave.Push(newHandler)
	return subConfig
}

func (c *Config) popEventBeforeSave() *Config {
	var subConfig = new(Config)
	*subConfig = *c
	if subConfig.eventBeforeSave == nil {
		return subConfig
	}
	subConfig.eventBeforeSave.Pop()
	return subConfig
}

func (c *Config) callEventBeforeSave(event *schema.AiOutputEvent) *schema.AiOutputEvent {
	if c.eventBeforeSave == nil || c.eventBeforeSave.Len() == 0 {
		return event
	}
	c.eventBeforeSave.ForeachStack(func(f func(e *schema.AiOutputEvent) *schema.AiOutputEvent) bool {
		event = f(event)
		return true
	})
	return event
}

func (r *Config) WaitForStream() {
	r.streamWaitGroup.Wait()
}

type SyncType string

const (
	SYNC_TYPE_PLAN          SyncType = "plan"
	SYNC_TYPE_CONSUMPTION   SyncType = "consumption"
	SYNC_TYPE_PING          SyncType = "ping"
	SYNC_TYPE_PROCESS_EVENT SyncType = "sync_process_event"

	ProcessID           string = "process_id"
	SyncProcessEeventID        = "sync_process_event_id"
)

func ParseSyncType(s string) (SyncType, bool) {
	for _, t := range []SyncType{
		SYNC_TYPE_PLAN, SYNC_TYPE_CONSUMPTION, SYNC_TYPE_PING, SYNC_TYPE_PROCESS_EVENT,
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
