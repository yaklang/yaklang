package aicommon

import (
	"context"
	"errors"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// reasonTestRuntime is a minimal AIInvokeRuntime stub used by the
// tool-call-reason unit tests. Only InvokeSpeedPriorityLiteForge has real
// behaviour (returns a fixed reason action, or an error when failSpeedForge is
// set); every other method is a zero-value no-op, which is fine because
// generateReasonByLiteForge only touches InvokeSpeedPriorityLiteForge.
type reasonTestRuntime struct {
	failSpeedForge bool
}

func (r *reasonTestRuntime) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	return "", nil, nil
}

func (r *reasonTestRuntime) AssembleLoopPrompt(tools []*aitool.Tool, input *LoopPromptAssemblyInput) (*LoopPromptAssemblyResult, error) {
	return nil, nil
}

func (r *reasonTestRuntime) SetCurrentTask(task AIStatefulTask) {}

func (r *reasonTestRuntime) GetCurrentTask() AIStatefulTask { return nil }

func (r *reasonTestRuntime) GetCurrentTaskId() string { return "" }

func (r *reasonTestRuntime) AddRuntimeTask(task AIStatefulTask) {}

func (r *reasonTestRuntime) ExecuteToolRequiredAndCall(ctx context.Context, name string, opt ...ToolCallerOption) (*aitool.ToolResult, bool, error) {
	return nil, false, nil
}

func (r *reasonTestRuntime) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams, opt ...ToolCallerOption) (*aitool.ToolResult, bool, error) {
	return nil, false, nil
}

func (r *reasonTestRuntime) DirectlyCallTool(ctx context.Context, toolName string, action *Action, prepare DirectlyCallPrepareFunc) (*aitool.ToolResult, bool, error) {
	return nil, false, nil
}

func (r *reasonTestRuntime) AskForClarification(ctx context.Context, question string, payloads []string) string {
	return ""
}

func (r *reasonTestRuntime) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	return "", nil
}

func (r *reasonTestRuntime) CompressLongTextWithDestination(ctx context.Context, i any, destination string, targetByteSize int64) (string, error) {
	return "", nil
}

func (r *reasonTestRuntime) QuickKnowledgeSearch(ctx context.Context, query string, keywords []string, collections ...string) (string, error) {
	return "", nil
}

func (r *reasonTestRuntime) EnhanceKnowledgeGetterEx(ctx context.Context, userQuery string, enhancePlans []string, collections ...string) (string, error) {
	return "", nil
}

func (r *reasonTestRuntime) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*VerifySatisfactionResult, error) {
	return nil, nil
}

func (r *reasonTestRuntime) RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error)) {
}

func (r *reasonTestRuntime) AsyncPlanOnly(ctx context.Context, planPayload string, onFinish func(error)) {
}

func (r *reasonTestRuntime) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error)) {
}

func (r *reasonTestRuntime) ReviewExecutePlan(ctx context.Context, input *ExecutePlanInput) (*ExecutePlanInput, error) {
	return input, nil
}

func (r *reasonTestRuntime) ForceReviewExecutePlan(ctx context.Context, input *ExecutePlanInput) (*ExecutePlanInput, error) {
	return input, nil
}

func (r *reasonTestRuntime) BeginPlanCoordinatorSession(ctx context.Context, input *ExecutePlanInput, forceManualReview bool) (PlanCoordinatorSession, error) {
	return nil, nil
}

func (r *reasonTestRuntime) PublishDetachedPlan(ctx context.Context, input *ExecutePlanInput, reactTaskID string) (coordinatorID string, err error) {
	return "", nil
}

func (r *reasonTestRuntime) AsyncExecutePlan(ctx context.Context, input *ExecutePlanInput, onFinish func(error)) {
}

func (r *reasonTestRuntime) AsyncExecuteCod(ctx context.Context, coordinatorID string, onFinish func(error)) {
}

func (r *reasonTestRuntime) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...GeneralKVConfigOption) (*Action, error) {
	return nil, nil
}

func (r *reasonTestRuntime) InvokeSpeedPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...GeneralKVConfigOption) (*Action, error) {
	if r.failSpeedForge {
		return nil, errors.New("force failure")
	}
	return ExtractAction(`{"@action": "tool-call-reason", "reason": "mocked tool-call reason"}`, "tool-call-reason")
}

func (r *reasonTestRuntime) InvokeQualityPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...GeneralKVConfigOption) (*Action, error) {
	return nil, nil
}

func (r *reasonTestRuntime) SelectKnowledgeBase(ctx context.Context, originQuery string) (*SelectedKnowledgeBaseResult, error) {
	return nil, nil
}

func (r *reasonTestRuntime) ExecuteLoopTaskIF(taskTypeName string, task AIStatefulTask, options ...any) (bool, error) {
	return false, nil
}

func (r *reasonTestRuntime) AddToTimeline(entry, content string) {}

func (r *reasonTestRuntime) GetConfig() AICallerConfigIf { return nil }

func (r *reasonTestRuntime) EmitFileArtifactWithExt(name, ext string, data any) string { return "" }

func (r *reasonTestRuntime) EmitResultAfterStream(any) {}

func (r *reasonTestRuntime) EmitResult(any) {}

var _ AIInvokeRuntime = (*reasonTestRuntime)(nil)
