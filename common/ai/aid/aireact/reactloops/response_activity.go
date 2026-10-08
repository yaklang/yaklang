package reactloops

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

var thinkingStatusNames = []actionStatusName{
	{"正在思考如何解决问题…", "Thinking through the problem…"},
	{"正在梳理线索…", "Connecting the clues…"},
	{"正在考虑下一步…", "Considering the next step…"},
	{"正在推敲可行的方案…", "Working through possible approaches…"},
	{"正在整理思路…", "Organizing my thoughts…"},
}

var waitingStatusNames = []actionStatusName{
	{"排队等待答复中…", "Waiting for a response…"},
	{"正在等待模型回应…", "Waiting for the model to respond…"},
	{"答复还在路上，稍等一下…", "The response is on its way…"},
	{"正在等候答复，请稍候…", "Awaiting a response, one moment…"},
}

// One concrete response owns its indicator. Retiring it before verification
// prevents late Reason/argument readers from overwriting execution statuses.
// Only display names are deduplicated; call identities and execution stay intact.
type responseActivity struct {
	mu              sync.Mutex
	loop            *ReActLoop
	allowed         map[string]bool
	closed          bool
	currentCall     string
	calls           map[string]string
	callOrder       []string
	tools           map[string][]string
	todoOperations  map[string]map[string]bool
	toolLabels      map[string]schema.I18n
	lastText        string
	lastReason      time.Time
	thinkingIndex   uint64
	waitingIndex    int
	streamClosers   []io.Closer
	stopWaiting     chan struct{}
	outputStarted   bool
	contentStarted  bool
	fieldStarted    bool
	general         *liveGeneralOutput
	headerSuccessor *responseActivity
}

func newResponseActivity(loop *ReActLoop, allowed []string) *responseActivity {
	if allowed == nil {
		allowed = loop.GetAllActionNames()
	}
	a := &responseActivity{loop: loop, allowed: make(map[string]bool, len(allowed)),
		calls: make(map[string]string), tools: make(map[string][]string), todoOperations: make(map[string]map[string]bool), toolLabels: make(map[string]schema.I18n),
		thinkingIndex: uint64(rand.Intn(len(thinkingStatusNames))), waitingIndex: rand.Intn(len(waitingStatusNames)), stopWaiting: make(chan struct{})}
	for _, name := range allowed {
		a.allowed[name] = true
	}
	return a
}

func (a *responseActivity) close() {
	a.mu.Lock()
	if !a.closed && !a.outputStarted {
		close(a.stopWaiting)
	}
	a.closed = true
	for _, stream := range a.streamClosers {
		_ = stream.Close()
	}
	a.streamClosers = nil
	a.mu.Unlock()
	a.general.close()
}

// Waiting describes absent output, not fabricated model reasoning. Its timer
// stops permanently on first data and is scoped to this response/context.
func (a *responseActivity) wait(ctx context.Context) {
	a.mu.Lock()
	if a.closed || a.outputStarted {
		a.mu.Unlock()
		return
	}
	index := a.waitingIndex
	a.emitWaiting(index)
	a.mu.Unlock()
	go func() {
		ticker := time.NewTicker(6 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.stopWaiting:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.mu.Lock()
				if a.closed || a.outputStarted {
					a.mu.Unlock()
					return
				}
				index = (index + 1) % len(waitingStatusNames)
				a.emitWaiting(index)
				a.mu.Unlock()
			}
		}
	}()
}

func (a *responseActivity) emitWaiting(index int) {
	label := waitingStatusNames[index]
	a.waitingIndex = index
	if a.lastText == label.zh {
		return
	}
	a.lastText = label.zh
	a.loop.UserStatus(label.zh, label.en, aicommon.WithStatusCode("response.waiting"))
}

// Header/request bookkeeping must not flash several random waiting messages
// before the same HTTP response produces its first byte.
func (a *responseActivity) inheritWaiting(previous *responseActivity) {
	if previous == nil {
		return
	}
	previous.mu.Lock()
	defer previous.mu.Unlock()
	if !previous.outputStarted {
		a.waitingIndex, a.lastText = previous.waitingIndex, previous.lastText
	}
}

func (a *responseActivity) startOutput() {
	if !a.outputStarted {
		a.outputStarted = true
		close(a.stopWaiting)
	}
}

func (a *responseActivity) content() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	a.startOutput()
	a.contentStarted = true
	const text = "正在生成回复正文…"
	if a.lastText == text {
		return
	}
	a.lastText = text
	a.loop.UserStatus(text, "Writing the response content…", aicommon.WithStatusCode("response.writing"))
}

