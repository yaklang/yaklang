package aireact

// ActionType represents the type of action to take
type ActionType string

const (
	ActionDirectlyAnswer          ActionType = "directly_answer"
	ActionRequireTool             ActionType = "require_tool"
	ActionRequireAIBlueprintForge ActionType = "require_ai_blueprint"
	ActionRequestPlanExecution    ActionType = "request_plan_and_execution"
	ActionAskForClarification     ActionType = "ask_for_clarification"
	ActionKnowledgeEnhanceAnswer  ActionType = "knowledge_enhance_answer"
	ActionWriteYaklangCode        ActionType = "write_yaklang_code"
)

// ReAct actions available
const (
	ReActActionObject = "object"
)
