package loopinfra

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_RequireAIBlueprintForgeSchema = &reactloops.LoopAction{
	AsyncMode:  true,
	ActionType: schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT_SCHEMA,
	Description: `Get the parameter schema of an AI Blueprint before invoking it. Use this action when you need to understand what parameters are required by a specific AI Blueprint.

WHEN TO USE:
- Before calling 'require_ai_blueprint' for the first time with a specific blueprint
- When you're unsure about the parameters required by a blueprint
- To understand what information you need to collect from the user

WORKFLOW:
1. Call this action to get the blueprint's parameter schema
2. Review the schema to understand what parameters are needed
3. Check if current context/memory contains all required information
4. If information is missing, use 'ask_for_clarification' to request it from the user
5. Once all required information is available, call 'require_ai_blueprint' with proper parameters

The schema will help you understand:
- What parameters are required vs optional
- The type and format of each parameter
- The description and purpose of each parameter
- What capabilities this blueprint provides`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"blueprint_payload",
			aitool.WithParam_Description("The name of the AI Blueprint whose schema you want to retrieve. Example: 'code_generator', 'vulnerability_scanner'"),
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

		blueprintSchema, err := invoker.RequireBlueprintSchema(task.GetContext(), forgeName)
		if err != nil {
			operator.Fail(err)
			return
		}

		// Mark that schema has been retrieved for this blueprint
		schemaKey := "blueprint_schema_retrieved_" + forgeName
		loop.Set(schemaKey, true)

		// Add detailed schema information to timeline for AI reflection
		schemaInfo := fmt.Sprintf(`Blueprint Schema Retrieved: '%s'

Schema Details:
%s

NEXT STEPS FOR AI:
1. Review the schema above to understand required and optional parameters
2. Check if your current context/memory contains all required information
3. If any required information is missing:
   - Use 'ask_for_clarification' to request missing information from the user
   - Be specific about what information you need and why
4. Once all required parameters can be satisfied:
   - Call 'require_ai_blueprint' with the blueprint name and properly formatted parameters
   - Ensure parameters match the schema types and requirements

Remember: Do NOT call 'require_ai_blueprint' until you have all required information!`, forgeName, blueprintSchema)

		invoker.AddToTimeline("[BLUEPRINT_SCHEMA_READY]", schemaInfo)
		operator.Exit()
	},
}
