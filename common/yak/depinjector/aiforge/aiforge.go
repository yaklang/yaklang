package aiforge

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/aiforge/contracts"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SimpleAiForge struct {
}

func (s *SimpleAiForge) SimpleExecute(ctx context.Context, input string, aitoolOptions []aitool.ToolOption, opts ...aid.Option) (aitool.InvokeParams, error) {
	param := aitool.WithStructParam("result", []aitool.PropertyOption{}, aitoolOptions...)
	lf, err := aiforge.NewLiteForge(
		"simple_ai_forge",
		aiforge.WithLiteForge_Prompt(input),
		aiforge.WithLiteForge_OutputSchemaRaw("object", aitool.NewObjectSchemaWithAction(param)),
	)
	if err != nil {
		return nil, err
	}
	result, err := lf.Execute(ctx, []*ypb.ExecParamItem{
		{
			Key:   "input",
			Value: input,
		},
	}, opts...)
	if err != nil {
		return nil, err
	}
	res := result.GetInvokeParams("result")
	return res, nil
}

var _ contracts.LiteForge = &SimpleAiForge{}

var SimpleAiForgeIns = &SimpleAiForge{}
