package loopinfra

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
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

		errChan := make(chan error, 1)
		invoker.RequireAIForgeAndAsyncExecute(task.GetContext(), forgeName, func(err error) {
			errChan <- err
		})
		forgeErr := <-errChan
		loop.FinishAsyncTask(task, forgeErr)

		if forgeErr != nil {
			operator.Feedback(forgeErr)
			operator.Continue()
			operator.RequestSyncContinuation()
		} else {
			// Forge 成功时调用 Exit()，使 exec 将此次执行视为正常终止。
			// 否则 FinishAsyncTask -> task.Finish -> SetStatus(Completed) -> task.Cancel()
			// 会取消 context，exec 随后的 context.Done() 检查会误判为错误并返回 "context canceled"。
			operator.Exit()
		}
	},
}
