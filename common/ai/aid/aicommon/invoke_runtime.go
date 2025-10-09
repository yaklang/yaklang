package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type AIInvokeRuntime interface {
	GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error)

	ExecuteToolRequiredAndCall(name string) (*aitool.ToolResult, bool, error)
	AskForClarification(question string, payloads []string) string
	DirectlyAnswer(query string, tools []*aitool.Tool) (string, error)
	EnhanceKnowledgeAnswer(context.Context, string) (string, error)
	VerifyUserSatisfaction(query string, isToolCall bool, payload string) (bool, error)
	RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error))
	AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error))

	// timeline operator
	AddToTimeline(entry, content string)

	GetConfig() AICallerConfigIf
	EmitFileArtifactWithExt(name, ext string, data any) string
	EmitResultAfterStream(any)
	EmitResult(any)
}
