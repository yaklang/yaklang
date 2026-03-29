package loopinfra

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_directlyCallTool = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
	Description: "directly call a recently used tool (skip require & param-generation phases). " +
		"Only tools listed in the CACHE_TOOL_CALL block are eligible. " +
		"Provide directly_call_tool_name AND directly_call_tool_params together.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"directly_call_tool_name",
			aitool.WithParam_Description(`MUST set when @action is "directly_call_tool". The name of the tool to call. Must be one of the cached recently-used tools.`),
		),
		aitool.WithStringParam(
			"directly_call_tool_params",
			aitool.WithParam_Description(`MUST set when @action is "directly_call_tool". A JSON object containing the tool invocation parameters. Refer to the cached tool's Params Schema in the CACHE_TOOL_CALL block for the correct structure.`),
		),
		aitool.WithStringParam(
			"directly_call_identifier",
			aitool.WithParam_Description(`short snake_case label describing the PURPOSE of this tool call, e.g. "scan_port_443", "query_large_file". Used for report file naming.`),
		),
		aitool.WithStringParam(
			"directly_call_expectations",
			aitool.WithParam_Description(`estimated timing and fallback strategy, e.g. "~3s, force stop if >10s". Used for interval review during execution.`),
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		toolName := action.GetString("directly_call_tool_name")
		if toolName == "" {
			toolName = action.GetInvokeParams("next_action").GetString("directly_call_tool_name")
		}
		if toolName == "" {
			return utils.Error("directly_call_tool_name is required for directly_call_tool but empty")
		}

		mgr := loop.GetConfig().GetAiToolManager()
		if mgr == nil || !mgr.IsRecentlyUsedTool(toolName) {
			return utils.Errorf("tool '%s' is not in the recently-used cache; use require_tool instead", toolName)
		}

		hasParams := len(action.GetInvokeParams("directly_call_tool_params")) > 0 ||
			action.GetString("directly_call_tool_params") != ""
		if !hasParams {
			nextAction := action.GetInvokeParams("next_action")
			hasParams = len(nextAction.GetObject("directly_call_tool_params")) > 0
		}
		if !hasParams {
			return utils.Error("directly_call_tool_params is required for directly_call_tool but empty")
		}

		loop.Set("directly_call_tool_name", toolName)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		toolName := loop.Get("directly_call_tool_name")
		if toolName == "" {
			operator.Feedback(utils.Error("directly_call_tool requires tool_name"))
			return
		}

		invoker := loop.GetInvoker()
		ctx := invoker.GetConfig().GetContext()
		if t := loop.GetCurrentTask(); t != nil {
			ctx = t.GetContext()
		}

		// 1. extract params: try string-based JSON parse first (since the field is WithStringParam),
		//    then fall back to object extraction.
		var params aitool.InvokeParams

		raw := action.GetString("directly_call_tool_params")
		if raw != "" {
			parsed := make(aitool.InvokeParams)
			if err := json.Unmarshal([]byte(raw), &parsed); err == nil && len(parsed) > 0 {
				params = parsed
			}
		}

		if len(params) == 0 {
			objParams := action.GetInvokeParams("directly_call_tool_params")
			delete(objParams, "__DEFAULT__")
			delete(objParams, "__FALLBACK__")
			delete(objParams, "__[yaklang-raw]__")
			if len(objParams) > 0 {
				params = objParams
			}
		}
		if len(params) == 0 {
			operator.Feedback(utils.Error("directly_call_tool: no valid params found"))
			operator.Continue()
			return
		}

		// 2. inject reserved keys from directly_call_ prefixed fields
		if id := action.GetString("directly_call_identifier"); id != "" {
			params[aicommon.ReservedKeyIdentifier] = id
		}
		if ce := action.GetString("directly_call_expectations"); ce != "" {
			params[aicommon.ReservedKeyCallExpectations] = ce
		}

		// 3. execute
		result, directly, callErr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)

		if callErr == nil && result != nil && result.Success {
			if cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(toolName); lookupErr == nil {
				loop.GetConfig().GetAiToolManager().AddRecentlyUsedTool(cachedTool)
				if realCfg, ok := loop.GetConfig().(*aicommon.Config); ok {
					realCfg.SaveRecentToolCache()
				}
			}
		}

		handleToolCallResult(loop, ctx, invoker, toolName, result, directly, callErr, operator)
	},
}
