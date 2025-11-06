package aicommon

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type BaseEmitter func(e *schema.AiOutputEvent) error
type EventProcesser func(e *schema.AiOutputEvent) *schema.AiOutputEvent
type Emitter struct {
	streamWG              *sync.WaitGroup
	id                    string
	baseEmitter           BaseEmitter
	eventProcesserStack   *utils.Stack[EventProcesser]
	interactiveEventSaver func(string, *schema.AiOutputEvent)
}

func (i *Emitter) Emit(e *schema.AiOutputEvent) error {
	return i.emit(e)
}

func (i *Emitter) AssociativeAIProcess(newProcess *schema.AiProcess) *Emitter {
	err := yakit.CreateAIProcess(consts.GetGormProjectDatabase(), newProcess)
	if err != nil {
		return nil
	}
	callBack := func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if newProcess.ProcessType == schema.AI_Call_Tool {
			event.CallToolID = newProcess.ProcessId
		}
		event.Processes = append(event.Processes, newProcess)
		return event
	}

	// push event processer to new emitter
	return i.PushEventProcesser(callBack)
}

func (i *Emitter) PushEventProcesser(newHandler EventProcesser) *Emitter {
	// copy emitter(deepcopy with event processer stack)
	var copyEmitter = new(Emitter)
	*copyEmitter = *i
	copyEmitter.eventProcesserStack = utils.NewStack[EventProcesser]()
	if i.eventProcesserStack != nil && i.eventProcesserStack.Len() > 0 {
		for j := 0; j < i.eventProcesserStack.Len(); j++ {
			copyEmitter.eventProcesserStack.Push(i.eventProcesserStack.PeekN(j))
		}
	}
	copyEmitter.eventProcesserStack.Push(newHandler)
	return copyEmitter
}

func (i *Emitter) PopEventProcesser() *Emitter {
	var copyEmitter = new(Emitter)
	*copyEmitter = *i
	copyEmitter.eventProcesserStack = utils.NewStack[EventProcesser]()

	// copy event processer stack
	if i.eventProcesserStack != nil && i.eventProcesserStack.Len() > 0 {
		for j := 0; j < i.eventProcesserStack.Len(); j++ {
			copyEmitter.eventProcesserStack.Push(i.eventProcesserStack.PeekN(j))
		}
	}
	// pop event processer
	copyEmitter.eventProcesserStack.Pop()
	return copyEmitter
}

func (i *Emitter) callEventBeforeSave(event *schema.AiOutputEvent) *schema.AiOutputEvent {
	if i.eventProcesserStack == nil || i.eventProcesserStack.Len() == 0 {
		return event
	}
	i.eventProcesserStack.ForeachStack(func(f EventProcesser) bool {
		event = f(event)
		return true
	})
	return event
}

func (i *Emitter) emit(e *schema.AiOutputEvent) (retErr error) {
	if err := recover(); err != nil {
		retErr = utils.Errorf("Emitter panic: %v", err)
		_ = retErr
	}
	if i.eventProcesserStack != nil {
		e = i.callEventBeforeSave(e)
	}
	if i.baseEmitter != nil {
		if err := i.baseEmitter(e); err != nil {
			return utils.Errorf("emit event failed: %v", err)
		}
	}
	return nil
}

func (i *Emitter) StoreInteractiveEvent(id string, e *schema.AiOutputEvent) {
	if i.interactiveEventSaver != nil {
		i.interactiveEventSaver(id, e)
	}
}

func (i *Emitter) WaitForStream() {
	i.streamWG.Wait()
}

func (r *Emitter) SetInteractiveEventSaver(saver func(string, *schema.AiOutputEvent)) {
	r.interactiveEventSaver = saver
}

func NewEmitter(id string, emitter BaseEmitter) *Emitter {
	return &Emitter{
		streamWG:    &sync.WaitGroup{},
		id:          id,
		baseEmitter: emitter,
	}
}

// NewDummyEmitter emit sends an AI output event using the emitter's function
func NewDummyEmitter() *Emitter {
	return NewEmitter(uuid.New().String(), nil)
}

func (r *Emitter) EmitJSON(typeName schema.EventType, id string, i any) {
	event := &schema.AiOutputEvent{
		CoordinatorId: r.id,
		Type:          typeName,
		NodeId:        id,
		IsJson:        true,
		Content:       utils.Jsonify(i),
		Timestamp:     time.Now().Unix(),
	}
	r.emit(event)
}

