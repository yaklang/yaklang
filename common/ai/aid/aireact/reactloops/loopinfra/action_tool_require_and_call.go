package loopinfra

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_toolRequireAndCall = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description: "申请工具调用，执行这个 @action 会进入工具申请流程，查看工具教程以及文档，来生成参数。仅当目标工具不在 CACHE_TOOL_CALL 最近缓存中时使用；如果缓存里已经有该工具，优先 directly_call_tool。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description(`MUST set in {"@action": "require_tool", ... }. 根据上下文信息，提供你想要申请的工具名，只说明工具名即可，严禁包含参数.`),
		),
		aitool.WithStringParam(
			"tool_call_reason",
			aitool.WithParam_Description(`Optional. A human-readable sentence describing WHY this specific tool call is needed AT THIS POINT in the task. Reference the specific finding, prior tool result, or task step that motivates this call — e.g. 'login endpoint returned sess_ent cookie, replaying with SQLi payload in username'. Avoid generic descriptions like 'test the target' or 'scan for vulnerabilities'. Omit only when human_readable_thought already states the reason. Shown to the user on the tool-call card.`),
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		payload := action.GetString("tool_require_payload")
		if payload == "" {
			payload = action.GetInvokeParams("next_action").GetString("tool_require_payload")
		}
		if payload == "" {
			return utils.Error("tool_require_payload is required for ActionRequireTool but empty")
		}
		reactloops.MaybeWarnBashBeforeEdit(loop, payload)
		loop.Set("tool_require_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		toolPayload := loop.Get("tool_require_payload")
		if toolPayload == "" {
			operator.Feedback(utils.Error("tool_require_payload is required for ActionRequireTool but empty"))
			return
		}
		invoker := loop.GetInvoker()
		ctx := invoker.GetConfig().GetContext()
		t := loop.GetCurrentTask()
		if t != nil {
			ctx = t.GetContext()
		}

		loopInfraStatus(loop, "准备工具调用 / Preparing Tool Call...")
		toolLoadMessage := fmt.Sprintf("loading tool: %s...", toolPayload)
		if toolIns, err := loop.GetConfig().GetAiToolManager().GetToolByName(toolPayload); err != nil {
			toolLoadMessage += fmt.Sprintf(" Error: %v", err)
		} else {
			displayName := toolIns.GetName()
			if toolIns.GetVerboseName() != "" {
				displayName = fmt.Sprintf("%s(%s)", toolIns.GetName(), toolIns.GetVerboseName())
			}
			toolLoadMessage += fmt.Sprintf(" done! %s is prepared", displayName)
		}
		loopInfraSystemLog(loop, "load_tool", toolLoadMessage)

		reason := resolveToolCallReason(action, "tool_call_reason")
		result, directly, callErr := invoker.ExecuteToolRequiredAndCall(ctx, toolPayload, aicommon.WithToolCaller_Reason(reason))

		// cache tool on successful execution (before satisfaction check)
		if callErr == nil && result != nil {
			if cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(toolPayload); lookupErr == nil {
				loop.GetConfig().GetAiToolManager().AddRecentlyUsedTool(cachedTool)
				if realCfg, ok := loop.GetConfig().(*aicommon.Config); ok {
					realCfg.SaveRecentToolCache()
				}
			}
		}

		handleToolCallResult(loop, ctx, invoker, toolPayload, result, directly, callErr, operator)
	},
}
