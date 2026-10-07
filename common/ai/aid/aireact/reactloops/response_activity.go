package reactloops

import (
	"context"
	"fmt"
	"io"
	"math/rand"
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
	actions         []string
	calls           map[string]string
	callOrder       []string
	tools           map[string][]string
	toolLabels      map[string]schema.I18n
	lastText        string
	lastReason      time.Time
	thinkingIndex   uint64
	waitingIndex    int
	streamClosers   []io.Closer
	stopWaiting     chan struct{}
	outputStarted   bool
	contentStarted  bool
	general         *liveGeneralOutput
	headerSuccessor *responseActivity
}

func newResponseActivity(loop *ReActLoop, allowed []string) *responseActivity {
	if allowed == nil {
		allowed = loop.GetAllActionNames()
	}
	a := &responseActivity{loop: loop, allowed: make(map[string]bool, len(allowed)),
		calls: make(map[string]string), tools: make(map[string][]string), toolLabels: make(map[string]schema.I18n),
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
	if a.contentStarted || len(a.actions) > 0 {
		return
	}
	a.contentStarted = true
	a.loop.UserStatus("正在回复…", "Responding…", aicommon.WithStatusCode("response.writing"))
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
	}
	a.calls[id] = name
	if !containsStatusName(a.actions, name) {
		a.actions = append(a.actions, name)
	}
	a.emitPreparing()
}

func (a *responseActivity) identifyCall(previous, id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if name, exists := a.calls[previous]; exists {
		a.calls[id] = name
		delete(a.calls, previous)
		a.tools[id] = a.tools[previous]
		delete(a.tools, previous)
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
	if !containsStatusName(a.tools[id], name) {
		a.tools[id] = append(a.tools[id], name)
	}
	if a.calls[id] == schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL {
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
	if len(a.actions) > 0 || a.contentStarted {
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

func (a *responseActivity) emitPreparing() {
	var zhNames, enNames []string
	var tools []aicommon.StatusTool
	for _, name := range a.actions {
		label := a.loop.statusNameForAction(name)
		if name == schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL {
			label = actionStatusName{zh: "工具", en: "tools"}
			var names []string
			// Preserve provider discovery order across distinct calls.
			for _, id := range a.callOrder {
				if a.calls[id] != name {
					continue
				}
				for _, tool := range a.tools[id] {
					if !containsStatusName(names, tool) {
						names = append(names, tool)
					}
				}
			}
			var zhTools, enTools []string
			for _, toolName := range names {
				toolLabel := a.toolLabel(toolName)
				tools = append(tools, aicommon.StatusTool{Name: toolName, DisplayName: toolLabel.Zh,
					DisplayNameI18n: &toolLabel, State: aicommon.StatusStateRunning})
				zhLabel, enLabel := toolLabel.Zh, toolLabel.En
				if zhLabel != toolName {
					zhLabel += "（" + toolName + "）"
				}
				if enLabel != toolName {
					enLabel += " (" + toolName + ")"
				}
				zhTools, enTools = append(zhTools, zhLabel), append(enTools, enLabel)
			}
			if len(names) > 0 {
				label.zh = joinedStatusNames(zhTools, false)
				label.en = joinedStatusNames(enTools, true)
			}
		}
		zhNames, enNames = append(zhNames, label.zh), append(enNames, label.en)
	}
	zh, en := "正在调用"+joinedStatusNames(zhNames, false), "Calling: "+joinedStatusNames(enNames, true)
	if zh == a.lastText {
		return
	}
	a.lastText = zh
	code := "action.preparing"
	if len(a.calls) > 1 {
		code = "action.batch.preparing"
	}
	a.loop.UserStatus(zh, en, aicommon.WithStatusCode(code), aicommon.WithStatusTools(tools...))
}

// Resolve each discovered tool once; successive fragments reuse its UI label.
func (a *responseActivity) toolLabel(name string) schema.I18n {
	if label, ok := a.toolLabels[name]; ok {
		return label
	}
	label := schema.I18n{Zh: name, En: name}
	if cfg := a.loop.GetConfig(); cfg != nil && cfg.GetAiToolManager() != nil {
		if tool, err := cfg.GetAiToolManager().GetToolByName(name); err == nil && tool != nil {
			label = tool.GetVerboseNameI18n()
		}
	}
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
