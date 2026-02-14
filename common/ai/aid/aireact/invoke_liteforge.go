package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) invokeLiteForgeWithCallback(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, aiCallback aicommon.AICallbackType, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	var rawOutputs []any
	for _, output := range outputs {
		var rawOpt any = output
		rawOutputs = append(rawOutputs, rawOpt)
	}

	gconfig := aicommon.NewGeneralKVConfig(opts...)

	fopts := []aiforge.LiteForgeOption{
		aiforge.WithLiteForge_Prompt(prompt),
		aiforge.WithLiteForge_OutputSchemaRaw(
			actionName,
			aitool.NewObjectSchemaWithActionName(actionName, rawOutputs...),
		),
	}
	for _, i := range gconfig.GetStreamableFields() {
		fopts = append(fopts, aiforge.WithLiteForge_StreamableFieldWithAINodeId(i.AINodeId(), i.FieldKey()))
	}
	// add user-defined field stream callbacks from GeneralKVConfig
	for _, item := range gconfig.GetStreamableFieldCallbacks() {
		fopts = append(fopts, aiforge.WithLiteForge_FieldStreamCallback(item.FieldKeys, aiforge.FieldStreamCallback(item.Callback)))
	}
	fopts = append(fopts, aiforge.WithLiteForge_Emitter(r.config.Emitter))

	f, err := aiforge.NewLiteForge(actionName, fopts...)
	if err != nil {
		return nil, utils.Wrap(err, "create liteforge failed")
	}
	forgeResult, err := f.Execute(ctx, []*ypb.ExecParamItem{
		{Key: "query", Value: prompt},
	}, aicommon.WithAgreeYOLO(), aicommon.WithAICallback(aiCallback))
	if err != nil {
		return nil, utils.Wrap(err, "invoke liteforge failed")
	}
	return forgeResult.Action, nil
}

func (r *ReAct) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	return r.invokeLiteForgeWithCallback(ctx, actionName, prompt, outputs, r.config.OriginalAICallback, opts...)
}

// InvokeLiteForgeSpeedPriority invokes LiteForge with speed-priority (lightweight) AI model.
// It prefers SpeedPriorityAICallback, falling back to OriginalAICallback if not available.
func (r *ReAct) InvokeLiteForgeSpeedPriority(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	cb := r.config.SpeedPriorityAICallback
	if cb == nil {
		cb = r.config.OriginalAICallback
	}
	return r.invokeLiteForgeWithCallback(ctx, actionName, prompt, outputs, cb, opts...)
}

// InvokeLiteForgeQualityPriority invokes LiteForge with quality-priority (intelligent) AI model.
// It prefers QualityPriorityAICallback, falling back to OriginalAICallback if not available.
func (r *ReAct) InvokeLiteForgeQualityPriority(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	cb := r.config.QualityPriorityAICallback
	if cb == nil {
		cb = r.config.OriginalAICallback
	}
	return r.invokeLiteForgeWithCallback(ctx, actionName, prompt, outputs, cb, opts...)
}
