package loopinfra

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const actionStateNativeDirectParams = "native_direct_params"

var nativeDirectToolAction = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
	Description: "执行业务工具；原生函数名是 directly_call_tool，不能用业务工具名称代替。" +
		"单调用填写 directly_call_tool_name 和 directly_call_tool_params；独立批次仍调用本函数，数组写在 arguments.directly_call_tool_calls，该字段不是函数名。" +
		"参数必须来自当前可见的完整 Schema；缺少定义先 require_tool，下一轮观察后再构参。参数错误按 Schema 修正，不委托其他模型生成参数。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam("directly_call_tool_name", aitool.WithParam_Description("单调用的业务工具名称；批次时省略。此名称不能作为原生 function.name。")),
		aitool.WithRawParam("directly_call_tool_params", map[string]any{"type": "object", "description": "单调用业务参数，字段必须遵循已加载的业务工具 Schema；无参数工具填 {}，批次时省略。长文本写入 JSON 字符串，不输出外置 AITAG。"}),
		aitool.WithStringParam("directly_call_reason", aitool.WithParam_Description("可选的单调用目的；批次时省略，各子调用分别填写目的。")),
		aitool.WithStructArrayParam("directly_call_tool_calls", []aitool.PropertyOption{
			aitool.WithParam_Description("执行低风险、独立且互不干扰的调用，每项参数来自完整 Schema；省略单调用名称和参数。各项使用不同 identifier 和明确目的，顶层目的不会复制给子调用。"),
			aitool.WithParam_Raw("minItems", 2), aitool.WithParam_Raw("maxItems", aicommon.DefaultToolBatchMaxCalls),
		}, nil,
			aitool.WithStringParam("tool_name", aitool.WithParam_Required(true), aitool.WithParam_Description("业务工具准确名称。")),
			aitool.WithRawParam("params", map[string]any{"type": "object", "additionalProperties": true}, aitool.WithParam_Required(true), aitool.WithParam_Description("遵循该业务工具完整 Schema 的参数对象。")),
			aitool.WithStringParam("identifier", aitool.WithParam_Description("本项独立的 snake_case 目的标识；同工具的不同调用也须区分。")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("向用户简述本项具体操作目的，与其他项区分。")),
			aitool.WithStringParam("expectations", aitool.WithParam_Description("可选的预期结果与判断依据。")),
		),
	},
	ActionVerifier: verifyNativeDirectTool,
	ActionHandler:  executeNativeDirectTool,
}

func verifyNativeDirectTool(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	loop.SetActionExecutionValue(action, actionStateDirectToolBatch, nil)
	loop.SetActionExecutionValue(action, actionStateNativeDirectParams, nil)
	loop.SetActionExecutionValue(action, "directly_call_tool_name", nil)
	if err := action.WaitParseResult(toolBatchVerifierContext(loop)); err != nil {
		return err
	}
	batch, hasBatch, err := parseDirectToolBatchActionWithMetadata(loop, action, true)
	if err != nil {
		return err
	}
	if hasBatch {
		loop.SetActionExecutionValue(action, actionStateDirectToolBatch, batch)
		return nil
	}
	rawName, _ := lookupCanonicalActionParam(action, "directly_call_tool_name")
	name, ok := rawName.(string)
	if !ok || strings.TrimSpace(name) == "" {
		return utils.Error("directly_call_tool requires directly_call_tool_name + directly_call_tool_params, or directly_call_tool_calls")
	}
	name = strings.TrimSpace(name)
	rawParams, _ := lookupCanonicalActionParam(action, "directly_call_tool_params")
	params, err := strictBatchParams(rawParams)
	if err != nil {
		return directToolParameterError(loop, name, utils.Wrap(err, "directly_call_tool_params"))
	}
	manager := loop.GetConfig().GetAiToolManager()
	if manager == nil {
		return utils.Error("tool manager is unavailable")
	}
	tool, err := manager.GetToolByName(name)
	if err != nil || tool == nil {
		return directToolParameterError(loop, name, utils.Errorf("tool %q is unavailable: %v", name, err))
	}
	if valid, errors := tool.ValidateParams(params); !valid {
		return directToolParameterError(loop, name, utils.Errorf("invalid arguments for %q: %s", name, strings.Join(errors, "; ")))
	}
	reactloops.MaybeWarnBashBeforeEdit(loop, name)
	loop.SetActionExecutionValue(action, "directly_call_tool_name", name)
	loop.SetActionExecutionValue(action, actionStateNativeDirectParams, params)
	return nil
}

func executeNativeDirectTool(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
	if executeVerifiedToolBatch(loop, action, actionStateDirectToolBatch, operator) {
		return
	}
	name, _ := loop.GetActionExecutionValue(action, "directly_call_tool_name").(string)
	params, _ := loop.GetActionExecutionValue(action, actionStateNativeDirectParams).(aitool.InvokeParams)
	if name == "" || params == nil {
		operator.Feedback("Missing verified direct-call parameters; no tool was executed.")
		operator.Continue()
		return
	}
	ctx, invoker := toolBatchVerifierContext(loop), loop.GetInvoker()
	result, directly, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, name, params,
		aicommon.WithToolCaller_Reason(resolveToolCallReason(action, "directly_call_reason")),
		aicommon.WithToolCaller_DestinationIdentifier(action.GetString("identifier")))
	recordSuccessfulToolCache(loop, name, result, err)
	handleToolCallResult(loop, ctx, invoker, name, result, directly, err, operator)
}
