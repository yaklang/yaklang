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
	AsyncMode:  true,
	ActionType: schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT,
	Description: `Invoke an AI Blueprint to accomplish complex tasks that need specialized AI capabilities.

IMPORTANT - PREREQUISITES:
- If you DON'T know the parameter schema for this blueprint, you MUST call 'require_ai_blueprint_params' FIRST
- Only call this action when you have ALL required parameters ready
- The blueprint will fail if required parameters are missing or incorrectly formatted

WORKFLOW:
1. First time using a blueprint? Call 'require_ai_blueprint_params' to get its schema
2. Review the schema to understand what parameters are needed
3. Ensure you have all required information (use 'ask_for_clarification' if needed)
4. Then call this action with the blueprint name and properly formatted parameters

WHEN TO USE:
- After you've reviewed the blueprint schema and have all required parameters
- When you need specialized AI capabilities beyond standard tools
- For complex tasks that require domain-specific AI processing`,
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

		// Check if schema has been retrieved for this blueprint
		schemaKey := "blueprint_schema_retrieved_" + forgeName
		schemaRetrieved := false
		if v := loop.GetVariable(schemaKey); v != nil {
			if b, ok := v.(bool); ok {
				schemaRetrieved = b
			}
		}
		if !schemaRetrieved {
			warningMsg := fmt.Sprintf(`[WARNING] Blueprint Schema Not Retrieved

You are attempting to call blueprint '%s' without first retrieving its parameter schema.

RECOMMENDED ACTION:
1. Call 'require_ai_blueprint_params' with blueprint_payload='%s' to get the schema
2. Review the schema to understand required parameters
3. Verify you have all required information (use 'ask_for_clarification' if needed)
4. Then call 'require_ai_blueprint' with proper parameters

Proceeding without schema may result in:
- Missing required parameters
- Incorrect parameter types or formats
- Blueprint execution failure

The action will proceed, but you should consider getting the schema first for better results.`, forgeName, forgeName)

			loop.GetInvoker().AddToTimeline("[BLUEPRINT_SCHEMA_WARNING]", warningMsg)
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

		invoker.RequireAIForgeAndAsyncExecute(task.GetContext(), forgeName, func(err error) {
			loop.FinishAsyncTask(task, err)
		})
	},
}
