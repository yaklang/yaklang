package aid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"strings"
	"time"
)

type EventType string

const (
	EVENT_TYPE_STREAM     EventType = "stream"
	EVENT_TYPE_STRUCTURED EventType = "structured"

	EVENT_TYPE_CONSUMPTION EventType = "consumption" // token consumption include `{"input_"}`
	EVENT_TYPE_PONG        EventType = "pong"        // ping response ping-pong is a check for alive item
	EVENT_TYPE_PRESSURE    EventType = "pressure"    // pressure for ai context percent

	EVENT_TYPE_REQUIRE_USER_INTERACTIVE = "require_user_interactive"

	// risk control prompt is the prompt for risk control
	// contains score, reason, and other information to help uesr interactivation
	EVENT_TYPE_RISK_CONTROL_PROMPT = "risk_control_prompt"

	EVENT_TYPE_PLAN                    EventType = "plan"
	EVENT_TYPE_SELECT                  EventType = "select"
	EVENT_TYPE_PERMISSION_REQUIRE      EventType = "permission_require"
	EVENT_TYPE_TASK_REVIEW_REQUIRE     EventType = "task_review_require"
	EVENT_TYPE_PLAN_REVIEW_REQUIRE     EventType = "plan_review_require"
	EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE EventType = "tool_use_review_require"

	EVENT_TYPE_REVIEW_RELEASE EventType = "review_release"

	EVENT_TYPE_INPUT EventType = "input"
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
				EVENT_TYPE_REQUIRE_USER_INTERACTIVE:
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
	isReason      bool
	isSystem      bool
	coordinatorId string
	nodeId        string
	handler       func(event *Event)
	timeStamp     int64
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
		CoordinatorId: e.coordinatorId,
		NodeId:        e.nodeId,
		Type:          EVENT_TYPE_STREAM,
		IsSystem:      e.isSystem,
		IsReason:      e.isReason,
		IsStream:      true,
		StreamDelta:   utils.CopyBytes(b),
		Timestamp:     e.timeStamp, // the event in same stream should have the same timestamp
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
	r.EmitStructured("status", utils.Jsonify(map[string]any{
		"key":   key,
		"value": value,
	}))
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
		"selectors": PlanReviewSuggestions,
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

func (r *Config) EmitToolCallStd(toolName string, stdOut, stdErr *bytes.Buffer) {
	startTime := time.Now()
	r.EmitStreamEvent(fmt.Sprintf("tool-%v-stdout", toolName), startTime, stdOut)
	r.EmitStreamEvent(fmt.Sprintf("tool-%v-stderr", toolName), startTime, stdErr)
}

func (r *Config) EmitStreamEvent(nodeId string, startTime time.Time, reader io.Reader) {
	r.emitExStreamEvent(&streamEvent{
		startTime: startTime,
		isSystem:  false,
		isReason:  false,
		reader:    reader,
		nodeId:    nodeId,
	})
}

func (r *Config) EmitSystemStreamEvent(nodeId string, startTime time.Time, reader io.Reader) {
	r.emitExStreamEvent(&streamEvent{
		startTime: startTime,
		isSystem:  true,
		isReason:  false,
		reader:    reader,
		nodeId:    nodeId,
	})
}

func (r *Config) EmitReasonStreamEvent(nodeId string, startTime time.Time, reader io.Reader) {
	r.emitExStreamEvent(&streamEvent{
		startTime: startTime,
		isSystem:  false,
		isReason:  true,
		reader:    reader,
		nodeId:    nodeId,
	})
}

func (r *Config) EmitSystemReasonStreamEvent(nodeId string, startTime time.Time, reader io.Reader) {
	r.emitExStreamEvent(&streamEvent{
		startTime: startTime,
		isSystem:  true,
		isReason:  true,
		reader:    reader,
		nodeId:    nodeId,
	})
}

func (r *Config) EmitInfo(fmtlog string, items ...any) {
	r.emitLogWithLevel("info", "system", fmtlog, items...)
}

func (r *Config) EmitPushTask(task *aiTask) {
	r.EmitStructured("system", map[string]any{
		"type": "push_task",
		"task": map[string]any{
			"name": task.Name,
			"goal": task.Goal,
		},
	})
}

func (r *Config) EmitPopTask(task *aiTask) {
	r.EmitStructured("system", map[string]any{
		"type": "pop_task",
		"task": map[string]any{
			"name": task.Name,
			"goal": task.Goal,
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
	startTime time.Time
	isSystem  bool
	isReason  bool
	reader    io.Reader
	nodeId    string
}

func (r *Config) emitExStreamEvent(e *streamEvent) {
	r.streamWaitGroup.Add(1)
	go func() {
		defer r.streamWaitGroup.Done()

		io.Copy(&eventWriteProducer{
			coordinatorId: r.id,
			nodeId:        e.nodeId,
			isSystem:      e.isSystem,
			isReason:      e.isReason,
			handler:       r.emit,
			timeStamp:     e.startTime.Unix(),
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
