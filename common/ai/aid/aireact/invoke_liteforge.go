package aireact

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
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
	}, aicommon.WithAgreeYOLO(), aicommon.WithAICallback(r.config.OriginalAICallback))
	if err != nil {
		return nil, utils.Wrap(err, "invoke liteforge failed")
	}
	return forgeResult.Action, nil
}
