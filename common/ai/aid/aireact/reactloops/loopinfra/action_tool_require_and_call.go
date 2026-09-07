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
	Description: "申请工具并由运行时阅读工具文档、生成参数。先枚举本轮已明确的真实调用：存在 2-8 个互不依赖、互不干扰且都需要生成参数的调用时，优先使用 tool_require_calls 一次并发申请，不要拆成多个单工具轮次；只有本轮恰好一个调用时才使用 tool_require_payload。若工具已在 CACHE_TOOL_CALL 且参数完整，改用 directly_call_tool。批量项严禁提供 params；严禁混用单调用和批量字段，也不要为了凑数量发明调用。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("仅当本轮恰好一个 require_tool 调用时填写；存在 tool_require_calls 时必须省略。只填写一个需要生成参数的工具准确名称，严禁包含参数。下面是经过 CI 校验且可执行的单调用格式：\n"+requireToolScalarOutputExampleJSON),
		),
		aitool.WithStringParam(
			"tool_call_reason",
			aitool.WithParam_Description(`可选。用简短短语说明这次调用具体做什么，例如“grep /api 路径寻找注入点”或“在 username 中注入 SQLi 并重放登录”。不要写前序总结或过渡语；仅当 human_readable_thought 已说明原因时省略。该内容会显示在工具调用卡片上。`),
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

		emitToolsPreparingStatus(loop, []string{toolPayload})
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