// Text mode is generating an action envelope. Its bytes must not be presented
// as reply content while we wait for the first decoded action or display field.
func (a *responseActivity) envelope() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	a.startOutput()
	if a.contentStarted || a.fieldStarted || a.currentCall != "" {
		return
	}
	a.contentStarted = true
	a.lastText = "正在准备下一步操作…"
	a.loop.UserStatus(a.lastText, "Preparing the next action…", aicommon.WithStatusCode("response.preparing"))
}

func (a *responseActivity) prepare(id, name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || !a.allowed[name] {
		return
	}
	a.startOutput()
	if _, exists := a.calls[id]; !exists {
		a.callOrder = append(a.callOrder, id)
		a.currentCall = id
	} else if a.currentCall != id || a.calls[id] == name {
		return
	}
	a.calls[id] = name
	a.emitPreparing()
}

func (a *responseActivity) identifyCall(previous, id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if name, exists := a.calls[previous]; exists {
		if a.currentCall == previous {
			a.currentCall = id
		}
		a.calls[id] = name
		delete(a.calls, previous)
		a.tools[id] = a.tools[previous]
		delete(a.tools, previous)
		a.todoOperations[id] = a.todoOperations[previous]
		delete(a.todoOperations, previous)
		for i, key := range a.callOrder {
			if key == previous {
				a.callOrder[i] = id
			}
		}
	}
}

func containsStatusName(names []string, name string) bool {
	for _, existing := range names {
		if existing == name {
			return true
		}
	}
	return false
}

// Decoded key paths distinguish call metadata from arbitrary business params.
// Array indices are omitted by jsonextractor's parent paths.
func (a *responseActivity) field(id string, key string, value any, parents []string) {
	a.todoOperation(id, key, value, parents)
	name, ok := value.(string)
	if !ok || strings.TrimSpace(name) == "" {
		return
	}
	if key == "@action" && len(parents) == 0 {
		a.prepare(id, name)
		return
	}
	toolField := key == "directly_call_tool_name" && (len(parents) == 0 ||
		len(parents) == 1 && parents[0] == "next_action")
	groupField := key == "tool_name" && len(parents) == 1 && parents[0] == "directly_call_tool_params_group"
	if !toolField && !groupField {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	name = strings.TrimSpace(name)
	if containsStatusName(a.tools[id], name) {
		return
	}
	a.tools[id] = append(a.tools[id], name)
	if id == a.currentCall && a.calls[id] == schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL {
		a.emitPreparing()
	}
}

func (a *responseActivity) reason(data []byte) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	now := time.Now()
	a.startOutput()
	if a.currentCall != "" || a.contentStarted || a.fieldStarted {
		return
	}
	if !a.lastReason.IsZero() && now.Sub(a.lastReason) < 6*time.Second {
		return
	}
	label := thinkingStatusNames[a.thinkingIndex%uint64(len(thinkingStatusNames))]
	a.thinkingIndex++
	a.lastReason = now
	a.lastText = label.zh
	a.loop.UserStatus(label.zh, label.en, aicommon.WithStatusCode("reasoning.thinking"))
}

// The latest discovered call owns the indicator until fresh content or the next
// call arrives. Earlier argument readers may finish later without reclaiming it.
func (a *responseActivity) emitPreparing() {
	name := a.calls[a.currentCall]
	label := a.loop.statusNameForAction(name)
	var tools []aicommon.StatusTool
	zh, en := "正在准备："+label.zh, "Preparing: "+label.en
	if name == schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL {
		var zhTools, enTools []string
		for _, toolName := range a.tools[a.currentCall] {
			toolLabel := a.toolLabel(toolName)
			tools = append(tools, aicommon.StatusTool{Name: toolName, DisplayName: toolLabel.Zh,
				DisplayNameI18n: &toolLabel, State: aicommon.StatusStateRunning})
			zhTools, enTools = append(zhTools, toolLabel.Zh), append(enTools, toolLabel.En)
		}
		zh, en = "正在准备工具调用…", "Preparing a tool call…"
		if len(tools) > 0 {
			zh, en = "正在准备工具："+joinedStatusNames(zhTools, false), "Preparing tools: "+joinedStatusNames(enTools, true)
		}
	} else if name == nativeAdjustTodolistActionName {
		zh, en = "调整待办事项中…", "Updating the task list…"
	}
	code := "action.preparing"
	if len(a.callOrder) > 1 {
		code = "action.batch.preparing"
		zh += fmt.Sprintf("（第 %d 个调用）", len(a.callOrder))
		en += fmt.Sprintf(" (call %d)", len(a.callOrder))
	}
	if zh == a.lastText {
		return
	}
	a.lastText = zh
	a.loop.UserStatus(zh, en, aicommon.WithStatusCode(code), aicommon.WithStatusTools(tools...),
		aicommon.WithStatusProgress(int64(len(a.callOrder)), 0, "call"))
}

