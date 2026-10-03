package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge"
)

func (r *ReAct) invokeLiteForgeWithCallback(cb aicommon.AICallbackType, ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	execute := r.config.LiteForgeExecutor
	if execute == nil {
		execute = liteforge.ExecuteTyped
	}
	if cb == nil {
		cb = r.config.GetOriginalAICallback()
	}
	result, err := execute(prompt, &aicommon.LiteForgeInvokeRequest{Context: ctx, ActionName: actionName, Outputs: outputs, Options: opts, Emitter: r.config.Emitter},
		aicommon.WithFastAICallback(cb), aicommon.WithEnableFunctionCallMode(r.config.EnableFunctionCallMode),
		aicommon.WithTimeline(r.config.Timeline), aicommon.WithAppendPersistentContext(r.config.PersistentMemory...),
		aicommon.WithPersistentSessionId(r.config.PersistentSessionId), aicommon.WithEventHandler(r.config.EventHandler),
		aicommon.WithAIAutoRetry(r.config.AiAutoRetry), aicommon.WithAITransactionAutoRetry(r.config.GetAITransactionAutoRetryCount()),
		aicommon.WithAIRetryWaitFunc(r.config.GetAIRetryWaitFunc()), aicommon.WithUserUsageCallback(r.config.GetUserUsageCallback()))
	if err != nil {
		return nil, err
	}
	return result.Action, nil
}

// InvokeSpeedPriorityLiteForge is retained for compatibility.
// Deprecated: production Speed tasks should use r.config.ScheduleAuxiliaryTask.
func (r *ReAct) InvokeSpeedPriorityLiteForge(
	ctx context.Context, actionName string, prompt string,
	outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption,
) (*aicommon.Action, error) {
	return r.invokeLiteForgeWithCallback(r.config.GetSpeedPriorityAICallback(), ctx, actionName, prompt, outputs, opts...)
}

func (r *ReAct) InvokeQualityPriorityLiteForge(
	ctx context.Context, actionName string, prompt string,
	outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption,
) (*aicommon.Action, error) {
	return r.invokeLiteForgeWithCallback(r.config.GetQualityPriorityAICallback(), ctx, actionName, prompt, outputs, opts...)
}

func (r *ReAct) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	return r.InvokeQualityPriorityLiteForge(ctx, actionName, prompt, outputs, opts...)
}
