package loopinfra

import (
	"fmt"

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
			loop.GetInvoker().AddToTimeline("[BLUEPRINT_MISSING_NAME]", "require_ai_blueprint action is missing 'blueprint_payload' field")
			loop.GetInvoker().AddToTimeline("[ACTION_VERIFIER]", "Failed to verify require_ai_blueprint action due to missing blueprint_payload")
			return utils.Error("require_ai_blueprint action must have 'blueprint_payload' field")
		}

		// Pre-check: try to resolve the identifier to detect misuse early
		// This prevents the async forge call from failing deep inside invoke_blueprint.go
		resolved := loop.ResolveIdentifier(forgeName)
		if resolved.IdentityType != aicommon.ResolvedAs_Unknown && resolved.IdentityType != aicommon.ResolvedAs_Forge {
			// The identifier exists but is NOT a forge - provide clear guidance
			loop.GetInvoker().AddToTimeline("[BLUEPRINT_WRONG_TYPE]",
				fmt.Sprintf("'%s' is not an AI Blueprint. %s", forgeName, resolved.Suggestion))
			return utils.Errorf("'%s' is not an AI Blueprint: %s", forgeName, resolved.Suggestion)
		}

		// Match runtime resolution (ExtendedForge + AiForgeManager) so invalid names fail
		// here and the AI transaction can retry instead of entering async mode and aborting.
		if cfg, ok := loop.GetInvoker().GetConfig().(*aicommon.Config); ok {
			if _, err := cfg.LookupAIForgeForInvoke(forgeName); err != nil {
				loop.GetInvoker().AddToTimeline("[BLUEPRINT_NOT_FOUND]",
					fmt.Sprintf("AI Blueprint '%s' does not exist or is not available. Error: %v", forgeName, err))
				loop.GetInvoker().AddToTimeline("[ACTION_VERIFIER]",
					fmt.Sprintf("require_ai_blueprint rejected: blueprint '%s' not found", forgeName))
				return utils.Errorf("AI Blueprint '%s' not found: %v", forgeName, err)
			}
		}

		// 记录准备调用的 Blueprint
		loop.GetInvoker().AddToTimeline("[BLUEPRINT_ACTION_VERIFIED]", "Verified require_ai_blueprint action with blueprint_payload: '"+forgeName+"'. The action passed ActionVerifier and is ready for execution with the specified AI Blueprint.")
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
		recommendCapabilitiesFromForgePrompts(loop, invoker, forgeName, "AI Blueprint "+forgeName)

		invoker.RequireAIForgeAndAsyncExecute(task.GetContext(), forgeName, func(err error) {
			loop.FinishAsyncTask(task, err)
		})
	},
}
