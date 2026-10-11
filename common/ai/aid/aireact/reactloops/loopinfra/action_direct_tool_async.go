package loopinfra

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const actionStateDirectToolAsync = "async_directly_call_tool"

// This optional scheduling hint tolerates common model formatting mistakes.
// Unknown values keep the synchronous default instead of retrying the tool call.
// Only explicit affirmative values opt in; objects and other numbers do not.
func normalizeDirectToolAsync(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "t", "1", "yes", "on":
			return true
		}
	case float64:
		return value == 1
	case int:
		return value == 1
	case int64:
		return value == 1
	case json.Number:
		number, err := value.Float64()
		return err == nil && number == 1
	}
	return false
}

func directToolActionAsync(action *aicommon.Action) bool {
	// Read only the canonical root (or next_action wrapper), never flattened
	// nested tool parameters or sibling batch members.
	raw, _ := lookupCanonicalActionParam(action, actionStateDirectToolAsync)
	return normalizeDirectToolAsync(raw)
}

// Submission is not a settled tool result: do not mark execution, verify
// satisfaction, or retain the action/operator in a background callback.
func submitAsyncDirectTool(loop *reactloops.ReActLoop, name string, params aitool.InvokeParams,
	operator *reactloops.LoopActionHandlerOperator, opts ...aicommon.ToolCallerOption) {
	receipt, err := loop.StartAsyncToolCall(name, params, opts...)
	if err != nil {
		operator.Feedback(fmt.Sprintf("异步工具调用提交失败：%v。未注册新的后台执行；不会自动改为同步调用。", err))
		operator.Continue()
		return
	}
	feedback := fmt.Sprintf("异步工具调用已提交，以下为提交回执，尚未取得执行结果：\nexecute_id: %q\ntool_name: %q\nstate: %s\n"+
		"通过 manage_async_execution_runtime 的 inspect 查看输出和结果，wait 等待结束，stop 请求停止；确认所需执行结束后再结束任务。",
		receipt.ID, name, receipt.State)
	loop.GetInvoker().AddToTimeline("ASYNC_TOOL_SUBMITTED", feedback)
	operator.Feedback(feedback)
	operator.Continue()
}
