package aid

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type EventType string

const (
	EVENT_TYPE_STREAM             EventType = "stream"
	EVENT_TYPE_STRUCTURED         EventType = "structured"
	EVEMT_TYPE_PLAN               EventType = "plan"
	EVENT_TYPE_SELECT             EventType = "select"
	EVENT_TYPE_PERMISSION_REQUIRE EventType = "permission_require"
	EVENT_TYPE_REVIEW_REQUIRE     EventType = "review_require"
	EVENT_TYPE_INPUT              EventType = "input"
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

func (r *Config) EmitRequireReview(id string, extraSelectors ...*ReviewSuggestion) {
	reqs := map[string]any{
		"id":        id,
		"selectors": append(TaskReviewSuggestions, extraSelectors...),
	}
	r.emitJson(EVENT_TYPE_REVIEW_REQUIRE, "review-require", reqs)
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

func (r *Config) EmitStreamEvent(nodeId string, reader io.Reader) {
	r.emitExStreamEvent(nodeId, false, false, reader)
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

func (r *Config) EmitSystemStreamEvent(nodeId string, reader io.Reader) {
	r.emitExStreamEvent(nodeId, true, false, reader)
}

func (r *Config) EmitReasonStreamEvent(nodeId string, reader io.Reader) {
	r.emitExStreamEvent(nodeId, false, true, reader)
}

func (r *Config) EmitSystemReasonStreamEvent(nodeId string, reader io.Reader) {
	r.emitExStreamEvent(nodeId, true, true, reader)
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

func (r *Config) emitExStreamEvent(nodeId string, isSystem, isReason bool, reader io.Reader) {
	go func() {
		io.Copy(&eventWriteProducer{
			coordinatorId: r.id,
			nodeId:        nodeId,
			isSystem:      isSystem,
			isReason:      isReason,
			handler:       r.emit,
		}, reader)
	}()
	return
}

type InputEvent struct {
	Id     string
	Params aitool.InvokeParams
}