// Observe actual decoded field bytes, before a display prefix is added. An
// empty annotation must not stop the waiting indicator or create a fake stream.
func (a *responseActivity) displayField(id, field string) {
	var label actionStatusName
	var code string
	switch field {
	case "answer_payload", "answer":
		label, code = actionStatusName{"正在生成回复正文…", "Writing the response content…"}, "response.writing"
	case "summary":
		label, code = actionStatusName{"正在撰写总结…", "Writing the summary…"}, "response.writing"
	case "human_readable_thought":
		label, code = actionStatusName{"正在整理执行说明…", "Preparing an execution note…"}, "response.noting"
	case "cumulative_summary":
		label, code = actionStatusName{"正在整理本轮进展…", "Summarizing this step's progress…"}, "response.progress"
	default:
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || len(a.todoOperations[id]) > 0 || a.calls[id] == schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL {
		return
	}
	// Late fields from an earlier call cannot replace the newer call's status.
	if a.currentCall != "" && a.currentCall != id {
		return
	}
	a.startOutput()
	a.fieldStarted = true
	if name := a.calls[id]; name != "" {
		actionLabel := a.loop.statusNameForAction(name)
		label.zh += "（" + actionLabel.zh + "）"
		label.en += " (" + actionLabel.en + ")"
	}
	if a.lastText != label.zh {
		a.lastText = label.zh
		a.loop.UserStatus(label.zh, label.en, aicommon.WithStatusCode(code))
	}
}

type displayFieldReader struct {
	io.Reader
	onData func()
}

func (r *displayFieldReader) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	if r.onData != nil && strings.TrimSpace(string(data[:n])) != "" {
		r.onData()
		r.onData = nil
	}
	return n, err
}

// Count completed canonical operations as their closing bytes arrive. These
// are proposals only: execution still validates/applies the full delta once.
func (a *responseActivity) todoOperation(id, key string, value any, parents []string) {
	path := parents
	if len(path) > 0 && path[0] == "next_action" {
		path = path[1:]
	}
	if len(path) == 0 || path[0] != "todo_delta" {
		return
	}
	operation := ""
	if len(path) == 1 && key == "current" {
		if _, valid := value.(string); valid || value == nil {
			operation = "current"
		}
	} else if len(path) == 2 && (path[1] == "add" || path[1] == "update" || path[1] == "close") {
		index, indexErr := strconv.Atoi(key)
		if _, valid := value.(map[string]any); valid && indexErr == nil && index >= 0 {
			operation = path[1] + "/" + key
		}
	}
	if operation == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.currentCall != "" && a.currentCall != id {
		return
	}
	if id != "text" && a.calls[id] != nativeAdjustTodolistActionName {
		return
	}
	if a.todoOperations[id] == nil {
		a.todoOperations[id] = make(map[string]bool)
	}
	if a.todoOperations[id][operation] {
		return
	}
	a.todoOperations[id][operation] = true
	a.startOutput()
	a.fieldStarted = true
	count := len(a.todoOperations[id])
	a.lastText = fmt.Sprintf("调整待办事项中… 已准备 %d 项调整", count)
	a.loop.UserStatus(a.lastText, fmt.Sprintf("Updating the task list… %d changes prepared", count),
		aicommon.WithStatusCode("todo.preparing"), aicommon.WithStatusProgress(int64(count), 0, "todo_change"))
}

// Resolve each discovered tool once; successive fragments reuse its UI label.
func (a *responseActivity) toolLabel(name string) schema.I18n {
	if label, ok := a.toolLabels[name]; ok {
		return label
	}
	label := a.loop.StatusToolLabel(name)
	a.toolLabels[name] = label
	return label
}

func joinedStatusNames(names []string, english bool) string {
	const visibleLimit = 3
	separator := "，"
	if english {
		separator = ", "
	}
	if len(names) <= visibleLimit {
		return strings.Join(names, separator)
	}
	text := strings.Join(names[:visibleLimit], separator)
	if english {
		return text + fmt.Sprintf(" and %d more", len(names)-visibleLimit)
	}
	return text + fmt.Sprintf("等 %d 项", len(names))
}