func (r *Emitter) EmitYakitExecResult(exec *ypb.ExecResult) {
	if exec == nil {
		return
	}
	r.EmitJSON(schema.EVENT_TYPE_YAKIT_EXEC_RESULT, "yakit", exec)
}

func (r *Emitter) EmitSchema(nodeId string, i any) {
	r.EmitJSON(schema.EVENT_TYPE_STRUCTURED, nodeId, i)
}

func (r *Emitter) EmitStatus(key string, value any) {
	r.EmitStructured("status", map[string]any{
		"key":   key,
		"value": value,
	})
}

func (r *Emitter) EmitThoughtStream(taskId string, fmtTpl string, item ...any) {
	content := fmtTpl
	if item != nil && len(item) > 0 {
		content = fmt.Sprintf(fmtTpl, item...)
	}
	r.EmitTextStreamWithTaskIndex("re-act-loop-thought", content, taskId)
}

func (r *Emitter) EmitThoughtStreamReader(taskId string, rd io.Reader, finished ...func()) {
	r.EmitStreamEvent("re-act-loop-thought", time.Now(), rd, taskId, finished...)
}

func (r *Emitter) EmitThoughtTypeWriterStreamReader(taskId string, origin io.Reader, finished ...func()) {
	pr, pw := utils.NewPipe()
	go func() {
		defer func() {
			pw.Close()
		}()
		TypeWriterCopy(pw, origin, 200)
	}()
	r.EmitThoughtStreamReader(taskId, pr, finished...)
}

func (r *Emitter) EmitTextStreamWithTaskIndex(nodeId string, content string, taskIndex string) {
	writer := utils.UTF8Reader(bytes.NewBufferString(content))
	pr, pw := utils.NewPipe()
	go func() {
		defer pw.Close()
		_, _ = TypeWriterCopy(pw, writer, 200)
	}()
	r.EmitStreamEvent(
		nodeId,
		time.Now(),
		utils.UTF8Reader(pr),
		taskIndex,
	)
}

func (r *Emitter) EmitStructured(id string, i any) {
	r.EmitJSON(schema.EVENT_TYPE_STRUCTURED, id, i)
}

func (r *Emitter) EmitRequirePermission(title string, description ...string) {
	reqs := map[string]any{
		"title":       title,
		"description": description,
	}
	r.EmitJSON(schema.EVENT_TYPE_PERMISSION_REQUIRE, "permission", reqs)
}

func (r *Emitter) EmitInteractiveJSON(id string, typeName schema.EventType, nodeId string, i any) {
	event := &schema.AiOutputEvent{
		CoordinatorId: r.id,
		Type:          typeName,
		NodeId:        nodeId,
		IsJson:        true,
		Content:       utils.Jsonify(i),
		Timestamp:     time.Now().Unix(),
	}
	r.StoreInteractiveEvent(id, event)
	r.emit(event)
}

func (r *Emitter) EmitInteractiveRelease(id string, invokeParams aitool.InvokeParams) {
	release := map[string]any{
		"id":     id,
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

func (r *Emitter) EmitLogWithLevel(level, name, fmtlog string, items ...any) {
	message := fmtlog
	if len(items) > 0 {
		message = fmt.Sprintf(fmtlog, items...)
	}

	nodeName := name
	if name == "" {
		nodeName = level
	}

	switch level {
	case "info":
		log.Info(message)
	case "warning":
		log.Warn(message)
	case "error":
		log.Error(message)
	}

	r.EmitStructured(nodeName, map[string]any{
		"level":   level,
		"message": message,
	})
}

func (r *Emitter) EmitWarningWithName(name string, fmtlog string, items ...any) {
	r.EmitLogWithLevel("warning", name, fmtlog, items...)
}

func (r *Emitter) EmitInfoWithName(name string, fmtlog string, items ...any) {
	r.EmitLogWithLevel("info", name, fmtlog, items...)
}

func (r *Emitter) EmitErrorWithName(name string, fmtlog string, items ...any) {
	r.EmitLogWithLevel("error", name, fmtlog, items...)
}

var ToolCallWatcher = []map[string]any{
	{
		"value":          "enough-cancel",
		"prompt":         "跳过",
		"prompt_english": "Tool output is sufficient, can cancel tool execution and continue with the next task",
	},
}

func (r *Emitter) EmitToolCallWatcher(toolCallID string, id string, tool *aitool.Tool, params aitool.InvokeParams) {
	reqs := map[string]any{
		"call_tool_id":     toolCallID,
		"id":               id,
		"tool":             tool.Name,
		"tool_description": tool.Description,
		"params":           params,
		"selectors":        ToolCallWatcher,
	}
	r.EmitInteractiveJSON(id, schema.EVENT_TYPE_TOOL_CALL_WATCHER, "review-require", reqs)
}

func (r *Emitter) EmitToolCallStart(callToolId string, tool *aitool.Tool) {
	r.EmitJSON(schema.EVENT_TOOL_CALL_START, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"tool": map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
		},
	})
}

