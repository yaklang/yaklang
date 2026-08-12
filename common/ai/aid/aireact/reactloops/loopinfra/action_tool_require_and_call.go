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
	Description: "申请工具调用，执行这个 @action 会进入工具申请流程，查看工具教程以及文档，来生成参数。仅当目标工具不在 CACHE_TOOL_CALL 最近缓存中时使用；如果缓存里已经有该工具，优先 directly_call_tool。单个工具使用 tool_require_payload；2-8 个彼此独立的工具使用 tool_require_calls，运行时会分别生成参数并并发执行。严禁同时使用两种形式。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("仅在 require_tool 的单调用形式中必填；存在 tool_require_calls 时必须省略。根据上下文信息，提供一个想要申请的工具名，严禁包含参数。只申请一个工具时使用下面这个经过 CI 校验且可执行的标量格式：\n"+requireToolScalarOutputExampleJSON),
		),
		aitool.WithStringParam(
			"tool_call_reason",
			aitool.WithParam_Description(`Optional. A terse phrase (under 15 words) stating WHAT this tool call does — e.g. 'grep /api路径寻找注入点' or 'replay login with SQLi in username'. No prior-step summaries or transitions. Omit only when human_readable_thought already states the reason. Shown to the user on the tool-call card.`),
		),
		requireToolBatchSchemaOption(),
	},
	OutputExamples: requireToolOutputExamples,
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		loop.Delete(loopVarRequireToolBatch)

		// tool_require_payload is the legacy one-call discriminator. Preserve its
		// field-level streaming behavior and only wait for a canonical object when
		// the scalar form is absent and this may actually be a batch action.
		payload := action.GetString("tool_require_payload")
		if payload == "" {
			payload = action.GetInvokeParams("next_action").GetString("tool_require_payload")
		}
		if payload != "" {
			reactloops.MaybeWarnBashBeforeEdit(loop, payload)
			loop.Set("tool_require_payload", payload)
			return nil
		}

		batch, hasBatch, batchErr := parseRequireToolBatchAction(loop, action)
		if batchErr != nil {
			return batchErr
		}
		if hasBatch {
			loop.Set(loopVarRequireToolBatch, batch)
			loop.Delete("tool_require_payload")
			return nil
		}

		return utils.Error("require_tool requires tool_require_payload or tool_require_calls")
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		if executeVerifiedToolBatch(loop, loopVarRequireToolBatch, operator) {
			return
		}
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
				if realCfg, ok := loop.GetConfig().(*aicommon.Config); ok {
					realCfg.RecordRecentlyUsedTool(cachedTool)
				} else {
					loop.GetConfig().GetAiToolManager().AddRecentlyUsedTool(cachedTool)
				}
			}
		}

		handleToolCallResult(loop, ctx, invoker, toolPayload, result, directly, callErr, operator)
	},
}
