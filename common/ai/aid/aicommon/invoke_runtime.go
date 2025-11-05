package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type AIInvokeRuntime interface {
	GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error)

	ExecuteToolRequiredAndCall(ctx context.Context, name string) (*aitool.ToolResult, bool, error)
	ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error)
	AskForClarification(ctx context.Context, question string, payloads []string) string
	DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool) (string, error)
	EnhanceKnowledgeAnswer(context.Context, string) (string, error)
	EnhanceKnowledgeGetter(ctx context.Context, userQuery string) (string, error)
	VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (bool, error)
	RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error))
	AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error))
	InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...GeneralKVConfigOption) (*Action, error)

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