func (r *Emitter) EmitToolCallStatus(callToolId string, status string) {
	r.EmitJSON(schema.EVENT_TOOL_CALL_STATUS, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"status":       status,
	})
}

func (r *Emitter) EmitToolCallDone(callToolId string) {
	r.EmitJSON(schema.EVENT_TOOL_CALL_DONE, callToolId, map[string]any{
		"call_tool_id": callToolId,
	})
}

func (r *Emitter) EmitToolCallError(callToolId string, err any) {
	r.EmitJSON(schema.EVENT_TOOL_CALL_ERROR, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"error":        fmt.Sprintf("%v", err),
	})
}

func (r *Emitter) EmitToolCallUserCancel(callToolId string) {
	r.EmitJSON(schema.EVENT_TOOL_CALL_USER_CANCEL, callToolId, map[string]any{
		"call_tool_id": callToolId,
	})
}

func (r *Emitter) EmitToolCallSummary(callToolId string, summary string) {
	r.EmitJSON(schema.EVENT_TOOL_CALL_SUMMARY, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"summary":      summary,
	})
}

func (r *Emitter) EmitToolCallDecision(callToolId string, action string, summary string) {
	r.EmitJSON(schema.EVENT_TOOL_CALL_DECISION, callToolId, map[string]any{
		"call_tool_id": callToolId,
		"action":       action,
		"i18n":         schema.GetActionI18n(action),
		"summary":      summary,
	})
}

func (r *Emitter) EmitToolCallStd(toolName string, stdOut, stdErr io.Reader, taskIndex string) {
	r.EmitStreamEventWithContentType(fmt.Sprintf("tool-%v-stdout", toolName), stdOut, taskIndex, "log/tool")
	r.EmitStreamEventWithContentType(fmt.Sprintf("tool-%v-stderr", toolName), stdErr, taskIndex, "log/tool")
}

func (r *Emitter) EmitStreamEvent(nodeId string, startTime time.Time, reader io.Reader, taskIndex string, finishCallback ...func()) {
	r.EmitStreamEventEx(nodeId, startTime, reader, taskIndex, false, finishCallback...)
}

func (r *Emitter) EmitTextPlainTextStreamEvent(
	nodeId string,
	reader io.Reader,
	taskIndex string,
	finishCallback ...func(),
) {
	r.EmitStreamEventWithContentType(nodeId, reader, taskIndex, "text/plain", finishCallback...)
}

func (r *Emitter) EmitTextMarkdownStreamEvent(
	nodeId string,
	reader io.Reader,
	taskIndex string,
	finishCallback ...func(),
) {
	r.EmitStreamEventWithContentType(nodeId, reader, taskIndex, "text/markdown", finishCallback...)
}

func (r *Emitter) EmitYaklangCodeStreamEvent(nodeId string, reader io.Reader, taskIndex string, finishCallback ...func()) {
	r.EmitStreamEventWithContentType(nodeId, reader, taskIndex, "code/yaklang", finishCallback...)
}

func (r *Emitter) EmitHTTPRequestStreamEvent(nodeId string, reader io.Reader, taskIndex string, finishCallback ...func()) {
	r.EmitStreamEventWithContentType(nodeId, reader, taskIndex, "code/http-request", finishCallback...)
}

func (r *Emitter) EmitDefaultStreamEvent(nodeId string, reader io.Reader, taskIndex string, finishCallback ...func()) {
	r.emitStreamEvent(&streamEvent{
		disableMarkdown:    true,
		startTime:          time.Now(),
		isSystem:           false,
		isReason:           false,
		reader:             reader,
		nodeId:             nodeId,
		contentType:        "",
		taskIndex:          taskIndex,
		emitFinishCallback: finishCallback,
	})
}

