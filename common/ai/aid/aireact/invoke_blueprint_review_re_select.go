package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func (r *ReAct) invokeBlueprintReviewChangeBlueprint(
	ins *schema.AIForge,
	invokeParams aitool.InvokeParams,
	cancel func(reason any),
) (*schema.AIForge, aitool.InvokeParams, bool, error) {
	return ins, invokeParams, false, nil
}
