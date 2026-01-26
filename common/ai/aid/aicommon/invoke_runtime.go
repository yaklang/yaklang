package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// VerifySatisfactionResult represents the result of user satisfaction verification
type VerifySatisfactionResult struct {
	Satisfied          bool   `json:"satisfied"`            // Whether the user is satisfied
	Reasoning          string `json:"reasoning"`            // The reasoning for the satisfaction status
	CompletedTaskIndex string `json:"completed_task_index"` // Index of completed task(s), e.g., "1-1" or "1-1,1-2"
	NextMovements      string `json:"next_movements"`       // AI's next action plan for in-progress status tracking
}

// NewVerifySatisfactionResult creates a new VerifySatisfactionResult
func NewVerifySatisfactionResult(satisfied bool, reasoning string, completedTaskIndex string) *VerifySatisfactionResult {
	return &VerifySatisfactionResult{
		Satisfied:          satisfied,
		Reasoning:          reasoning,
		CompletedTaskIndex: completedTaskIndex,
	}
}

// NewVerifySatisfactionResultWithNextMovements creates a new VerifySatisfactionResult with next movements
func NewVerifySatisfactionResultWithNextMovements(satisfied bool, reasoning string, completedTaskIndex string, nextMovements string) *VerifySatisfactionResult {
	return &VerifySatisfactionResult{
		Satisfied:          satisfied,
		Reasoning:          reasoning,
		CompletedTaskIndex: completedTaskIndex,
		NextMovements:      nextMovements,
	}
}

type AIInvokeRuntime interface {
	GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error)

	ExecuteToolRequiredAndCall(ctx context.Context, name string) (*aitool.ToolResult, bool, error)
	ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error)
	AskForClarification(ctx context.Context, question string, payloads []string) string
	DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error)
	EnhanceKnowledgeAnswer(context.Context, string) (string, error)
	EnhanceKnowledgeGetter(ctx context.Context, userQuery string, collections ...string) (string, error)
	// EnhanceKnowledgeGetterEx 支持多种 EnhancePlan 的知识增强获取器
	// enhancePlans 参数可选，支持：
	//   - nil 或空切片：使用默认完整增强流程
	//   - []string{"exact_keyword_search"}: 仅使用精准关键词搜索
	//   - []string{"hypothetical_answer", "generalize_query"}: 指定增强策略组合
	EnhanceKnowledgeGetterEx(ctx context.Context, userQuery string, enhancePlans []string, collections ...string) (string, error)
	EnhanceKnowledgeGetRandomN(ctx context.Context, n int, collections ...string) (string, error)
	// VerifyUserSatisfaction verifies if the user is satisfied with the result
	VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*VerifySatisfactionResult, error)
	RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error))
	AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error))
	InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...GeneralKVConfigOption) (*Action, error)

	ExecuteLoopTaskIF(taskTypeName string, task AIStatefulTask, options ...any) (bool, error)
	// timeline operator
	AddToTimeline(entry, content string)

	GetConfig() AICallerConfigIf
	EmitFileArtifactWithExt(name, ext string, data any) string
	EmitResultAfterStream(any)
	EmitResult(any)
}

type AITaskInvokeRuntime interface {
	AIInvokeRuntime
	SetCurrentTask(task AIStatefulTask)
	GetCurrentTask() AIStatefulTask
}

var AIRuntimeInvokerGetter = func(ctx context.Context, options ...ConfigOption) (AITaskInvokeRuntime, error) {
	return nil, utils.Errorf("not registered default AI runtime invoker")
}

func RegisterDefaultAIRuntimeInvoker(getter func(ctx context.Context, options ...ConfigOption) (AITaskInvokeRuntime, error)) {
	AIRuntimeInvokerGetter = getter
}
