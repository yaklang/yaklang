package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// ActionType represents the type of action to take
type ActionType string

const (
	ActionDirectlyAnswer       ActionType = "directly_answer"
	ActionRequireTool          ActionType = "require_tool"
	ActionUseAIBlueprintForge  ActionType = "require_blueprint_forge"
	ActionRequestPlanExecution ActionType = "request_plan_and_execution"
	ActionAskForClarification  ActionType = "ask_for_clarification"
)

// ReAct actions available
const (
	ReActActionObject = "object"
)

// parseReActAction parses the AI response to extract the ReAct action using aicommon.ExtractAction
func (r *ReAct) parseReActAction(response string) (*aicommon.Action, error) {
	// Use aicommon.ExtractAction for more robust parsing
	action, err := aicommon.ExtractAction(response, ReActActionObject)
	if err != nil {
		return nil, utils.Errorf("failed to extract ReAct action: %v", err)
	}

	// Validate required fields
	if action.GetString("human_readable_thought") == "" {
		return nil, utils.Error("human_readable_thought is required but empty")
	}

	actionType := action.GetInvokeParams("next_action").GetString("type")
	if actionType == "" {
		return nil, utils.Error("action.type is required but empty")
	}

	if !utils.StringSliceContain([]string{
		string(ActionDirectlyAnswer),
		string(ActionRequireTool),
		string(ActionRequestPlanExecution),
		string(ActionAskForClarification),
		string(ActionUseAIBlueprintForge),
	}, actionType) {
		return nil, utils.Errorf("invalid action type '%s', must be one of: %v", actionType, []any{
			ActionDirectlyAnswer,
			ActionRequireTool,
			ActionRequestPlanExecution,
			ActionAskForClarification,
			ActionUseAIBlueprintForge,
		})
	}
	return action, nil
}
