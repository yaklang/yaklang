package aireact

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption) (*aicommon.Action, error) {
	var rawOutputs []any
	for _, output := range outputs {
		var rawOpt any = output
		rawOutputs = append(rawOutputs, rawOpt)
	}
	f, err := aiforge.NewLiteForge(
		actionName,
		aiforge.WithLiteForge_Prompt(prompt),
		aiforge.WithLiteForge_OutputSchemaRaw(
			actionName,
			aitool.NewObjectSchemaWithActionName(actionName, rawOutputs...),
		),
	)
	if err != nil {
		return nil, utils.Wrap(err, "create liteforge failed")
	}
	forgeResult, err := f.Execute(ctx, []*ypb.ExecParamItem{
		{Key: "query", Value: prompt},
	}, aid.WithAgreeYOLO(true), aid.WithAICallback(r.config.aiCallback))
	if err != nil {
		return nil, utils.Wrap(err, "invoke liteforge failed")
	}
	return forgeResult.Action, nil
}