func (r *Emitter) EmitStreamEventWithContentType(nodeId string, reader io.Reader, taskIndex string, contentType string, finishCallback ...func()) {
	r.emitStreamEvent(&streamEvent{
		disableMarkdown:    true,
		startTime:          time.Now(),
		isSystem:           false,
		isReason:           false,
		reader:             reader,
		nodeId:             nodeId,
		contentType:        contentType,
		taskIndex:          taskIndex,
		emitFinishCallback: finishCallback,
	})
}

func (r *Emitter) EmitStreamEventEx(nodeId string, startTime time.Time, reader io.Reader, taskIndex string, disableMarkdown bool, finishCallback ...func()) {
	reader = utils.UTF8Reader(reader)

	r.emitStreamEvent(&streamEvent{
		disableMarkdown:    disableMarkdown,
		startTime:          startTime,
		isSystem:           false,
		isReason:           false,
		reader:             reader,
		nodeId:             nodeId,
		taskIndex:          taskIndex,
		emitFinishCallback: finishCallback,
	})
}

func (r *Emitter) EmitSystemStreamEvent(nodeId string, startTime time.Time, reader io.Reader, taskIndex string, finishCallback ...func()) {
	r.emitStreamEvent(&streamEvent{
		startTime:          startTime,
		isSystem:           true,
		isReason:           false,
		reader:             reader,
		nodeId:             nodeId,
		taskIndex:          taskIndex,
		emitFinishCallback: finishCallback,
	})
}

func (r *Emitter) EmitSystemReasonStreamEvent(nodeId string, startTime time.Time, reader io.Reader, taskIndex string, finishCallback ...func()) {
	r.emitStreamEvent(&streamEvent{
		startTime:          startTime,
		isSystem:           true,
		isReason:           true,
		reader:             reader,
		nodeId:             nodeId,
		taskIndex:          taskIndex,
		emitFinishCallback: finishCallback,
	})
}

func (r *Emitter) EmitReasonStreamEvent(nodeId string, startTime time.Time, reader io.Reader, taskIndex string, finishCallback ...func()) {
	r.emitStreamEvent(&streamEvent{
		startTime:          startTime,
		isSystem:           false,
		isReason:           true,
		reader:             reader,
		nodeId:             nodeId,
		taskIndex:          taskIndex,
		emitFinishCallback: finishCallback,
	})
}

func (r *Emitter) emitStreamEvent(e *streamEvent) {
	r.streamWG.Add(1)

	if e.contentType == "" {
		e.contentType = "default"
	}

	if e.startTime.IsZero() {
		e.startTime = time.Now()
	}
	startTS := e.startTime.Unix()

	ewid := ksuid.New().String()
	producer := newStreamAIOutputEventWriter(r.id, r.emit, startTS, ewid, e)

	go func() {
		defer r.streamWG.Done()
		defer func() {
			for _, f := range e.emitFinishCallback {
				if f == nil {
					continue
				}
				f()
			}
		}()
		n, _ := io.Copy(producer, e.reader)
		if n > 0 {
			du := time.Since(e.startTime)
			r.EmitStructured("stream-finished", map[string]any{
				"node_id":         e.nodeId,
				"coordinator_id":  r.id,
				"is_system":       e.isSystem,
				"is_reason":       e.isReason,
				"start_timestamp": startTS,
				"task_index":      e.taskIndex,
				"event_writer_id": ewid,
				"duration_ms":     du.Milliseconds(),
			})
		}
	}()
}

func (e *Emitter) EmitInfo(fmtlog string, items ...any) {
	e.EmitLogWithLevel("info", "system", fmtlog, items...)
}

func (e *Emitter) EmitWarning(fmtlog string, items ...any) {
	e.EmitLogWithLevel("warning", "system", fmtlog, items...)
}

func (e *Emitter) EmitError(fmtlog string, items ...any) {
	e.EmitLogWithLevel("error", "system", fmtlog, items...)
}

func (e *Emitter) EmitPrompt(step string, prompt string) {
	e.EmitStructured("prompt", map[string]any{
		"system": false,
		"step":   step,
		"prompt": prompt,
	})
}

func (e *Emitter) EmitSystemPrompt(step string, prompt string) {
	e.EmitStructured("prompt", map[string]any{
		"system": true,
		"step":   step,
		"prompt": prompt,
	})
}

