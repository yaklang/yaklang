package aireact

import "github.com/yaklang/yaklang/common/schema"

// ActionType represents the type of action to take
type ActionType string

const (
	ActionDirectlyAnswer          ActionType = schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER
	ActionRequireTool             ActionType = schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL
	ActionRequireAIBlueprintForge ActionType = schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT
	ActionRequestPlanExecution    ActionType = schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION
	ActionAskForClarification     ActionType = schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION
	ActionKnowledgeEnhanceAnswer  ActionType = schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE
	ActionWriteYaklangCode        ActionType = "write_yaklang_code"
)

// ReAct actions available
const (
	ReActActionObject = "object"
)
