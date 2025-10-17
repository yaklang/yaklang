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
	Description: "Require tool call",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_tool'. Provide the exact name of the tool you need to use (e.g., 'check-yaklang-syntax', 'yak-document'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here."),
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
		result, directly, err := invoker.ExecuteToolRequiredAndCall(toolPayload)
		if err != nil {
			operator.Fail(utils.Error("ExecuteToolRequiredAndCall fail"))
			return
		}
		if directly {
			answer, err := invoker.DirectlyAnswer("在上一次工具调用中，用户中断了工具执行，要求直接回答一些问题", nil)
			if err != nil {
				operator.Fail(utils.Error("DirectlyAnswer fail, reason: " + err.Error()))
				return
			}
			invoker.AddToTimeline("directly-answer", answer)
			operator.Exit()
			return
		}

		if result == nil {
			msg := fmt.Sprintf("ExecuteToolRequiredAndCall[%v] returned nil result", toolPayload)
			invoker.AddToTimeline("error", msg)
			operator.Continue()
			return
		}

		if result.Error != "" {
			invoker.AddToTimeline("call["+toolPayload+"] error", result.Error)
		}

		task := loop.GetCurrentTask()
		satisfied, err := invoker.VerifyUserSatisfaction(task.GetUserInput(), true, toolPayload)
		if err != nil {
			operator.Fail(err)
			return
		}

		if satisfied {
			operator.Exit()
			return
		}
	},
}
