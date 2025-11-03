package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_RequireAIBlueprintForge = &reactloops.LoopAction{
	AsyncMode:   true,
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT,
	Description: `Require an AI Blueprint to accomplish complex tasks that need specialized AI capabilities.`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"blueprint_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_ai_blueprint'. Provide the name of the AI Blueprint you want to use. Example: 'code_generator'"),
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		forgeName := action.GetString("blueprint_payload")
		if forgeName == "" {
			forgeName = action.GetInvokeParams("next_action").GetString("blueprint_payload")
		}
		if forgeName == "" {
			loop.GetInvoker().AddToTimeline("❌[BLUEPRINT_MISSING_NAME]", "require_ai_blueprint action is missing 'blueprint_payload' field")
			return utils.Error("require_ai_blueprint action must have 'blueprint_payload' field")
		}
		
		// 记录准备调用的 Blueprint
		loop.GetInvoker().AddToTimeline("🔄[BLUEPRINT_ACTION_VERIFIED]", "Blueprint name: "+forgeName)
		loop.Set("blueprint_payload", forgeName)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		forgeName := action.GetString("blueprint_payload")
		if forgeName == "" {
			forgeName = action.GetInvokeParams("next_action").GetString("blueprint_payload")
		}
		if forgeName == "" {
			forgeName = loop.Get("blueprint_payload")
		}
		invoker := loop.GetInvoker()

		task := operator.GetTask()

		invoker.RequireAIForgeAndAsyncExecute(task.GetContext(), forgeName, func(err error) {
			loop.FinishAsyncTask(task, err)
		})
	},
}
