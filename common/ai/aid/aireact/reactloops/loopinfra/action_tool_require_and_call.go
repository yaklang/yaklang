package loopinfra

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// loopAction_toolRequireAndCall is the action-mode variant of require_tool.
// Both the native (function-call) and action (text/JSON) modes share the same
// semantics: require_tool only loads tool parameter schemas into the timeline
// (CACHE_TOOL_CALL). It does NOT generate parameters or execute any tool.
//
// After observing the loaded schema in the next response, the model constructs
// arguments itself and uses directly_call_tool (single or batch) to execute.
// If a tool's schema is already visible in CACHE_TOOL_CALL with complete
// parameters, the model should use directly_call_tool directly.
var loopAction_toolRequireAndCall = &reactloops.LoopAction{
	FunctionCallAction: nativeToolSchemaLoadAction,
	ActionType:         schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description: "加载工具参数 Schema 到 timeline（CACHE_TOOL_CALL）。不生成参数、不执行工具。" +
		"使用 tool_require_payload 加载单个工具，或 tool_require_calls 加载多个。" +
		"在下一轮响应中观察已加载的 Schema，自行构造参数并使用 directly_call_tool 执行。" +
		"如果 Schema 已在 CACHE_TOOL_CALL 中且参数完整，直接使用 directly_call_tool。" +
		"批量项只提供工具名、identifier 和 reason，严禁提供 params；严禁混用单调用和批量字段，也不要为了凑数量发明调用。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("选择单调用形式时填写；存在 tool_require_calls 时必须省略。只填写一个需要加载 Schema 的工具准确名称，严禁包含参数。格式：\n"+requireToolScalarOutputExampleJSON),
		),
		aitool.WithStringParam(
			"tool_call_reason",
			aitool.WithParam_Description(`可选。用简短短语说明这次调用具体做什么，例如"grep /api 路径寻找注入点"或"在 username 中注入 SQLi 并重放登录"。不要写前序总结或过渡语；仅当 human_readable_thought 已说明原因时省略。该内容会显示在工具调用卡片上。`),
		),
		requireToolBatchSchemaOption(),
	},
	OutputExamples: requireToolOutputExamples,
	ActionVerifier: verifyRequireToolSchemaLoad,
	// ActionHandler is shared with the native (function-call) variant. Both
	// modes load tool schemas into the timeline and return; neither generates
	// parameters nor executes tools.
	ActionHandler: loadToolSchemas,
}

// verifyRequireToolSchemaLoad mirrors verifyToolSchemaLoad but preserves the
// action-mode scalar streaming contract: when tool_require_payload is present,
// return immediately without waiting for the full response to finish parsing.
func verifyRequireToolSchemaLoad(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, nil)
	raw, exists := action.LookupParam("tool_require_payload")
	if !exists {
		raw = action.GetInvokeParams("next_action")["tool_require_payload"]
	}
	if raw == nil {
		return verifyToolSchemaLoad(loop, action)
	}
	name, valid := raw.(string)
	name = strings.TrimSpace(name)
	if !valid || name == "" {
		return utils.Error("tool_require_payload must be a non-empty tool name")
	}
	reactloops.MaybeWarnBashBeforeEdit(loop, name)
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, []string{name})
	return nil
}
