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
	Description: "Execute business tools using arguments you construct from their complete schemas in CACHE_TOOL_CALL. " +
		"Use directly_call_tool_name + directly_call_tool_params for one call, or directly_call_tool_calls for independent calls. " +
		"If the schema is missing, require_tool only loads it; observe it before constructing arguments. " +
		"Correct invalid arguments here; the runtime will not delegate parameter generation to another model.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam("directly_call_tool_name", aitool.WithParam_Description("Single-call tool name; omit with directly_call_tool_calls.")),
		aitool.WithRawParam("directly_call_tool_params", map[string]any{"type": "object", "description": "Single-call arguments following the business-tool schema. Use {} for a parameterless tool; omit with directly_call_tool_calls."}),
		aitool.WithStringParam("directly_call_reason", aitool.WithParam_Description("Optional reason; for a batch, used by children that omit reason.")),
		aitool.WithStructArrayParam("directly_call_tool_calls", []aitool.PropertyOption{
			aitool.WithParam_Description("Execute independent, low-risk calls with complete arguments. Omit single-call name/params. Optional top-level reason applies to children without reason."),
			aitool.WithParam_Raw("minItems", 2), aitool.WithParam_Raw("maxItems", aicommon.DefaultToolBatchMaxCalls),
		}, nil,
			aitool.WithStringParam("tool_name", aitool.WithParam_Required(true)),
			aitool.WithRawParam("params", map[string]any{"type": "object", "additionalProperties": true}, aitool.WithParam_Required(true)),
			aitool.WithStringParam("identifier"), aitool.WithStringParam("reason"), aitool.WithStringParam("expectations"),
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
		return utils.Wrap(err, "directly_call_tool_params")
	}
	manager := loop.GetConfig().GetAiToolManager()
	if manager == nil {
		return utils.Error("tool manager is unavailable")
	}
	tool, err := manager.GetToolByName(name)
	if err != nil || tool == nil {
		return utils.Errorf("tool %q is unavailable; require_tool can load available schemas but does not execute tools", name)
	}
	if valid, errors := tool.ValidateParams(params); !valid {
		return utils.Errorf("invalid arguments for %q: %s; correct directly_call_tool_params using the tool schema. require_tool only loads schemas, never generates parameters", name, strings.Join(errors, "; "))
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
	recordSuccessfulToolCache(loop.GetConfig(), name, result, err)
	handleToolCallResult(loop, ctx, invoker, name, result, directly, err, operator)
}