// AI 推理过程通用方法

// EmitThought emits a thought event for AI reasoning process
func (e *Emitter) EmitThought(nodeId string, thought string) {
	e.EmitJSON(schema.EVENT_TYPE_THOUGHT, nodeId, map[string]any{
		"thought":   thought,
		"timestamp": time.Now().Unix(),
	})
}

// EmitAction emits an action event for AI execution
func (e *Emitter) EmitAction(nodeId string, action string, actionType string) {
	e.EmitJSON(schema.EVENT_TYPE_ACTION, nodeId, map[string]any{
		"action":      action,
		"action_type": actionType,
		"timestamp":   time.Now().Unix(),
	})
}

// EmitObservation emits an observation event for AI feedback
func (e *Emitter) EmitObservation(nodeId string, observation string, source string) {
	e.EmitJSON(schema.EVENT_TYPE_OBSERVATION, nodeId, map[string]any{
		"observation": observation,
		"source":      source, // "tool", "environment", "user", etc.
		"timestamp":   time.Now().Unix(),
	})
}

// EmitIteration emits an iteration event for AI reasoning loops
func (e *Emitter) EmitIteration(nodeId string, current int, max int, description string) {
	e.EmitJSON(schema.EVENT_TYPE_ITERATION, nodeId, map[string]any{
		"current":     current,
		"max":         max,
		"description": description,
		"timestamp":   time.Now().Unix(),
	})
}

// EmitResult emits a result event for AI final output
func (e *Emitter) EmitResult(nodeId string, result interface{}, success bool) {
	e.EmitJSON(schema.EVENT_TYPE_RESULT, nodeId, map[string]any{
		"result":       result,
		"success":      success,
		"finished":     true,
		"after_stream": false,
		"timestamp":    time.Now().Unix(),
	})
}

func (e *Emitter) EmitPinDirectory(path string) {
	e.EmitJSON(schema.EVENT_TYPE_FILESYSTEM_PIN_DIRECTORY, "filesystem", map[string]any{
		"path":      path,
		"timestamp": time.Now().Unix(),
	})
}

func (e *Emitter) EmitPinFilename(path string) {
	log.Infof("Emitting pin filename event for path: %s", path)
	e.EmitJSON(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME, "filesystem", map[string]any{
		"path":      path,
		"timestamp": time.Now().Unix(),
	})
}

// EmitResult emits a result event for AI final output
func (e *Emitter) EmitResultAfterStream(nodeId string, result interface{}, success bool) {
	e.EmitJSON(schema.EVENT_TYPE_RESULT, nodeId, map[string]any{
		"result":       result,
		"success":      success,
		"finished":     true,
		"after_stream": true,
		"timestamp":    time.Now().Unix(),
	})
}

// EmitKnowledge emits a knowledge enhancement data for AI processes
func (e *Emitter) EmitKnowledge(nodeId string, enhanceID string, result EnhanceKnowledge) {
	knowledgeMap := map[string]any{
		"Content":     result.GetContent(),
		"Source":      result.GetSource(),
		"Score":       result.GetScore(),
		"Type":        result.GetType(),
		"Title":       result.GetTitle(),
		"UUID":        result.GetUUID(),
		"ScoreMethod": result.GetScoreMethod(),
	}
	e.EmitJSON(schema.EVENT_TYPE_KNOWLEDGE, nodeId, map[string]any{
		"data":      knowledgeMap,
		"id":        enhanceID,
		"timestamp": time.Now().Unix(),
	})
}

func (e *Emitter) EmitKnowledgeListAboutTask(nodeId string, taskID string, results []EnhanceKnowledge) {
	knowledgeList := make([]map[string]any, 0, len(results))
	for _, result := range results {
		knowledgeMap := map[string]any{
			"Content":     result.GetContent(),
			"Source":      result.GetSource(),
			"Score":       result.GetScore(),
			"Type":        result.GetType(),
			"Title":       result.GetTitle(),
			"UUID":        result.GetUUID(),
			"ScoreMethod": result.GetScoreMethod(),
		}
		knowledgeList = append(knowledgeList, knowledgeMap)
	}
	e.EmitJSON(schema.EVENT_TYPE_TASK_ABOUT_KNOWLEDGE, nodeId, map[string]any{
		"task_id":   taskID,
		"data_list": knowledgeList,
		"timestamp": time.Now().Unix(),
	})

}
