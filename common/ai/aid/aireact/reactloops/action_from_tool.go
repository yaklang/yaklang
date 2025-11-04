package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// ConvertAIToolToLoopAction converts an AI Tool to a LoopAction.
// It extracts the tool's parameters and converts them to ToolOptions for the LoopAction.
func ConvertAIToolToLoopAction(tool *aitool.Tool) *LoopAction {
	// Use the built-in BuildParamsOptions method to extract tool parameters
	options := tool.BuildParamsOptions()

	// extractToolParams extracts the actual tool parameters from the action,
	// removing metadata fields like @action, tool, etc.
	// Handles multiple AI response formats:
	// 1. Standard: {"@action": "call-tool", "tool": "xxx", "params": {...}}
	// 2. No @action: {"tool": "xxx", "params": {...}}
	// 3. No tool field: {"@action": "xxx", "params": {...}}
	// 4. Simplified: {"@action": "xxx", param1: val1, ...}
	// 5. Direct params: {"tool": "xxx", param1: val1, ...}
	extractToolParams := func(action *aicommon.Action) map[string]any {
		// Get the parsed parameters from the action
		// Note: Use GetParams() not GetInvokeParams("") as it returns the actual parsed data
		allParams := action.GetParams()
		if len(allParams) == 0 {
			return make(map[string]any)
		}

		// Case 1 & 2 & 3: Check if there's a "params" field (nested format)
		if paramsField, hasParams := allParams["params"]; hasParams {
			if paramsMap, ok := paramsField.(map[string]any); ok {
				return paramsMap
			}
		}

		// Case 4 & 5: No "params" field, extract all non-metadata fields
		// Filter out metadata fields (@action, tool, params)
		cleanParams := make(map[string]any)
		metadataFields := map[string]bool{
			"@action": true,
			"tool":    true,
			"params":  true,
		}

		for key, value := range allParams {
			if !metadataFields[key] {
				cleanParams[key] = value
			}
		}

		return cleanParams
	}

	return &LoopAction{
		AsyncMode:   false,
		ActionType:  tool.GetName(),
		Description: tool.GetDescription(),
		Options:     options,
		ActionVerifier: func(loop *ReActLoop, action *aicommon.Action) error {
			// Extract clean tool parameters
			params := extractToolParams(action)

			// Use AITool's built-in validation
			valid, validationErrors := tool.ValidateParams(params)
			if !valid {
				// Combine all validation errors into a single message
				errMsg := utils.Errorf(
					"Tool '%s' parameter validation failed: %v",
					tool.GetName(),
					validationErrors,
				)

				// Add error to timeline so AI can learn from it
				loop.GetInvoker().AddToTimeline(
					"[PARAMETER_VALIDATION_ERROR]",
					errMsg.Error(),
				)
				return errMsg
			}

			return nil
		},
		ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
			// Get parameters using the same strategy as ActionVerifier
			// Handle both standard and simplified formats
			invokeParams := action.GetParams()

			if len(invokeParams) == 0 {
				invokeParams = action.GetInvokeParams(tool.GetName())
			}

			if len(invokeParams) == 0 {
				invokeParams = action.GetInvokeParams("next_action").GetObject(tool.GetName())
			}

			// Handle simplified format: {@action: "tool_name", param1: val1, ...}
			if len(invokeParams) == 0 && action.ActionType() == tool.GetName() {
				// Directly extract all parameters from the action
				invokeParams = make(map[string]any)

				for _, paramName := range tool.Tool.InputSchema.Properties.Keys() {
					// Use GetInvokeParams to get the value as an object, then extract it
					paramValue := action.GetInvokeParams(paramName)
					if len(paramValue) > 0 {
						invokeParams[paramName] = paramValue
					} else {
						// Try to get it as a simple value
						strVal := action.GetAnyToString(paramName)
						if strVal != "" {
							invokeParams[paramName] = strVal
							continue
						}

						floatVal := action.GetFloat(paramName)
						if floatVal != 0 {
							invokeParams[paramName] = floatVal
							continue
						}

						boolVal := action.GetBool(paramName)
						invokeParams[paramName] = boolVal
					}
				}
			}

			// Get context
			invoker := loop.GetInvoker()
			ctx := invoker.GetConfig().GetContext()
			t := loop.GetCurrentTask()
			if t != nil && !utils.IsNil(t.GetContext()) {
				ctx = t.GetContext()
			}

			// Execute the tool without requiring parameter generation
			result, directly, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, tool.GetName(), invokeParams)
			if err != nil {
				errMsg := utils.Errorf("tool '%s' execution failed: %v", tool.GetName(), err)
				invoker.AddToTimeline("[TOOL_EXECUTION_ERROR]", errMsg.Error())
				operator.Fail(errMsg)
				return
			}

			// Handle direct answer case
			if directly {
				answer, err := invoker.DirectlyAnswer(ctx, "在上一次工具调用中，用户中断了工具执行，要求直接回答一些问题", nil)
				if err != nil {
					operator.Fail(utils.Errorf("DirectlyAnswer failed: %v", err))
					return
				}
				invoker.AddToTimeline("directly-answer", answer)
				operator.Exit()
				return
			}

			// Handle nil result
			if result == nil {
				msg := utils.Errorf("tool '%s' returned nil result", tool.GetName())
				invoker.AddToTimeline("[TOOL_EXECUTION_WARNING]", msg.Error())
				operator.Continue()
				return
			}

			// Log error in result if present
			if result.Error != "" {
				invoker.AddToTimeline(
					"[TOOL_EXECUTION_ERROR]",
					utils.Errorf("tool '%s' returned error: %s", tool.GetName(), result.Error).Error(),
				)
				operator.Fail(utils.Errorf("tool execution returned error: %s", result.Error))
				return
			}

			// Log success
			invoker.AddToTimeline(
				"[TOOL_EXECUTION_SUCCESS]",
				utils.Errorf("tool '%s' executed successfully", tool.GetName()).Error(),
			)

			// Verify user satisfaction
			task := loop.GetCurrentTask()
			if task != nil {
				satisfied, err := invoker.VerifyUserSatisfaction(ctx, task.GetUserInput(), true, tool.GetName())
				if err != nil {
					operator.Fail(err)
					return
				}

				if satisfied {
					operator.Exit()
					return
				}
			}

			// Continue to next action
			operator.Continue()
		},
	}
}
